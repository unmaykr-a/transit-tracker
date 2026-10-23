package eta

import (
	"os"
	"testing"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
	"github.com/unmaykr-a/transit-tracker/internal/state"
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
	pg := g.rail[0]
	mid := pg.Shape.Length() / 2
	pt := pg.Shape.PointAt(mid) // [lon, lat]
	far := state.Vehicle{ID: "train-2", Mode: feeds.ModeTrain, Lat: pt[1] + 0.05, Lon: pt[0]}
	vs := []state.Vehicle{
		{ID: "train-1", Mode: feeds.ModeTrain, Lat: pt[1], Lon: pt[0], Destination: pg.Pattern.Headsign},
		far,
	}
	tr.MatchTrains(vs)
	got := vs[0]
	if got.PatternID == "" || got.NextStopID == "" {
		t.Fatalf("train on the track not matched: %+v", got)
	}
	m, _ := g.byIDOrRail(got.PatternID)
	if m == nil || m.Pattern.RouteID != pg.Pattern.RouteID {
		t.Errorf("matched %s, want a pattern of %s", got.PatternID, pg.Pattern.RouteID)
	}
	if got.DistAlongM <= 0 {
		t.Errorf("dist along = %v", got.DistAlongM)
	}
	if vs[1].PatternID != "" {
		t.Errorf("train 5 km off the track matched %s", vs[1].PatternID)
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
