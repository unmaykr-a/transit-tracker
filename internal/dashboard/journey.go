package dashboard

import (
	"context"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/departures"
	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/geo"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
)

// ParseTransfers reads a trip's stored changes: comma-separated stop areas,
// "alight" or "alight>board".
func ParseTransfers(s string) []staticdata.Transfer {
	if s == "" {
		return nil
	}
	var out []staticdata.Transfer
	for _, part := range strings.Split(s, ",") {
		alight, board, ok := strings.Cut(part, ">")
		if !ok {
			board = alight
		}
		out = append(out, staticdata.Transfer{Alight: alight, Board: board})
	}
	return out
}

// FormatTransfers is the inverse of ParseTransfers.
func FormatTransfers(ts []staticdata.Transfer) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = t.Alight
		if t.Board != "" && t.Board != t.Alight {
			parts[i] += ">" + t.Board
		}
	}
	return strings.Join(parts, ",")
}

// TransferRef is a change as clients show it.
type TransferRef struct {
	Alight *AreaRef `json:"alight"`
	Board  *AreaRef `json:"board"`
	// WalkM is the straight distance between the two areas (0 for a change in one area).
	WalkM float64 `json:"walk_m"`
}

// TripLeg is one ride of a trip with changes, and the lines that make it.
type TripLeg struct {
	From  *AreaRef   `json:"from"`
	To    *AreaRef   `json:"to"`
	Lines []TripLine `json:"lines"`
}

// LegAreas lists each leg's start and end area for a trip.
func LegAreas(from, to string, ts []staticdata.Transfer) [][2]string {
	legs := make([][2]string, 0, len(ts)+1)
	cur := from
	for _, t := range ts {
		legs = append(legs, [2]string{cur, t.Alight})
		cur = t.Board
	}
	return append(legs, [2]string{cur, to})
}

func tripLines(ix *staticdata.Index, from, to string) []TripLine {
	out := []TripLine{}
	for _, c := range ix.Connections(from, to) {
		r, ok := ix.Route(c.RouteID)
		if !ok {
			continue
		}
		board, _ := ix.Stop(c.BoardStopID)
		alight, _ := ix.Stop(c.AlightStopID)
		out = append(out, TripLine{RouteID: r.ID, Mode: r.Mode, ShortName: r.ShortName, Board: stopRef(board), Alight: stopRef(alight), Stops: c.Stops, Headsigns: c.Headsigns})
	}
	return out
}

// JourneyLeg is one ride of a journey: when it leaves (from departures) and
// when it gets there (estimated from the ride-time model).
type JourneyLeg struct {
	RouteID  string     `json:"route_id"`
	Mode     feeds.Mode `json:"mode"`
	Line     string     `json:"line"`
	Headsign string     `json:"headsign"`
	Board    *StopRef   `json:"board_stop"`
	DepartAt time.Time  `json:"depart_at"`
	Realtime bool       `json:"realtime"`
	Alight   *StopRef   `json:"alight_stop"`
	ArriveAt time.Time  `json:"arrive_at"`
	// WalkMin and WaitMin are the change before this leg: walking from the
	// previous leg's stop, then waiting at this one (0 on the first leg).
	WalkMin int `json:"walk_min"`
	WaitMin int `json:"wait_min"`
}

// Journey follows one departure to the destination: for a trip with changes,
// the departure that can be caught at each change.
type Journey struct {
	Legs []JourneyLeg `json:"legs"`
	// ArriveAt estimates the arrival; nil when a later leg has no departure
	// soon enough to be known.
	ArriveAt *time.Time `json:"arrive_at,omitempty"`
}

// transferMargin is added to every change on top of the walk between platforms.
const transferMargin = time.Minute

// rideTime estimates riding route from stop board to stop alight.
func (b *Builder) rideTime(ix *staticdata.Index, routeID, board, alight string) (time.Duration, bool) {
	for _, p := range ix.Patterns(routeID) {
		i := slices.Index(p.StopIDs, board)
		if i < 0 {
			continue
		}
		j := slices.Index(p.StopIDs[i+1:], alight)
		if j < 0 {
			continue
		}
		j += i + 1
		var times []float64
		if b.Times != nil {
			times = b.Times(p)
		}
		secs := float64(j-i) * 90
		if len(times) == len(p.StopIDs) {
			secs = times[j] - times[i]
		}
		return time.Duration(math.Round(secs)) * time.Second, true
	}
	return 0, false
}

func connectionFor(conns []staticdata.Connection, d departures.Departure) *staticdata.Connection {
	for i, c := range conns {
		if c.RouteID == d.RouteID && c.BoardStopID == d.StopID {
			return &conns[i]
		}
	}
	return nil
}

// walkBetween is the walk between two platforms at a change, at the user's speed.
func walkBetween(a, c *staticdata.Stop, speedKmh float64) time.Duration {
	if a == nil || c == nil || a.ID == c.ID {
		return 0
	}
	m := geo.DistanceM(a.Lat, a.Lon, c.Lat, c.Lon) * 1.3
	if speedKmh <= 0 {
		speedKmh = DefaultPrefs.WalkSpeedKmh
	}
	return time.Duration(m / (speedKmh / 3.6) * float64(time.Second))
}

// laterLeg is a leg after a change: its connections and the departures from their platforms.
type laterLeg struct {
	conns []staticdata.Connection
	deps  []departures.Departure // picked to the leg's connections, soonest first
}

// journey follows departure d (on leg one, made by conn) through the later legs.
func (b *Builder) journey(ix *staticdata.Index, d departures.Departure, conn *staticdata.Connection, later []laterLeg, prefs Prefs) *Journey {
	leg, ok := b.leg(ix, d, conn)
	if !ok {
		return nil
	}
	j := &Journey{Legs: []JourneyLeg{leg}}
	for _, l := range later {
		prev := j.Legs[len(j.Legs)-1]
		from, _ := ix.Stop(prev.Alight.ID)
		var next *JourneyLeg
		for _, nd := range l.deps {
			to, _ := ix.Stop(nd.StopID)
			walk := walkBetween(from, to, prefs.WalkSpeedKmh)
			if nd.ExpectedAt.Before(prev.ArriveAt.Add(walk + transferMargin)) {
				continue
			}
			c := connectionFor(l.conns, nd)
			nl, ok := b.leg(ix, nd, c)
			if !ok {
				continue
			}
			nl.WalkMin = int(math.Ceil(walk.Minutes()))
			nl.WaitMin = max(0, int(nd.ExpectedAt.Sub(prev.ArriveAt.Add(walk)).Minutes()))
			next = &nl
			break
		}
		if next == nil {
			return j // the rest is beyond the departures we know
		}
		j.Legs = append(j.Legs, *next)
	}
	at := j.Legs[len(j.Legs)-1].ArriveAt
	j.ArriveAt = &at
	return j
}

func (b *Builder) leg(ix *staticdata.Index, d departures.Departure, conn *staticdata.Connection) (JourneyLeg, bool) {
	if conn == nil {
		return JourneyLeg{}, false
	}
	ride, ok := b.rideTime(ix, d.RouteID, conn.BoardStopID, conn.AlightStopID)
	if !ok {
		return JourneyLeg{}, false
	}
	board, _ := ix.Stop(conn.BoardStopID)
	alight, _ := ix.Stop(conn.AlightStopID)
	return JourneyLeg{
		RouteID: d.RouteID, Mode: d.Mode, Line: d.Line, Headsign: d.Headsign,
		Board: stopRef(board), DepartAt: d.ExpectedAt, Realtime: d.Realtime,
		Alight: stopRef(alight), ArriveAt: d.ExpectedAt.Add(ride),
	}, true
}

// laterLegs fetches the departures of every leg after the first.
func (b *Builder) laterLegs(ctx context.Context, ix *staticdata.Index, legs [][2]string) ([]laterLeg, error) {
	out := make([]laterLeg, 0, max(0, len(legs)-1))
	for _, l := range legs[1:] {
		conns := ix.Connections(l[0], l[1])
		var boards []*staticdata.Stop
		for _, c := range conns {
			if st, ok := ix.Stop(c.BoardStopID); ok && !slices.Contains(boards, st) {
				boards = append(boards, st)
			}
		}
		deps, err := b.Departures.ForStops(ctx, boards, 0, 3*time.Hour)
		if err != nil {
			return nil, err
		}
		out = append(out, laterLeg{conns: conns, deps: pickTrip(deps, conns)})
	}
	return out, nil
}
