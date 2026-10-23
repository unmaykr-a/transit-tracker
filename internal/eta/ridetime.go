package eta

import (
	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/geo"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
)

// PatternTimes estimates the riding time, in seconds, from a pattern's first
// stop to each of its stops (out[0] is 0), so a ride from stop i to stop j
// takes out[j]-out[i]. City patterns use the learned paces and dwell of each
// segment (the route speed where a segment has no data yet); rail and county
// patterns, which have no live positions, use distance at a typical speed.
// Departures give the real start of a ride; this fills in when it ends.
func (t *Tracker) PatternTimes(ix *staticdata.Index, p *staticdata.Pattern) []float64 {
	if g := t.geo.Load(); g != nil {
		if pg, ok := g.byID[p.ID]; ok && t.hasLive(ix, p) {
			return t.learnedTimes(pg)
		}
	}
	return StaticTimes(ix, p)
}

// hasLive reports whether live data can inform a pattern's ride times: city
// lines always (their default speeds are city speeds), county lines once one of
// their buses has been followed (until then StaticTimes' county speed is better).
func (t *Tracker) hasLive(ix *staticdata.Index, p *staticdata.Pattern) bool {
	if r, ok := ix.Route(p.RouteID); !ok || r.Source != "county" {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.routeSpeed[p.RouteID]
	return ok
}

func (t *Tracker) learnedTimes(pg *PatternGeom) []float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	speed, ok := t.routeSpeed[pg.Pattern.RouteID]
	if !ok {
		speed = defaultSpeed[modeOf(pg.Pattern.RouteID)]
	}
	out := make([]float64, len(pg.StopAlong))
	tr := &track{along: pg.StopAlong[0], nextStop: 1}
	for _, a := range t.ahead(pg, tr, max(1, speed), len(pg.StopAlong)) {
		out[a.index] = a.secs
	}
	return out
}

// Typical speeds between stops, dwell included, for patterns without live data.
var staticSpeed = map[feeds.Mode]float64{feeds.ModeTram: 4.7, feeds.ModeBus: 5.5, feeds.ModeTrolley: 5.0, feeds.ModeTrain: 15}

const (
	countySpeed = 8.0 // county buses run mostly outside the city
	roadDetour  = 1.25
	railDetour  = 1.1
)

// StaticTimes estimates riding times from the straight distances between
// consecutive stops, a detour factor and a typical speed for the mode.
func StaticTimes(ix *staticdata.Index, p *staticdata.Pattern) []float64 {
	out := make([]float64, len(p.StopIDs))
	r, _ := ix.Route(p.RouteID)
	mode := modeOf(p.RouteID)
	speed, detour := staticSpeed[mode], roadDetour
	if mode == feeds.ModeTrain {
		detour = railDetour
	}
	if r != nil && r.Source == "county" {
		speed = countySpeed
	}
	if speed == 0 {
		speed = staticSpeed[feeds.ModeBus]
	}
	var prev *staticdata.Stop
	for i, id := range p.StopIDs {
		s, ok := ix.Stop(id)
		if i > 0 {
			out[i] = out[i-1]
			if ok && prev != nil {
				out[i] += geo.DistanceM(prev.Lat, prev.Lon, s.Lat, s.Lon) * detour / speed
			} else {
				out[i] += 90 // a stop missing from the index: about one stop's ride
			}
		}
		if ok {
			prev = s
		}
	}
	return out
}

// modeOf reads the mode from a route id ("tram:T4").
func modeOf(routeID string) feeds.Mode {
	for i := range len(routeID) {
		if routeID[i] == ':' {
			return feeds.Mode(routeID[:i])
		}
	}
	return feeds.ModeBus
}
