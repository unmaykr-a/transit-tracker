package eta

import (
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
	"github.com/unmaykr-a/transit-tracker/internal/state"
)

// Trains report a route name ("Tallinn - Turba") and a trip number, not a GTFS
// pattern, so they are matched to rail patterns by position: the pattern whose
// track runs closest to the train, preferring one heading to the train's
// destination and the pattern it had before. This gives trains a pattern_id,
// so the map can draw a selected train's line and move it along the track.

const (
	railMaxOffsetM   = 300 // further from every track: not matched (depot, siding, bad fix)
	railHeadsignBonM = 150
	railStickyBonM   = 100
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
		head := p.Headsign
		if s, ok := ix.Stop(p.StopIDs[len(p.StopIDs)-1]); ok && head == "" {
			head = s.Name
		}
		sh := NewShape(p.Shape)
		out = append(out, &PatternGeom{Pattern: p, Shape: sh, StopAlong: sh.StopPositions(stops), headsign: staticdata.Fold(head)})
	}
	return out
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
		dest := staticdata.Fold(v.Destination)
		var best *PatternGeom
		var bestProj Projection
		bestScore := 0.0
		for _, pg := range g.rail {
			pr, ok := pg.Shape.Project(v.Lat, v.Lon, 0)
			if !ok || pr.Offset > railMaxOffsetM {
				continue
			}
			score := pr.Offset
			if dest != "" && pg.headsign == dest {
				score -= railHeadsignBonM
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
