package dashboard

import (
	"context"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/departures"
	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/geo"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
	"github.com/unmaykr-a/transit-tracker/internal/store/db"
	"github.com/unmaykr-a/transit-tracker/internal/walk"
)

// Shown is how many departures each card shows (PLAN.md: next 3).
const Shown = 3

// StopRef is a stop as the dashboard shows it.
type StopRef struct {
	ID       string  `json:"id"`
	AreaID   string  `json:"area_id"`
	Name     string  `json:"name"`
	Platform string  `json:"platform"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
}

func stopRef(s *staticdata.Stop) *StopRef {
	if s == nil {
		return nil
	}
	return &StopRef{s.ID, s.AreaID, s.Name, s.Platform, s.Lat, s.Lon}
}

// Line is a saved line with its static context resolved.
type Line struct {
	ID          int64      `json:"id"`
	RouteID     string     `json:"route_id"`
	Mode        feeds.Mode `json:"mode"`
	ShortName   string     `json:"short_name"`
	Direction   int        `json:"direction"`
	Headsign    string     `json:"headsign"`
	Label       string     `json:"label"`
	Board       *StopRef   `json:"board_stop"`
	Alight      *StopRef   `json:"alight_stop,omitempty"`
	NotifyMin   int        `json:"notify_threshold_min"`
	Unavailable bool       `json:"unavailable,omitempty"` // route or stop missing after a static import
	// OriginPlaceID is the saved place walking times start from when the
	// request carries no location of its own.
	OriginPlaceID *int64 `json:"origin_place_id"`
	// Kind is "line" (one route from one platform) or "trip" (any line from one
	// stop area to another; From, To and Lines are set, RouteID and Board are not).
	Kind  string     `json:"kind"`
	From  *AreaRef   `json:"from,omitempty"`
	To    *AreaRef   `json:"to,omitempty"`
	Lines []TripLine `json:"lines,omitempty"` // a trip's lines (of its first leg when it has changes)
	// Transfers and Legs describe a trip with changes: where it changes, and
	// each ride with the lines that make it (Lines repeats the first leg's).
	Transfers []TransferRef `json:"transfers,omitempty"`
	Legs      []TripLeg     `json:"legs,omitempty"`
}

// AreaRef is a stop area as trips show it.
type AreaRef struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

// TripLine is one route that makes a saved trip, from one platform.
type TripLine struct {
	RouteID   string     `json:"route_id"`
	Mode      feeds.Mode `json:"mode"`
	ShortName string     `json:"short_name"`
	Board     *StopRef   `json:"board_stop"`
	Alight    *StopRef   `json:"alight_stop"`
	Stops     int        `json:"stops"`
	Headsigns []string   `json:"headsigns"`
}

// ResolveLine attaches static data to a stored saved line or trip.
func ResolveLine(ix *staticdata.Index, sl db.SavedLine) Line {
	l := resolve(ix, sl)
	if sl.OriginPlaceID.Valid {
		l.OriginPlaceID = &sl.OriginPlaceID.Int64
	}
	return l
}

func resolve(ix *staticdata.Index, sl db.SavedLine) Line {
	if sl.Kind == KindTrip {
		return resolveTrip(ix, sl)
	}
	l := Line{ID: sl.ID, Kind: KindLine, RouteID: sl.RouteID, Direction: int(sl.Direction), Headsign: sl.Headsign, Label: sl.Label, NotifyMin: int(sl.NotifyThresholdMin)}
	r, ok := ix.Route(sl.RouteID)
	if ok {
		l.Mode, l.ShortName = r.Mode, r.ShortName
	}
	board, okB := ix.Stop(sl.BoardStopID)
	l.Board = stopRef(board)
	if sl.AlightStopID != "" {
		alight, _ := ix.Stop(sl.AlightStopID)
		l.Alight = stopRef(alight)
	}
	l.Unavailable = !ok || !okB
	return l
}

// Saved line kinds.
const (
	KindLine = "line"
	KindTrip = "trip"
)

func areaRef(ix *staticdata.Index, id string) *AreaRef {
	a, ok := ix.Area(id)
	if !ok {
		return nil
	}
	return &AreaRef{a.ID, a.Name, a.Lat, a.Lon}
}

func resolveTrip(ix *staticdata.Index, sl db.SavedLine) Line {
	l := Line{ID: sl.ID, Kind: KindTrip, Label: sl.Label, NotifyMin: int(sl.NotifyThresholdMin),
		From: areaRef(ix, sl.FromAreaID), To: areaRef(ix, sl.ToAreaID)}
	transfers := ParseTransfers(sl.Transfers)
	legs := LegAreas(sl.FromAreaID, sl.ToAreaID, transfers)
	l.Lines = tripLines(ix, legs[0][0], legs[0][1])
	l.Unavailable = l.From == nil || l.To == nil || len(l.Lines) == 0
	if len(transfers) > 0 {
		for _, t := range transfers {
			tr := TransferRef{Alight: areaRef(ix, t.Alight), Board: areaRef(ix, t.Board)}
			a, okA := ix.Area(t.Alight)
			b, okB := ix.Area(t.Board)
			if okA && okB {
				tr.WalkM = math.Round(geo.DistanceM(a.Lat, a.Lon, b.Lat, b.Lon))
			}
			l.Unavailable = l.Unavailable || tr.Alight == nil || tr.Board == nil
			l.Transfers = append(l.Transfers, tr)
		}
		for _, lg := range legs {
			leg := TripLeg{From: areaRef(ix, lg[0]), To: areaRef(ix, lg[1]), Lines: tripLines(ix, lg[0], lg[1])}
			l.Unavailable = l.Unavailable || len(leg.Lines) == 0
			l.Legs = append(l.Legs, leg)
		}
	}
	if l.To != nil {
		l.Headsign = l.To.Name
	}
	return l
}

// Walk is the walking leg to the boarding stop.
type Walk struct {
	Minutes   int     `json:"minutes"`
	DistanceM float64 `json:"distance_m"`
	Provider  string  `json:"provider"`
	Approx    bool    `json:"approx"`
}

// DepartureVerdict is a departure with its verdict.
type DepartureVerdict struct {
	departures.Departure
	Verdict Verdict `json:"verdict"`
	// Journey follows the departure to the destination (trips, and lines saved
	// with a stop to get off at): changes and an estimated arrival.
	Journey *Journey `json:"journey,omitempty"`
}

// Entry is one dashboard card.
type Entry struct {
	Line       Line               `json:"line"`
	Departures []DepartureVerdict `json:"departures"`
	Walk       *Walk              `json:"walk,omitempty"`
	// NextCatchable is the index in Departures of the first departure that can
	// still be made, or -1. When the first is red, the UI highlights this one.
	NextCatchable int    `json:"next_catchable"`
	Error         string `json:"error,omitempty"`
	// Walks is the walk to each boarding platform of a trip (its departures
	// leave from several), so clients can judge every departure on its own.
	Walks map[string]*Walk `json:"walks,omitempty"`
	// WalkFrom names the line's own starting place when walks start there
	// (the request had no location); empty otherwise.
	WalkFrom string `json:"walk_from,omitempty"`
}

// Place is a saved starting point, by id, for lines that have one.
type Place struct {
	Label string
	Point walk.Point
}

// Prefs are the user's walking settings.
type Prefs struct {
	WalkSpeedKmh float64 `json:"walk_speed_kmh"`
	BufferMin    int     `json:"buffer_min"`
	// ShowRail draws railway lines on the live map (off by default).
	ShowRail bool `json:"show_rail"`
	// VehicleIcons is how the map draws vehicles: "pill" (line numbers) or "topdown" (vehicles seen from above).
	VehicleIcons string `json:"vehicle_icons"`
	// PredictPositions moves markers on along their route between updates (on by default).
	PredictPositions bool `json:"predict_positions"`
}

// DefaultPrefs match PLAN.md (4.8 km/h, 1 minute buffer).
var DefaultPrefs = Prefs{WalkSpeedKmh: 4.8, BufferMin: 1, VehicleIcons: "pill", PredictPositions: true}

// Builder assembles dashboards.
type Builder struct {
	Index      func() *staticdata.Index
	Departures *departures.Service
	Walk       *walk.Service
	// Times estimates riding times along a pattern (eta.Tracker.PatternTimes);
	// without it a ride takes 90 s per stop.
	Times staticdata.PatternTimer
	Now   func() time.Time
}

func (b *Builder) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// Build returns one entry per saved line, in order. Walks start at origin;
// without one, at the line's own place (looked up in places), else there is
// no walk and no verdict.
func (b *Builder) Build(ctx context.Context, lines []db.SavedLine, origin *walk.Point, places map[int64]Place, prefs Prefs) []Entry {
	return b.BuildN(ctx, lines, origin, places, prefs, Shown)
}

// BuildN is Build showing up to shown departures per line.
func (b *Builder) BuildN(ctx context.Context, lines []db.SavedLine, origin *walk.Point, places map[int64]Place, prefs Prefs, shown int) []Entry {
	ix := b.Index()
	out := make([]Entry, len(lines))
	var wg sync.WaitGroup
	for i, sl := range lines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o, from := origin, ""
			if o == nil && sl.OriginPlaceID.Valid {
				if p, ok := places[sl.OriginPlaceID.Int64]; ok {
					o, from = &p.Point, p.Label
				}
			}
			out[i] = b.entry(ctx, ix, sl, o, prefs, shown)
			out[i].WalkFrom = from
		}()
	}
	wg.Wait()
	return out
}

// pick keeps the line's departures from the boarding platform, preferring its
// saved headsign when the platform also serves the other direction.
func pick(all []departures.Departure, routeID, headsign string) []departures.Departure {
	var same, other []departures.Departure
	for _, d := range all {
		if d.RouteID != routeID {
			continue
		}
		if staticdata.Fold(d.Headsign) == staticdata.Fold(headsign) {
			same = append(same, d)
		} else {
			other = append(other, d)
		}
	}
	if len(same) > 0 {
		return same
	}
	return other
}

// pickTrip keeps departures of the trip's connecting routes from their
// boarding platforms, in the direction that reaches the destination.
func pickTrip(all []departures.Departure, conns []staticdata.Connection) []departures.Departure {
	var out []departures.Departure
	for _, d := range all {
		i := slices.IndexFunc(conns, func(c staticdata.Connection) bool { return c.RouteID == d.RouteID && c.BoardStopID == d.StopID })
		if i < 0 {
			continue
		}
		c := conns[i]
		if c.AnyHeadsign || slices.ContainsFunc(c.Headsigns, func(h string) bool { return staticdata.Fold(h) == staticdata.Fold(d.Headsign) }) {
			out = append(out, d)
		}
	}
	return out
}

func nearestWalk(walks map[string]*Walk) *Walk {
	var best *Walk
	for _, w := range walks {
		if best == nil || w.Minutes < best.Minutes {
			best = w
		}
	}
	return best
}

func (b *Builder) entry(ctx context.Context, ix *staticdata.Index, sl db.SavedLine, origin *walk.Point, prefs Prefs, shown int) Entry {
	e := Entry{Line: ResolveLine(ix, sl), NextCatchable: -1, Departures: []DepartureVerdict{}}
	if e.Line.Unavailable {
		return e
	}
	// The platforms to board at: one for a line, one per connecting route and
	// platform for a trip.
	var boards []*staticdata.Stop
	if sl.Kind == KindTrip {
		for _, tl := range e.Line.Lines {
			if st, ok := ix.Stop(tl.Board.ID); ok && !slices.Contains(boards, st) {
				boards = append(boards, st)
			}
		}
	} else if st, ok := ix.Stop(sl.BoardStopID); ok {
		boards = append(boards, st)
	}
	if len(boards) == 0 {
		e.Line.Unavailable = true
		return e
	}
	var wg sync.WaitGroup
	var deps []departures.Departure
	var depErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Whole boards: at a busy platform the saved line may be far down the list.
		deps, depErr = b.Departures.ForStops(ctx, boards, 0, 3*time.Hour)
	}()
	// A trip with changes also needs the departures at each change.
	var legs [][2]string
	var later []laterLeg
	var laterErr error
	if sl.Kind == KindTrip {
		legs = LegAreas(sl.FromAreaID, sl.ToAreaID, ParseTransfers(sl.Transfers))
		if len(legs) > 1 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				later, laterErr = b.laterLegs(ctx, ix, legs)
			}()
		}
	}
	walks := map[string]*Walk{} // by boarding platform
	if origin != nil && b.Walk != nil {
		var mu sync.Mutex
		var ww sync.WaitGroup
		for _, st := range boards {
			ww.Add(1)
			go func() {
				defer ww.Done()
				r, err := b.Walk.Route(ctx, *origin, st.ID, walk.Point{Lat: st.Lat, Lon: st.Lon})
				if err == nil {
					mu.Lock()
					walks[st.ID] = &Walk{Minutes: walk.Minutes(r.DistanceM, prefs.WalkSpeedKmh), DistanceM: r.DistanceM, Provider: r.Provider, Approx: r.Approx}
					mu.Unlock()
				}
			}()
		}
		ww.Wait()
	}
	wg.Wait()
	if depErr != nil {
		e.Error = "departures unavailable"
		return e
	}
	var mine []departures.Departure
	var conns []staticdata.Connection // what each departure rides, for journeys
	if sl.Kind == KindTrip {
		conns = ix.Connections(legs[0][0], legs[0][1])
		mine = pickTrip(deps, conns)
		e.Walk = nearestWalk(walks)
		if len(walks) > 0 {
			e.Walks = walks
		}
	} else {
		mine = pick(deps, sl.RouteID, sl.Headsign)
		e.Walk = walks[sl.BoardStopID]
		if sl.AlightStopID != "" {
			conns = []staticdata.Connection{{RouteID: sl.RouteID, BoardStopID: sl.BoardStopID, AlightStopID: sl.AlightStopID}}
		}
	}
	if laterErr != nil {
		later = nil // no onward departures: journeys stop after the first ride
	}
	now := b.now()
	for i, d := range mine {
		v := Verdict{Level: LevelNone, SlackMin: MinutesUntil(d.ExpectedAt, now)}
		if w := walks[d.StopID]; w != nil {
			v = Judge(d.ExpectedAt, now, w.Minutes, prefs.BufferMin)
		}
		// With walks, a departure from a platform whose walk failed (no verdict) is not known to be catchable.
		if e.NextCatchable < 0 && v.Level != LevelRed && (len(walks) == 0 || v.Level != LevelNone) {
			e.NextCatchable = i
			if w := walks[d.StopID]; w != nil && sl.Kind == KindTrip {
				e.Walk = w // the walk that matters is to the platform of the next catchable departure
			}
		}
		if i < shown || i == e.NextCatchable {
			dv := DepartureVerdict{Departure: d, Verdict: v}
			if c := connectionFor(conns, d); c != nil && (laterErr == nil || len(legs) <= 1) {
				dv.Journey = b.journey(ix, d, c, later, prefs)
			}
			e.Departures = append(e.Departures, dv)
		}
		if i >= shown && e.NextCatchable >= 0 {
			break
		}
	}
	// Re-index NextCatchable into the (possibly shortened) slice.
	if e.NextCatchable >= 0 {
		e.NextCatchable = slices.IndexFunc(e.Departures, func(d DepartureVerdict) bool { return d.Verdict.Level != LevelRed })
	}
	return e
}
