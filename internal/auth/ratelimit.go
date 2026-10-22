package auth

import (
	"net/netip"
	"strings"
	"sync"
	"time"
)

// Limiter counts attempts per key in a sliding window: PLAN.md asks for at
// most 5 failed logins per 15 minutes per IP and per username.
type Limiter struct {
	Max    int
	Window time.Duration
	Now    func() time.Time

	mu       sync.Mutex
	failures map[string][]time.Time
	calls    int
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

// sweep drops expired keys now and then, so keys never seen again (random
// usernames, rotating addresses) do not accumulate.
func (l *Limiter) sweep(now time.Time) {
	l.calls++
	if l.calls%256 != 0 && len(l.failures) < 10000 {
		return
	}
	for k := range l.failures {
		l.recent(k, now)
	}
}

// Reserve counts an attempt for every key, unless one of them has already
// reached the limit (then it reports false and counts nothing). Checking and
// counting in one step means parallel attempts cannot all slip past the
// check; a successful login clears the keys with Reset.
func (l *Limiter) Reserve(keys ...string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)
	for _, k := range keys {
		if len(l.recent(k, now)) >= l.Max {
			return false
		}
	}
	for _, k := range keys {
		l.failures[k] = append(l.recent(k, now), now)
	}
	return true
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

// Reset clears keys after a successful login.
func (l *Limiter) Reset(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, k := range keys {
		delete(l.failures, k)
	}
}

// limitKeys are the per-client and per-account keys for a login attempt.
// IPv6 clients are grouped by /64 (one subscriber's addresses), and usernames
// are capped so a request cannot store an arbitrarily large key.
func limitKeys(ip, username string) []string {
	if a, err := netip.ParseAddr(ip); err == nil && a.Is6() && !a.Is4In6() {
		if p, err := a.Prefix(64); err == nil {
			ip = p.String()
		}
	}
	u := strings.ToLower(strings.TrimSpace(username))
	if len(u) > 64 {
		u = u[:64]
	}
	return []string{"ip:" + ip, "user:" + u}
}
