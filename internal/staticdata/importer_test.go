package staticdata

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/store"
)

func TestNextNightly(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Tallinn")
	for _, tt := range []struct{ in, want time.Time }{
		{time.Date(2026, 10, 22, 1, 0, 0, 0, loc), time.Date(2026, 10, 22, 3, 30, 0, 0, loc)},
		{time.Date(2026, 10, 22, 3, 30, 0, 0, loc), time.Date(2026, 10, 23, 3, 30, 0, 0, loc)},
		{time.Date(2026, 10, 22, 22, 0, 0, 0, loc), time.Date(2026, 10, 23, 3, 30, 0, 0, loc)},
		// DST ends on 2026-10-25 at 04:00 → 03:00; 03:30 still exists once.
		{time.Date(2026, 10, 24, 12, 0, 0, 0, loc), time.Date(2026, 10, 25, 3, 30, 0, 0, loc)},
	} {
		if got := NextNightly(tt.in, loc); !got.Equal(tt.want) {
			t.Errorf("NextNightly(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestImporterEndToEnd(t *testing.T) {
	otp, _ := os.ReadFile(testdata + "otp_agency_routes.json")
	stops, _ := os.ReadFile(testdata + "tallinn_stops.txt")
	gtfs := zipFixtureGTFS(t)
	var failElron, failOTP atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/otp":
			if failOTP.Load() {
				http.Error(w, "down", 503)
				return
			}
			_, _ = w.Write(otp)
		case r.URL.Path == "/stops.txt":
			_, _ = w.Write(stops)
		case r.URL.Path == "/elron.zip":
			if failElron.Load() {
				http.Error(w, "down", 503)
				return
			}
			_, _ = w.Write(gtfs)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	loc, _ := time.LoadLocation("Europe/Tallinn")
	f := feeds.NewFetcher(srv.Client(), "test")
	var swaps atomic.Int32
	newImporter := func() *Importer {
		return &Importer{
			Fetcher: f, OTP: &feeds.OTP{URL: feeds.Const(srv.URL + "/otp"), Fetcher: f},
			StopsURL: feeds.Const(srv.URL + "/stops.txt"), ElronGTFSURL: feeds.Const(srv.URL + "/elron.zip"), AgencyID: feeds.TallinnAgencyID,
			Store: st, Log: slog.New(slog.DiscardHandler), Location: loc,
			OnSwap: func(*Index) { swaps.Add(1) },
		}
	}
	im := newImporter()
	if !im.Index().Empty() {
		t.Fatal("index should start empty")
	}
	if err := im.Import(ctx); err != nil {
		t.Fatal(err)
	}
	s := im.Status()
	if s.Routes != 82 || s.Stops < 400 || s.LastError != "" || s.ImportedAt.IsZero() {
		t.Fatalf("status = %+v", s)
	}
	if res := im.Index().SearchAreas("Balti jaam", 1); len(res) == 0 {
		t.Fatal("search after import found nothing")
	}

	// Elron down after rail was imported: keep yesterday's data (with rail) and retry.
	failElron.Store(true)
	if err := im.Import(ctx); err == nil || !strings.Contains(err.Error(), "Elron") {
		t.Fatalf("want an Elron error, got %v", err)
	}
	if im.Status().Routes != 82 || im.Index().Empty() {
		t.Errorf("rail dropped: routes = %d", im.Status().Routes)
	}
	// A first import (nothing to keep) still goes ahead without rail.
	first := newImporter()
	first.Store, _ = store.Open(ctx, t.TempDir(), slog.New(slog.DiscardHandler))
	defer first.Store.Close()
	if err := first.Import(ctx); err != nil || first.Status().Routes != 80 {
		t.Fatalf("first import without rail: routes %d, err %v", first.Status().Routes, err)
	}
	failElron.Store(false)
	if err := im.Import(ctx); err != nil {
		t.Fatal(err)
	}

	// A failed import keeps the previous data and records the error.
	failOTP.Store(true)
	if err := im.Import(ctx); err == nil || !strings.Contains(err.Error(), "city routes") {
		t.Fatalf("want city routes error, got %v", err)
	}
	if im.Status().LastError == "" || im.Index().Empty() || im.Status().Routes != 82 {
		t.Fatalf("failed import clobbered data: %+v", im.Status())
	}

	// A fresh process loads the saved dataset without any network.
	im2 := newImporter()
	if err := im2.LoadSaved(ctx); err != nil {
		t.Fatal(err)
	}
	if im2.Index().Empty() || im2.Status().Routes != 82 {
		t.Fatalf("reloaded status = %+v", im2.Status())
	}
	im2.Now = func() time.Time { return im2.Status().ImportedAt.Add(time.Hour) }
	if im2.needsImport() {
		t.Error("fresh dataset of the current build version should not re-import")
	}
	im2.savedVersion.Store("1")
	if !im2.needsImport() {
		t.Error("dataset from an older build version must re-import")
	}
	im2.Now = func() time.Time { return im2.Status().ImportedAt.Add(MaxAge + time.Minute) }
	im2.savedVersion.Store(BuildVersion)
	if !im2.needsImport() {
		t.Error("stale dataset must re-import")
	}
	if swaps.Load() != 4 { // im: 2 good imports, first: 1, im2: the load
		t.Errorf("swaps = %d", swaps.Load())
	}
}
