package eta

import (
	"math"
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
	// rail holds the train patterns, matched by MatchTrains only (not by Annotate,
	// and not used for ETAs).
	rail []*PatternGeom
}

// BuildGeometry prepares the city patterns of an index (a few hundred; milliseconds).
func BuildGeometry(ix *staticdata.Index) *Geometry {
	g := &Geometry{byRoute: map[string][]*PatternGeom{}, byID: map[string]*PatternGeom{}}
	for _, r := range ix.Routes() {
		if r.Mode == feeds.ModeTrain {
			g.rail = append(g.rail, railGeoms(ix, r.ID)...)
			continue
		}
		if r.Source != "tallinn" {
			continue // county buses report no live positions
		}
		for _, p := range ix.Patterns(r.ID) {
			stops := make([][2]float64, 0, len(p.StopIDs))
			for _, id := range p.StopIDs {
				if s, ok := ix.Stop(id); ok {
					stops = append(stops, [2]float64{s.Lon, s.Lat})
				}
			}
			// At least two stops: the tracker indexes the first stop and the one ahead.
			if len(stops) != len(p.StopIDs) || len(stops) < 2 || len(p.Shape) < 2 {
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
	pattern   string
	along     float64
	nextStop  int
	history   []sample // along-pattern progress, newest last, within progressWindow
	probes    []probe  // own predictions waiting to be checked against the arrival
	lastProbe time.Time
	// stillSince is when the vehicle stopped making progress (zero while moving).
	stillSince time.Time
}

// probe is a prediction the tracker made for itself: from a fix at at, the
// model said secs until the vehicle reaches stop. When it does, the ratio of
// actual to predicted time calibrates the model (see calibrate).
type probe struct {
	stop int
	at   time.Time
	secs float64
}

// Self-calibration: the per-mode factor scaling predictions, learned from the
// tracker's own predictions against arrivals seen in the GPS feed.
const (
	probeEvery     = time.Minute
	probeMinSecs   = 120  // shorter predictions are too noisy to learn from
	probeMaxSecs   = 1200 // and longer ones rarely resolve on the same pattern
	probeMaxAge    = 45 * time.Minute
	probesPerTrack = 30
	calibAlpha     = 0.01 // per resolved probe; hundreds resolve per minute at peak
	calibBands     = 3    // horizons < 5 min, 5–10 min, ≥ 10 min: errors are not proportional to the horizon
	// calibStrength applies part of the learned correction. Probes only resolve
	// for vehicles that reach the stop on the same pattern, a biased sample; on
	// two recorded peaks (docs/feeds.md) half strength matched observed arrivals
	// best (81 % and 74 % within ±1 min, against 79 %/72 % uncorrected and
	// 76 %/71 % at full strength).
	calibStrength = 0.5
)

func calibBand(secs float64) int {
	switch {
	case secs < 300:
		return 0
	case secs < 600:
		return 1
	}
	return 2
}

// scale is the learned factor for a prediction of secs (callers hold t.mu).
func (t *Tracker) scale(mode feeds.Mode, secs float64) float64 {
	if c := t.calib[mode]; c != nil {
		return math.Exp(calibStrength * c[calibBand(secs)])
	}
	return 1
}

// segPace accumulates time and distance driven on one stop-to-stop segment,
// traffic stops included but not dwell at the stops themselves. Old
// observations decay so the pace follows traffic through the day.
type segPace struct{ secs, metres float64 }

// stopDwell accumulates time stood at a stop and the number of passes, so
// secs/passes is the mean dwell per pass (skipped stops count as zero).
type stopDwell struct{ secs, passes float64 }

const (
	dwellRadiusM = 25 // standing this close to a stop is dwelling there
	// A vehicle standing longer than this (breakdown, driver's break, a frozen
	// GPS unit) stops teaching paces and dwell: otherwise one parked vehicle
	// slows every line sharing the street for hours.
	maxStill = 3 * time.Minute
	// A fix older than this gives no ETAs.
	staleFix = 2 * time.Minute
	// Progress below this between fixes is GPS jitter, not driving.
	stillM         = 5.0
	dwellMinPasses = 3 // trust a stop's dwell after this many passes
)

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
	// Paces are shared by every pattern driving the same stop-to-stop segment
	// (many bus lines share streets), dwell by every pattern calling at a stop.
	paces  map[[2]string]*segPace
	dwells map[string]*stopDwell
	calib  map[feeds.Mode]*[calibBands]float64 // log of the prediction scale per mode and horizon band; 0 = unscaled
	// trainPattern is each train's matched rail pattern, for stickiness (MatchTrains).
	trainPattern map[string]string
}

func NewTracker() *Tracker {
	return &Tracker{tracks: map[string]*track{}, lastPos: map[string][2]float64{}, routeSpeed: map[string]float64{}, paces: map[[2]string]*segPace{}, dwells: map[string]*stopDwell{}, calib: map[feeds.Mode]*[calibBands]float64{}}
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

// observe credits dt seconds and the distance from a to b: standing at a stop
// is dwell there, any other time goes to the segments driven.
func (t *Tracker) observe(pg *PatternGeom, a, b, dt float64) {
	if dt <= 0 || dt > 120 || b < a || b-a > 1500 || len(pg.StopAlong) < 2 {
		return
	}
	stops := pg.Pattern.StopIDs
	seg := func(i int) *segPace {
		k := [2]string{stops[i], stops[i+1]}
		p := t.paces[k]
		if p == nil {
			p = &segPace{}
			t.paces[k] = p
		}
		return p
	}
	dw := func(i int) *stopDwell {
		d := t.dwells[stops[i]]
		if d == nil {
			d = &stopDwell{}
			t.dwells[stops[i]] = d
		}
		return d
	}
	add := func(i int, secs, metres float64) {
		p := seg(i)
		p.secs = p.secs*paceDecay + secs
		p.metres = p.metres*paceDecay + metres
	}
	nseg := len(pg.StopAlong) - 1
	if math.Abs(b-a) < stillM { // standing still (or GPS jitter)
		switch {
		case atTerminus(pg, a): // layover is timetable slack, not travel time
		case nearStop(pg, a) >= 0:
			dw(nearStop(pg, a)).secs += dt
		default: // traffic: part of the segment's travel time
			add(segment(pg, a), dt, 0)
		}
		return
	}
	// Count passes of intermediate stops, then split time across segments in
	// proportion to the distance driven in each.
	for i := 1; i < len(pg.StopAlong)-1; i++ {
		if a < pg.StopAlong[i] && pg.StopAlong[i] <= b {
			d := dw(i)
			d.secs *= paceDecay
			d.passes = d.passes*paceDecay + 1
		}
	}
	for i := segment(pg, a); i < nseg && a < b; i++ {
		end := min(b, pg.StopAlong[i+1])
		if i == nseg-1 {
			end = b
		}
		d := end - a
		if d > 0 {
			add(i, dt*d/(b-a+1e-9), d)
		}
		a = end
	}
}

// nearStop returns the intermediate stop within dwellRadiusM of along, or -1.
func nearStop(pg *PatternGeom, along float64) int {
	for i := 1; i < len(pg.StopAlong)-1; i++ {
		if d := along - pg.StopAlong[i]; d > -dwellRadiusM && d < dwellRadiusM {
			return i
		}
	}
	return -1
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
			last := tr.history[n-1]
			if math.Abs(pr.Along-last.along) >= stillM {
				tr.stillSince = time.Time{}
			} else if tr.stillSince.IsZero() {
				tr.stillSince = last.at
			}
			if tr.stillSince.IsZero() || v.UpdatedAt.Sub(tr.stillSince) <= maxStill {
				t.observe(pg, last.along, pr.Along, v.UpdatedAt.Sub(last.at).Seconds())
			}
		}
		tr.along = pr.Along
		tr.nextStop = nextStop(pg, pr.Along)
		tr.history = appendSample(tr.history, sample{v.UpdatedAt, pr.Along})
		// A stationary vehicle's rate (≈0) would drag the route's speed towards nothing.
		if rate, ok := progressRate(tr.history); ok && rate >= 0.5 && !atTerminus(pg, pr.Along) {
			rid := pg.Pattern.RouteID
			if cur, ok := t.routeSpeed[rid]; ok {
				t.routeSpeed[rid] = 0.9*cur + 0.1*rate
			} else {
				t.routeSpeed[rid] = rate
			}
		}
		t.calibrate(v, pg, tr)
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
	// From an old fix every stop within the gap would read as "now": better no ETAs.
	if now.Sub(v.UpdatedAt) > staleFix {
		return Trip{}, false
	}
	pg, ok := g.byID[tr.pattern]
	if !ok {
		return Trip{}, false
	}
	speed := t.speedFor(v, tr, pg.Pattern.RouteID)
	// Time from the fix to each stop ahead; then subtract the time already
	// elapsed since the fix (never below zero).
	elapsed := max(0, now.Sub(v.UpdatedAt).Seconds())
	out := Trip{PatternID: pg.Pattern.ID, Along: tr.along, NextStop: tr.nextStop, SpeedMps: speed}
	for _, a := range t.ahead(pg, tr, speed, limit) {
		left := max(0, a.secs*t.scale(v.Mode, a.secs)-elapsed)
		d := max(0, pg.StopAlong[a.index]-tr.along)
		out.Ahead = append(out.Ahead, StopETA{StopID: pg.Pattern.StopIDs[a.index], Index: a.index, DistanceM: float64(int(d)), ETA: now.Add(time.Duration(left * float64(time.Second)))})
	}
	return out, true
}

type stopSecs struct {
	index   int
	secs    float64 // from the fix, unscaled
	learned bool    // every segment up to here has its own measured pace
}

// ahead is the model: seconds from the vehicle's last fix to each of up to
// limit stops ahead, summing segment paces and dwell at the stops in between.
// Callers hold t.mu.
func (t *Tracker) ahead(pg *PatternGeom, tr *track, speed float64, limit int) []stopSecs {
	ids := pg.Pattern.StopIDs
	segPaceOf := func(seg int) *segPace {
		if seg+1 >= len(ids) {
			return nil
		}
		p := t.paces[[2]string{ids[seg], ids[seg+1]}]
		if p == nil || p.metres < paceMinMetres {
			return nil
		}
		return p
	}
	pace := func(seg int) float64 { // seconds per metre
		if p := segPaceOf(seg); p != nil {
			return p.secs / p.metres
		}
		return 1 / speed // the route speed already includes dwell
	}
	dwell := func(stop int) (float64, bool) { // seconds stood at a stop the vehicle will pass
		d := t.dwells[ids[stop]]
		if d != nil && d.passes >= dwellMinPasses && segPaceOf(stop) != nil {
			return d.secs / d.passes, true
		}
		return 0, false
	}
	var out []stopSecs
	pos, secs, learned := tr.along, 0.0, true
	for i := tr.nextStop; i < len(pg.StopAlong) && len(out) < limit; i++ {
		for pos < pg.StopAlong[i] {
			seg := segment(pg, pos)
			learned = learned && segPaceOf(seg) != nil
			end := min(pg.StopAlong[i], pg.StopAlong[min(seg+1, len(pg.StopAlong)-1)])
			if end <= pos {
				end = pg.StopAlong[i]
			}
			secs += (end - pos) * pace(seg)
			pos = end
		}
		out = append(out, stopSecs{i, secs, learned})
		d, ok := dwell(i) // it stands at this stop before driving on
		secs += d
		learned = learned && ok
	}
	return out
}

// calibrate checks the vehicle's earlier probes against where it is now and
// takes new ones. Callers hold t.mu.
func (t *Tracker) calibrate(v *state.Vehicle, pg *PatternGeom, tr *track) {
	keep := tr.probes[:0]
	for _, p := range tr.probes {
		switch {
		case v.UpdatedAt.Sub(p.at) > probeMaxAge:
			// stuck or diverted: no arrival to learn from
		case tr.along >= pg.StopAlong[p.stop]-nextStopMarginM:
			if actual := v.UpdatedAt.Sub(p.at).Seconds(); actual > 0 {
				r := min(max(actual/p.secs, 0.5), 2)
				c := t.calib[v.Mode]
				if c == nil {
					c = &[calibBands]float64{}
					t.calib[v.Mode] = c
				}
				b := calibBand(p.secs)
				c[b] = (1-calibAlpha)*c[b] + calibAlpha*math.Log(r)
			}
		default:
			keep = append(keep, p)
		}
	}
	tr.probes = keep
	if v.UpdatedAt.Sub(tr.lastProbe) < probeEvery || len(tr.probes) >= probesPerTrack {
		return
	}
	tr.lastProbe = v.UpdatedAt
	// One probe per horizon band per vehicle and minute.
	var near, mid, far *stopSecs
	for _, a := range t.ahead(pg, tr, t.speedFor(*v, tr, pg.Pattern.RouteID), 200) {
		if !a.learned {
			break // fallback speeds would teach the scale about the fallback, not the paces
		}
		if a.secs < probeMinSecs {
			continue
		}
		if a.secs > probeMaxSecs {
			break
		}
		switch {
		case near == nil:
			near = &a
		case a.secs >= 300 && mid == nil:
			mid = &a
		case a.secs >= 600 && far == nil:
			far = &a
		}
	}
	for _, a := range []*stopSecs{near, mid, far} {
		if a != nil {
			tr.probes = append(tr.probes, probe{a.index, v.UpdatedAt, a.secs})
		}
	}
}

// Calibration returns the learned prediction scales per mode and horizon band
// (< 5 min, 5–10 min, ≥ 10 min; 1 = unscaled).
func (t *Tracker) Calibration() map[feeds.Mode][calibBands]float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[feeds.Mode][calibBands]float64, len(t.calib))
	for m, c := range t.calib {
		var k [calibBands]float64
		for i, l := range c {
			k[i] = math.Exp(l)
		}
		out[m] = k
	}
	return out
}

// Prediction is one vehicle's predicted arrival at a stop.
type Prediction struct {
	VehicleID string
	PatternID string
	StopIndex int // position of the stop in the pattern
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
				out = append(out, Prediction{VehicleID: v.ID, PatternID: trip.PatternID, StopIndex: a.Index, ETA: a.ETA, StopsAway: k, DistanceM: a.DistanceM, AtStart: trip.NextStop <= 1 && trip.Along < 30})
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
