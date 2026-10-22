// Package cache has a small TTL cache with request coalescing, so many clients
// asking for the same stop cost one upstream request.
package cache

import (
	"context"
	"sync"
	"time"
)

// TTL caches values per key for a fixed time. Concurrent misses for the same
// key share one call (singleflight). A value older than TTL but younger than
// StaleFor is served at once while one background call refreshes it, and is
// served instead of the error when that refresh fails. A failure with nothing
// to fall back on is remembered for ErrTTL, so a dead upstream is not asked
// again on every request.
type TTL[K comparable, V any] struct {
	TTL      time.Duration
	StaleFor time.Duration
	// ErrTTL is how long a failure is remembered (default min(TTL, 10s)).
	ErrTTL time.Duration
	Now    func() time.Time

	mu       sync.Mutex
	entries  map[K]entry[V]
	inflight map[K]*call[V]
	fails    map[K]failure
}

type entry[V any] struct {
	v  V
	at time.Time
}

type failure struct {
	err error
	at  time.Time
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

func (c *TTL[K, V]) errTTL() time.Duration {
	if c.ErrTTL > 0 {
		return c.ErrTTL
	}
	return min(c.TTL, 10*time.Second)
}

// Get returns the cached value for key or calls fn to fill it.
func (c *TTL[K, V]) Get(ctx context.Context, key K, fn func(context.Context) (V, error)) (V, error) {
	c.mu.Lock()
	if c.entries == nil {
		c.entries = map[K]entry[V]{}
		c.inflight = map[K]*call[V]{}
		c.fails = map[K]failure{}
	}
	now := c.now()
	if e, ok := c.entries[key]; ok {
		age := now.Sub(e.at)
		if age < c.TTL {
			c.mu.Unlock()
			return e.v, nil
		}
		if age < c.StaleFor { // stale but usable: answer now, refresh behind the scenes
			if _, busy := c.inflight[key]; !busy {
				go c.run(context.WithoutCancel(ctx), key, c.start(key), fn)
			}
			c.mu.Unlock()
			return e.v, nil
		}
	}
	if f, ok := c.fails[key]; ok && now.Sub(f.at) < c.errTTL() {
		c.mu.Unlock()
		var zero V
		return zero, f.err
	}
	cl, busy := c.inflight[key]
	if !busy {
		cl = c.start(key)
	}
	c.mu.Unlock()
	if !busy {
		// The upstream call must not be cancelled by the first caller going away,
		// because other callers are waiting on the same result.
		c.run(context.WithoutCancel(ctx), key, cl, fn)
	}
	select {
	case <-cl.done:
		return cl.v, cl.err
	case <-ctx.Done():
		var zero V
		return zero, ctx.Err()
	}
}

// start registers an in-flight call for key. Called with mu held.
func (c *TTL[K, V]) start(key K) *call[V] {
	cl := &call[V]{done: make(chan struct{})}
	c.inflight[key] = cl
	return cl
}

// run makes the upstream call and publishes its result to cl's waiters.
func (c *TTL[K, V]) run(ctx context.Context, key K, cl *call[V], fn func(context.Context) (V, error)) {
	callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	v, err := fn(callCtx)
	cancel()

	c.mu.Lock()
	now := c.now()
	if err == nil {
		c.entries[key] = entry[V]{v, now}
		delete(c.fails, key)
	} else {
		c.fails[key] = failure{err, now}
		if e, ok := c.entries[key]; ok && now.Sub(e.at) < c.StaleFor {
			v, err = e.v, nil
		}
	}
	cl.v, cl.err = v, err
	delete(c.inflight, key)
	c.prune()
	c.mu.Unlock()
	close(cl.done)
}

// prune drops entries too old to be served even as stale, and old failures.
// Called with mu held.
func (c *TTL[K, V]) prune() {
	if len(c.entries)+len(c.fails) < 256 {
		return
	}
	limit := max(c.TTL, c.StaleFor)
	now := c.now()
	for k, e := range c.entries {
		if now.Sub(e.at) > limit {
			delete(c.entries, k)
		}
	}
	for k, f := range c.fails {
		if now.Sub(f.at) > c.errTTL() {
			delete(c.fails, k)
		}
	}
}
