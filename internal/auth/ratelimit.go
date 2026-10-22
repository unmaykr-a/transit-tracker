package auth

import (
	"sync"
	"time"
)

// Limiter counts failures per key in a sliding window: PLAN.md asks for at
// most 5 failed logins per 15 minutes per IP and per username.
type Limiter struct {
	Max    int
	Window time.Duration
	Now    func() time.Time

	mu       sync.Mutex
	failures map[string][]time.Time
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{Max: max, Window: window, failures: map[string][]time.Time{}}
}

func (l *Limiter) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

func (l *Limiter) recent(key string, now time.Time) []time.Time {
	fs := l.failures[key]
	i := 0
	for i < len(fs) && now.Sub(fs[i]) >= l.Window {
		i++
	}
	fs = fs[i:]
	if len(fs) == 0 {
		delete(l.failures, key)
		return nil
	}
	l.failures[key] = fs
	return fs
}

// Blocked reports whether any key has reached the limit.
func (l *Limiter) Blocked(keys ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for _, k := range keys {
		if len(l.recent(k, now)) >= l.Max {
			return true
		}
	}
	return false
}

// Fail records a failure for every key.
func (l *Limiter) Fail(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	for _, k := range keys {
		l.failures[k] = append(l.recent(k, now), now)
	}
}

// Reset clears keys after a successful login.
func (l *Limiter) Reset(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		delete(l.failures, k)
	}
}
