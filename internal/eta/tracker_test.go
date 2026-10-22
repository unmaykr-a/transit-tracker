package eta

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/state"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
)

const testdata = "../feeds/testdata/"

func fixtureIndex(t *testing.T) *staticdata.Index {
	t.Helper()
	b, _ := os.ReadFile(testdata + "otp_agency_routes.json")
	routes, err := feeds.ParseAgencyRoutes(b)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(testdata + "tallinn_stops.txt")
	defer f.Close()
	stops, _, _ := feeds.ParseTallinnStops(f)
	return staticdata.NewIndex(staticdata.Build(staticdata.Sources{CityRoutes: routes, TallinnStops: stops}, time.Now()))
}

func mainPattern(ix *staticdata.Index, route string, dir int) *staticdata.Pattern {
	for _, p := range ix.Patterns(route) {
		if p.Direction == dir {
			return p
		}
	}
	return nil
}

// tramAt places a T4 vehicle on a pattern's shape, heading along it.
func tramAt(g *Geometry, p *staticdata.Pattern, along float64, at time.Time) state.Vehicle {
	pg, _ := g.Pattern(p.ID)
	pt := pg.Shape.PointAt(along)
	ahead := pg.Shape.PointAt(along + 20)
	pr, _ := pg.Shape.Project(ahead[1], ahead[0], along)
	return state.Vehicle{ID: "tram-1", Mode: feeds.ModeTram, Line: "T4", Lat: pt[1], Lon: pt[0], Bearing: pr.Bearing, SpeedMps: 5, Destination: p.Headsign, UpdatedAt: at}
}

func TestMatchingDirectionAndNextStop(t *testing.T) {
	ix := fixtureIndex(t)
	g := BuildGeometry(ix)
	tr := NewTracker()
	tr.SetGeometry(g)
	out, back := mainPattern(ix, "tram:T4", 0), mainPattern(ix, "tram:T4", 1)
	pgOut, _ := g.Pattern(out.ID)
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)

	along := pgOut.Shape.Length() * 0.4
	v := []state.Vehicle{tramAt(g, out, along, now)}
	tr.Annotate(v)
	if v[0].PatternID != out.ID || math.Abs(v[0].DistAlongM-along) > 5 {
		t.Fatalf("outbound match: pattern %q along %.0f (want %q %.0f)", v[0].PatternID, v[0].DistAlongM, out.ID, along)
	}
	// The next stop is the first one beyond the vehicle.
	want := ""
	for i, a := range pgOut.StopAlong {
		if a > along+nextStopMarginM {
			want = out.StopIDs[i]
			break
		}
	}
	if v[0].NextStopID != want {
		t.Errorf("next stop %s, want %s", v[0].NextStopID, want)
	}

	// Same street, opposite heading → the return pattern.
	pgBack, _ := g.Pattern(back.ID)
	r := []state.Vehicle{tramAt(g, back, pgBack.Shape.Length()*0.6, now)}
	r[0].ID = "tram-2"
	r[0].Destination = "" // even without the destination, heading decides
	tr.Annotate(r)
	if r[0].PatternID == "" || g.byID[r[0].PatternID].Pattern.Direction != 1 {
		t.Errorf("return match = %q", r[0].PatternID)
	}

	// A tram turning a corner (heading 100° off the shape) still matches.
	turning := []state.Vehicle{tramAt(g, out, along, now)}
	turning[0].ID = "tram-4"
	turning[0].Bearing = math.Mod(turning[0].Bearing+100, 360)
	tr.Annotate(turning)
	if turning[0].PatternID == "" {
		t.Error("turning tram unmatched")
	}

	// Far from any T4 shape → unmatched, annotations cleared.
	off := []state.Vehicle{{ID: "tram-3", Mode: feeds.ModeTram, Line: "T4", Lat: 59.30, Lon: 24.50, Bearing: -1, UpdatedAt: now}}
	tr.Annotate(off)
	if off[0].PatternID != "" {
		t.Errorf("off-route vehicle matched %q", off[0].PatternID)
	}
}

func TestProgressAndETAs(t *testing.T) {
	ix := fixtureIndex(t)
	g := BuildGeometry(ix)
	tr := NewTracker()
	tr.SetGeometry(g)
	out := mainPattern(ix, "tram:T4", 0)
	pg, _ := g.Pattern(out.ID)
	start := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)

	// 4 minutes of fixes every 10 s at 6 m/s.
	var v state.Vehicle
	for s := 0; s <= 240; s += 10 {
		v = tramAt(g, out, 300+6*float64(s), start.Add(time.Duration(s)*time.Second))
		vs := []state.Vehicle{v}
		tr.Annotate(vs)
		v = vs[0]
	}
	trip, ok := tr.Trip(v, v.UpdatedAt, 5)
	if !ok || len(trip.Ahead) == 0 {
		t.Fatal("no trip")
	}
	if math.Abs(trip.SpeedMps-6) > 0.5 {
		t.Errorf("speed = %.2f, want ~6", trip.SpeedMps)
	}
	for i, a := range trip.Ahead {
		// The learned segment paces say 6 m/s; allow 3 % (3 s minimum: a segment not yet
		// observed for paceMinMetres falls back to the blended speed).
		want := a.DistanceM / 6
		got := a.ETA.Sub(v.UpdatedAt).Seconds()
		if math.Abs(got-want) > math.Max(3, 0.03*want) {
			t.Errorf("stop %d ETA %.0fs, want ~%.0fs", i, got, want)
		}
		if i > 0 && !a.ETA.After(trip.Ahead[i-1].ETA) {
			t.Errorf("ETAs not increasing at %d", i)
		}
		if a.StopID != out.StopIDs[a.Index] || pg.StopAlong[a.Index] <= trip.Along {
			t.Errorf("ahead stop %d is not ahead", i)
		}
	}
	// ETAsAt finds the same prediction for one of those stops.
	target := trip.Ahead[2]
	got := tr.ETAsAt(map[string]state.Vehicle{v.ID: v}, "tram:T4", target.StopID, v.UpdatedAt)
	if len(got) != 1 || got[0].Sub(target.ETA).Abs() > time.Second {
		t.Errorf("ETAsAt = %v, want %v", got, target.ETA)
	}
}

func TestSegmentPacesShapeETAs(t *testing.T) {
	ix := fixtureIndex(t)
	g := BuildGeometry(ix)
	tr := NewTracker()
	tr.SetGeometry(g)
	out := mainPattern(ix, "tram:T4", 0)
	pg, _ := g.Pattern(out.ID)
	start := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	// A first tram drives the whole route: slow (3 m/s) on segment 2, fast (8 m/s) elsewhere.
	slowFrom, slowTo := pg.StopAlong[2], pg.StopAlong[3]
	along, at := 10.0, start
	for along < pg.StopAlong[len(pg.StopAlong)-1]-20 {
		v := []state.Vehicle{tramAt(g, out, along, at)}
		tr.Annotate(v)
		speed := 8.0
		if along >= slowFrom && along < slowTo {
			speed = 3
		}
		along += speed * 10
		at = at.Add(10 * time.Second)
	}
	// A second tram at the start: its ETA across segment 2 reflects the slow pace.
	v := tramAt(g, out, 5, at)
	v.ID = "tram-2"
	vs := []state.Vehicle{v}
	tr.Annotate(vs)
	trip, ok := tr.Trip(vs[0], at, 10)
	if !ok {
		t.Fatal("no trip")
	}
	eta := map[int]time.Time{}
	for _, a := range trip.Ahead {
		eta[a.Index] = a.ETA
	}
	slow := eta[3].Sub(eta[2]).Seconds() / (slowTo - slowFrom)
	fast := eta[5].Sub(eta[4]).Seconds() / (pg.StopAlong[5] - pg.StopAlong[4])
	if slow < 0.25 || slow > 0.45 || fast < 0.1 || fast > 0.16 {
		t.Errorf("pace slow %.3f s/m (want ~0.33), fast %.3f s/m (want ~0.125)", slow, fast)
	}
}

func TestTerminusLayoverIsNotTravelTime(t *testing.T) {
	ix := fixtureIndex(t)
	g := BuildGeometry(ix)
	tr := NewTracker()
	tr.SetGeometry(g)
	out := mainPattern(ix, "tram:T4", 0)
	pg, _ := g.Pattern(out.ID)
	at := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	drive := func(from, to float64) {
		for along := from; along < to; along += 80 {
			tr.Annotate([]state.Vehicle{tramAt(g, out, along, at)})
			at = at.Add(10 * time.Second)
		}
	}
	// Ten minutes parked at the first stop, then 8 m/s to the end, then ten minutes parked there.
	first, last := pg.StopAlong[0], pg.StopAlong[len(pg.StopAlong)-1]
	for range 60 {
		tr.Annotate([]state.Vehicle{tramAt(g, out, first, at)})
		at = at.Add(10 * time.Second)
	}
	drive(first, last)
	for range 60 {
		tr.Annotate([]state.Vehicle{tramAt(g, out, last, at)})
		at = at.Add(10 * time.Second)
	}
	ps := tr.paces[out.ID]
	for _, i := range []int{0, len(ps) - 1} {
		if pace := ps[i].secs / ps[i].metres; pace > 0.16 {
			t.Errorf("segment %d pace %.3f s/m, want ~0.125 (layover counted as travel)", i, pace)
		}
	}
	if s := tr.routeSpeed[out.RouteID]; s < 7 {
		t.Errorf("route speed %.1f m/s, want ~8 (layover counted)", s)
	}
}
