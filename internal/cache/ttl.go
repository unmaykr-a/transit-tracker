// Package cache has a small TTL cache with request coalescing, so many clients
// asking for the same stop cost one upstream request.
package cache

import (
	"context"
	"sync"
	"time"
)

// TTL caches values per key for a fixed time. Concurrent misses for the same
// key share one call (singleflight). When a refresh fails, a value up to
// StaleFor old is served instead of the error.
type TTL[K comparable, V any] struct {
	TTL      time.Duration
	StaleFor time.Duration
	Now      func() time.Time

	mu       sync.Mutex
	entries  map[K]entry[V]
	inflight map[K]*call[V]
}

type entry[V any] struct {
	v  V
	at time.Time
}

type call[V any] struct {
	done chan struct{}
	v    V
	err  error
}

func (c *TTL[K, V]) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Get returns the cached value for key or calls fn to fill it.
func (c *TTL[K, V]) Get(ctx context.Context, key K, fn func(context.Context) (V, error)) (V, error) {
	c.mu.Lock()
	if c.entries == nil {
		c.entries = map[K]entry[V]{}
		c.inflight = map[K]*call[V]{}
	}
	now := c.now()
	if e, ok := c.entries[key]; ok && now.Sub(e.at) < c.TTL {
		c.mu.Unlock()
		return e.v, nil
	}
	if cl, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		select {
		case <-cl.done:
			return cl.v, cl.err
		case <-ctx.Done():
			var zero V
			return zero, ctx.Err()
		}
	}
	cl := &call[V]{done: make(chan struct{})}
	c.inflight[key] = cl
	c.mu.Unlock()

	// The upstream call must not be cancelled by the first caller going away,
	// because other callers are waiting on the same result.
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
	v, err := fn(callCtx)
	cancel()

	c.mu.Lock()
	if err == nil {
		c.entries[key] = entry[V]{v, c.now()}
	} else if e, ok := c.entries[key]; ok && c.now().Sub(e.at) < c.StaleFor {
		v, err = e.v, nil
	}
	cl.v, cl.err = v, err
	delete(c.inflight, key)
	c.prune()
	c.mu.Unlock()
	close(cl.done)
	return v, err
}

// prune drops entries too old to be served even as stale. Called with mu held.
func (c *TTL[K, V]) prune() {
	if len(c.entries) < 256 {
		return
	}
	limit := max(c.TTL, c.StaleFor)
	now := c.now()
	for k, e := range c.entries {
		if now.Sub(e.at) > limit {
			delete(c.entries, k)
		}
	}
}
