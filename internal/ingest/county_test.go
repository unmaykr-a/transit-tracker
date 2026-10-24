package ingest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/state"
)

// The fixture was recorded at 13:32 UTC on 23 Oct 2026.
var countyAt = time.Date(2026, 10, 23, 13, 33, 0, 0, time.UTC)

func TestCountyPoller(t *testing.T) {
	srv, _ := fixtureServer(t, "ytkpohja_vehicle_positions.pb", "application/octet-stream", "")
	store := state.NewStore()
	annotated := 0
	p := &CountyPoller{
		URL: feeds.Const(srv.URL), Fetcher: feeds.NewFetcher(srv.Client(), "test"), Store: store,
		Now:      func() time.Time { return countyAt },
		Known:    func(line string) bool { return line != "174" }, // pretend 174 is missing from the timetable
		Annotate: func(vs []state.Vehicle) { annotated += len(vs) },
	}
	res, err := p.Poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_, vs := store.Snapshot()
	if res.Count < 100 || len(vs) != res.Count || annotated != res.Count {
		t.Fatalf("count %d, stored %d, annotated %d", res.Count, len(vs), annotated)
	}
	for _, v := range vs {
		if !strings.HasPrefix(v.ID, "bus-") || v.Mode != feeds.ModeBus || !v.County || v.Line == "" || v.Line == "174" || !v.SpeedKnown {
			t.Errorf("odd vehicle %+v", v)
		}
		// Harju county only: its buses stay within ~100 km of Tallinn.
		if v.Lat < 58.9 || v.Lat > 59.7 || v.Lon < 23.4 || v.Lon > 26.2 {
			t.Errorf("%s (line %s) outside Harju county: %f, %f", v.ID, v.Line, v.Lat, v.Lon)
		}
	}
	// The same snapshot again changes nothing; an empty URL clears the source.
	if res, _ := p.Poll(context.Background()); !res.Unchanged {
		t.Error("identical snapshot not reported unchanged")
	}
	p.URL = feeds.Const("")
	_, _ = p.Poll(context.Background())
	if _, vs := store.Snapshot(); len(vs) != 0 {
		t.Errorf("%d county vehicles left with the feed off", len(vs))
	}

	// Fixes older than five minutes (a parked bus's last report) are left out.
	p.URL, p.lastHash = feeds.Const(srv.URL), [32]byte{}
	p.Now = func() time.Time { return countyAt.Add(time.Hour) }
	if res, _ := p.Poll(context.Background()); res.Count != 0 {
		t.Errorf("%d vehicles from an hour-old snapshot", res.Count)
	}
}

func TestCountyTrips(t *testing.T) {
	srv, _ := fixtureServer(t, "ytkpohja_trip_updates.pb", "application/octet-stream", "")
	now := countyAt
	c := &CountyTrips{URL: feeds.Const(srv.URL), Fetcher: feeds.NewFetcher(srv.Client(), "test"), Now: func() time.Time { return now }}
	res, err := c.Poll(context.Background())
	if err != nil || res.Count < 50 {
		t.Fatalf("count %d err %v", res.Count, err)
	}
	// Any trip of the fixture, by the timetable's id form, at one of its stops.
	var trip, stop string
	for k, stops := range c.trips {
		for s := range stops {
			trip, stop = k, s
		}
		break
	}
	at, ok := c.Expected("estonia:"+trip, stop)
	if !ok || at.Before(countyAt.Add(-time.Hour)) || at.After(countyAt.Add(3*time.Hour)) {
		t.Errorf("Expected(%s, %s) = %v, %v", trip, stop, at, ok)
	}
	if _, ok := c.Expected("estonia:"+trip, "no-such-stop"); ok {
		t.Error("a stop the trip does not call at has a time")
	}
	// Predictions go stale with the feed.
	now = countyAt.Add(10 * time.Minute)
	if _, ok := c.Expected("estonia:"+trip, stop); ok {
		t.Error("stale predictions still used")
	}
	var nilTrips *CountyTrips
	if _, ok := nilTrips.Expected("x", "y"); ok {
		t.Error("nil CountyTrips has times")
	}
}

func TestTripKey(t *testing.T) {
	for in, want := range map[string]string{
		"44_ATL_al_21.09-25.10.26_Orumetsa+FRK-Fr-harjumaa174-A-OB-1620-4cd6234":      "ATL_al_21.09-25.10.26_Orumetsa+FRK-Fr-harjumaa174-A-OB-1620-4cd6234",
		"estonia:ATL_al_21.09-25.10.26_Orumetsa+FRK-Fr-harjumaa174-A-OB-1620-4cd6234": "ATL_al_21.09-25.10.26_Orumetsa+FRK-Fr-harjumaa174-A-OB-1620-4cd6234",
		"KOV_KP_06.-25.10.2026-Fr-harjumaaJ7-14:45-OB-1625-fea48ce":                   "KOV_KP_06.-25.10.2026-Fr-harjumaaJ7-14:45-OB-1625-fea48ce",
		"estonia:19726": "19726",
	} {
		if got := TripKey(in); got != want {
			t.Errorf("TripKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCountyPollerRegions(t *testing.T) {
	srv, _ := fixtureServer(t, "ytkpohja_vehicle_positions.pb", "application/octet-stream", "")
	store := state.NewStore()
	p := &CountyPoller{
		URL: feeds.Const(srv.URL), Fetcher: feeds.NewFetcher(srv.Client(), "test"), Store: store,
		Now:     func() time.Time { return countyAt },
		Regions: map[string]string{CountyRegion: "", "raplamaa": "raplamaa-", "laane_viru": "laane-virumaa-"},
	}
	if _, err := p.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, vs := store.Snapshot()
	byPrefix := map[string]int{}
	for _, v := range vs {
		switch {
		case strings.HasPrefix(v.Line, "raplamaa-"):
			byPrefix["rapla"]++
			if v.Label != strings.TrimPrefix(v.Line, "raplamaa-") || !v.County {
				t.Errorf("Rapla bus %+v", v)
			}
		case strings.HasPrefix(v.Line, "laane-virumaa-"):
			byPrefix["laane-viru"]++
		case strings.Contains(v.Line, "-"):
			t.Errorf("unexpected line key %q", v.Line)
		default:
			byPrefix["harju"]++
		}
	}
	// The fixture has 140 Harju, 21 Rapla, 18 Lääne-Viru and 7 Läänemaa (not followed here) buses.
	if byPrefix["harju"] < 100 || byPrefix["rapla"] < 10 || byPrefix["laane-viru"] < 10 || len(vs) > 140+21+18 {
		t.Errorf("by region %v of %d", byPrefix, len(vs))
	}
}
