package dashboard

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/departures"
	"github.com/unmaykr-a/transit-tracker/internal/feeds"
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
}

// ResolveLine attaches static data to a stored saved line.
func ResolveLine(ix *staticdata.Index, sl db.SavedLine) Line {
	l := Line{ID: sl.ID, RouteID: sl.RouteID, Direction: int(sl.Direction), Headsign: sl.Headsign, Label: sl.Label, NotifyMin: int(sl.NotifyThresholdMin)}
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
}

// Prefs are the user's walking settings.
type Prefs struct {
	WalkSpeedKmh float64 `json:"walk_speed_kmh"`
	BufferMin    int     `json:"buffer_min"`
}

// DefaultPrefs match PLAN.md (4.8 km/h, 1 minute buffer).
var DefaultPrefs = Prefs{WalkSpeedKmh: 4.8, BufferMin: 1}

// Builder assembles dashboards.
type Builder struct {
	Index      func() *staticdata.Index
	Departures *departures.Service
	Walk       *walk.Service
	Now        func() time.Time
}

func (b *Builder) now() time.Time {
	if b.Now != nil {
		return b.Now()
	}
	return time.Now()
}

// Build returns one entry per saved line, in order. origin may be nil (no verdicts).
func (b *Builder) Build(ctx context.Context, lines []db.SavedLine, origin *walk.Point, prefs Prefs) []Entry {
	return b.BuildN(ctx, lines, origin, prefs, Shown)
}

// BuildN is Build showing up to shown departures per line.
func (b *Builder) BuildN(ctx context.Context, lines []db.SavedLine, origin *walk.Point, prefs Prefs, shown int) []Entry {
	ix := b.Index()
	out := make([]Entry, len(lines))
	var wg sync.WaitGroup
	for i, sl := range lines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i] = b.entry(ctx, ix, sl, origin, prefs, shown)
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

func (b *Builder) entry(ctx context.Context, ix *staticdata.Index, sl db.SavedLine, origin *walk.Point, prefs Prefs, shown int) Entry {
	e := Entry{Line: ResolveLine(ix, sl), NextCatchable: -1, Departures: []DepartureVerdict{}}
	board, ok := ix.Stop(sl.BoardStopID)
	if e.Line.Unavailable || !ok {
		return e
	}
	var wg sync.WaitGroup
	var deps []departures.Departure
	var depErr error
	wg.Add(1)
	go func() {
		defer wg.Done()
		// The whole board: at a busy platform the saved line may be far down the list.
		deps, depErr = b.Departures.ForStops(ctx, []*staticdata.Stop{board}, 0, 3*time.Hour)
	}()
	if origin != nil && b.Walk != nil {
		r, err := b.Walk.Route(ctx, *origin, board.ID, walk.Point{Lat: board.Lat, Lon: board.Lon})
		if err == nil {
			e.Walk = &Walk{Minutes: walk.Minutes(r.DistanceM, prefs.WalkSpeedKmh), DistanceM: r.DistanceM, Provider: r.Provider, Approx: r.Approx}
		}
	}
	wg.Wait()
	if depErr != nil {
		e.Error = "departures unavailable"
		return e
	}
	mine := pick(deps, sl.RouteID, sl.Headsign)
	now := b.now()
	for i, d := range mine {
		v := Verdict{Level: LevelNone, SlackMin: MinutesUntil(d.ExpectedAt, now)}
		if e.Walk != nil {
			v = Judge(d.ExpectedAt, now, e.Walk.Minutes, prefs.BufferMin)
		}
		if e.NextCatchable < 0 && v.Level != LevelRed {
			e.NextCatchable = i
		}
		if i < shown || i == e.NextCatchable {
			e.Departures = append(e.Departures, DepartureVerdict{d, v})
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
