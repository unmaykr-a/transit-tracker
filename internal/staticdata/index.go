package staticdata

import (
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
)

// LineRef is a route serving a stop, with the destinations it goes to from there.
type LineRef struct {
	RouteID   string     `json:"route_id"`
	Mode      feeds.Mode `json:"mode"`
	ShortName string     `json:"short_name"`
	Headsigns []string   `json:"headsigns"`
}

// Index is an immutable, query-ready view of a Dataset. Swap it atomically on import.
type Index struct {
	ImportedAt time.Time
	stops      map[string]*Stop
	byCode     map[string]*Stop
	areas      map[string]*Area
	routes     map[string]*Route
	routeList  []*Route
	patterns   map[string][]*Pattern // by route id, main pattern per direction first
	stopLines  map[string][]LineRef  // by stop id
	areaSearch []areaKey
	// conns memoises Connections per "from|to": the index never changes, and
	// every dashboard and Home Assistant poll asks again for each saved trip.
	conns sync.Map
}

type areaKey struct {
	area    *Area
	names   []string // folded platform names
	aliases []string // folded GTFS aliases, ranked below names
	lines   int
}

// NewIndex builds the lookup structures.
func NewIndex(d Dataset) *Index {
	ix := &Index{
		ImportedAt: d.ImportedAt,
		stops:      make(map[string]*Stop, len(d.Stops)),
		byCode:     make(map[string]*Stop, len(d.Stops)),
		areas:      make(map[string]*Area, len(d.Areas)),
		routes:     make(map[string]*Route, len(d.Routes)),
		patterns:   map[string][]*Pattern{},
		stopLines:  map[string][]LineRef{},
	}
	for i := range d.Stops {
		s := &d.Stops[i]
		ix.stops[s.ID] = s
		if s.Code != "" {
			ix.byCode[s.Code] = s
		}
	}
	for i := range d.Areas {
		a := &d.Areas[i]
		// Platforms in letter order (A, B, … Z, 27, …) regardless of how they were loaded.
		sort.SliceStable(a.StopIDs, func(x, y int) bool {
			px, py := ix.stops[a.StopIDs[x]].Platform, ix.stops[a.StopIDs[y]].Platform
			if len(px) != len(py) {
				return len(px) < len(py)
			}
			return px < py
		})
		ix.areas[a.ID] = a
	}
	for i := range d.Routes {
		r := &d.Routes[i]
		ix.routes[r.ID] = r
		ix.routeList = append(ix.routeList, r)
	}
	sort.SliceStable(ix.routeList, func(a, b int) bool { return ix.routeList[a].Sort < ix.routeList[b].Sort })

	type lineKey struct{ stop, route string }
	heads := map[lineKey]map[string]bool{}
	for i := range d.Patterns {
		p := &d.Patterns[i]
		ix.patterns[p.RouteID] = append(ix.patterns[p.RouteID], p)
		for n, sid := range p.StopIDs {
			if n == len(p.StopIDs)-1 {
				continue // nothing departs towards anywhere from the last stop
			}
			k := lineKey{sid, p.RouteID}
			if heads[k] == nil {
				heads[k] = map[string]bool{}
			}
			if p.Headsign != "" {
				heads[k][p.Headsign] = true
			}
		}
	}
	for _, ps := range ix.patterns {
		sortPatterns(ps)
	}
	for k, hs := range heads {
		r := ix.routes[k.route]
		if r == nil {
			continue
		}
		list := make([]string, 0, len(hs))
		for h := range hs {
			list = append(list, h)
		}
		sort.Strings(list)
		ix.stopLines[k.stop] = append(ix.stopLines[k.stop], LineRef{RouteID: r.ID, Mode: r.Mode, ShortName: r.ShortName, Headsigns: list})
	}
	for id, refs := range ix.stopLines {
		sort.Slice(refs, func(a, b int) bool { return ix.routes[refs[a].RouteID].Sort < ix.routes[refs[b].RouteID].Sort })
		ix.stopLines[id] = refs
	}
	for _, a := range ix.areas {
		k := areaKey{area: a}
		seen := map[string]bool{}
		add := func(list *[]string, n string) {
			if f := Fold(n); f != "" && !seen[f] {
				seen[f] = true
				*list = append(*list, f)
			}
		}
		add(&k.names, a.Name)
		lines := map[string]bool{}
		for _, sid := range a.StopIDs {
			s := ix.stops[sid]
			add(&k.names, cleanName(s.Name))
		}
		for _, sid := range a.StopIDs {
			s := ix.stops[sid]
			for _, al := range s.Aliases {
				add(&k.aliases, al)
			}
			for _, l := range ix.stopLines[sid] {
				lines[l.RouteID] = true
			}
		}
		k.lines = len(lines)
		ix.areaSearch = append(ix.areaSearch, k)
	}
	sort.Slice(ix.areaSearch, func(i, j int) bool { return ix.areaSearch[i].area.ID < ix.areaSearch[j].area.ID })
	return ix
}

// sortPatterns puts the busiest pattern of each direction first, direction 0 before 1.
func sortPatterns(ps []*Pattern) {
	sort.SliceStable(ps, func(a, b int) bool {
		if ps[a].Direction != ps[b].Direction {
			return ps[a].Direction < ps[b].Direction
		}
		if ps[a].Trips != ps[b].Trips {
			return ps[a].Trips > ps[b].Trips
		}
		return ps[a].Sort < ps[b].Sort
	})
}

func (ix *Index) Empty() bool { return ix == nil || len(ix.stops) == 0 }

func (ix *Index) Stop(id string) (*Stop, bool) { s, ok := ix.stops[id]; return s, ok }

func (ix *Index) StopByCode(code string) (*Stop, bool) { s, ok := ix.byCode[code]; return s, ok }

func (ix *Index) Area(id string) (*Area, bool) { a, ok := ix.areas[id]; return a, ok }

func (ix *Index) Route(id string) (*Route, bool) { r, ok := ix.routes[id]; return r, ok }

// Patterns returns a route's patterns, main pattern of each direction first.
func (ix *Index) Patterns(routeID string) []*Pattern { return ix.patterns[routeID] }

// Lines returns the routes calling at a stop (not counting where they terminate).
func (ix *Index) Lines(stopID string) []LineRef { return ix.stopLines[stopID] }

// Stops returns every stop, sorted by id.
func (ix *Index) Stops() []*Stop {
	out := make([]*Stop, 0, len(ix.stops))
	for _, s := range ix.stops {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Routes returns every route in display order.
func (ix *Index) Routes() []*Route { return ix.routeList }

// AreaResult is a stop search hit.
type AreaResult struct {
	Area  *Area
	Score int
}

// SearchAreas finds stop areas by name, ignoring case and diacritics.
// Exact name beats prefix beats word prefix beats substring; ties go to busier stops.
func (ix *Index) SearchAreas(q string, limit int) []AreaResult {
	fq := Fold(q)
	if fq == "" {
		return nil
	}
	var out []AreaResult
	for _, k := range ix.areaSearch {
		score := 0
		for _, n := range k.names {
			switch {
			case n == fq:
				score = max(score, 400)
			case strings.HasPrefix(n, fq):
				score = max(score, 300)
			case strings.Contains(" "+n, " "+fq):
				score = max(score, 200)
			case strings.Contains(n, fq):
				score = max(score, 100)
			}
		}
		for _, n := range k.aliases {
			if strings.HasPrefix(n, fq) || strings.Contains(" "+n, " "+fq) {
				score = max(score, 150)
			}
		}
		if score == 0 {
			continue
		}
		out = append(out, AreaResult{Area: k.area, Score: score + min(k.lines, 50)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Area.Name < out[j].Area.Name
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// SearchRoutes finds lines by number or name. "4" matches tram T4, bus 4 and
// trolleybus 4; "17" also matches 17A by prefix.
func (ix *Index) SearchRoutes(q string, limit int) []*Route {
	fq := Fold(q)
	if fq == "" {
		return nil
	}
	type hit struct {
		r     *Route
		score int
	}
	var hits []hit
	for _, r := range ix.routeList {
		short := strings.ToLower(r.ShortName)
		long := Fold(r.LongName)
		score := 0
		switch {
		case short == fq || (r.Mode == feeds.ModeTram && short == "t"+fq):
			score = 400
		case strings.HasPrefix(short, fq) || (r.Mode == feeds.ModeTram && strings.HasPrefix(short, "t"+fq)):
			score = 300
		case strings.Contains(" "+long, " "+fq):
			score = 200
		case len(fq) >= 3 && strings.Contains(long, fq):
			score = 100
		}
		if score > 0 {
			hits = append(hits, hit{r, score})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	out := make([]*Route, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.r)
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// AreaModes is the set of modes at an area, in display order.
func (ix *Index) AreaModes(a *Area) []feeds.Mode { return slices.Clone(a.Modes) }
