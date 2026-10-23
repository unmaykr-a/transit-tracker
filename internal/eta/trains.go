package eta

import (
	"slices"
	"strings"

	"github.com/unmaykr-a/transit-tracker/internal/state"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
)

// Trains report a route name ("Keila - Tallinn") and a trip number, not a GTFS
// pattern. In the city every line shares the same tracks, so position alone
// cannot tell them apart; the route name can: it names the first and last
// station, which is also how Elron's GTFS names its trips. A train is matched
// only to patterns calling at both stations in that order (best: starting and
// ending there), then by distance to the track, heading and the pattern it had
// before. No such pattern (a line missing from the timetable data, a renamed
// station): no match, rather than another line's track.

const (
	// Further from every qualifying track: not matched (depot, bad fix). Generous,
	// because the station names already pick the line; this only rules out
	// trains nowhere near it, and tolerates coarse GTFS shapes.
	railMaxOffsetM = 1500
	railExactBonM  = 400 // the pattern starts and ends at the route name's stations
	railStickyBonM = 50
	railWrongWayM  = 300 // heading against the pattern's direction (when Elron reports a heading)
)

func railGeoms(ix *staticdata.Index, routeID string) []*PatternGeom {
	var out []*PatternGeom
	for _, p := range ix.Patterns(routeID) {
		stops := make([][2]float64, 0, len(p.StopIDs))
		for _, id := range p.StopIDs {
			if s, ok := ix.Stop(id); ok {
				stops = append(stops, [2]float64{s.Lon, s.Lat})
			}
		}
		if len(stops) != len(p.StopIDs) || len(stops) < 2 || len(p.Shape) < 2 {
			continue
		}
		names := make([]string, len(p.StopIDs))
		for i, id := range p.StopIDs {
			s, _ := ix.Stop(id)
			names[i] = stationName(s.Name)
		}
		sh := NewShape(p.Shape)
		out = append(out, &PatternGeom{Pattern: p, Shape: sh, StopAlong: sh.StopPositions(stops), headsign: staticdata.Fold(p.Headsign), stations: names})
	}
	return out
}

// stationName folds a station name for matching; Tallinn's station is also called Balti jaam.
func stationName(name string) string {
	n := staticdata.Fold(name)
	if n == "balti jaam" {
		return "tallinn"
	}
	return n
}

// routeEnds reads the first and last station from an Elron route name.
func routeEnds(route string) (from, to string) {
	i := strings.LastIndex(route, "-")
	if i < 0 {
		return "", stationName(route)
	}
	return stationName(route[:i]), stationName(route[i+1:])
}

// railFit reports whether pattern pg can be the train's route from..to: it
// calls at both in order (ok), and whether it starts and ends there (exact).
func railFit(pg *PatternGeom, from, to string) (ok, exact bool) {
	n := len(pg.stations)
	if from == "" || to == "" || n < 2 {
		return false, false
	}
	i := slices.Index(pg.stations, from)
	if i < 0 {
		return false, false
	}
	j := slices.Index(pg.stations[i+1:], to)
	if j < 0 {
		return false, false
	}
	return true, i == 0 && i+1+j == n-1
}

// MatchTrains sets the pattern, progress and next stop of train vehicles.
func (t *Tracker) MatchTrains(vs []state.Vehicle) {
	g := t.geo.Load()
	if g == nil || len(g.rail) == 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.trainPattern == nil {
		t.trainPattern = map[string]string{}
	}
	seen := make(map[string]bool, len(vs))
	for i := range vs {
		v := &vs[i]
		from, to := routeEnds(v.Line)
		var best *PatternGeom
		var bestProj Projection
		bestScore := 0.0
		for _, pg := range g.rail {
			fits, exact := railFit(pg, from, to)
			if !fits {
				continue
			}
			pr, ok := pg.Shape.Project(v.Lat, v.Lon, 0)
			if !ok || pr.Offset > railMaxOffsetM {
				continue
			}
			score := pr.Offset
			if exact {
				score -= railExactBonM
			}
			if v.Bearing >= 0 && angleDiff(v.Bearing, pr.Bearing) > 120 {
				score += railWrongWayM
			}
			if t.trainPattern[v.ID] == pg.Pattern.ID {
				score -= railStickyBonM
			}
			if best == nil || score < bestScore {
				best, bestProj, bestScore = pg, pr, score
			}
		}
		seen[v.ID] = true
		if best == nil {
			delete(t.trainPattern, v.ID)
			continue
		}
		t.trainPattern[v.ID] = best.Pattern.ID
		v.PatternID, v.DistAlongM = best.Pattern.ID, bestProj.Along
		if n := nextStop(best, bestProj.Along); n < len(best.Pattern.StopIDs) {
			v.NextStopID = best.Pattern.StopIDs[n]
		}
	}
	for id := range t.trainPattern {
		if !seen[id] {
			delete(t.trainPattern, id) // trains that left the feed
		}
	}
}
