package eta

import (
	"os"
	"testing"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/state"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
)

func TestMatchTrains(t *testing.T) {
	b, _ := os.ReadFile(testdata + "otp_agency_routes.json")
	routes, _ := feeds.ParseAgencyRoutes(b)
	f, _ := os.Open(testdata + "tallinn_stops.txt")
	defer f.Close()
	stops, _, _ := feeds.ParseTallinnStops(f)
	gtfs, err := feeds.ReadGTFS(os.DirFS(testdata + "elron_gtfs"))
	if err != nil {
		t.Fatal(err)
	}
	ix := staticdata.NewIndex(staticdata.Build(staticdata.Sources{CityRoutes: routes, TallinnStops: stops, Elron: gtfs}, time.Now()))
	tr := NewTracker()
	g := BuildGeometry(ix)
	tr.SetGeometry(g)
	if len(g.rail) == 0 {
		t.Fatal("no rail patterns in the fixture")
	}
	var r14in *PatternGeom // Paldiski → Tallinn
	for _, pg := range g.rail {
		if pg.Pattern.RouteID == "train:R14" {
			r14in = pg
		}
	}
	// A point on the track shared by R12 and R14, 1.5 km before Tallinn.
	pt := r14in.Shape.PointAt(r14in.Shape.Length() - 1500)
	at := func(id, route string, lat float64) state.Vehicle {
		return state.Vehicle{ID: id, Mode: feeds.ModeTrain, Line: route, Lat: lat, Lon: pt[0], Bearing: -1}
	}
	vs := []state.Vehicle{
		at("train-546", "Paldiski - Tallinn", pt[1]),
		at("train-748", "Keila-Tallinn", pt[1]),
		at("train-747", "Tallinn - Keila", pt[1]),
		at("train-645", "Tallinn - Turba", pt[1]), // not in the fixture's timetable
		at("train-9", "Paldiski - Tallinn", pt[1]+0.05),
	}
	tr.MatchTrains(vs)
	pattern := func(v state.Vehicle) (route, first, last string) {
		pg, ok := g.byIDOrRail(v.PatternID)
		if !ok {
			return "", "", ""
		}
		return pg.Pattern.RouteID, pg.stations[0], pg.stations[len(pg.stations)-1]
	}
	for i, want := range [][3]string{
		{"train:R14", "paldiski", "tallinn"},
		{"train:R12", "keila", "tallinn"},
		{"train:R12", "tallinn", "keila"},
		{},
		{},
	} {
		r, a, z := pattern(vs[i])
		if [3]string{r, a, z} != want {
			t.Errorf("%s (%s): matched %s %s→%s, want %v", vs[i].ID, vs[i].Line, r, a, z, want)
		}
	}
	if vs[0].DistAlongM <= 0 || vs[0].NextStopID == "" {
		t.Errorf("progress not set: %+v", vs[0])
	}
	// A heading against the pattern's direction is not that pattern.
	wrong := []state.Vehicle{at("train-1", "Keila - Tallinn", pt[1])}
	wrong[0].Bearing = 180
	if pr, ok := r14in.Shape.Project(pt[1], pt[0], 0); ok {
		wrong[0].Bearing = pr.Bearing // heading the right way still matches
	}
	tr.MatchTrains(wrong)
	if wrong[0].PatternID == "" {
		t.Error("a train heading the right way was not matched")
	}
}

func (g *Geometry) byIDOrRail(id string) (*PatternGeom, bool) {
	for _, pg := range g.rail {
		if pg.Pattern.ID == id {
			return pg, true
		}
	}
	return nil, false
}
