package eta

import (
	"testing"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/state"
)

// A vehicle parked for half an hour (breakdown, driver's break, frozen GPS)
// must not teach the shared paces, dwell and route speed that the street is
// nearly impassable.
func TestParkedVehicleDoesNotPoisonPaces(t *testing.T) {
	ix := fixtureIndex(t)
	g := BuildGeometry(ix)
	tr := NewTracker()
	tr.SetGeometry(g)
	out := mainPattern(ix, "tram:T4", 0)
	pg, _ := g.Pattern(out.ID)
	at := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	last := pg.StopAlong[len(pg.StopAlong)-1]
	for range 5 {
		for along := 10.0; along < last-20; along += 80 {
			tr.Annotate([]state.Vehicle{tramAt(g, out, along, at)})
			at = at.Add(10 * time.Second)
		}
		tr.Annotate(nil)
	}
	k := [2]string{out.StopIDs[5], out.StopIDs[6]}
	pace := func() float64 { p := tr.paces[k]; return p.secs / p.metres }
	dwell := func() float64 { d := tr.dwells[out.StopIDs[5]]; return d.secs / d.passes }
	basePace, baseDwell := pace(), dwell()

	park := func(along float64, id string, jitter float64) {
		for i := range 180 { // 30 minutes of 10 s fixes
			v := tramAt(g, out, along+jitter*float64(i%2), at)
			v.ID, v.SpeedMps = id, 0
			tr.Annotate([]state.Vehicle{v})
			at = at.Add(10 * time.Second)
		}
		tr.Annotate(nil)
	}
	park((pg.StopAlong[5]+pg.StopAlong[6])/2, "tram-2", 2) // mid-segment, with GPS jitter
	park(pg.StopAlong[5]+5, "tram-3", 0)                   // at the stop

	if got := pace(); got > 1.5*basePace {
		t.Errorf("segment pace %.3f s/m after parking, baseline %.3f", got, basePace)
	}
	if got := dwell(); got > baseDwell+200 {
		t.Errorf("stop dwell %.0f s per pass after parking, baseline %.0f", got, baseDwell)
	}
	if sp := tr.routeSpeed[out.RouteID]; sp < 2 {
		t.Errorf("route speed %.2f m/s after parking", sp)
	}
}

func TestPatternTimes(t *testing.T) {
	ix := fixtureIndex(t)
	tr := NewTracker()
	tr.SetGeometry(BuildGeometry(ix))
	check := func(name string, times []float64, n int) {
		t.Helper()
		if len(times) != n || times[0] != 0 {
			t.Fatalf("%s: %v", name, times)
		}
		for i := 1; i < n; i++ {
			if times[i] <= times[i-1] {
				t.Fatalf("%s: not increasing at %d: %v", name, i, times)
			}
		}
	}
	tram := mainPattern(ix, "tram:T4", 0)
	times := tr.PatternTimes(ix, tram)
	check("tram", times, len(tram.StopIDs))
	// A whole T4 run takes the better part of an hour, not seconds or days.
	if total := times[len(times)-1]; total < 15*60 || total > 75*60 {
		t.Errorf("T4 end to end: %.0f s", total)
	}
	// Rail is not tracked: the static estimate.
	for _, r := range ix.Routes() {
		if r.Mode == feeds.ModeTrain {
			p := ix.Patterns(r.ID)[0]
			check("rail", tr.PatternTimes(ix, p), len(p.StopIDs))
			break
		}
	}
}
