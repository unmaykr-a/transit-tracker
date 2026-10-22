package ingest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/state"
)

func fixtureServer(t *testing.T, path, contentType string, lastModified string) (*httptest.Server, *int) {
	t.Helper()
	body, err := os.ReadFile("../feeds/testdata/" + path)
	if err != nil {
		t.Fatal(err)
	}
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if lastModified != "" {
			if r.Header.Get("If-Modified-Since") == lastModified {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("Last-Modified", lastModified)
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestGPSPoller(t *testing.T) {
	const lm = "Thu, 22 Oct 2026 06:23:02 GMT"
	srv, hits := fixtureServer(t, "tallinn_gps.txt", "text/plain", lm)
	store := state.NewStore()
	now := time.Date(2026, 10, 22, 6, 23, 10, 0, time.UTC)
	p := &GPSPoller{URL: feeds.Const(srv.URL), Fetcher: feeds.NewFetcher(srv.Client(), "test"), Store: store, Now: func() time.Time { return now }}

	r, err := p.Poll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if r.Count != 481 || r.DataAt.Format(time.RFC1123) != "Thu, 22 Oct 2026 06:23:02 UTC" {
		t.Fatalf("result %+v", r)
	}
	v, ok := store.Get("tram-98")
	if !ok || v.Line != "T6" || v.Label != "T6" || v.Bearing != 137 || !v.LowFloor {
		t.Fatalf("tram-98 = %+v", v)
	}
	r, err = p.Poll(context.Background())
	if err != nil || !r.Unchanged || *hits != 2 {
		t.Fatalf("second poll %+v err=%v hits=%d", r, err, *hits)
	}
}

func TestGPSPollerDistrustsBadClock(t *testing.T) {
	srv, _ := fixtureServer(t, "tallinn_gps.txt", "text/plain", "Thu, 22 Oct 2026 06:23:02 GMT")
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC) // Last-Modified 5½ h old
	p := &GPSPoller{URL: feeds.Const(srv.URL), Fetcher: feeds.NewFetcher(srv.Client(), "test"), Store: state.NewStore(), Now: func() time.Time { return now }}
	r, err := p.Poll(context.Background())
	if err != nil || !r.DataAt.Equal(now) {
		t.Fatalf("DataAt = %v err=%v", r.DataAt, err)
	}
}

func TestElronPoller(t *testing.T) {
	srv, _ := fixtureServer(t, "elron_map_data.json", "application/json", "")
	loc, _ := time.LoadLocation("Europe/Tallinn")
	store := state.NewStore()
	p := &ElronPoller{URL: feeds.Const(srv.URL), Fetcher: feeds.NewFetcher(srv.Client(), "test"), Store: store, Location: loc}
	r, err := p.Poll(context.Background())
	if err != nil || r.Count != 17 {
		t.Fatalf("result %+v err=%v", r, err)
	}
	v, ok := store.Get("train-10")
	if !ok || v.Mode != feeds.ModeTrain || v.Destination != "Tartu" || v.Label != "10" || v.DelayS == nil || *v.DelayS != 60 {
		t.Fatalf("train-10 = %+v", v)
	}
	if v.SpeedMps < 33.3 || v.SpeedMps > 33.4 {
		t.Errorf("speed = %v", v.SpeedMps)
	}
}

func TestTrainDestination(t *testing.T) {
	for in, want := range map[string]string{"Tallinn - Tartu": "Tartu", "Tartu-Tallinn": "Tallinn", "Riga": "Riga", "": ""} {
		if got := TrainDestination(in); got != want {
			t.Errorf("TrainDestination(%q) = %q, want %q", in, got, want)
		}
	}
}

// An error page served with 200, or a sudden collapse of the feed, must not
// wipe the map; a collapse that persists is accepted.
func TestGPSPollerRefusesGarbage(t *testing.T) {
	full, err := os.ReadFile("../feeds/testdata/tallinn_gps.txt")
	if err != nil {
		t.Fatal(err)
	}
	body := full
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	t.Cleanup(srv.Close)
	store := state.NewStore()
	p := &GPSPoller{URL: feeds.Const(srv.URL), Fetcher: feeds.NewFetcher(srv.Client(), "test"), Store: store}
	if r, err := p.Poll(context.Background()); err != nil || r.Count != 481 {
		t.Fatalf("first poll %+v %v", r, err)
	}
	body = []byte("<html><body>Just a moment...</body></html>\n<p>checking your browser</p>\n")
	if _, err := p.Poll(context.Background()); err == nil || count(store) != 481 {
		t.Fatalf("garbage accepted: err=%v vehicles=%d", err, count(store))
	}
	// Ten vehicles instead of 481: refused twice, then believed.
	lines := strings.SplitAfter(string(full), "\n")
	body = []byte(strings.Join(lines[:10], ""))
	for i := range 3 {
		_, err := p.Poll(context.Background())
		if (err != nil) != (i < 2) {
			t.Fatalf("collapse poll %d: err=%v", i, err)
		}
	}
	if n := count(store); n != 10 {
		t.Fatalf("persistent collapse not accepted: %d vehicles", n)
	}
}

func count(s *state.Store) int { _, m := s.Snapshot(); return len(m) }
