package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/state"
)

// Tartu city buses report through Ridango's realtime WebSocket: after a
// subscription for region 32 it pushes arrays of vehicle statuses every few
// seconds (sometimes only the vehicles that changed). TartuStream keeps one
// connection open in the background and Poll hands the latest set to the
// store, so the feed's health and backoff work like the polled feeds'.

// TartuRegion is Tartu's region in Ridango's realtime service.
const TartuRegion = 32

// tartuForget drops a vehicle not reported for this long (partial updates
// never say a bus left).
const tartuForget = 2 * time.Minute

// TartuStream follows Tartu's buses into the store (source "tartu").
type TartuStream struct {
	URL       func() string
	UserAgent func() string
	Client    *http.Client // nil: http.DefaultClient (honours the proxy environment)
	Store     *state.Store
	// Known reports whether a line (keyed "tartu-2") is in the static data.
	Known    func(line string) bool
	Annotate func([]state.Vehicle)
	Log      *slog.Logger
	Now      func() time.Time

	mu       sync.Mutex
	vehicles map[string]state.Vehicle
	lastMsg  time.Time
	lastErr  error
	running  string // URL of the running connection loop
	cancel   context.CancelFunc
}

type tartuStatus struct {
	DeviceID   string `json:"deviceId"`
	LocationID string `json:"locationId"`
	UpdateTime string `json:"updateTime"`
	Trip       *struct {
		TripID         string `json:"tripId"`
		RouteShortName string `json:"routeShortName"`
		StartTime      string `json:"startTime"`
	} `json:"trip"`
	Position *struct {
		Speed     float64 `json:"speed"`
		Bearing   float64 `json:"bearing"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"position"`
}

func (t *TartuStream) now() time.Time {
	if t.Now != nil {
		return t.Now()
	}
	return time.Now()
}

// Poll starts (or restarts, after a URL change) the connection and stores the
// vehicles seen recently.
func (t *TartuStream) Poll(ctx context.Context) (Result, error) {
	url := t.URL()
	t.mu.Lock()
	if t.running != url {
		if t.cancel != nil {
			t.cancel()
			t.cancel = nil
		}
		t.vehicles, t.lastMsg, t.lastErr = map[string]state.Vehicle{}, time.Time{}, nil
		if url != "" {
			cctx, cancel := context.WithCancel(context.Background())
			t.cancel = cancel
			go t.run(cctx, url)
		}
		t.running = url
	}
	now := t.now()
	if url == "" { // turned off: nothing to show, and nothing stale either
		t.mu.Unlock()
		t.Store.Replace("tartu", nil)
		return Result{DataAt: now}, nil
	}
	vs := make([]state.Vehicle, 0, len(t.vehicles))
	for id, v := range t.vehicles {
		if now.Sub(v.UpdatedAt) > tartuForget {
			delete(t.vehicles, id)
			continue
		}
		vs = append(vs, v)
	}
	last, lastErr := t.lastMsg, t.lastErr
	t.mu.Unlock()
	if last.IsZero() {
		if lastErr != nil {
			return Result{}, lastErr
		}
		return Result{Unchanged: true}, nil
	}
	if t.Annotate != nil {
		t.Annotate(vs)
	}
	t.Store.Replace("tartu", vs)
	return Result{Count: len(vs), DataAt: last}, nil
}

// Close stops the background connection.
func (t *TartuStream) Close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancel != nil {
		t.cancel()
		t.cancel = nil
	}
	t.running = ""
}

// run keeps a connection open, reconnecting with backoff, until ctx ends.
func (t *TartuStream) run(ctx context.Context, url string) {
	backoff := 2 * time.Second
	for ctx.Err() == nil {
		start := time.Now()
		err := t.session(ctx, url)
		if ctx.Err() != nil {
			return
		}
		t.mu.Lock()
		t.lastErr = err
		t.mu.Unlock()
		if t.Log != nil {
			t.Log.Debug("tartu stream closed", "err", err)
		}
		if time.Since(start) > time.Minute {
			backoff = 2 * time.Second // it worked for a while: retry soon
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, time.Minute)
	}
}

func (t *TartuStream) session(ctx context.Context, url string) error {
	h := http.Header{}
	if t.UserAgent != nil {
		h.Set("User-Agent", t.UserAgent())
	}
	dctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	c, _, err := websocket.Dial(dctx, url, &websocket.DialOptions{HTTPClient: t.Client, HTTPHeader: h})
	cancel()
	if err != nil {
		return fmt.Errorf("tartu: %w", err)
	}
	defer c.CloseNow()
	c.SetReadLimit(4 << 20)
	sub, _ := json.Marshal(map[string]any{
		"regionId":               TartuRegion,
		"topLeftCoordinates":     map[string]float64{"longitude": 0, "latitude": 180},
		"bottomRightCoordinates": map[string]float64{"longitude": 180, "latitude": -180},
	})
	if err := c.Write(ctx, websocket.MessageText, sub); err != nil {
		return fmt.Errorf("tartu: subscribe: %w", err)
	}
	for {
		// Updates come every few seconds; a silent minute is a dead connection.
		rctx, cancel := context.WithTimeout(ctx, time.Minute)
		_, b, err := c.Read(rctx)
		cancel()
		if err != nil {
			return fmt.Errorf("tartu: %w", err)
		}
		if len(b) == 0 || b[0] != '[' {
			continue // {"connectionEstablished":true,…}
		}
		var batch []tartuStatus
		if err := json.Unmarshal(b, &batch); err != nil {
			continue
		}
		t.apply(batch)
	}
}

// apply merges one batch of statuses.
func (t *TartuStream) apply(batch []tartuStatus) {
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, s := range batch {
		v, ok := TartuVehicle(s, now)
		if !ok || (t.Known != nil && !t.Known(v.Line)) {
			continue
		}
		t.vehicles[v.ID] = v
	}
	t.lastMsg, t.lastErr = now, nil
}

// TartuVehicle normalises one status: id bus-tartu-<plate>, line "tartu-<short>".
func TartuVehicle(s tartuStatus, now time.Time) (state.Vehicle, bool) {
	if s.Trip == nil || s.Position == nil || s.Trip.RouteShortName == "" || (s.Position.Latitude == 0 && s.Position.Longitude == 0) {
		return state.Vehicle{}, false
	}
	name := strings.TrimSpace(s.LocationID)
	if name == "" {
		name = strings.TrimSpace(s.DeviceID)
	}
	if name == "" {
		return state.Vehicle{}, false
	}
	at, err := time.Parse(time.RFC3339, s.UpdateTime)
	if err != nil || at.After(now.Add(time.Minute)) {
		at = now
	}
	trip := s.Trip.TripID
	if i := strings.LastIndexByte(trip, '|'); i >= 0 {
		trip = trip[i+1:] // the national timetable's trip id (after "|")
	}
	return state.Vehicle{
		ID:         "bus-tartu-" + name,
		Mode:       feeds.ModeBus,
		Line:       "tartu-" + s.Trip.RouteShortName,
		Label:      s.Trip.RouteShortName,
		Lat:        s.Position.Latitude,
		Lon:        s.Position.Longitude,
		Bearing:    s.Position.Bearing,
		SpeedMps:   s.Position.Speed,
		SpeedKnown: true,
		TripID:     trip,
		UpdatedAt:  at,
	}, true
}
