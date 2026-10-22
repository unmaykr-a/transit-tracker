// Package ingest runs one polling loop per upstream feed with backoff and
// records per-feed health for the stale banner and the admin health page.
package ingest

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sort"
	"sync"
	"time"
)

// StaleAfter is how long a feed may go without fresh data before clients are told it is delayed.
const StaleAfter = 60 * time.Second

// Status is one feed's health, safe to serialise to clients.
type Status struct {
	Name        string    `json:"name"`
	LastSuccess time.Time `json:"last_success,omitzero"`
	LastData    time.Time `json:"last_data,omitzero"` // upstream timestamp of the newest data
	LastError   string    `json:"last_error,omitempty"`
	LastErrorAt time.Time `json:"last_error_at,omitzero"`
	LatencyMs   int64     `json:"latency_ms"`
	Count       int       `json:"count"`
	Failures    int       `json:"consecutive_failures"`
	Stale       bool      `json:"stale"`
}

// Result is what a poll reports back on success.
type Result struct {
	Count     int           // items in the current snapshot
	DataAt    time.Time     // upstream data time; zero means "now"
	Latency   time.Duration // upstream request time
	Unchanged bool          // 304 / identical snapshot: keeps Count, refreshes freshness
}

// PollFunc fetches and applies one snapshot.
type PollFunc func(ctx context.Context) (Result, error)

// Health is the registry of feed statuses.
type Health struct {
	mu    sync.RWMutex
	feeds map[string]*Status
	now   func() time.Time
}

func NewHealth() *Health { return &Health{feeds: map[string]*Status{}, now: time.Now} }

func (h *Health) register(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.feeds[name]; !ok {
		h.feeds[name] = &Status{Name: name, Stale: true}
	}
}

func (h *Health) success(name string, r Result) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.feeds[name]
	now := h.now()
	s.LastSuccess = now
	s.LatencyMs = r.Latency.Milliseconds()
	s.Failures = 0
	if !r.Unchanged {
		s.Count = r.Count
	}
	switch {
	case !r.DataAt.IsZero():
		s.LastData = r.DataAt
	case !r.Unchanged || s.LastData.IsZero():
		s.LastData = now
	}
}

func (h *Health) failure(name string, err error) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.feeds[name]
	s.LastError = err.Error()
	s.LastErrorAt = h.now()
	s.Failures++
	return s.Failures
}

// Snapshot returns all statuses sorted by name with Stale computed for now.
func (h *Health) Snapshot() []Status {
	h.mu.RLock()
	defer h.mu.RUnlock()
	now := h.now()
	out := make([]Status, 0, len(h.feeds))
	for _, s := range h.feeds {
		c := *s
		c.Stale = c.LastData.IsZero() || now.Sub(c.LastData) > StaleAfter
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// StaleMap reports which feeds are currently stale.
func (h *Health) StaleMap() map[string]bool {
	m := map[string]bool{}
	for _, s := range h.Snapshot() {
		m[s.Name] = s.Stale
	}
	return m
}

// Feed describes one polling loop.
type Feed struct {
	Name     string
	Interval time.Duration
	Poll     PollFunc
	// MaxBackoff caps the retry delay after failures; default 5 minutes.
	MaxBackoff time.Duration
}

// Run polls until ctx is cancelled. The first poll happens immediately.
func (h *Health) Run(ctx context.Context, log *slog.Logger, f Feed) {
	h.register(f.Name)
	maxBackoff := f.MaxBackoff
	if maxBackoff == 0 {
		maxBackoff = 5 * time.Minute
	}
	for {
		pollCtx, cancel := context.WithTimeout(ctx, max(f.Interval*3, 20*time.Second))
		r, err := f.Poll(pollCtx)
		cancel()
		delay := f.Interval
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			n := h.failure(f.Name, err)
			delay = Backoff(f.Interval, maxBackoff, n)
			log.Warn("feed poll failed", "feed", f.Name, "err", err, "failures", n, "retry_in", delay.Round(time.Second))
		} else {
			h.success(f.Name, r)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

// Backoff returns interval·2^(failures-1) capped at maxDelay, with ±20 % jitter.
func Backoff(interval, maxDelay time.Duration, failures int) time.Duration {
	d := interval
	for i := 1; i < failures && d < maxDelay; i++ {
		d *= 2
	}
	d = min(d, maxDelay)
	jitter := 0.8 + 0.4*rand.Float64()
	return time.Duration(float64(d) * jitter)
}
