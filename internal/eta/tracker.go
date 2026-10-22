package eta

import (
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/state"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
)

// PatternGeom is a pattern with its shape prepared and stops placed along it.
type PatternGeom struct {
	Pattern   *staticdata.Pattern
	Shape     *Shape
	StopAlong []float64
	headsign  string
}

// Geometry indexes every pattern's geometry by route.
type Geometry struct {
	byRoute map[string][]*PatternGeom
	byID    map[string]*PatternGeom
}

// BuildGeometry prepares all patterns of an index (a few hundred; milliseconds).
func BuildGeometry(ix *staticdata.Index) *Geometry {
	g := &Geometry{byRoute: map[string][]*PatternGeom{}, byID: map[string]*PatternGeom{}}
	for _, r := range ix.Routes() {
		for _, p := range ix.Patterns(r.ID) {
			stops := make([][2]float64, 0, len(p.StopIDs))
			for _, id := range p.StopIDs {
				if s, ok := ix.Stop(id); ok {
					stops = append(stops, [2]float64{s.Lon, s.Lat})
				}
			}
			if len(stops) != len(p.StopIDs) || len(p.Shape) < 2 {
				continue
			}
			sh := NewShape(p.Shape)
			pg := &PatternGeom{Pattern: p, Shape: sh, StopAlong: sh.StopPositions(stops), headsign: staticdata.Fold(p.Headsign)}
			g.byRoute[r.ID] = append(g.byRoute[r.ID], pg)
			g.byID[p.ID] = pg
		}
	}
	return g
}

// Pattern returns one pattern's geometry.
func (g *Geometry) Pattern(id string) (*PatternGeom, bool) {
	pg, ok := g.byID[id]
	return pg, ok
}

// Matching thresholds.
const (
	maxOffsetM      = 120 // further from every shape than this: unmatched (detour, depot)
	oppositeBearing = 150 // a moving vehicle heading this far off the shape is going the other way
	backtrackM      = 60  // GPS jitter may move a vehicle back this much along its pattern
	headsignBonusM  = 60
	stickyBonusM    = 40
	bearingPenalty  = 1.5 // metres of score per degree of heading disagreement
	nextStopMarginM = 15  // a vehicle this close to a stop is treated as past it
	progressWindow  = 3 * time.Minute
)

// Typical commercial speeds (including stops) until a route has measurements.
var defaultSpeed = map[feeds.Mode]float64{feeds.ModeTram: 4.7, feeds.ModeBus: 5.5, feeds.ModeTrolley: 5.0, feeds.ModeTrain: 15}

type sample struct {
	at    time.Time
	along float64
}

type track struct {
	pattern  string
	along    float64
	nextStop int
	history  []sample // along-pattern progress, newest last, within progressWindow
}

// segPace accumulates time and distance driven on one stop-to-stop segment,
// dwell at the end stop included. Old observations decay so the pace follows
// traffic through the day.
type segPace struct{ secs, metres float64 }

const (
	paceDecay     = 0.995 // per observation on the segment
	paceMinMetres = 150   // trust a segment's own pace after this much observed driving
)

// Tracker matches vehicles to patterns and remembers their progress.
type Tracker struct {
	geo        atomic.Pointer[Geometry]
	mu         sync.Mutex
	tracks     map[string]*track
	lastPos    map[string][2]float64 // previous fix per vehicle, matched or not
	routeSpeed map[string]float64    // EWMA of observed progress rates, m/s
	paces      map[string][]segPace  // per pattern, one per segment (stop i → i+1)
}

func NewTracker() *Tracker {
	return &Tracker{tracks: map[string]*track{}, lastPos: map[string][2]float64{}, routeSpeed: map[string]float64{}, paces: map[string][]segPace{}}
}

// segment returns i such that StopAlong[i] <= along < StopAlong[i+1].
func segment(pg *PatternGeom, along float64) int {
	n := len(pg.StopAlong)
	for i := n - 2; i >= 0; i-- {
		if along >= pg.StopAlong[i] {
			return i
		}
	}
	return 0
}

// observe credits dt seconds and the distance from a to b to the segments driven.
func (t *Tracker) observe(pg *PatternGeom, a, b, dt float64) {
	if dt <= 0 || dt > 120 || b < a || b-a > 1500 || len(pg.StopAlong) < 2 {
		return
	}
	ps := t.paces[pg.Pattern.ID]
	if ps == nil {
		ps = make([]segPace, len(pg.StopAlong)-1)
		t.paces[pg.Pattern.ID] = ps
	}
	add := func(i int, secs, metres float64) {
		ps[i].secs = ps[i].secs*paceDecay + secs
		ps[i].metres = ps[i].metres*paceDecay + metres
	}
	if b == a { // standing still: dwell or traffic, charged to the segment it is on
		// Layover at either terminus is timetable slack, not travel time.
		if !atTerminus(pg, a) {
			add(segment(pg, a), dt, 0)
		}
		return
	}
	// Split time across segments in proportion to the distance driven in each.
	for i := segment(pg, a); i < len(ps) && a < b; i++ {
		end := min(b, pg.StopAlong[i+1])
		if i == len(ps)-1 {
			end = b
		}
		d := end - a
		if d > 0 {
			add(i, dt*d/(b-a+1e-9), d)
		}
		a = end
	}
}

// SetGeometry installs geometry after a static import.
func (t *Tracker) SetGeometry(g *Geometry) { t.geo.Store(g) }

// Geometry returns the current geometry (nil before the first import).
func (t *Tracker) Geometry() *Geometry { return t.geo.Load() }

// Annotate matches vehicles in place: PatternID, NextStopID and DistAlongM.
func (t *Tracker) Annotate(vs []state.Vehicle) {
	g := t.geo.Load()
	if g == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	seen := make(map[string]bool, len(vs))
	for i := range vs {
		v := &vs[i]
		seen[v.ID] = true
		// The store derives speed after annotation, so movement is judged here
		// from the previous fix.
		pos := [2]float64{v.Lat, v.Lon}
		prev, known := t.lastPos[v.ID]
		t.lastPos[v.ID] = pos
		moving := v.SpeedMps >= 1 || (known && prev != pos)
		tr := t.tracks[v.ID]
		pg, pr, ok := match(g, v, tr, moving)
		if !ok {
			delete(t.tracks, v.ID)
			continue
		}
		if tr == nil || tr.pattern != pg.Pattern.ID {
			tr = &track{pattern: pg.Pattern.ID}
			t.tracks[v.ID] = tr
		} else if n := len(tr.history); n > 0 {
			t.observe(pg, tr.history[n-1].along, pr.Along, v.UpdatedAt.Sub(tr.history[n-1].at).Seconds())
		}
		tr.along = pr.Along
		tr.nextStop = nextStop(pg, pr.Along)
		tr.history = appendSample(tr.history, sample{v.UpdatedAt, pr.Along})
		if rate, ok := progressRate(tr.history); ok && !atTerminus(pg, pr.Along) {
			rid := pg.Pattern.RouteID
			if cur, ok := t.routeSpeed[rid]; ok {
				t.routeSpeed[rid] = 0.9*cur + 0.1*rate
			} else {
				t.routeSpeed[rid] = rate
			}
		}
		v.PatternID = pg.Pattern.ID
		v.NextStopID = pg.Pattern.StopIDs[tr.nextStop]
		v.DistAlongM = float64(int(pr.Along))
	}
	for id := range t.tracks {
		if !seen[id] {
			delete(t.tracks, id)
		}
	}
	for id := range t.lastPos {
		if !seen[id] {
			delete(t.lastPos, id)
		}
	}
}

func match(g *Geometry, v *state.Vehicle, prev *track, moving bool) (*PatternGeom, Projection, bool) {
	var best *PatternGeom
	var bestPr Projection
	bestScore := 0.0
	dest := staticdata.Fold(v.Destination)
	for _, pg := range g.byRoute[staticdata.RouteID(v.Mode, v.Line)] {
		minAlong := 0.0
		sticky := prev != nil && prev.pattern == pg.Pattern.ID
		if sticky {
			minAlong = prev.along - backtrackM
		}
		pr, ok := pg.Shape.Project(v.Lat, v.Lon, minAlong)
		if !ok || pr.Offset > maxOffsetM {
			continue
		}
		score := pr.Offset
		if v.Bearing >= 0 {
			// Heading is a weighted hint, not a filter: vehicles turn at corners,
			// and a parked vehicle's reported heading is unreliable. On a two-way
			// street the opposite pattern still loses on this penalty.
			d := angleDiff(v.Bearing, pr.Bearing)
			if moving && d > oppositeBearing {
				continue
			}
			w := bearingPenalty
			if !moving {
				w /= 3
			}
			score += d * w
		}
		if dest != "" && dest == pg.headsign {
			score -= headsignBonusM
		}
		if sticky {
			score -= stickyBonusM
		}
		if best == nil || score < bestScore {
			best, bestPr, bestScore = pg, pr, score
		}
	}
	return best, bestPr, best != nil
}

// atTerminus reports whether along is at the pattern's first or last stop.
func atTerminus(pg *PatternGeom, along float64) bool {
	return along <= pg.StopAlong[0]+nextStopMarginM || along >= pg.StopAlong[len(pg.StopAlong)-1]-nextStopMarginM
}

func nextStop(pg *PatternGeom, along float64) int {
	for i, a := range pg.StopAlong {
		if a > along+nextStopMarginM {
			return i
		}
	}
	return len(pg.StopAlong) - 1
}

func appendSample(h []sample, s sample) []sample {
	if n := len(h); n > 0 && !s.at.After(h[n-1].at) {
		return h
	}
	h = append(h, s)
	i := 0
	for i < len(h)-1 && s.at.Sub(h[i].at) > progressWindow {
		i++
	}
	return h[i:]
}

// progressRate is metres per second along the pattern over the window, dwell included.
func progressRate(h []sample) (float64, bool) {
	if len(h) < 2 {
		return 0, false
	}
	first, last := h[0], h[len(h)-1]
	dt := last.at.Sub(first.at).Seconds()
	if dt < 60 {
		return 0, false
	}
	rate := (last.along - first.along) / dt
	if rate < 0 || rate > 30 {
		return 0, false
	}
	return rate, true
}

// StopETA is a predicted arrival.
type StopETA struct {
	StopID    string    `json:"stop_id"`
	Index     int       `json:"index"` // position in the pattern
	DistanceM float64   `json:"distance_m"`
	ETA       time.Time `json:"eta"`
}

// Trip is a vehicle's position on its pattern and the stops ahead.
type Trip struct {
	PatternID string    `json:"pattern_id"`
	Along     float64   `json:"along_m"`
	NextStop  int       `json:"next_stop_index"`
	SpeedMps  float64   `json:"speed_mps"` // the speed used for predictions
	Ahead     []StopETA `json:"ahead"`
}

// speedFor blends the vehicle's own recent progress with its route's average.
func (t *Tracker) speedFor(v state.Vehicle, tr *track, routeID string) float64 {
	route, haveRoute := t.routeSpeed[routeID]
	if !haveRoute {
		route = defaultSpeed[v.Mode]
	}
	if own, ok := progressRate(tr.history); ok {
		return max(1, 0.5*own+0.5*route)
	}
	return max(1, route)
}

// Trip predicts arrivals at up to limit stops ahead of a matched vehicle.
func (t *Tracker) Trip(v state.Vehicle, now time.Time, limit int) (Trip, bool) {
	g := t.geo.Load()
	if g == nil {
		return Trip{}, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	tr, ok := t.tracks[v.ID]
	if !ok {
		return Trip{}, false
	}
	pg, ok := g.byID[tr.pattern]
	if !ok {
		return Trip{}, false
	}
	speed := t.speedFor(v, tr, pg.Pattern.RouteID)
	paces := t.paces[pg.Pattern.ID]
	pace := func(seg int) float64 { // seconds per metre
		if seg < len(paces) && paces[seg].metres >= paceMinMetres {
			return paces[seg].secs / paces[seg].metres
		}
		return 1 / speed
	}
	// Time from the fix to each stop ahead, segment by segment; then subtract
	// the time already elapsed since the fix (never below zero).
	elapsed := max(0, now.Sub(v.UpdatedAt).Seconds())
	out := Trip{PatternID: pg.Pattern.ID, Along: tr.along, NextStop: tr.nextStop, SpeedMps: speed}
	pos, secs := tr.along, 0.0
	for i := tr.nextStop; i < len(pg.StopAlong) && len(out.Ahead) < limit; i++ {
		for pos < pg.StopAlong[i] {
			seg := segment(pg, pos)
			end := min(pg.StopAlong[i], pg.StopAlong[min(seg+1, len(pg.StopAlong)-1)])
			if end <= pos {
				end = pg.StopAlong[i]
			}
			secs += (end - pos) * pace(seg)
			pos = end
		}
		left := max(0, secs-elapsed)
		d := max(0, pg.StopAlong[i]-tr.along)
		out.Ahead = append(out.Ahead, StopETA{StopID: pg.Pattern.StopIDs[i], Index: i, DistanceM: float64(int(d)), ETA: now.Add(time.Duration(left * float64(time.Second)))})
	}
	return out, true
}

// Prediction is one vehicle's predicted arrival at a stop.
type Prediction struct {
	VehicleID string
	ETA       time.Time
	StopsAway int     // stops between the vehicle and this one (0 = it is the next stop)
	DistanceM float64 // along the pattern
	AtStart   bool    // the vehicle has not left its first stop (terminus layover)
}

// Predictions lists matched vehicles of a route due at a stop, soonest first.
func (t *Tracker) Predictions(vehicles map[string]state.Vehicle, routeID, stopID string, now time.Time) []Prediction {
	g := t.geo.Load()
	if g == nil {
		return nil
	}
	var out []Prediction
	for _, v := range vehicles {
		if staticdata.RouteID(v.Mode, v.Line) != routeID || v.PatternID == "" {
			continue
		}
		trip, ok := t.Trip(v, now, 200)
		if !ok {
			continue
		}
		for k, a := range trip.Ahead {
			if a.StopID == stopID {
				out = append(out, Prediction{VehicleID: v.ID, ETA: a.ETA, StopsAway: k, DistanceM: a.DistanceM, AtStart: trip.NextStop <= 1 && trip.Along < 30})
				break
			}
		}
	}
	slices.SortFunc(out, func(a, b Prediction) int { return a.ETA.Compare(b.ETA) })
	return out
}

// ETAsAt predicts when matched vehicles of a route reach a stop, soonest first.
func (t *Tracker) ETAsAt(vehicles map[string]state.Vehicle, routeID, stopID string, now time.Time) []time.Time {
	ps := t.Predictions(vehicles, routeID, stopID, now)
	out := make([]time.Time, len(ps))
	for i, p := range ps {
		out[i] = p.ETA
	}
	return out
}
