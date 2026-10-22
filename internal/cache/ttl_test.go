package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTTLCachesAndCoalesces(t *testing.T) {
	now := time.Unix(0, 0)
	c := &TTL[string, int]{TTL: 15 * time.Second, StaleFor: time.Minute, Now: func() time.Time { return now }}
	var calls atomic.Int32
	release := make(chan struct{})
	fn := func(context.Context) (int, error) {
		calls.Add(1)
		<-release
		return 42, nil
	}
	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v, err := c.Get(context.Background(), "stop", fn); v != 42 || err != nil {
				t.Errorf("got %d %v", v, err)
			}
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1 (coalesced)", calls.Load())
	}
	// Fresh hit.
	if v, _ := c.Get(context.Background(), "stop", func(context.Context) (int, error) { return 0, errors.New("unused") }); v != 42 {
		t.Fatal("fresh value not served")
	}
	// Expired + failing refresh → stale value.
	now = now.Add(30 * time.Second)
	if v, err := c.Get(context.Background(), "stop", func(context.Context) (int, error) { return 0, errors.New("down") }); v != 42 || err != nil {
		t.Fatalf("stale fallback: %d %v", v, err)
	}
	// Too old for stale → error.
	now = now.Add(2 * time.Minute)
	if _, err := c.Get(context.Background(), "stop", func(context.Context) (int, error) { return 0, errors.New("down") }); err == nil {
		t.Fatal("want error past StaleFor")
	}
}

// Past its TTL, a value is still served at once while one background call
// refreshes it.
func TestStaleWhileRevalidate(t *testing.T) {
	now := time.Unix(0, 0)
	c := &TTL[string, int]{TTL: time.Second, StaleFor: time.Minute, Now: func() time.Time { return now }}
	var calls atomic.Int32
	release := make(chan struct{})
	fn := func(context.Context) (int, error) {
		n := calls.Add(1)
		if n > 1 {
			<-release // the refresh is slow
		}
		return int(n), nil
	}
	if v, _ := c.Get(context.Background(), "k", fn); v != 1 {
		t.Fatal(v)
	}
	now = now.Add(5 * time.Second)
	for range 3 { // stale: answered immediately, one refresh started
		if v, err := c.Get(context.Background(), "k", fn); v != 1 || err != nil {
			t.Fatalf("stale get = %d %v", v, err)
		}
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if v, _ := c.Get(context.Background(), "k", fn); v == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background refresh never landed")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("upstream calls = %d, want 2", n)
	}
}

// A failure with nothing to fall back on is remembered briefly.
func TestNegativeCaching(t *testing.T) {
	now := time.Unix(0, 0)
	c := &TTL[string, int]{TTL: time.Minute, ErrTTL: 5 * time.Second, Now: func() time.Time { return now }}
	var calls atomic.Int32
	fail := func(context.Context) (int, error) { calls.Add(1); return 0, errors.New("down") }
	for range 3 {
		if _, err := c.Get(context.Background(), "k", fail); err == nil {
			t.Fatal("want error")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1 within ErrTTL", calls.Load())
	}
	now = now.Add(6 * time.Second)
	_, _ = c.Get(context.Background(), "k", fail)
	if calls.Load() != 2 {
		t.Fatalf("calls = %d after ErrTTL", calls.Load())
	}
}
