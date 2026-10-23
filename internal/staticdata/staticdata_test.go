package staticdata

import (
	"archive/zip"
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/store"
)

const testdata = "../feeds/testdata/"

func fixtureSources(t *testing.T) Sources {
	t.Helper()
	b, err := os.ReadFile(testdata + "otp_agency_routes.json")
	if err != nil {
		t.Fatal(err)
	}
	routes, err := feeds.ParseAgencyRoutes(b)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(testdata + "tallinn_stops.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	stops, _, err := feeds.ParseTallinnStops(f)
	if err != nil {
		t.Fatal(err)
	}
	g, err := feeds.ReadGTFS(os.DirFS(testdata + "elron_gtfs"))
	if err != nil {
		t.Fatal(err)
	}
	return Sources{CityRoutes: routes, TallinnStops: stops, Elron: g}
}

var fixtureTime = time.Date(2026, 10, 22, 3, 30, 0, 0, time.UTC)

func fixtureIndex(t *testing.T) (*Index, Dataset) {
	t.Helper()
	d := Build(fixtureSources(t), fixtureTime)
	return NewIndex(d), d
}

func TestFold(t *testing.T) {
	for in, want := range map[string]string{
		"Ülemiste":              "ulemiste",
		"  Balti-jaam ":         "balti jaam",
		"Pärnu mnt / Tõnismägi": "parnu mnt tonismagi",
		"ŽŠÕ":                   "zso",
		"Cēsis":                 "cesis",
	} {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanName(t *testing.T) {
	for in, want := range map[string]string{
		"Balti jaam (train station)": "Balti jaam",
		"Bussijaam (bus station)":    "Bussijaam",
		"Ülemiste jaam (ajutine)":    "Ülemiste jaam",
		"Kosmos":                     "Kosmos",
		"A. H. Tammsaare tee (nõlv)": "A. H. Tammsaare tee (nõlv)",
	} {
		if got := cleanName(in); got != want {
			t.Errorf("cleanName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildCityAndRail(t *testing.T) {
	ix, d := fixtureIndex(t)
	if len(d.Routes) != 80+2 {
		t.Errorf("routes = %d, want 80 city + 2 rail", len(d.Routes))
	}
	t4, ok := ix.Route("tram:T4")
	if !ok || t4.LongName != "Tondi - Lennujaam" || t4.Color != "#016e12" {
		t.Fatalf("T4 = %+v", t4)
	}
	if _, ok := ix.Route("trolley:72"); !ok {
		t.Error("trolleybus 72 missing")
	}
	if r, ok := ix.Route("train:R12"); !ok || r.Mode != feeds.ModeTrain {
		t.Errorf("R12 = %+v", r)
	}
	ps := ix.Patterns("tram:T4")
	if len(ps) < 2 || ps[0].Direction != 0 || len(ps[0].Shape) < 50 {
		t.Fatalf("T4 patterns: %d, first %+v", len(ps), ps[0])
	}
	// The main pattern per direction is the longest, not a depot short-turn.
	for _, p := range ps[1:] {
		if p.Direction == ps[0].Direction && len(p.StopIDs) > len(ps[0].StopIDs) {
			t.Errorf("pattern %s longer than main %s", p.ID, ps[0].ID)
		}
	}
	// Names come from the operator file: "Balti jaam", not "Balti jaam (train station)".
	s, ok := ix.StopByCode("21101-1")
	if !ok || s.Name != "Balti jaam" || s.SiriID == "" {
		t.Errorf("21101-1 = %+v", s)
	}
	// Every pattern stop resolves.
	for _, p := range d.Patterns {
		for _, id := range p.StopIDs {
			if _, ok := ix.Stop(id); !ok {
				t.Fatalf("pattern %s references unknown stop %s", p.ID, id)
			}
		}
	}
}

func TestBaltiJaamGroupsCityAndRail(t *testing.T) {
	ix, _ := fixtureIndex(t)
	res := ix.SearchAreas("balti jaam", 5)
	if len(res) == 0 || res[0].Area.Name != "Balti jaam" {
		t.Fatalf("search = %+v", res)
	}
	a := res[0].Area
	if !slices.Contains(a.Modes, feeds.ModeTram) || !slices.Contains(a.Modes, feeds.ModeTrain) || !slices.Contains(a.Modes, feeds.ModeBus) {
		t.Fatalf("Balti jaam modes = %v", a.Modes)
	}
	rail, _ := ix.Stop("10137")
	if rail.AreaID != a.ID {
		t.Errorf("rail station Tallinn (alias Balti Jaam) in area %s, want %s", rail.AreaID, a.ID)
	}
	letters := map[string]bool{}
	for _, id := range a.StopIDs {
		s, _ := ix.Stop(id)
		if s.Platform == "" || letters[s.Platform] {
			t.Errorf("platform letter %q for %s not unique", s.Platform, id)
		}
		letters[s.Platform] = true
	}
	// Trams sort first, so platform A is a tram stop.
	first, _ := ix.Stop(a.StopIDs[0])
	if first.Platform != "A" || !slices.Contains(first.Modes, feeds.ModeTram) {
		t.Errorf("platform A = %+v", first)
	}
	if lines := ix.Lines(rail.ID); len(lines) == 0 || lines[0].Mode != feeds.ModeTrain {
		t.Errorf("rail lines = %+v", lines)
	}
}

func TestRailJoinsCityAreaByAlias(t *testing.T) {
	d := Build(Sources{}, fixtureTime)
	d.Stops = []Stop{
		{ID: "1362", Code: "13403-1", Name: "Ülemiste jaam", Lat: 59.42316, Lon: 24.79516, Modes: []feeds.Mode{feeds.ModeBus}, Aliases: []string{"Ülemiste"}, Source: "tallinn"},
		{ID: "1408", Code: "14703-1", Name: "Suur-Paala", Lat: 59.42678, Lon: 24.81467, Modes: []feeds.Mode{feeds.ModeBus}, Aliases: []string{"Ülemiste"}, Source: "tallinn"},
		{ID: "1409", Code: "99999-1", Name: "Lähedal", Lat: 59.42330, Lon: 24.79600, Modes: []feeds.Mode{feeds.ModeBus}, Aliases: []string{"Ülemiste"}, Source: "tallinn"},
		{ID: "10878", Code: "13412-1", Name: "Ülemiste", Lat: 59.42330, Lon: 24.79581, Modes: []feeds.Mode{feeds.ModeTrain}, Source: "elron"},
	}
	areas := groupAreas(d.Stops)
	area := map[string]string{}
	for _, s := range d.Stops {
		area[s.ID] = s.AreaID
	}
	if area["10878"] != area["1362"] {
		t.Errorf("rail Ülemiste not grouped with Ülemiste jaam: %v", area)
	}
	if area["1408"] == area["1362"] || area["1409"] == area["1362"] {
		t.Errorf("aliases must not merge city stops: %v", area)
	}
	if len(areas) != 3 {
		t.Errorf("areas = %d", len(areas))
	}
}

func TestSearch(t *testing.T) {
	ix, _ := fixtureIndex(t)
	if res := ix.SearchAreas("ulemiste", 3); len(res) == 0 || !strings.HasPrefix(Fold(res[0].Area.Name), "ulemiste") {
		t.Errorf("ulemiste → %+v", res)
	}
	if res := ix.SearchAreas("hobuj", 3); len(res) == 0 || res[0].Area.Name != "Hobujaama" {
		t.Errorf("hobuj → %+v", res)
	}
	if res := ix.SearchAreas("", 3); res != nil {
		t.Error("empty query must return nothing")
	}
	routes := ix.SearchRoutes("4", 10)
	ids := []string{}
	for _, r := range routes {
		ids = append(ids, r.ID)
	}
	if len(ids) < 2 || ids[0] != "tram:T4" && ids[0] != "bus:4" || !slices.Contains(ids, "tram:T4") || !slices.Contains(ids, "bus:4") {
		t.Errorf("search 4 → %v", ids)
	}
	if rs := ix.SearchRoutes("lennujaam", 10); len(rs) == 0 {
		t.Error("search by long name failed")
	}
}

func TestNearestAreas(t *testing.T) {
	ix, _ := fixtureIndex(t)
	// Liivalaia 53 (Stockmann): the nearest served areas, nearest first, all within reach.
	near := ix.NearestAreas(59.4316, 24.7562, 600, 4)
	if len(near) == 0 {
		t.Fatal("no stop areas near Liivalaia 53")
	}
	for i, n := range near {
		if n.DistM > 600 || (i > 0 && n.DistM < near[i-1].DistM) {
			t.Errorf("%d: %s at %.0f m", i, n.Area.Name, n.DistM)
		}
	}
	if got := ix.NearestAreas(58.0, 22.0, 1000, 4); len(got) != 0 {
		t.Errorf("stops near Saaremaa in a Tallinn index: %v", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	empty, err := Load(ctx, st)
	if err != nil || len(empty.Stops) != 0 || !empty.ImportedAt.IsZero() {
		t.Fatalf("empty load: %+v %v", empty.ImportedAt, err)
	}
	_, d := fixtureIndex(t)
	if err := Save(ctx, st, d); err != nil {
		t.Fatal(err)
	}
	// Saving twice replaces, not duplicates.
	if err := Save(ctx, st, d); err != nil {
		t.Fatal(err)
	}
	got, err := Load(ctx, st)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Stops) != len(d.Stops) || len(got.Areas) != len(d.Areas) || len(got.Routes) != len(d.Routes) || len(got.Patterns) != len(d.Patterns) {
		t.Fatalf("counts differ: %d/%d stops, %d/%d areas, %d/%d routes, %d/%d patterns",
			len(got.Stops), len(d.Stops), len(got.Areas), len(d.Areas), len(got.Routes), len(d.Routes), len(got.Patterns), len(d.Patterns))
	}
	if !got.ImportedAt.Equal(fixtureTime) {
		t.Errorf("imported_at = %v", got.ImportedAt)
	}
	ix := NewIndex(got)
	if res := ix.SearchAreas("balti jaam", 1); len(res) == 0 || len(res[0].Area.StopIDs) < 3 {
		t.Errorf("loaded index search = %+v", res)
	}
	if ps := ix.Patterns("tram:T4"); len(ps) == 0 || len(ps[0].Shape) < 50 {
		t.Error("shape lost in round trip")
	}
	// Platforms come back in letter order, not id order.
	a := ix.SearchAreas("balti jaam", 1)[0].Area
	for i, id := range a.StopIDs {
		if s, _ := ix.Stop(id); s.Platform != string(rune('A'+i)) {
			t.Errorf("platform %d is %q", i, s.Platform)
		}
	}
}

// zipFixtureGTFS is shared with importer tests.
func zipFixtureGTFS(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	entries, _ := os.ReadDir(testdata + "elron_gtfs")
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(testdata+"elron_gtfs", e.Name()))
		w, _ := zw.Create(e.Name())
		_, _ = w.Write(b)
	}
	_ = zw.Close()
	return buf.Bytes()
}

func TestBuildCountyRoutes(t *testing.T) {
	src := fixtureSources(t)
	b, err := os.ReadFile(testdata + "otp_county_routes.json")
	if err != nil {
		t.Fatal(err)
	}
	county, err := feeds.ParseAgencyRoutes(b)
	if err != nil {
		t.Fatal(err)
	}
	// A county route sharing a city line's short name must not take over its id.
	clash := county[0]
	clash.ShortName = src.CityRoutes[0].ShortName
	src.CountyRoutes = append(county, clash)
	ix := NewIndex(Build(src, fixtureTime))

	r, ok := ix.Route("bus:120")
	if !ok || r.Source != "county" || r.Mode != feeds.ModeBus || r.LongName != "Tallinn - Jüri" || len(ix.Patterns("bus:120")) == 0 {
		t.Fatalf("county route 120 = %+v", r)
	}
	if _, ok := ix.Route("bus:277"); !ok {
		t.Error("county route 277 missing")
	}
	city := src.CityRoutes[0]
	if r, _ := ix.Route(RouteID(city.CityMode(), city.ShortName)); r.Source != "tallinn" {
		t.Errorf("city route %s replaced by county: %+v", city.ShortName, r)
	}
	// County-only platforms have no SIRI id (the city feed knows nothing about them),
	// and are searchable.
	var countyOnly *Stop
	for _, p := range ix.Patterns("bus:277") {
		for _, id := range p.StopIDs {
			if s, _ := ix.Stop(id); s.Source == "county" {
				countyOnly = s
			}
		}
	}
	if countyOnly == nil || countyOnly.SiriID != "" {
		t.Fatalf("county-only stop = %+v", countyOnly)
	}
	if res := ix.SearchRoutes("277", 5); len(res) == 0 || res[0].ID != "bus:277" {
		t.Errorf("search 277 = %v", res)
	}
}

func TestConnections(t *testing.T) {
	ix, _ := fixtureIndex(t)
	var t4 *Pattern
	for _, p := range ix.Patterns("tram:T4") {
		if p.Direction == 0 {
			t4 = p
			break
		}
	}
	areaOf := func(stopID string) string {
		s, _ := ix.Stop(stopID)
		return s.AreaID
	}
	from, to := areaOf(t4.StopIDs[2]), areaOf(t4.StopIDs[6])
	cs := ix.Connections(from, to)
	var got *Connection
	for i, c := range cs {
		if c.RouteID == "tram:T4" {
			got = &cs[i]
		}
		// Every connection really rides from one area to the other.
		if bs, _ := ix.Stop(c.BoardStopID); bs.AreaID != from {
			t.Errorf("%s boards outside the origin: %s", c.RouteID, c.BoardStopID)
		}
		if as, _ := ix.Stop(c.AlightStopID); as.AreaID != to {
			t.Errorf("%s alights outside the destination: %s", c.RouteID, c.AlightStopID)
		}
	}
	if got == nil || got.BoardStopID != t4.StopIDs[2] || got.Stops != 4 || !slices.Contains(got.Headsigns, t4.Headsign) {
		t.Fatalf("T4 connection = %+v (all: %+v)", got, cs)
	}
	// The other way round, T4 runs from the opposite platform (the other direction).
	for _, c := range ix.Connections(to, from) {
		if c.RouteID == "tram:T4" && c.BoardStopID == t4.StopIDs[6] {
			t.Errorf("reverse trip boards the outbound platform: %+v", c)
		}
	}
	if ix.Connections(from, from) != nil || ix.Connections("nope", to) != nil {
		t.Error("degenerate trips should have no connections")
	}
}

// A city route listed twice by OTP (same mode and short name) becomes one
// route with both sets of patterns; a duplicate id would fail the import.
func TestBuildMergesDuplicateCityRoutes(t *testing.T) {
	src := fixtureSources(t)
	dup := src.CityRoutes[0]
	dup.Patterns = slices.Clone(dup.Patterns)
	for i := range dup.Patterns {
		dup.Patterns[i].Code += "-variant"
	}
	src.CityRoutes = append(src.CityRoutes, dup)
	d := Build(src, fixtureTime)
	seen := map[string]bool{}
	for _, r := range d.Routes {
		if seen[r.ID] {
			t.Fatalf("duplicate route %s", r.ID)
		}
		seen[r.ID] = true
	}
	id := RouteID(dup.CityMode(), dup.ShortName)
	n := 0
	for _, p := range d.Patterns {
		if p.RouteID == id {
			n++
		}
	}
	if n != 2*len(dup.Patterns) {
		t.Errorf("%s has %d patterns, want %d", id, n, 2*len(dup.Patterns))
	}
}
