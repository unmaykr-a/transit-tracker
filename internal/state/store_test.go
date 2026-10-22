package state

import (
	"math"
	"testing"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
)

var t0 = time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)

func bus(id string, lat, lon float64, at time.Time) Vehicle {
	return Vehicle{ID: id, Mode: feeds.ModeBus, Line: "17", Label: "17", Lat: lat, Lon: lon, Bearing: -1, UpdatedAt: at}
}

func TestReplaceAddsUpdatesRemoves(t *testing.T) {
	s := NewStore()
	ch, cancel := s.Subscribe()
	defer cancel()

	if n := s.Replace("gps", []Vehicle{bus("bus-1", 59.4, 24.7, t0), bus("bus-2", 59.41, 24.71, t0)}); n != 2 {
		t.Fatalf("added = %d", n)
	}
	select {
	case <-ch:
	default:
		t.Fatal("no notification after change")
	}
	// Same data again: no change, no notification, version unchanged.
	v := s.Version()
	if n := s.Replace("gps", []Vehicle{bus("bus-1", 59.4, 24.7, t0), bus("bus-2", 59.41, 24.71, t0)}); n != 0 {
		t.Fatalf("unchanged replace reported %d", n)
	}
	if s.Version() != v {
		t.Fatal("version moved without a change")
	}
	select {
	case <-ch:
		t.Fatal("notified without change")
	default:
	}
	// Another source does not remove gps vehicles.
	s.Replace("elron", []Vehicle{{ID: "train-1", Mode: feeds.ModeTrain, Lat: 59, Lon: 25, Bearing: 90, UpdatedAt: t0}})
	// bus-2 disappears from the gps snapshot.
	if n := s.Replace("gps", []Vehicle{bus("bus-1", 59.4, 24.7, t0)}); n != 1 {
		t.Fatalf("removed = %d", n)
	}
	_, all := s.Snapshot()
	if _, ok := all["bus-2"]; ok || len(all) != 2 {
		t.Fatalf("snapshot = %v", all)
	}
}

func TestDerivedSpeedAndBearing(t *testing.T) {
	s := NewStore()
	s.Replace("gps", []Vehicle{bus("bus-1", 59.4, 24.7, t0)})
	// ~111 m north in 10 s → instantaneous 11.1 m/s, smoothed with 0 → ~5.6 m/s.
	s.Replace("gps", []Vehicle{bus("bus-1", 59.401, 24.7, t0.Add(10*time.Second))})
	v, _ := s.Get("bus-1")
	if math.Abs(v.SpeedMps-5.56) > 0.1 || v.Bearing != 0 {
		t.Fatalf("after move north: speed %.2f bearing %.1f", v.SpeedMps, v.Bearing)
	}
	// Moving east keeps smoothing and turns the bearing.
	s.Replace("gps", []Vehicle{bus("bus-1", 59.401, 24.702, t0.Add(20*time.Second))})
	v, _ = s.Get("bus-1")
	if v.Bearing < 85 || v.Bearing > 95 || v.SpeedMps < 5 {
		t.Fatalf("after move east: %+v", v)
	}
	// Stationary across snapshots → speed 0, bearing kept.
	s.Replace("gps", []Vehicle{bus("bus-1", 59.401, 24.702, t0.Add(30*time.Second))})
	v, _ = s.Get("bus-1")
	if v.SpeedMps != 0 || v.Bearing < 85 || v.Bearing > 95 {
		t.Fatalf("stationary: %+v", v)
	}
	// A GPS jump (2 km in 10 s) is not believed.
	s.Replace("gps", []Vehicle{bus("bus-1", 59.42, 24.702, t0.Add(40*time.Second))})
	v, _ = s.Get("bus-1")
	if v.SpeedMps > 1 {
		t.Fatalf("jump produced speed %.1f", v.SpeedMps)
	}
}

func TestFeedHeadingAndKnownSpeedWin(t *testing.T) {
	s := NewStore()
	tr := Vehicle{ID: "train-10", Mode: feeds.ModeTrain, Lat: 58.6, Lon: 26.5, Bearing: 147, SpeedMps: 33.3, SpeedKnown: true, UpdatedAt: t0}
	s.Replace("elron", []Vehicle{tr})
	tr.Lat, tr.UpdatedAt = 58.61, t0.Add(time.Minute)
	s.Replace("elron", []Vehicle{tr})
	v, _ := s.Get("train-10")
	if v.Bearing != 147 || v.SpeedMps != 33.3 {
		t.Fatalf("feed values overridden: %+v", v)
	}
}

func TestEqual(t *testing.T) {
	d1, d2, d3 := 60, 60, 120
	a := Vehicle{ID: "x", DelayS: &d1}
	if !a.Equal(Vehicle{ID: "x", DelayS: &d2}) {
		t.Error("equal delays by value")
	}
	if a.Equal(Vehicle{ID: "x", DelayS: &d3}) || a.Equal(Vehicle{ID: "x"}) {
		t.Error("different delays")
	}
}
