package api

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/ingest"
	"github.com/unmaykr-a/transit-tracker/internal/state"
)

// Filter narrows the vehicles a client receives. Empty fields match everything.
type Filter struct {
	Modes []feeds.Mode `json:"modes,omitempty"`
	Lines []string     `json:"lines,omitempty"` // matched case-insensitively against Vehicle.Line
}

func (f Filter) match(v state.Vehicle) bool {
	if len(f.Modes) > 0 && !slices.Contains(f.Modes, v.Mode) {
		return false
	}
	if len(f.Lines) > 0 && !slices.ContainsFunc(f.Lines, func(l string) bool { return strings.EqualFold(l, v.Line) }) {
		return false
	}
	return true
}

func filterFromQuery(r *http.Request) Filter {
	var f Filter
	for _, m := range splitList(r.URL.Query().Get("mode")) {
		f.Modes = append(f.Modes, feeds.Mode(strings.ToLower(m)))
	}
	f.Lines = splitList(r.URL.Query().Get("line"))
	return f
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// liveMessage is one server→client message on /ws and the SSE stream.
type liveMessage struct {
	Type       string          `json:"type"` // snapshot | diff | stale
	Version    uint64          `json:"version"`
	ServerTime time.Time       `json:"server_time"`
	Vehicles   []state.Vehicle `json:"vehicles,omitempty"` // snapshot
	Upsert     []state.Vehicle `json:"upsert,omitempty"`   // diff: new vehicles or changed details
	Move       []move          `json:"move,omitempty"`     // diff: only the position changed
	Remove     []string        `json:"remove,omitempty"`   // diff
	Feeds      []ingest.Status `json:"feeds,omitempty"`    // snapshot, stale
}

// move is a position-only update, sent as
// [id, lat, lon, bearing, speed_mps, updated_at_unix_ms, dist_along_m]
// because it is by far the most common change: ~45 bytes instead of ~250 per vehicle.
type move struct{ v state.Vehicle }

func (m move) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{m.v.ID, m.v.Lat, m.v.Lon, m.v.Bearing, m.v.SpeedMps, m.v.UpdatedAt.UnixMilli(), m.v.DistAlongM})
}

// positionOnly reports whether old and cur differ only in where and when
// (including progress along the same pattern).
func positionOnly(old, cur state.Vehicle) bool {
	old.Lat, old.Lon, old.Bearing, old.SpeedMps, old.UpdatedAt, old.DistAlongM = cur.Lat, cur.Lon, cur.Bearing, cur.SpeedMps, cur.UpdatedAt, cur.DistAlongM
	return old.Equal(cur)
}

func (s *Server) filtered(f Filter) (uint64, map[string]state.Vehicle) {
	fr := s.frame()
	return fr.version, fr.view(f).vehicles
}

func sortedVehicles(m map[string]state.Vehicle) []state.Vehicle {
	out := slices.Collect(maps.Values(m))
	slices.SortFunc(out, func(a, b state.Vehicle) int { return strings.Compare(a.ID, b.ID) })
	return out
}

// liveSession streams a snapshot, then diffs against what this client last
// received (see livehub.go: clients that kept up share one encoded diff per
// update; the others get their own, so slow clients self-heal).
func (s *Server) liveSession(ctx context.Context, r *http.Request, initial Filter, filters <-chan Filter, send func(typ string, b []byte) error) error {
	notify, unsubscribe := s.store.Subscribe()
	defer unsubscribe()
	staleTick := time.NewTicker(5 * time.Second)
	defer staleTick.Stop()

	filter := initial
	var sent map[string]state.Vehicle // never modified: it is shared with other clients
	var sentVersion uint64
	snapshot := func() error {
		fr := s.frame()
		sent, sentVersion = fr.view(filter).vehicles, fr.version
		return send("snapshot", fr.snapshot(filter, s))
	}
	lastStale := s.health.StaleMap()
	ticks := 0
	if err := snapshot(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case f := <-filters:
			filter = f
			if err := snapshot(); err != nil {
				return err
			}
		case <-notify:
			fr := s.frame()
			if fr.version == sentVersion {
				continue
			}
			b, shared := fr.sharedDiff(filter, sentVersion)
			cur := fr.view(filter).vehicles
			if !shared {
				msg, changed := diffMessage(sent, cur, fr.version, time.Now())
				if changed {
					var err error
					if b, err = json.Marshal(msg); err != nil {
						return err
					}
				}
			}
			sent, sentVersion = cur, fr.version
			if b == nil {
				continue
			}
			if err := send("diff", b); err != nil {
				return err
			}
		case <-staleTick.C:
			// Access is checked when a stream opens; re-check now and then, so turning the
			// public map off, logging out or disabling the user also ends open streams.
			if ticks++; ticks%6 == 0 && !s.mayStream(r) {
				return nil
			}
			if st := s.health.StaleMap(); !maps.Equal(st, lastStale) {
				lastStale = st
				b, err := json.Marshal(liveMessage{Type: "stale", Version: s.store.Version(), ServerTime: time.Now(), Feeds: s.health.Snapshot()})
				if err != nil {
					return err
				}
				if err := send("stale", b); err != nil {
					return err
				}
			}
		}
	}
}

// clientMessage is what a client may send on /ws.
type clientMessage struct {
	Filter *Filter `json:"filter"`
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionNoContextTakeover,
		OriginPatterns:  s.originPatterns,
	})
	if err != nil {
		return // Accept already wrote the HTTP error
	}
	defer c.CloseNow()
	s.metrics.wsClients.Add(1)
	defer s.metrics.wsClients.Add(-1)
	c.SetReadLimit(16 << 10)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	filters := make(chan Filter, 1)
	go func() {
		defer cancel()
		for {
			_, b, err := c.Read(ctx)
			if err != nil {
				return
			}
			var m clientMessage
			if json.Unmarshal(b, &m) != nil || m.Filter == nil {
				continue
			}
			select { // keep only the newest filter
			case <-filters:
			default:
			}
			filters <- *m.Filter
		}
	}()
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pctx, pcancel := context.WithTimeout(ctx, 10*time.Second)
				err := c.Ping(pctx)
				pcancel()
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()

	err = s.liveSession(ctx, r, filterFromQuery(r), filters, func(_ string, b []byte) error {
		wctx, wcancel := context.WithTimeout(ctx, 10*time.Second)
		defer wcancel()
		return c.Write(wctx, websocket.MessageText, b)
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		s.log.Debug("ws closed", "err", err)
	}
	c.Close(websocket.StatusNormalClosure, "")
}

// vehicleStream is the SSE fallback for clients or proxies without WebSockets.
func (s *Server) vehicleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no") // nginx: do not buffer the stream
	s.metrics.sseClients.Add(1)
	defer s.metrics.sseClients.Add(-1)
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// The pinger must stop before this handler returns: writing to w afterwards is not allowed.
	ctx, cancel := context.WithCancel(r.Context())
	var wg sync.WaitGroup
	defer wg.Wait()
	defer cancel()
	var mu sync.Mutex
	write := func(chunk []byte) error {
		mu.Lock()
		defer mu.Unlock()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, err := w.Write(chunk); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}
	wg.Go(func() {
		t := time.NewTicker(25 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = write([]byte(": ping\n\n"))
			}
		}
	})
	_ = s.liveSession(ctx, r, filterFromQuery(r), nil, func(typ string, b []byte) error {
		chunk := make([]byte, 0, len(b)+len(typ)+16)
		chunk = append(append(append(append(chunk, "event: "...), typ...), "\ndata: "...), b...)
		return write(append(chunk, "\n\n"...))
	})
}

func (s *Server) vehicles(w http.ResponseWriter, r *http.Request) {
	version, cur := s.filtered(filterFromQuery(r))
	writeJSON(w, http.StatusOK, liveMessage{Type: "snapshot", Version: version, ServerTime: time.Now(), Vehicles: sortedVehicles(cur), Feeds: s.health.Snapshot()})
}

func (s *Server) vehicle(w http.ResponseWriter, r *http.Request, id string) {
	v, ok := s.store.Get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "vehicle not found"})
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{"feeds": s.health.Snapshot(), "server_time": time.Now()}
	if s.staticStatus != nil {
		out["static"] = s.staticStatus()
	}
	writeJSON(w, http.StatusOK, out)
}

// mayStream reports whether the client behind an open stream may still see
// the live map: it is public, or the session or token it opened with still works.
func (s *Server) mayStream(r *http.Request) bool {
	if s.settings == nil || s.settingsNow().PublicMap {
		return true
	}
	if principal(r) == nil || s.auth == nil {
		return false
	}
	ctx, ip := r.Context(), meta(r).IP
	if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		p, err := s.auth.AuthenticateToken(ctx, strings.TrimSpace(bearer), ip)
		return err != nil || p != nil // a database hiccup is not a reason to cut the stream
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	p, err := s.auth.Authenticate(ctx, c.Value, ip)
	return err != nil || p != nil
}
