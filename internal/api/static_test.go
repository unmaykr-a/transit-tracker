package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/departures"
	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/ingest"
	"github.com/unmaykr-a/transit-tracker/internal/state"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
	"github.com/unmaykr-a/transit-tracker/internal/store"
)

// staticServer runs the real importer and departures service against the fake upstream.
func staticServer(t *testing.T) (*httptest.Server, *state.Store) {
	t.Helper()
	loc, _ := time.LoadLocation("Europe/Tallinn")
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
	st, err := store.Open(context.Background(), t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	im = &staticdata.Importer{Fetcher: f, OTP: otp, StopsURL: feeds.Const(staticdata.DefaultStopsURL), ElronGTFSURL: feeds.Const(staticdata.DefaultElronGTFSURL),
		AgencyID: feeds.TallinnAgencyID, Store: st, Log: slog.New(slog.DiscardHandler), Location: loc}
	if err := im.Import(context.Background()); err != nil {
		t.Fatal(err)
	}
	vehicles := state.NewStore()
	deps := &departures.Service{Fetcher: f, OTP: otp, SiriURL: feeds.Const(departures.DefaultSiriURL), Index: im.Index, Location: loc}
	srv := httptest.NewServer(NewServer(slog.New(slog.DiscardHandler), Deps{
		Store: vehicles, Health: ingest.NewHealth(), Static: im.Index, StaticStatus: im.Status, Departures: deps,
	}).Handler(SPA(fstest.MapFS{})))
	t.Cleanup(srv.Close)
	return srv, vehicles
}

func getJSON(t *testing.T, url string, v any) int {
	t.Helper()
	res, body := get(t, url)
	if err := json.Unmarshal([]byte(body), v); err != nil {
		t.Fatalf("%s: %v (%s)", url, err, body)
	}
	return res.StatusCode
}

func TestSearchAndStopPage(t *testing.T) {
	srv, _ := staticServer(t)
	var search struct {
		Lines []struct{ ID, ShortName string } `json:"lines"`
		Stops []struct {
			ID, Name string
			Modes    []string
			Lines    []struct{ ShortName string } `json:"lines"`
		} `json:"stops"`
	}
	getJSON(t, srv.URL+"/api/v1/search?q=balti%20jaam", &search)
	if len(search.Stops) == 0 || search.Stops[0].Name != "Balti jaam" {
		t.Fatalf("search = %+v", search)
	}
	area := search.Stops[0]
	if !strings.Contains(strings.Join(area.Modes, ","), "train") || len(area.Lines) < 5 {
		t.Errorf("Balti jaam = %+v", area)
	}

	var stop struct {
		ID, Name  string
		Platforms []struct {
			ID, Platform string
			Modes        []string
			Lines        []struct {
				ShortName string
				Headsigns []string
			} `json:"lines"`
		} `json:"platforms"`
	}
	if code := getJSON(t, srv.URL+"/api/v1/stops/"+area.ID, &stop); code != 200 || len(stop.Platforms) < 3 {
		t.Fatalf("stop page %d %+v", code, stop)
	}
	// A platform id resolves to its area too.
	var viaPlatform struct{ ID, Focus string }
	getJSON(t, srv.URL+"/api/v1/stops/"+stop.Platforms[0].ID, &viaPlatform)
	if viaPlatform.ID != stop.ID || viaPlatform.Focus != stop.Platforms[0].ID {
		t.Errorf("platform lookup = %+v", viaPlatform)
	}

	var deps struct {
		Departures []departures.Departure `json:"departures"`
	}
	if code := getJSON(t, srv.URL+"/api/v1/stops/"+area.ID+"/departures?limit=60", &deps); code != 200 {
		t.Fatalf("departures status %d", code)
	}
	modes := map[feeds.Mode]bool{}
	for i, d := range deps.Departures {
		modes[d.Mode] = true
		if i > 0 && d.ExpectedAt.Before(deps.Departures[i-1].ExpectedAt) {
			t.Fatal("departures not sorted")
		}
		if staticdata.Fold(d.Headsign) == "balti jaam" || staticdata.Fold(d.Headsign) == "tallinn" {
			t.Errorf("arrival listed as departure: %+v", d)
		}
	}
	if !modes[feeds.ModeTram] || !modes[feeds.ModeTrain] {
		t.Errorf("Balti jaam departures should include trams and trains, got %v", modes)
	}

	for _, path := range []string{"/api/v1/stops/nope", "/api/v1/stops/nope/departures", "/api/v1/lines/tram/T99"} {
		if res, _ := get(t, srv.URL+path); res.StatusCode != 404 {
			t.Errorf("%s status %d", path, res.StatusCode)
		}
	}
}

func TestLinePage(t *testing.T) {
	srv, vehicles := staticServer(t)
	vehicles.Replace("gps", []state.Vehicle{veh("tram-1", feeds.ModeTram, "T4", 59.43), veh("tram-2", feeds.ModeTram, "T1", 59.43)})
	var line struct {
		Route    struct{ ID, LongName, Color string } `json:"route"`
		Patterns []struct {
			Direction int
			Headsign  string
			Main      bool
			Stops     []struct{ ID, Name string } `json:"stops"`
			Shape     [][2]float64
		} `json:"patterns"`
		Vehicles []state.Vehicle `json:"vehicles"`
	}
	for _, path := range []string{"/api/v1/lines/tram/T4", "/api/v1/lines/tram/4"} {
		if code := getJSON(t, srv.URL+path, &line); code != 200 {
			t.Fatalf("%s status %d", path, code)
		}
		if line.Route.ID != "tram:T4" || len(line.Patterns) < 2 || !line.Patterns[0].Main || len(line.Patterns[0].Shape) < 50 {
			t.Fatalf("%s = %+v", path, line.Route)
		}
	}
	mains := 0
	for _, p := range line.Patterns {
		if p.Main {
			mains++
		}
	}
	if mains != 2 {
		t.Errorf("main patterns = %d, want one per direction", mains)
	}
	if len(line.Vehicles) != 1 || line.Vehicles[0].ID != "tram-1" {
		t.Errorf("vehicles = %+v", line.Vehicles)
	}
	var lines struct {
		Lines []struct{ ID string } `json:"lines"`
	}
	getJSON(t, srv.URL+"/api/v1/lines/search?q=24", &lines)
	if len(lines.Lines) != 2 || lines.Lines[0].ID != "bus:24" || lines.Lines[1].ID != "bus:24A" {
		t.Errorf("line search 24 = %+v", lines)
	}
}

func TestAllStopsCaching(t *testing.T) {
	srv, _ := staticServer(t)
	res, body := get(t, srv.URL+"/api/v1/stops")
	etag := res.Header.Get("ETag")
	if res.StatusCode != 200 || etag == "" || !strings.Contains(res.Header.Get("Cache-Control"), "max-age") {
		t.Fatalf("status %d etag %q cc %q", res.StatusCode, etag, res.Header.Get("Cache-Control"))
	}
	var d struct {
		Stops []struct{ ID, AreaID string } `json:"stops"`
	}
	if err := json.Unmarshal([]byte(body), &d); err != nil || len(d.Stops) < 400 {
		t.Fatalf("stops %d %v", len(d.Stops), err)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/stops", nil)
	req.Header.Set("If-None-Match", etag)
	res2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res2.Body.Close()
	if res2.StatusCode != http.StatusNotModified {
		t.Errorf("conditional status %d", res2.StatusCode)
	}
}
