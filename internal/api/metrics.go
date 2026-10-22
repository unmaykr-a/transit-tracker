package api

import (
	"context"
	"crypto/subtle"
	"net/http"
	"net/netip"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/unmaykr-a/transit-tracker/internal/buildinfo"
	"github.com/unmaykr-a/transit-tracker/internal/metrics"
)

// serverMetrics are updated on the request path; everything else is
// collected from live state when /metrics is scraped.
type serverMetrics struct {
	reg        *metrics.Registry
	requests   *metrics.CounterVec
	duration   *metrics.HistogramVec
	wsClients  atomic.Int64
	sseClients atomic.Int64
}

func (s *Server) initMetrics() {
	m := &serverMetrics{reg: metrics.NewRegistry()}
	s.metrics = m
	r := m.reg
	r.GaugeFunc("bus_build_info", "Version and commit of the running build.", func() []metrics.Sample {
		return []metrics.Sample{{Labels: []string{"version", buildinfo.Version, "commit", buildinfo.Commit, "goversion", runtime.Version()}, Value: 1}}
	})
	r.GaugeFunc("bus_start_time_seconds", "Unix time the server started.", func() []metrics.Sample {
		return []metrics.Sample{{Value: float64(s.started.Unix())}}
	})
	m.requests = r.Counter("bus_http_requests_total", "HTTP requests by route pattern, method and status code.", "route", "method", "code")
	m.duration = r.Histogram("bus_http_request_duration_seconds", "HTTP request duration by route pattern (streams excluded).", metrics.DefBuckets, "route")
	r.GaugeFunc("bus_live_clients", "Connected live map clients.", func() []metrics.Sample {
		return []metrics.Sample{
			{Labels: []string{"transport", "websocket"}, Value: float64(m.wsClients.Load())},
			{Labels: []string{"transport", "sse"}, Value: float64(m.sseClients.Load())},
		}
	})
	r.GaugeFunc("bus_vehicles", "Live vehicles by mode, and how many are matched to a route pattern.", func() []metrics.Sample {
		_, vs := s.store.Snapshot()
		total, matched := map[string]int{}, map[string]int{}
		for _, v := range vs {
			total[string(v.Mode)]++
			if v.PatternID != "" {
				matched[string(v.Mode)]++
			}
		}
		var out []metrics.Sample
		for _, mode := range []string{"bus", "tram", "trolley", "train"} {
			out = append(out,
				metrics.Sample{Labels: []string{"mode", mode, "state", "all"}, Value: float64(total[mode])},
				metrics.Sample{Labels: []string{"mode", mode, "state", "matched"}, Value: float64(matched[mode])})
		}
		return out
	})
	feeds := func(value func(name string, polls, errors int64, latencyMs int64, count int, stale bool, last time.Time) float64) func() []metrics.Sample {
		return func() []metrics.Sample {
			var out []metrics.Sample
			for _, f := range s.health.Snapshot() {
				out = append(out, metrics.Sample{Labels: []string{"feed", f.Name}, Value: value(f.Name, f.Polls, f.Errors, f.LatencyMs, f.Count, f.Stale, f.LastSuccess)})
			}
			return out
		}
	}
	r.CounterFunc("bus_feed_polls_total", "Upstream polls per feed.", feeds(func(_ string, polls, _, _ int64, _ int, _ bool, _ time.Time) float64 { return float64(polls) }))
	r.CounterFunc("bus_feed_errors_total", "Failed upstream polls per feed.", feeds(func(_ string, _, errs, _ int64, _ int, _ bool, _ time.Time) float64 { return float64(errs) }))
	r.GaugeFunc("bus_feed_latency_seconds", "Latency of the last successful poll.", feeds(func(_ string, _, _, ms int64, _ int, _ bool, _ time.Time) float64 { return float64(ms) / 1000 }))
	r.GaugeFunc("bus_feed_items", "Items in the feed's latest snapshot.", feeds(func(_ string, _, _, _ int64, n int, _ bool, _ time.Time) float64 { return float64(n) }))
	r.GaugeFunc("bus_feed_stale", "1 while the feed has had no fresh data for 60 s.", feeds(func(_ string, _, _, _ int64, _ int, stale bool, _ time.Time) float64 {
		if stale {
			return 1
		}
		return 0
	}))
	r.GaugeFunc("bus_feed_last_success_timestamp_seconds", "Unix time of the last successful poll (0 = never).", feeds(func(_ string, _, _, _ int64, _ int, _ bool, at time.Time) float64 {
		if at.IsZero() {
			return 0
		}
		return float64(at.Unix())
	}))
	if s.db != nil {
		r.GaugeFunc("bus_users", "Registered users.", func() []metrics.Sample {
			n, err := s.db.Q.CountUsers(context.Background())
			if err != nil {
				return nil
			}
			return []metrics.Sample{{Value: float64(n)}}
		})
	}
	r.GaugeFunc("go_goroutines", "Goroutines.", func() []metrics.Sample {
		return []metrics.Sample{{Value: float64(runtime.NumGoroutine())}}
	})
	r.GaugeFunc("go_memstats_heap_alloc_bytes", "Heap bytes allocated and in use.", func() []metrics.Sample {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		return []metrics.Sample{{Value: float64(ms.HeapAlloc)}}
	})
	r.GaugeFunc("go_memstats_sys_bytes", "Bytes obtained from the OS.", func() []metrics.Sample {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		return []metrics.Sample{{Value: float64(ms.Sys)}}
	})
}

// observeRequest records one finished request under its chi route pattern,
// so ids in paths do not explode the label set.
func (s *Server) observeRequest(r *http.Request, status int, d time.Duration) {
	if s.metrics == nil {
		return
	}
	route := "other"
	if rc := chi.RouteContext(r.Context()); rc != nil && rc.RoutePattern() != "" && rc.RoutePattern() != "/*" {
		route = rc.RoutePattern()
	}
	if status == 0 {
		status = http.StatusOK
	}
	s.metrics.requests.Inc(route, r.Method, strconv.Itoa(status))
	if route != "/ws" && route != "/api/v1/vehicles/stream" {
		s.metrics.duration.Observe(d.Seconds(), route)
	}
}

// handleMetrics serves the Prometheus exposition, guarded by BUS_METRICS.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if !s.metricsAllowed(r) {
		writeError(w, http.StatusForbidden, "metrics are not available from here", "")
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	s.metrics.reg.Write(w)
}

func (s *Server) metricsAllowed(r *http.Request) bool {
	if s.metricsToken != "" {
		got, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		return subtle.ConstantTimeCompare([]byte(got), []byte(s.metricsToken)) == 1
	}
	switch s.metricsMode {
	case "public":
		return true
	case "off":
		return false
	}
	// private: the real client (after trusted-proxy resolution) must be on a
	// loopback or private network, e.g. a Prometheus on the LAN, not a visitor
	// coming in through the reverse proxy.
	a, err := netip.ParseAddr(meta(r).IP)
	if err != nil {
		return false
	}
	a = a.Unmap()
	return a.IsLoopback() || a.IsPrivate()
}
