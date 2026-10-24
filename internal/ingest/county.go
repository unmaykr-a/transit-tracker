package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/state"
)

// CountyRegion is Harju county in the county feed; its lines are keyed by
// number alone ("174"), other counties' with a prefix (see Regions).
const CountyRegion = "harjumaa"

// lineKey is a feed line's key in the static data for a region in regions
// (feed region → line prefix; nil means Harju only), or false for regions not followed.
func lineKey(regions map[string]string, region, line string) (string, bool) {
	if regions == nil {
		return line, region == CountyRegion
	}
	prefix, ok := regions[region]
	return prefix + line, ok
}

// CountyPoller polls the ÜTK Põhja GTFS-RT VehiclePositions into the store
// (source "county"): buses of the followed counties on lines the static data knows.
type CountyPoller struct {
	URL     func() string
	Fetcher *feeds.Fetcher
	Store   *state.Store
	// Regions maps the feed's regions to line prefixes ("harjumaa" → "",
	// "raplamaa" → "raplamaa-"); nil follows Harju county only.
	Regions map[string]string
	// Known reports whether a line is a county route of the static data; others
	// (other counties' buses, lines missing from the timetable) are left out.
	Known func(line string) bool
	// Annotate, if set, matches vehicles to patterns (and may fill Destination) before they are stored.
	Annotate func([]state.Vehicle)
	Now      func() time.Time

	lastHash [32]byte
}

func (p *CountyPoller) Poll(ctx context.Context) (Result, error) {
	url := p.URL()
	if url == "" { // turned off: nothing to show, and nothing stale either
		p.Store.Replace("county", nil)
		return Result{DataAt: time.Now()}, nil
	}
	res, err := p.Fetcher.Get(ctx, url, true)
	if err != nil {
		return Result{}, err
	}
	if res.NotModified {
		return Result{Unchanged: true, Latency: res.Latency}, nil
	}
	h := sha256.Sum256(res.Body)
	if h == p.lastHash {
		return Result{Unchanged: true, Latency: res.Latency}, nil
	}
	f, _, err := feeds.ParseGTFSRT(bytes.NewReader(res.Body))
	if err != nil {
		return Result{}, err
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	vs := make([]state.Vehicle, 0, len(f.Vehicles))
	seen := map[string]bool{}
	for _, rv := range f.Vehicles {
		line, ok := lineKey(p.Regions, rv.Region, rv.Line)
		if !ok || rv.Line == "" || (p.Known != nil && !p.Known(line)) {
			continue
		}
		id := "bus-" + rv.ID
		if seen[id] {
			continue
		}
		seen[id] = true
		at := rv.At
		// A fix in the future or long ago is a clock problem or a parked device.
		if at.IsZero() || at.After(now.Add(time.Minute)) {
			at = now
		}
		if now.Sub(at) > 5*time.Minute {
			continue
		}
		v := state.Vehicle{
			ID:        id,
			Mode:      feeds.ModeBus,
			Line:      line,
			Label:     rv.Line,
			Lat:       rv.Lat,
			Lon:       rv.Lon,
			Bearing:   rv.Bearing,
			UpdatedAt: at,
			County:    true,
			TripID:    rv.TripID,
		}
		if rv.HasSpeed {
			v.SpeedMps, v.SpeedKnown = rv.SpeedMps, true
		}
		vs = append(vs, v)
	}
	if p.Annotate != nil {
		p.Annotate(vs)
	}
	p.Store.Replace("county", vs)
	p.lastHash = h
	at := f.At
	if at.IsZero() || at.After(now) {
		at = now
	}
	return Result{Count: len(vs), DataAt: at, Latency: res.Latency}, nil
}

// CountyTrips holds the latest GTFS-RT TripUpdates: predicted departure (or
// arrival) times of county trips at the stops still ahead, by trip and stop code.
type CountyTrips struct {
	URL     func() string
	Fetcher *feeds.Fetcher
	Regions map[string]string // as CountyPoller.Regions
	Now     func() time.Time

	mu    sync.RWMutex
	trips map[string]map[string]time.Time // trip key → stop code → expected time
	at    time.Time
}

// TripKey is the part of a trip id both the GTFS-RT feed ("44_ATL_al_…") and
// the national timetable ("estonia:ATL_al_…") share.
func TripKey(id string) string {
	id = strings.TrimPrefix(id, "estonia:")
	if i := strings.IndexByte(id, '_'); i > 0 && strings.Trim(id[:i], "0123456789") == "" {
		id = id[i+1:]
	}
	return id
}

func (c *CountyTrips) Poll(ctx context.Context) (Result, error) {
	url := c.URL()
	if url == "" {
		c.mu.Lock()
		c.trips = nil
		c.mu.Unlock()
		return Result{DataAt: time.Now()}, nil
	}
	res, err := c.Fetcher.Get(ctx, url, true)
	if err != nil {
		return Result{}, err
	}
	if res.NotModified {
		return Result{Unchanged: true, Latency: res.Latency}, nil
	}
	f, _, err := feeds.ParseGTFSRT(bytes.NewReader(res.Body))
	if err != nil {
		return Result{}, err
	}
	if len(f.Trips) == 0 && len(res.Body) > 1024 {
		return Result{}, fmt.Errorf("county trips: no trip updates in %d bytes", len(res.Body))
	}
	trips := make(map[string]map[string]time.Time, len(f.Trips))
	n := 0
	for _, t := range f.Trips {
		if _, ok := lineKey(c.Regions, t.Region, ""); !ok {
			continue
		}
		stops := make(map[string]time.Time, len(t.Stops))
		for _, st := range t.Stops {
			at := st.Departure
			if at.IsZero() {
				at = st.Arrival
			}
			if st.StopID != "" && !at.IsZero() {
				stops[st.StopID] = at
			}
		}
		if len(stops) > 0 {
			trips[TripKey(t.TripID)] = stops
			n++
		}
	}
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	at := f.At
	if at.IsZero() || at.After(now) {
		at = now
	}
	c.mu.Lock()
	c.trips, c.at = trips, at
	c.mu.Unlock()
	return Result{Count: n, DataAt: at, Latency: res.Latency}, nil
}

// Expected returns the predicted time of a county trip (any id form) at a stop
// (its national code), if the last update has one that is recent enough.
func (c *CountyTrips) Expected(tripID, stopCode string) (time.Time, bool) {
	if c == nil {
		return time.Time{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	now := time.Now()
	if c.Now != nil {
		now = c.Now()
	}
	if c.trips == nil || now.Sub(c.at) > 3*time.Minute {
		return time.Time{}, false
	}
	t, ok := c.trips[TripKey(tripID)][stopCode]
	return t, ok
}
