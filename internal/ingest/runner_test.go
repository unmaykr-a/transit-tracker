package ingest

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
)

func TestBackoff(t *testing.T) {
	for _, tt := range []struct {
		failures int
		base     time.Duration
	}{{1, 5 * time.Second}, {2, 10 * time.Second}, {3, 20 * time.Second}, {10, time.Minute}, {100, time.Minute}} {
		got := Backoff(5*time.Second, time.Minute, tt.failures)
		lo, hi := time.Duration(float64(tt.base)*0.8), time.Duration(float64(tt.base)*1.2)
		if got < lo || got > hi {
			t.Errorf("Backoff(failures=%d) = %v, want in [%v, %v]", tt.failures, got, lo, hi)
		}
	}
}

func TestHealthStaleness(t *testing.T) {
	h := NewHealth()
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	h.now = func() time.Time { return now }
	h.register("gps")
	if !h.StaleMap()["gps"] {
		t.Fatal("never-polled feed must be stale")
	}
	h.success("gps", Result{Count: 480, DataAt: now.Add(-5 * time.Second), Latency: 120 * time.Millisecond})
	s := h.Snapshot()[0]
	if s.Stale || s.Count != 480 || s.LatencyMs != 120 {
		t.Fatalf("after success: %+v", s)
	}
	// 304s keep the count and do not move the upstream data time forward.
	now = now.Add(30 * time.Second)
	h.success("gps", Result{Unchanged: true})
	if s := h.Snapshot()[0]; s.Count != 480 || !s.LastData.Equal(time.Date(2026, 10, 22, 8, 59, 55, 0, time.UTC)) {
		t.Fatalf("after 304: %+v", s)
	}
	now = now.Add(40 * time.Second)
	if !h.StaleMap()["gps"] {
		t.Fatal("feed with 75 s old data must be stale")
	}
	if hist := h.History("gps"); len(hist) != 1 || hist[0].Count != 480 { // both polls in the same minute
		t.Fatalf("history = %+v", hist)
	}
	for i := range 70 {
		now = now.Add(time.Minute)
		h.success("gps", Result{Count: i})
	}
	if hist := h.History("gps"); len(hist) != 60 || hist[59].Count != 69 {
		t.Fatalf("history not capped: %d points", len(hist))
	}
	if n := h.failure("gps", errors.New("boom")); n != 1 {
		t.Fatalf("failures = %d", n)
	}
	if s := h.Snapshot()[0]; s.LastError != "boom" || s.Failures != 1 {
		t.Fatalf("after failure: %+v", s)
	}
}

func TestRunPollsAndBacksOff(t *testing.T) {
	h := NewHealth()
	var calls atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		h.Run(ctx, slog.New(slog.DiscardHandler), Feed{
			Name: "x", Interval: feeds.Const(10 * time.Millisecond), MaxBackoff: 20 * time.Millisecond,
			Poll: func(context.Context) (Result, error) {
				if calls.Add(1)%2 == 0 {
					return Result{}, errors.New("flaky")
				}
				return Result{Count: 3}, nil
			},
		})
		close(done)
	}()
	deadline := time.After(2 * time.Second)
	for calls.Load() < 5 {
		select {
		case <-deadline:
			t.Fatal("poller did not run")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	<-done
	s := h.Snapshot()[0]
	if s.Count != 3 || s.LastError != "flaky" {
		t.Fatalf("status %+v", s)
	}
}
