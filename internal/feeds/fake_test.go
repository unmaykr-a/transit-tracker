package feeds

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"
)

func TestFakeTransportMovesVehicles(t *testing.T) {
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	ft := &FakeTransport{Now: func() time.Time { return now }}
	f := NewFetcher(&http.Client{Transport: ft}, "test")
	ctx := context.Background()

	r1, err := f.Get(ctx, "https://transport.tallinn.ee/gps.txt", true)
	if err != nil {
		t.Fatal(err)
	}
	v1, skipped, _ := ParseGPS(bytes.NewReader(r1.Body))
	if len(v1) != 481 || skipped != 0 {
		t.Fatalf("got %d vehicles (%d skipped)", len(v1), skipped)
	}
	// Same 10 s window → 304.
	now = now.Add(3 * time.Second)
	if r, err := f.Get(ctx, "https://transport.tallinn.ee/gps.txt", true); err != nil || !r.NotModified {
		t.Fatalf("want 304 within the same tick, got %+v %v", r, err)
	}
	now = now.Add(20 * time.Second)
	r2, err := f.Get(ctx, "https://transport.tallinn.ee/gps.txt", true)
	if err != nil || r2.NotModified {
		t.Fatalf("want new snapshot, got %+v %v", r2, err)
	}
	v2, _, _ := ParseGPS(bytes.NewReader(r2.Body))
	moved := 0
	for i := range v1 {
		if v1[i].Lat != v2[i].Lat || v1[i].Lon != v2[i].Lon {
			moved++
		}
	}
	if moved < 300 || moved == len(v1) {
		t.Errorf("moved %d of %d; want most but not all", moved, len(v1))
	}

	r3, err := f.Get(ctx, "https://elron.ee/map_data.json", false)
	if err != nil {
		t.Fatal(err)
	}
	trains, _, err := ParseElronTrains(bytes.NewReader(r3.Body), time.UTC)
	if err != nil || len(trains) != 17 {
		t.Fatalf("trains %d err %v", len(trains), err)
	}
	if _, err := f.Get(ctx, "https://example.com/unknown", false); err == nil {
		t.Error("unknown URL should 404")
	}
}

func TestFakeStaticAndDepartures(t *testing.T) {
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	ft := FakeTransport{Now: func() time.Time { return now }, Lines: func(id string) []FakeLine {
		if id == "1298" {
			return []FakeLine{{ModeTram, "T4", "Lennujaam"}, {ModeBus, "17", "Väike-Õismäe"}}
		}
		if id == "10137" {
			return []FakeLine{{ModeTrain, "R12", "Keila"}}
		}
		return nil
	}}
	client := &http.Client{Transport: ft}
	f := NewFetcher(client, "test")
	ctx := context.Background()

	r, err := f.Get(ctx, "https://transport.tallinn.ee/data/stops.txt", false)
	if err != nil || !bytes.Contains(r.Body, []byte("Hobujaama")) {
		t.Fatalf("stops.txt: %v", err)
	}
	r, err = f.Get(ctx, "https://eu-gtfs.remix.com/elron.zip", false)
	if err != nil {
		t.Fatal(err)
	}
	if g, err := ReadGTFSZip(r.Body); err != nil || len(g.Routes) != 2 {
		t.Fatalf("elron.zip: %v", err)
	}
	otp := &OTP{URL: DefaultOTPURL, Fetcher: f}
	if routes, err := otp.AgencyRoutes(ctx, TallinnAgencyID); err != nil || len(routes) != 80 {
		t.Fatalf("agency routes: %d %v", len(routes), err)
	}
	r, err = f.Get(ctx, "https://transport.tallinn.ee/siri-stop-departures.php?stopid=1298", false)
	if err != nil {
		t.Fatal(err)
	}
	board, skipped, err := ParseSiri(bytes.NewReader(r.Body))
	if err != nil || skipped != 0 || board.StopID != "1298" || len(board.Departures) < 8 {
		t.Fatalf("siri board %+v skipped=%d err=%v", board, skipped, err)
	}
	for _, d := range board.Departures {
		if d.SecondsUntil < 0 || d.SecondsUntil > 3600+150 {
			t.Errorf("departure out of window: %+v", d)
		}
	}
	times, err := otp.StopTimes(ctx, []string{"estonia:10137"}, 10, 3600)
	if err != nil || len(times["estonia:10137"]) == 0 || times["estonia:10137"][0].Trip.Route.Mode != "RAIL" {
		t.Fatalf("otp stoptimes: %+v %v", times, err)
	}
}
