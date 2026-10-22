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
