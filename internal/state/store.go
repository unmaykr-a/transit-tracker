// Package state holds live vehicles in memory. Nothing here is persisted.
package state

import (
	"sync"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/geo"
)

// Vehicle is the normalised live vehicle sent to clients (PLAN.md, "Normalized vehicle").
type Vehicle struct {
	ID          string     `json:"id"`
	Mode        feeds.Mode `json:"mode"`
	Line        string     `json:"line"`  // route short name as published ("T4", "17A") or Elron route name
	Label       string     `json:"label"` // text for the map pill
	Lat         float64    `json:"lat"`
	Lon         float64    `json:"lon"`
	Bearing     float64    `json:"bearing"` // degrees; -1 unknown
	SpeedMps    float64    `json:"speed_mps"`
	Destination string     `json:"destination"`
	LowFloor    bool       `json:"low_floor"`
	DelayS      *int       `json:"delay_s,omitempty"`
	PatternID   string     `json:"pattern_id,omitempty"`
	NextStopID  string     `json:"next_stop_id,omitempty"`
	DistAlongM  float64    `json:"dist_along_m,omitempty"` // metres along PatternID's shape
	County      bool       `json:"county,omitempty"`       // a county bus (live from the county feed)
	TripID      string     `json:"-"`                      // the national timetable's trip, when the feed says (county, Tartu)
	UpdatedAt   time.Time  `json:"updated_at"`

	// Sources that measure speed (Elron) set this; GPS-only sources leave it
	// false and speed is derived from consecutive fixes.
	SpeedKnown bool `json:"-"`
}

// Equal reports whether two vehicles would serialise identically.
func (v Vehicle) Equal(o Vehicle) bool {
	d1, d2 := v.DelayS, o.DelayS
	if (d1 == nil) != (d2 == nil) || (d1 != nil && *d1 != *d2) {
		return false
	}
	v.DelayS, o.DelayS = nil, nil
	return v == o
}

const maxFixes = 5

type fix struct {
	lat, lon float64
	at       time.Time
}

type track struct {
	source string
	v      Vehicle
	fixes  []fix // oldest first, at most maxFixes
}

// Store is the live vehicle set, replaced source by source.
type Store struct {
	mu      sync.RWMutex
	tracks  map[string]*track
	version uint64
	subs    map[chan struct{}]struct{}
}

func NewStore() *Store {
	return &Store{tracks: map[string]*track{}, subs: map[chan struct{}]struct{}{}}
}

// Replace sets the full vehicle list for one source. Vehicles of that source
// missing from vs are removed. It returns how many vehicles were added,
// changed or removed; subscribers are notified only when that is non-zero.
func (s *Store) Replace(source string, vs []Vehicle) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := 0
	seen := make(map[string]bool, len(vs))
	for _, in := range vs {
		seen[in.ID] = true
		t, ok := s.tracks[in.ID]
		if !ok || t.source != source {
			t = &track{source: source}
			s.tracks[in.ID] = t
			if in.Bearing < 0 {
				in.Bearing = -1
			}
			t.fixes = append(t.fixes, fix{in.Lat, in.Lon, in.UpdatedAt})
			t.v = in
			changed++
			continue
		}
		next := derive(t, in)
		if !next.Equal(t.v) {
			changed++
		}
		t.v = next
	}
	for id, t := range s.tracks {
		if t.source == source && !seen[id] {
			delete(s.tracks, id)
			changed++
		}
	}
	if changed > 0 {
		s.version++
		for ch := range s.subs {
			select {
			case ch <- struct{}{}:
			default: // subscriber already has a pending notification
			}
		}
	}
	return changed
}

// derive fills speed and bearing from the vehicle's recent fixes.
func derive(t *track, in Vehicle) Vehicle {
	last := t.fixes[len(t.fixes)-1]
	moved := in.Lat != last.lat || in.Lon != last.lon
	if moved || in.UpdatedAt.Sub(last.at) > 2*time.Minute {
		t.fixes = append(t.fixes, fix{in.Lat, in.Lon, in.UpdatedAt})
		if len(t.fixes) > maxFixes {
			t.fixes = t.fixes[len(t.fixes)-maxFixes:]
		}
	}
	if !in.SpeedKnown {
		switch dt := in.UpdatedAt.Sub(last.at).Seconds(); {
		case !moved && dt > 0:
			in.SpeedMps = 0 // stationary across two snapshots: at a stop or in traffic
		case moved && dt > 0 && dt < 120:
			inst := geo.DistanceM(last.lat, last.lon, in.Lat, in.Lon) / dt
			if inst > 40 { // >144 km/h is a GPS jump, not a city vehicle
				inst = t.v.SpeedMps
			}
			in.SpeedMps = 0.5*inst + 0.5*t.v.SpeedMps
		default:
			in.SpeedMps = t.v.SpeedMps
		}
	}
	if in.Bearing < 0 {
		switch {
		case moved:
			in.Bearing = geo.BearingDeg(last.lat, last.lon, in.Lat, in.Lon)
		default:
			in.Bearing = t.v.Bearing
		}
	}
	in.SpeedMps = round2(in.SpeedMps)
	in.Bearing = round2(in.Bearing)
	return in
}

func round2(f float64) float64 {
	if f < 0 {
		return f
	}
	return float64(int64(f*100+0.5)) / 100
}

// Snapshot returns the current version and a copy of every vehicle keyed by id.
func (s *Store) Snapshot() (uint64, map[string]Vehicle) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]Vehicle, len(s.tracks))
	for id, t := range s.tracks {
		out[id] = t.v
	}
	return s.version, out
}

// Get returns one vehicle.
func (s *Store) Get(id string) (Vehicle, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tracks[id]
	if !ok {
		return Vehicle{}, false
	}
	return t.v, true
}

// Version is incremented on every change.
func (s *Store) Version() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.version
}

// Subscribe returns a channel that receives a value (coalesced) after each change.
func (s *Store) Subscribe() (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch, func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}
}
