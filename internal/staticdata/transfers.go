package staticdata

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/unmaykr-a/transit-tracker/internal/geo"
)

// Transfer is a change between two rides: get off in stop area Alight and get
// on again in area Board (the same area, or one a short walk away).
type Transfer struct {
	Alight string `json:"alight"`
	Board  string `json:"board"`
}

// PatternTimer estimates the riding seconds from a pattern's first stop to
// each of its stops (eta.Tracker.PatternTimes).
type PatternTimer func(p *Pattern) []float64

// TransferOption is one way to make a trip with changes.
type TransferOption struct {
	Transfers []Transfer
	// RouteIDs is the quickest route of each leg (the trip's lines per leg come
	// from Connections between the leg's areas).
	RouteIDs []string
	// Seconds estimates the whole trip from the first departure: rides, walks
	// between stops and an average wait at each change.
	Seconds float64
}

const (
	// MaxTransfers is how many changes a saved trip may have.
	MaxTransfers = 2
	// MaxTransferWalkM is the furthest walk between stop areas offered at a change.
	MaxTransferWalkM = 400
	transferWaitS    = 240 // getting off, on and an average wait for the next vehicle
	walkMps          = 1.3
	walkDetour       = 1.3
)

// TransferWalkS is the walking time between two stop areas at a change.
func TransferWalkS(a, b *Area) float64 {
	if a.ID == b.ID {
		return 0
	}
	return geo.DistanceM(a.Lat, a.Lon, b.Lat, b.Lon) * walkDetour / walkMps
}

// reach is, per stop area, the quickest ride to (or from) it by each route.
type reach map[string]map[string]float64

func (r reach) best(area string) (float64, string) {
	secs, route := math.Inf(1), ""
	for rt, s := range r[area] {
		if s < secs || (s == secs && rt < route) {
			secs, route = s, rt
		}
	}
	return secs, route
}

type searcher struct {
	ix    *Index
	timer PatternTimer
	times map[string][]float64
}

func (s *searcher) timesOf(p *Pattern) []float64 {
	if t, ok := s.times[p.ID]; ok {
		return t
	}
	t := s.timer(p)
	if len(t) != len(p.StopIDs) {
		t = make([]float64, len(p.StopIDs)) // a broken timer must not break the search
		for i := range t {
			t[i] = float64(i) * 90
		}
	}
	s.times[p.ID] = t
	return t
}

// rides visits every ride starting (forward) or ending (backward) in area.
func (s *searcher) rides(area string, forward bool) reach {
	a, ok := s.ix.areas[area]
	if !ok {
		return nil
	}
	out := reach{}
	add := func(other, route string, secs float64) {
		m := out[other]
		if m == nil {
			m = map[string]float64{}
			out[other] = m
		}
		if old, ok := m[route]; !ok || secs < old {
			m[route] = secs
		}
	}
	for routeID, pats := range s.ix.patterns {
		for _, p := range pats {
			var times []float64
			for k, sid := range p.StopIDs {
				if !slices.Contains(a.StopIDs, sid) {
					continue
				}
				if times == nil {
					times = s.timesOf(p)
				}
				if forward {
					for j := k + 1; j < len(p.StopIDs); j++ {
						if st, ok := s.ix.stops[p.StopIDs[j]]; ok && st.AreaID != area {
							add(st.AreaID, routeID, times[j]-times[k])
						}
					}
				} else {
					for i := k - 1; i >= 0; i-- {
						if st, ok := s.ix.stops[p.StopIDs[i]]; ok && st.AreaID != area {
							add(st.AreaID, routeID, times[k]-times[i])
						}
					}
				}
			}
		}
	}
	return out
}

// TransferOptions finds ways from area from to area to with one change, or two
// when one is not enough, quickest first: up to limit options, each with a
// different combination of quickest lines. Changes are in one stop area or
// between areas up to MaxTransferWalkM apart.
func (ix *Index) TransferOptions(from, to string, timer PatternTimer, limit int) []TransferOption {
	_, okA := ix.areas[from]
	_, okB := ix.areas[to]
	if !okA || !okB || from == to || limit <= 0 {
		return nil
	}
	s := &searcher{ix: ix, timer: timer, times: map[string][]float64{}}
	fwd, back := s.rides(from, true), s.rides(to, false)
	near := func(x string) []string { // areas within walking distance of x that reach the destination
		ax := ix.areas[x]
		var out []string
		for y := range back {
			ay := ix.areas[y]
			if y == from || ax == nil || ay == nil {
				continue
			}
			if y == x || geo.DistanceM(ax.Lat, ax.Lon, ay.Lat, ay.Lon) <= MaxTransferWalkM {
				out = append(out, y)
			}
		}
		return out
	}

	var opts []TransferOption
	for x := range fwd {
		if x == to {
			continue // a direct ride, not a change
		}
		r1s, r1 := fwd.best(x)
		for _, y := range near(x) {
			r2s, r2 := back.best(y)
			if r1 == r2 && x == y {
				continue // staying on the same line is not a change
			}
			opts = append(opts, TransferOption{
				Transfers: []Transfer{{x, y}}, RouteIDs: []string{r1, r2},
				Seconds: r1s + TransferWalkS(ix.areas[x], ix.areas[y]) + transferWaitS + r2s,
			})
		}
	}
	out := pickOptions(opts, limit)
	if len(out) >= 2 || MaxTransfers < 2 {
		return out
	}

	// Two changes: from the quickest first-change areas, one more ride to an
	// area within walking distance of a ride to the destination.
	firsts := make([]string, 0, len(fwd))
	for x := range fwd {
		if x != to {
			firsts = append(firsts, x)
		}
	}
	slices.SortFunc(firsts, func(p, q string) int {
		ps, _ := fwd.best(p)
		qs, _ := fwd.best(q)
		return cmp.Or(cmp.Compare(ps, qs), strings.Compare(p, q))
	})
	for _, x := range firsts[:min(len(firsts), 40)] {
		r1s, r1 := fwd.best(x)
		mid := s.rides(x, true)
		for z := range mid {
			if z == from || z == to {
				continue
			}
			r2s, r2 := mid.best(z)
			if r2 == r1 {
				continue
			}
			for _, y := range near(z) {
				r3s, r3 := back.best(y)
				if r3 == r2 && y == z {
					continue
				}
				opts = append(opts, TransferOption{
					Transfers: []Transfer{{x, x}, {z, y}}, RouteIDs: []string{r1, r2, r3},
					Seconds: r1s + transferWaitS + r2s + TransferWalkS(ix.areas[z], ix.areas[y]) + transferWaitS + r3s,
				})
			}
		}
	}
	return pickOptions(opts, limit)
}

// pickOptions keeps the quickest option per combination of lines.
func pickOptions(opts []TransferOption, limit int) []TransferOption {
	slices.SortFunc(opts, func(p, q TransferOption) int {
		return cmp.Or(cmp.Compare(len(p.Transfers), len(q.Transfers)), cmp.Compare(p.Seconds, q.Seconds),
			strings.Compare(optionKey(p), optionKey(q)), strings.Compare(transfersKey(p), transfersKey(q)))
	})
	seen := map[string]bool{}
	var out []TransferOption
	for _, o := range opts {
		k := optionKey(o)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, o)
		if len(out) == limit {
			break
		}
	}
	return out
}

func optionKey(o TransferOption) string { return strings.Join(o.RouteIDs, ">") }

func transfersKey(o TransferOption) string {
	var b strings.Builder
	for _, t := range o.Transfers {
		b.WriteString(t.Alight + "~" + t.Board + ",")
	}
	return b.String()
}
