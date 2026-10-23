package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/auth"
	"github.com/unmaykr-a/transit-tracker/internal/dashboard"
	"github.com/unmaykr-a/transit-tracker/internal/departures"
	"github.com/unmaykr-a/transit-tracker/internal/eta"
	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/ingest"
	"github.com/unmaykr-a/transit-tracker/internal/settings"
	"github.com/unmaykr-a/transit-tracker/internal/state"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
	"github.com/unmaykr-a/transit-tracker/internal/store"
	"github.com/unmaykr-a/transit-tracker/internal/walk"
)

// fullServer runs everything against the fake upstream: import, departures, accounts, dashboard.
// geocodeCalls counts the test geocoder's upstream calls (the server caches them).
var geocodeCalls atomic.Int64

func fullServer(t *testing.T) (*client, *staticdata.Importer) {
	c, im, _, _ := fullServerWithLive(t)
	return c, im
}

func fullServerWithLive(t *testing.T) (*client, *staticdata.Importer, *state.Store, *eta.Tracker) {
	t.Helper()
	ctx := context.Background()
	loc, _ := time.LoadLocation("Europe/Tallinn")
	dir := t.TempDir()
	st, err := store.Open(ctx, dir, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	var im *staticdata.Importer
	ft := feeds.FakeTransport{Lines: func(id string) []feeds.FakeLine {
		var out []feeds.FakeLine
		for _, l := range im.Index().Lines(id) {
			for _, h := range l.Headsigns {
				out = append(out, feeds.FakeLine{Mode: l.Mode, Line: l.ShortName, Headsign: h})
			}
		}
		return out
	}}
	f := feeds.NewFetcher(&http.Client{Transport: ft}, "test")
	otp := &feeds.OTP{URL: feeds.Const(feeds.DefaultOTPURL), Fetcher: f}
	im = &staticdata.Importer{Fetcher: f, OTP: otp, StopsURL: feeds.Const(staticdata.DefaultStopsURL), ElronGTFSURL: feeds.Const(staticdata.DefaultElronGTFSURL),
		AgencyID: feeds.TallinnAgencyID, Store: st, Log: slog.New(slog.DiscardHandler), Location: loc}
	if err := im.Import(ctx); err != nil {
		t.Fatal(err)
	}
	set, _ := settings.New(ctx, st, testSettings())
	accounts := auth.NewService(st)
	deps := &departures.Service{Fetcher: f, OTP: otp, SiriURL: feeds.Const(departures.DefaultSiriURL), Index: im.Index, Location: loc}
	walker := walk.NewService([]walk.Provider{walk.Haversine{}}, func() []string { return []string{walk.ProviderHaversine} })
	tracker := eta.NewTracker()
	tracker.SetGeometry(eta.BuildGeometry(im.Index()))
	vehicles := state.NewStore()
	srv := httptest.NewServer(NewServer(slog.New(slog.DiscardHandler), Deps{
		Store: vehicles, Health: ingest.NewHealth(), Static: im.Index, StaticStatus: im.Status, Departures: deps,
		Auth: accounts, Settings: set, DB: st, DataDir: dir, Tracker: tracker,
		Dashboard: &dashboard.Builder{Index: im.Index, Departures: deps, Walk: walker},
		Geocode: func(_ context.Context, q string) ([]feeds.Address, error) {
			geocodeCalls.Add(1)
			if strings.Contains(strings.ToLower(q), "fail") {
				return nil, errors.New("upstream down")
			}
			return []feeds.Address{{Label: "Pärnu mnt 20", Detail: "Kesklinna linnaosa, Tallinn", Lat: 59.4319, Lon: 24.7451}}, nil
		},
	}).Handler(SPA(fstest.MapFS{})))
	t.Cleanup(srv.Close)
	jar, _ := cookiejar.New(nil)
	c := &client{t: t, base: srv.URL, http: &http.Client{Jar: jar}}
	if code, _ := c.do("POST", "/api/v1/auth/setup", map[string]string{"username": "andri", "password": "hunter2hunter2"}); code != 200 {
		t.Fatal("setup failed")
	}
	return c, im, vehicles, tracker
}

func TestSavedLinesAndDashboard(t *testing.T) {
	c, im := fullServer(t)
	ix := im.Index()
	pick := func(route string, dir, idx int) map[string]any {
		for _, p := range ix.Patterns(route) {
			if p.Direction == dir {
				return map[string]any{"route_id": route, "direction": dir, "board_stop_id": p.StopIDs[idx]}
			}
		}
		t.Fatalf("no pattern for %s", route)
		return nil
	}
	t4 := pick("tram:T4", 0, 3)
	bus := pick("bus:24", 1, 2)
	trolley := pick("trolley:72", 0, 1)

	for _, bad := range []map[string]any{
		{"route_id": "tram:T99", "direction": 0, "board_stop_id": "1"},
		{"route_id": "tram:T4", "direction": 7, "board_stop_id": t4["board_stop_id"]},
		{"route_id": "tram:T4", "direction": 0, "board_stop_id": "nope"},
		{"route_id": "tram:T4", "direction": 0, "board_stop_id": t4["board_stop_id"], "alight_stop_id": ix.Patterns("tram:T4")[0].StopIDs[0]},
	} {
		if code, out := c.do("POST", "/api/v1/me/lines", bad); code != 400 {
			t.Errorf("POST %v = %d %v", bad, code, out)
		}
	}
	var ids []string
	for _, l := range []map[string]any{t4, bus, trolley} {
		code, out := c.do("POST", "/api/v1/me/lines", l)
		if code != 201 {
			t.Fatalf("POST %v = %d %v", l, code, out)
		}
		line := out["line"].(map[string]any)
		if line["headsign"] == "" || line["board_stop"] == nil {
			t.Errorf("resolved line = %v", line)
		}
		ids = append(ids, jsonNum(line["id"]))
	}

	// Stand right at the T4 boarding stop: walk is ~0, so the verdict follows the countdown.
	board, _ := ix.Stop(t4["board_stop_id"].(string))
	code, dash := c.do("GET", "/api/v1/me/dashboard?lat="+strconv.FormatFloat(board.Lat, 'f', 6, 64)+"&lon="+strconv.FormatFloat(board.Lon, 'f', 6, 64), nil)
	if code != 200 || dash["has_origin"] != true {
		t.Fatalf("dashboard: %d %v", code, dash)
	}
	entries := dash["entries"].([]any)
	if len(entries) != 3 {
		t.Fatalf("entries = %d", len(entries))
	}
	for i, e := range entries {
		e := e.(map[string]any)
		deps := e["departures"].([]any)
		if len(deps) < 3 {
			t.Fatalf("entry %d has %d departures: %v", i, len(deps), e)
		}
		walkMin := int(e["walk"].(map[string]any)["minutes"].(float64))
		prev := time.Time{}
		for _, d := range deps[:3] {
			d := d.(map[string]any)
			at, _ := time.Parse(time.RFC3339, d["expected_at"].(string))
			if at.Before(prev) {
				t.Errorf("entry %d departures not sorted", i)
			}
			prev = at
			v := d["verdict"].(map[string]any)
			want := dashboard.Judge(at, time.Now(), walkMin, 1)
			if v["level"] != string(want.Level) {
				// The clock may tick between server and test; allow a one-minute difference.
				alt := dashboard.Judge(at, time.Now().Add(-time.Minute), walkMin, 1)
				if v["level"] != string(alt.Level) {
					t.Errorf("entry %d verdict %v, want %v", i, v, want)
				}
			}
			if d["route_id"] != e["line"].(map[string]any)["route_id"] {
				t.Errorf("departure of another line in entry %d: %v", i, d)
			}
		}
	}
	if w := entries[0].(map[string]any)["walk"].(map[string]any); w["minutes"].(float64) > 1 || w["approx"] != true {
		t.Errorf("walk at the stop = %v", w)
	}

	// Without an origin there is no verdict.
	_, dash = c.do("GET", "/api/v1/me/dashboard", nil)
	d0 := dash["entries"].([]any)[0].(map[string]any)["departures"].([]any)[0].(map[string]any)
	if d0["verdict"].(map[string]any)["level"] != "none" || dash["has_origin"] != false {
		t.Errorf("no-origin verdict = %v", d0["verdict"])
	}

	// Reorder, update, delete.
	if code, out := c.do("POST", "/api/v1/me/lines/reorder", map[string]any{"ids": []any{toInt(ids[2]), toInt(ids[0]), toInt(ids[1])}}); code != 200 || jsonNum(out["lines"].([]any)[0].(map[string]any)["id"]) != ids[2] {
		t.Fatalf("reorder: %d %v", code, out)
	}
	if code, _ := c.do("POST", "/api/v1/me/lines/reorder", map[string]any{"ids": []any{toInt(ids[0])}}); code != 400 {
		t.Errorf("partial reorder = %d", code)
	}
	if code, out := c.do("PATCH", "/api/v1/me/lines/"+ids[0], map[string]any{"label": "Work"}); code != 200 || out["line"].(map[string]any)["label"] != "Work" {
		t.Errorf("label: %d %v", code, out)
	}
	if code, _ := c.do("DELETE", "/api/v1/me/lines/"+ids[1], nil); code != 204 {
		t.Errorf("delete = %d", code)
	}
	if code, _ := c.do("DELETE", "/api/v1/me/lines/"+ids[1], nil); code != 404 {
		t.Errorf("second delete = %d", code)
	}

	// Saved places as origin, and prefs.
	code, out := c.do("POST", "/api/v1/me/places", map[string]any{"label": "Home", "lat": board.Lat + 0.009, "lon": board.Lon})
	if code != 201 {
		t.Fatalf("place: %d %v", code, out)
	}
	pid := jsonNum(out["place"].(map[string]any)["id"])
	if code, _ := c.do("POST", "/api/v1/me/places", map[string]any{"label": "", "lat": 59, "lon": 24}); code != 400 {
		t.Errorf("empty label = %d", code)
	}
	if code, out := c.do("PATCH", "/api/v1/me/prefs", map[string]any{"walk_speed_kmh": 6, "buffer_min": 2}); code != 200 || out["buffer_min"].(float64) != 2 {
		t.Errorf("prefs: %d %v", code, out)
	}
	if code, _ := c.do("PATCH", "/api/v1/me/prefs", map[string]any{"walk_speed_kmh": 40}); code != 400 {
		t.Errorf("silly speed = %d", code)
	}
	// Railway lines are off until asked for; turning them on keeps the other prefs.
	if _, out := c.do("GET", "/api/v1/me/prefs", nil); out["show_rail"] != false {
		t.Errorf("show_rail default = %v", out["show_rail"])
	}
	if code, out := c.do("PATCH", "/api/v1/me/prefs", map[string]any{"show_rail": true}); code != 200 || out["show_rail"] != true || out["buffer_min"].(float64) != 2 {
		t.Errorf("show_rail: %d %v", code, out)
	}
	_, dash = c.do("GET", "/api/v1/me/dashboard?place="+pid, nil)
	var t4Entry map[string]any
	for _, e := range dash["entries"].([]any) {
		if e.(map[string]any)["line"].(map[string]any)["route_id"] == "tram:T4" {
			t4Entry = e.(map[string]any)
		}
	}
	// ~1 km × 1.3 detour at 6 km/h ≈ 12–14 min.
	if m := t4Entry["walk"].(map[string]any)["minutes"].(float64); m < 10 || m > 16 {
		t.Errorf("walk from Home = %v min", m)
	}
	if code, _ := c.do("GET", "/api/v1/me/dashboard?place=999", nil); code != 400 {
		t.Errorf("unknown place = %d", code)
	}
}

func toInt(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func TestTripAndPatterns(t *testing.T) {
	c, im, vehicles, tracker := fullServerWithLive(t)
	code, out := c.do("GET", "/api/v1/patterns", nil)
	pats := out["patterns"].([]any)
	if code != 200 || len(pats) < 150 {
		t.Fatalf("patterns: %d, %d patterns", code, len(pats))
	}
	p0 := pats[0].(map[string]any)
	if p0["polyline"] == "" || len(p0["stop_ids"].([]any)) != len(p0["stop_along_m"].([]any)) {
		t.Errorf("pattern = %v", p0)
	}

	// A T4 tram halfway along its outbound pattern.
	var out4 *staticdata.Pattern
	for _, p := range im.Index().Patterns("tram:T4") {
		if p.Direction == 0 {
			out4 = p
			break
		}
	}
	pg, _ := tracker.Geometry().Pattern(out4.ID)
	pt := pg.Shape.PointAt(pg.Shape.Length() / 2)
	v := []state.Vehicle{{ID: "tram-9", Mode: feeds.ModeTram, Line: "T4", Lat: pt[1], Lon: pt[0], Bearing: -1, Destination: out4.Headsign, UpdatedAt: time.Now()}}
	tracker.Annotate(v)
	vehicles.Replace("gps", v)

	code, trip := c.do("GET", "/api/v1/vehicles/tram-9/trip?limit=3", nil)
	if code != 200 || trip["trip"] == nil {
		t.Fatalf("trip: %d %v", code, trip)
	}
	tj := trip["trip"].(map[string]any)
	ahead := tj["ahead"].([]any)
	if tj["pattern_id"] != out4.ID || len(ahead) != 3 || ahead[0].(map[string]any)["name"] == "" || trip["headsign"] != out4.Headsign {
		t.Errorf("trip = %v", trip)
	}
	if code, one := c.do("GET", "/api/v1/patterns/"+out4.ID, nil); code != 200 || one["headsign"] != out4.Headsign {
		t.Errorf("pattern: %d %v", code, one)
	}
	if code, _ := c.do("GET", "/api/v1/vehicles/nope/trip", nil); code != 404 {
		t.Errorf("trip of unknown vehicle = %d", code)
	}
}

func TestGeocode(t *testing.T) {
	c, _ := fullServer(t)
	before := geocodeCalls.Load()
	for range 2 {
		code, out := c.do("GET", "/api/v1/geocode?q=P%C3%A4rnu+mnt++20", nil)
		a := out["addresses"].([]any)
		if code != 200 || len(a) != 1 || a[0].(map[string]any)["label"] != "Pärnu mnt 20" {
			t.Fatalf("geocode = %d %v", code, out)
		}
	}
	if n := geocodeCalls.Load() - before; n != 1 {
		t.Errorf("upstream calls = %d, want 1 (cached)", n)
	}
	for q, want := range map[string]int{"ab": 400, "fail here": 502} {
		if code, _ := c.do("GET", "/api/v1/geocode?q="+url.QueryEscape(q), nil); code != want {
			t.Errorf("q=%q: %d, want %d", q, code, want)
		}
	}
	// Signed-in users only: it proxies an upstream service.
	anon := &client{t: t, base: c.base, http: &http.Client{}}
	if code, _ := anon.do("GET", "/api/v1/geocode?q=narva", nil); code != 401 {
		t.Errorf("anonymous geocode = %d", code)
	}
}

func TestInputLimits(t *testing.T) {
	c, _ := fullServer(t)
	// 25 letters with õ/ä/ü are 25 characters (more bytes): allowed.
	if code, out := c.do("POST", "/api/v1/me/places", map[string]any{"label": strings.Repeat("õäü", 8) + "a", "lat": 59.43, "lon": 24.75}); code != 201 {
		t.Fatalf("estonian label = %d %v", code, out)
	}
	for i := 1; i < MaxSavedPlaces; i++ {
		c.do("POST", "/api/v1/me/places", map[string]any{"label": fmt.Sprintf("P%d", i), "lat": 59.43, "lon": 24.75})
	}
	if code, _ := c.do("POST", "/api/v1/me/places", map[string]any{"label": "one too many", "lat": 59.43, "lon": 24.75}); code != 400 {
		t.Errorf("place over the limit = %d", code)
	}
	if code, _ := c.do("POST", "/api/v1/me/lines", map[string]any{"route_id": "tram:T4", "direction": 0, "board_stop_id": "x", "label": strings.Repeat("x", 41)}); code != 400 {
		t.Errorf("long label = %d", code)
	}
	for email, want := range map[string]int{"andri@example.com": 200, "": 200, "not-an-email": 400, strings.Repeat("a", 250) + "@x.ee": 400} {
		if code, _ := c.do("PATCH", "/api/v1/me", map[string]any{"email": email}); code != want {
			t.Errorf("email %.20q = %d, want %d", email, code, want)
		}
	}
}
