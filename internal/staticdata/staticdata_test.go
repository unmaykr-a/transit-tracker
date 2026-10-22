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
