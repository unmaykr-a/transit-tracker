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
