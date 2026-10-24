package staticdata

import (
	"crypto/sha1"
	"encoding/hex"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/geo"
)

// Sources are the raw inputs of one import.
type Sources struct {
	CityRoutes   []feeds.OTPRoute    // Tallinn city routes with patterns (peatus.ee OTP)
	CountyRoutes []feeds.OTPRoute    // Harju county bus routes
	TartuRoutes  []feeds.OTPRoute    // Tartu city bus routes
	Regions      []RegionRoutes      // more counties and long-distance lines (see Regions)
	TallinnStops []feeds.TallinnStop // names and SIRI ids (transport.tallinn.ee)
	Elron        *feeds.GTFS         // rail routes, trips, shapes (Elron GTFS)
}

// AreaRadiusM is how far apart platforms with the same name may be and still form one stop area.
const AreaRadiusM = 400

var modeRank = map[feeds.Mode]int{feeds.ModeTram: 0, feeds.ModeTrolley: 1, feeds.ModeBus: 2, feeds.ModeTrain: 3}

// Build turns raw sources into a normalised dataset.
func Build(src Sources, now time.Time) Dataset {
	b := &builder{stops: map[string]*Stop{}}
	byCode := make(map[string]feeds.TallinnStop, len(src.TallinnStops))
	for _, s := range src.TallinnStops {
		byCode[s.Code] = s
	}
	aliases := map[string][]string{} // national stop code → GTFS aliases
	if src.Elron != nil {
		for _, gs := range src.Elron.Stops {
			if gs.Code != "" && gs.Alias != "" {
				aliases[gs.Code] = splitAliases(gs.Alias, gs.Name)
			}
		}
	}
	b.city(src.CityRoutes, byCode, aliases)
	b.otherBuses(src.CountyRoutes, byCode, aliases, "county", "")
	b.otherBuses(src.TartuRoutes, byCode, aliases, "tartu", TartuPrefix)
	for _, r := range src.Regions {
		b.otherBuses(r.Routes, byCode, aliases, r.Source, r.Prefix)
	}
	if src.Elron != nil {
		b.rail(src.Elron)
	}
	d := Dataset{Routes: b.routes, Patterns: b.patterns, ImportedAt: now}
	for _, s := range b.stops {
		d.Stops = append(d.Stops, *s)
	}
	sort.Slice(d.Stops, func(i, j int) bool { return d.Stops[i].ID < d.Stops[j].ID })
	d.Areas = groupAreas(d.Stops)
	sortRoutes(d.Routes)
	return d
}

type builder struct {
	stops    map[string]*Stop
	routes   []Route
	patterns []Pattern
}

func (b *builder) addStop(s Stop, mode feeds.Mode) {
	if cur, ok := b.stops[s.ID]; ok {
		if !slices.Contains(cur.Modes, mode) {
			cur.Modes = append(cur.Modes, mode)
		}
		return
	}
	s.Modes = []feeds.Mode{mode}
	b.stops[s.ID] = &s
}

func otpStopID(gtfsID string) string {
	if i := strings.LastIndexByte(gtfsID, ':'); i >= 0 {
		return gtfsID[i+1:]
	}
	return gtfsID
}

func color(c string) string {
	c = strings.TrimPrefix(strings.TrimSpace(c), "#")
	if len(c) != 6 {
		return ""
	}
	return "#" + strings.ToLower(c)
}

func splitAliases(alias, name string) []string {
	var out []string
	for _, al := range strings.Split(alias, ";") {
		if al = strings.TrimSpace(al); al != "" && Fold(al) != Fold(cleanName(name)) {
			out = append(out, al)
		}
	}
	return out
}

func (b *builder) city(routes []feeds.OTPRoute, byCode map[string]feeds.TallinnStop, aliases map[string][]string) {
	seen := map[string]bool{}
	for _, r := range routes {
		mode := r.CityMode()
		route := Route{ID: RouteID(mode, r.ShortName), Mode: mode, ShortName: r.ShortName, LongName: r.LongName,
			Color: color(r.Color), TextColor: color(r.TextColor), Source: "tallinn"}
		// Two OTP routes with one short name (a seasonal variant) become one route
		// with both sets of patterns: a duplicate id would fail every import.
		if !seen[route.ID] {
			seen[route.ID] = true
			b.routes = append(b.routes, route)
		}
		for i, p := range r.Patterns {
			ids := make([]string, 0, len(p.Stops))
			for _, os := range p.Stops {
				s := Stop{ID: otpStopID(os.GtfsID), Code: os.Code, Name: cleanName(os.Name), Lat: os.Lat, Lon: os.Lon,
					Aliases: aliases[os.Code], Source: "tallinn"}
				if ts, ok := byCode[os.Code]; ok {
					// The operator's own file has the canonical Estonian names and the SIRI id.
					s.Name, s.SiriID, s.Street = cleanName(ts.Name), ts.SiriID, ts.Street
				}
				if s.SiriID == "" {
					s.SiriID = s.ID
				}
				b.addStop(s, mode)
				ids = append(ids, s.ID)
			}
			var shape [][2]float64
			if p.Geometry != nil {
				shape, _ = geo.DecodePolyline(p.Geometry.Points)
			}
			b.patterns = append(b.patterns, Pattern{
				ID: p.Code, RouteID: route.ID, Direction: p.DirectionID, Headsign: p.Headsign,
				StopIDs: ids, Shape: orStraight(shape, ids, b.stops), Trips: len(ids), Sort: i,
			})
		}
	}
}

// TartuPrefix keys Tartu's lines apart from Tallinn's: Tartu's bus 2 is
// "tartu-2" (route bus:tartu-2, /lines/bus/tartu-2); the web shows it as "2".
const TartuPrefix = "tartu-"

// Region is a set of national timetable agencies imported like Harju county:
// another county (source "county", live from the same feed as Harju) or the
// long-distance lines (source "intercity", timetable only). Their short names
// are keyed with Prefix because numbers repeat across regions (Rapla's bus 5,
// Tallinn's bus 5); the web shows them without it.
type Region struct {
	Agency string // agency id prefix in the national timetable
	Source string
	Prefix string
	// RT is the region's name in the county GTFS-RT feed ("" = no live buses).
	RT string
}

// Regions are imported when county buses (Rapla, Lääne-Viru, Läänemaa) or
// long-distance lines are on. Harju county and Tartu have their own fields.
var Regions = []Region{
	{Agency: "estonia:raplamaa_", Source: "county", Prefix: "raplamaa-", RT: "raplamaa"},
	{Agency: "estonia:laane_virumaa_", Source: "county", Prefix: "laane-virumaa-", RT: "laane_viru"},
	{Agency: "estonia:laanemaa_", Source: "county", Prefix: "laanemaa-", RT: "laanemaa"},
	{Agency: "estonia:kaugliinid", Source: "intercity", Prefix: "intercity-"},
}

// RegionRoutes are one Region's routes from an import.
type RegionRoutes struct {
	Region
	Routes []feeds.OTPRoute
}

// LinePrefixes are the prefixes of lines keyed apart from Tallinn's (Tartu and Regions).
func LinePrefixes() []string {
	out := []string{TartuPrefix}
	for _, r := range Regions {
		out = append(out, r.Prefix)
	}
	return out
}

// otherBuses adds bus routes from another source: Harju county (source
// "county"; short names never clash with city routes today, and a clash is
// skipped rather than let a county route take over a city line's id) or Tartu
// city (source "tartu", short names keyed with TartuPrefix). Stops only they
// serve get no SIRI id: the Tallinn feed has no rows for them.
func (b *builder) otherBuses(routes []feeds.OTPRoute, byCode map[string]feeds.TallinnStop, aliases map[string][]string, source, prefix string) {
	taken := map[string]bool{}
	for _, r := range b.routes {
		taken[r.ID] = true
	}
	for _, r := range routes {
		short := prefix + r.ShortName
		id := RouteID(feeds.ModeBus, short)
		if r.ShortName == "" {
			continue
		}
		if taken[id] {
			// The same line from another agency (a shared or seasonal route): one route, both timetable ids.
			for i := range b.routes {
				if b.routes[i].ID == id && b.routes[i].Source == source && r.GtfsID != "" {
					b.routes[i].OTPIDs = append(b.routes[i].OTPIDs, r.GtfsID)
				}
			}
			continue
		}
		taken[id] = true
		route := Route{ID: id, Mode: feeds.ModeBus, ShortName: short, LongName: r.LongName,
			Color: color(r.Color), TextColor: color(r.TextColor), Source: source}
		if r.GtfsID != "" {
			route.OTPIDs = []string{r.GtfsID}
		}
		b.routes = append(b.routes, route)
		for i, p := range r.Patterns {
			ids := make([]string, 0, len(p.Stops))
			for _, os := range p.Stops {
				s := Stop{ID: otpStopID(os.GtfsID), Code: os.Code, Name: cleanName(os.Name), Lat: os.Lat, Lon: os.Lon,
					Aliases: aliases[os.Code], Source: source}
				if ts, ok := byCode[os.Code]; ok {
					s.Name, s.Street = cleanName(ts.Name), ts.Street
				}
				b.addStop(s, feeds.ModeBus)
				ids = append(ids, s.ID)
			}
			var shape [][2]float64
			if p.Geometry != nil {
				shape, _ = geo.DecodePolyline(p.Geometry.Points)
			}
			b.patterns = append(b.patterns, Pattern{
				ID: p.Code, RouteID: route.ID, Direction: p.DirectionID, Headsign: p.Headsign,
				StopIDs: ids, Shape: orStraight(shape, ids, b.stops), Trips: len(ids), Sort: i,
			})
		}
	}
}

func (b *builder) rail(g *feeds.GTFS) {
	type key struct {
		short string
		dir   int
		seq   string
	}
	type agg struct {
		trips     int
		headsigns map[string]int
		shapeID   string
		stops     []string
	}
	routesByID := map[string]feeds.GTFSRoute{}
	seen := map[string]bool{}
	for _, r := range g.Routes {
		if r.Type != 2 && r.Type != 100 && r.Type != 109 { // GTFS rail types
			continue
		}
		routesByID[r.ID] = r
		id := RouteID(feeds.ModeTrain, r.ShortName)
		if seen[id] {
			continue // several GTFS routes share a public short name (seasonal variants)
		}
		seen[id] = true
		b.routes = append(b.routes, Route{ID: id, Mode: feeds.ModeTrain, ShortName: r.ShortName, LongName: r.LongName,
			Color: color(r.Color), TextColor: color(r.TextColor), Source: "elron"})
	}
	pats := map[key]*agg{}
	for _, t := range g.Trips {
		r, ok := routesByID[t.RouteID]
		seq := g.StopTimes[t.ID]
		if !ok || len(seq) < 2 {
			continue
		}
		k := key{r.ShortName, t.DirectionID, strings.Join(seq, ",")}
		a := pats[k]
		if a == nil {
			a = &agg{headsigns: map[string]int{}, shapeID: t.ShapeID, stops: seq}
			pats[k] = a
		}
		a.trips++
		if t.Headsign != "" {
			a.headsigns[t.Headsign]++
		}
	}
	keys := make([]key, 0, len(pats))
	for k := range pats {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].short != keys[j].short {
			return keys[i].short < keys[j].short
		}
		if keys[i].dir != keys[j].dir {
			return keys[i].dir < keys[j].dir
		}
		return pats[keys[i]].trips > pats[keys[j]].trips
	})
	for i, k := range keys {
		a := pats[k]
		for _, sid := range a.stops {
			gs, ok := g.Stops[sid]
			if !ok {
				continue
			}
			b.addStop(Stop{ID: gs.ID, Code: gs.Code, Name: gs.Name, Lat: gs.Lat, Lon: gs.Lon,
				Aliases: splitAliases(gs.Alias, gs.Name), Source: "elron"}, feeds.ModeTrain)
		}
		headsign := topKey(a.headsigns)
		if headsign == "" {
			if last, ok := g.Stops[a.stops[len(a.stops)-1]]; ok {
				headsign = last.Name
			}
		}
		sum := sha1.Sum([]byte(k.seq))
		b.patterns = append(b.patterns, Pattern{
			ID: "elron:" + k.short + ":" + strconv.Itoa(k.dir) + ":" + hex.EncodeToString(sum[:4]), RouteID: RouteID(feeds.ModeTrain, k.short),
			Direction: k.dir, Headsign: headsign, StopIDs: a.stops, Shape: orStraight(g.Shapes[a.shapeID], a.stops, b.stops),
			Trips: a.trips, Sort: i,
		})
	}
}

func topKey(m map[string]int) string {
	best, n := "", 0
	for k, v := range m {
		if v > n || (v == n && k < best) {
			best, n = k, v
		}
	}
	return best
}

// orStraight falls back to straight segments between stops when no shape exists.
func orStraight(shape [][2]float64, ids []string, stops map[string]*Stop) [][2]float64 {
	if len(shape) >= 2 {
		return shape
	}
	out := make([][2]float64, 0, len(ids))
	for _, id := range ids {
		if s, ok := stops[id]; ok {
			out = append(out, [2]float64{s.Lon, s.Lat})
		}
	}
	return out
}

// groupAreas clusters platforms into stop areas: same folded name within
// AreaRadiusM. City stops are grouped first so their names win. A rail station
// may also join a nearby city area through a GTFS alias, in either direction
// ("Tallinn" has alias "Balti Jaam"; city "Ülemiste jaam" has alias "Ülemiste").
// Aliases never merge city stops with each other: the column is loose (it often
// holds a district name). Platform letters are assigned per area.
func groupAreas(stops []Stop) []Area {
	order := make([]int, len(stops))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		sa, sb := stops[order[a]], stops[order[b]]
		if (sa.Source == "elron") != (sb.Source == "elron") {
			return sb.Source == "elron"
		}
		return sa.ID < sb.ID
	})
	type cluster struct {
		area      Area
		key       string
		aliasKeys map[string]bool // aliases of city members
		lat, lon  float64         // first member, the anchor for the radius test
		members   []int
	}
	var clusters []*cluster
	near := func(c *cluster, s Stop) bool { return geo.DistanceM(c.lat, c.lon, s.Lat, s.Lon) <= AreaRadiusM }
	find := func(s Stop) *cluster {
		key := Fold(cleanName(s.Name))
		for _, c := range clusters {
			if c.key == key && near(c, s) {
				return c
			}
		}
		if s.Source != "elron" {
			return nil
		}
		for _, c := range clusters {
			if !near(c, s) {
				continue
			}
			if c.aliasKeys[key] {
				return c
			}
			for _, al := range s.Aliases {
				if f := Fold(al); f == c.key || c.aliasKeys[f] {
					return c
				}
			}
		}
		return nil
	}
	for _, i := range order {
		s := stops[i]
		c := find(s)
		if c == nil {
			c = &cluster{key: Fold(cleanName(s.Name)), aliasKeys: map[string]bool{}, lat: s.Lat, lon: s.Lon, area: Area{ID: "a" + s.ID, Name: cleanName(s.Name)}}
			clusters = append(clusters, c)
		}
		if s.Source != "elron" {
			for _, al := range s.Aliases {
				c.aliasKeys[Fold(al)] = true
			}
		}
		c.members = append(c.members, i)
	}
	areas := make([]Area, 0, len(clusters))
	for _, c := range clusters {
		sort.Slice(c.members, func(a, b int) bool {
			sa, sb := stops[c.members[a]], stops[c.members[b]]
			ra, rb := modeRank[firstMode(sa.Modes)], modeRank[firstMode(sb.Modes)]
			if ra != rb {
				return ra < rb
			}
			return sa.Code < sb.Code
		})
		var lat, lon float64
		modes := map[feeds.Mode]bool{}
		for n, i := range c.members {
			s := &stops[i]
			s.AreaID = c.area.ID
			if len(c.members) > 1 {
				s.Platform = platformLetter(n)
			}
			lat += s.Lat
			lon += s.Lon
			for _, m := range s.Modes {
				modes[m] = true
			}
			c.area.StopIDs = append(c.area.StopIDs, s.ID)
		}
		c.area.Lat, c.area.Lon = lat/float64(len(c.members)), lon/float64(len(c.members))
		c.area.Modes = sortedModes(modes)
		areas = append(areas, c.area)
	}
	sort.Slice(areas, func(i, j int) bool { return areas[i].ID < areas[j].ID })
	return areas
}

func firstMode(ms []feeds.Mode) feeds.Mode {
	best := feeds.ModeTrain
	for _, m := range ms {
		if modeRank[m] < modeRank[best] {
			best = m
		}
	}
	return best
}

func platformLetter(n int) string {
	if n < 26 {
		return string(rune('A' + n))
	}
	return strconv.Itoa(n + 1)
}

func sortedModes(set map[feeds.Mode]bool) []feeds.Mode {
	out := make([]feeds.Mode, 0, len(set))
	for m := range set {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return modeRank[out[i]] < modeRank[out[j]] })
	return out
}

// lineSortKey orders line numbers naturally: 2 < 10 < 17 < 17A, trams T1 < T2.
func lineSortKey(short string) (prefix string, num int, suffix string) {
	i := 0
	for i < len(short) && (short[i] < '0' || short[i] > '9') {
		i++
	}
	j := i
	for j < len(short) && short[j] >= '0' && short[j] <= '9' {
		j++
	}
	num, _ = strconv.Atoi(short[i:j])
	return short[:i], num, short[j:]
}

func sortRoutes(rs []Route) {
	sort.SliceStable(rs, func(a, b int) bool {
		ra, rb := rs[a], rs[b]
		if ra.Mode != rb.Mode {
			return modeRank[ra.Mode] < modeRank[rb.Mode]
		}
		pa, na, sa := lineSortKey(ra.ShortName)
		pb, nb, sb := lineSortKey(rb.ShortName)
		if pa != pb {
			return pa < pb
		}
		if na != nb {
			return na < nb
		}
		return sa < sb
	})
	for i := range rs {
		rs[i].Sort = i
	}
}
