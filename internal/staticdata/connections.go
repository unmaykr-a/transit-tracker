package staticdata

import (
	"slices"
	"strings"
)

// Connection is one way to ride from one stop area to another without
// changing: a route, the platform to board it at, and where to get off.
type Connection struct {
	RouteID      string `json:"route_id"`
	BoardStopID  string `json:"board_stop_id"`
	AlightStopID string `json:"alight_stop_id"`
	Stops        int    `json:"stops"` // stops travelled on the shortest pattern
	// Headsigns of the route's patterns that make the trip from this platform
	// (with their last stop's name, which departure boards sometimes show
	// instead). AnyHeadsign is set when every pattern of the route calling at
	// the platform makes the trip, so departures need no direction check.
	Headsigns   []string `json:"headsigns"`
	AnyHeadsign bool     `json:"-"`
}

// Connections lists the direct rides from area from to area to, one per route
// and boarding platform, fewest stops first. Patterns that call at the origin
// more than once (loops) board at their first call before the destination.
func (ix *Index) Connections(from, to string) []Connection {
	a, okA := ix.areas[from]
	b, okB := ix.areas[to]
	if !okA || !okB || from == to {
		return nil
	}
	type key struct{ route, board string }
	byKey := map[key]*Connection{}
	var order []key
	for routeID, pats := range ix.patterns {
		for _, p := range pats {
			board, alight, stops := ix.ride(p, a.StopIDs, b.StopIDs)
			if board == "" {
				continue
			}
			k := key{routeID, board}
			c := byKey[k]
			if c == nil {
				c = &Connection{RouteID: routeID, BoardStopID: board, AlightStopID: alight, Stops: stops}
				byKey[k] = c
				order = append(order, k)
			} else if stops < c.Stops {
				c.AlightStopID, c.Stops = alight, stops
			}
			for _, h := range []string{p.Headsign, ix.stopName(p.StopIDs[len(p.StopIDs)-1])} {
				if h != "" && !slices.Contains(c.Headsigns, h) {
					c.Headsigns = append(c.Headsigns, h)
				}
			}
		}
	}
	out := make([]Connection, 0, len(order))
	for _, k := range order {
		c := byKey[k]
		c.AnyHeadsign = true
		for _, p := range ix.patterns[k.route] {
			i := slices.Index(p.StopIDs, k.board)
			if i < 0 || i == len(p.StopIDs)-1 {
				continue // does not depart from this platform
			}
			if board, _, _ := ix.ride(p, []string{k.board}, b.StopIDs); board == "" {
				c.AnyHeadsign = false // some trips from here go elsewhere
				break
			}
		}
		out = append(out, *c)
	}
	slices.SortFunc(out, func(x, y Connection) int {
		if x.Stops != y.Stops {
			return x.Stops - y.Stops
		}
		if d := ix.routes[x.RouteID].Sort - ix.routes[y.RouteID].Sort; d != 0 {
			return d
		}
		return strings.Compare(x.BoardStopID, y.BoardStopID)
	})
	return out
}

// ride finds where pattern p can be boarded at one of from and left at one of
// to, later on the same pattern; empty when it cannot.
func (ix *Index) ride(p *Pattern, from, to []string) (board, alight string, stops int) {
	for i, sid := range p.StopIDs[:max(0, len(p.StopIDs)-1)] {
		if !slices.Contains(from, sid) {
			continue
		}
		for j := i + 1; j < len(p.StopIDs); j++ {
			if slices.Contains(to, p.StopIDs[j]) {
				return sid, p.StopIDs[j], j - i
			}
		}
	}
	return "", "", 0
}

func (ix *Index) stopName(id string) string {
	if s, ok := ix.stops[id]; ok {
		return s.Name
	}
	return ""
}
