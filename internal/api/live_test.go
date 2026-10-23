package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/coder/websocket"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/ingest"
	"github.com/unmaykr-a/transit-tracker/internal/state"
)

var at = time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)

func veh(id string, mode feeds.Mode, line string, lat float64) state.Vehicle {
	return state.Vehicle{ID: id, Mode: mode, Line: line, Label: line, Lat: lat, Lon: 24.7, Bearing: 90, UpdatedAt: at}
}

func liveServer(t *testing.T) (*httptest.Server, *state.Store) {
	t.Helper()
	store := state.NewStore()
	store.Replace("gps", []state.Vehicle{
		veh("tram-1", feeds.ModeTram, "T4", 59.40),
		veh("bus-1", feeds.ModeBus, "17", 59.41),
		veh("bus-2", feeds.ModeBus, "17A", 59.42),
	})
	srv := httptest.NewServer(NewServer(slog.New(slog.DiscardHandler), Deps{Store: store, Health: ingest.NewHealth()}).Handler(SPA(fstest.MapFS{})))
	t.Cleanup(srv.Close)
	return srv, store
}

// wireMessage mirrors liveMessage as clients decode it.
type wireMessage struct {
	liveMessage
	Move [][]any `json:"move"`
}

func (m wireMessage) moved() string {
	var out []string
	for _, mv := range m.Move {
		out = append(out, mv[0].(string))
	}
	return strings.Join(out, ",")
}

func readMsg(t *testing.T, ctx context.Context, c *websocket.Conn) wireMessage {
	t.Helper()
	_, b, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var m wireMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func ids(vs []state.Vehicle) string {
	var out []string
	for _, v := range vs {
		out = append(out, v.ID)
	}
	return strings.Join(out, ",")
}

func TestWebSocketSnapshotDiffFilter(t *testing.T) {
	srv, store := liveServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()

	m := readMsg(t, ctx, c)
	if m.Type != "snapshot" || ids(m.Vehicles) != "bus-1,bus-2,tram-1" {
		t.Fatalf("snapshot = %+v", m)
	}

	// Move one bus, re-route the other, drop the tram: the diff carries exactly that,
	// with the position-only change as a compact tuple.
	moved := veh("bus-1", feeds.ModeBus, "17", 59.411)
	moved.UpdatedAt = at.Add(10 * time.Second)
	rerouted := veh("bus-2", feeds.ModeBus, "17A", 59.42)
	rerouted.Destination = "Depoo"
	store.Replace("gps", []state.Vehicle{moved, rerouted})
	m = readMsg(t, ctx, c)
	if m.Type != "diff" || m.moved() != "bus-1" || ids(m.Upsert) != "bus-2" || strings.Join(m.Remove, ",") != "tram-1" {
		t.Fatalf("diff = %+v", m)
	}
	mv := m.Move[0]
	if len(mv) != 7 || mv[1].(float64) != 59.411 || mv[4].(float64) == 0 || int64(mv[5].(float64)) != moved.UpdatedAt.UnixMilli() {
		t.Errorf("move tuple = %v (speed must be derived)", mv)
	}

	// A filter message re-snapshots with only matching vehicles.
	if err := c.Write(ctx, websocket.MessageText, []byte(`{"filter":{"lines":["17a"]}}`)); err != nil {
		t.Fatal(err)
	}
	m = readMsg(t, ctx, c)
	if m.Type != "snapshot" || ids(m.Vehicles) != "bus-2" {
		t.Fatalf("filtered snapshot = %+v", m)
	}
	// Changes outside the filter produce no message; inside it, a diff.
	rerouted.Lat = 59.43
	store.Replace("gps", []state.Vehicle{moved, rerouted, veh("tram-9", feeds.ModeTram, "T1", 59.4)})
	m = readMsg(t, ctx, c)
	if m.Type != "diff" || m.moved() != "bus-2" || len(m.Upsert) != 0 || len(m.Remove) != 0 {
		t.Fatalf("filtered diff = %+v", m)
	}
}

func TestWebSocketRejectsCrossOrigin(t *testing.T) {
	srv, _ := liveServer(t)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/ws", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Origin", "https://evil.example")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin upgrade status = %d", res.StatusCode)
	}
}

func TestSSEStream(t *testing.T) {
	srv, store := liveServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/api/v1/vehicles/stream?mode=tram", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type %q", ct)
	}
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(nil, 1<<20)
	next := func() (string, wireMessage) {
		var event string
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				var m wireMessage
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &m); err != nil {
					t.Fatal(err)
				}
				return event, m
			}
		}
		t.Fatalf("stream ended: %v", sc.Err())
		return "", wireMessage{}
	}
	if ev, m := next(); ev != "snapshot" || ids(m.Vehicles) != "tram-1" {
		t.Fatalf("got %s %+v", ev, m)
	}
	store.Replace("gps", []state.Vehicle{veh("tram-1", feeds.ModeTram, "T4", 59.401)})
	if ev, m := next(); ev != "diff" || len(m.Move) != 1 {
		t.Fatalf("got %s %+v", ev, m)
	}
}

func TestVehiclesREST(t *testing.T) {
	srv, _ := liveServer(t)
	for _, tt := range []struct{ query, want string }{
		{"", "bus-1,bus-2,tram-1"},
		{"?mode=bus", "bus-1,bus-2"},
		{"?mode=BUS,tram&line=17,T4", "bus-1,tram-1"},
		{"?line=99", ""},
	} {
		res, body := get(t, srv.URL+"/api/v1/vehicles"+tt.query)
		var m liveMessage
		if err := json.Unmarshal([]byte(body), &m); err != nil || res.StatusCode != 200 {
			t.Fatalf("%s: %d %v", tt.query, res.StatusCode, err)
		}
		if got := ids(m.Vehicles); got != tt.want {
			t.Errorf("%s → %q, want %q", tt.query, got, tt.want)
		}
	}
	if res, body := get(t, srv.URL+"/api/v1/vehicles/bus-2"); res.StatusCode != 200 || !strings.Contains(body, `"line":"17A"`) {
		t.Errorf("single vehicle: %d %s", res.StatusCode, body)
	}
	if res, _ := get(t, srv.URL+"/api/v1/vehicles/nope"); res.StatusCode != 404 {
		t.Errorf("missing vehicle status %d", res.StatusCode)
	}
	if res, body := get(t, srv.URL+"/api/v1/status"); res.StatusCode != 200 || !strings.Contains(body, `"feeds"`) {
		t.Errorf("status: %d %s", res.StatusCode, body)
	}
}

// Open streams end when their access goes: logging out, or the map stops being public.
func TestStreamsEndWhenAccessGoes(t *testing.T) {
	old := streamRecheck
	streamRecheck = 50 * time.Millisecond
	t.Cleanup(func() { streamRecheck = old })
	srv, newClient, _ := accountServer(t)
	admin := newClient()
	admin.do("POST", "/api/v1/auth/setup", map[string]string{"username": "andri", "password": "hunter2hunter2"})

	open := func(c *client) (done chan struct{}) {
		t.Helper()
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/vehicles/stream", nil)
		res, err := c.http.Do(req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("stream: %v %v", err, res)
		}
		done = make(chan struct{})
		go func() {
			defer close(done)
			defer res.Body.Close()
			_, _ = io.Copy(io.Discard, res.Body) // returns when the server ends the stream
		}()
		return done
	}
	ended := func(done chan struct{}, what string) {
		t.Helper()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatalf("stream still open after %s", what)
		}
	}

	anon := newClient()
	anonStream := open(anon)
	if code, _ := admin.do("PATCH", "/api/v1/admin/settings", map[string]any{"public_map": false}); code != 200 {
		t.Fatal("settings")
	}
	ended(anonStream, "the map stopped being public")

	adminStream := open(admin)
	select {
	case <-adminStream:
		t.Fatal("a signed-in stream must stay open")
	case <-time.After(200 * time.Millisecond):
	}
	admin.do("POST", "/api/v1/auth/logout", nil)
	ended(adminStream, "logout")
}

// Feed errors can name upstream hosts or internal addresses: only admins see them.
func TestPublicStatusHidesFeedErrors(t *testing.T) {
	health := ingest.NewHealth()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go health.Run(ctx, slog.New(slog.DiscardHandler), ingest.Feed{
		Name: "gps", Interval: func() time.Duration { return time.Hour },
		Poll: func(context.Context) (ingest.Result, error) {
			return ingest.Result{}, errors.New("dial tcp 10.1.2.3:443: connect: connection refused")
		},
	})
	deadline := time.Now().Add(5 * time.Second)
	for fs := health.Snapshot(); len(fs) == 0 || fs[0].LastError == ""; fs = health.Snapshot() {
		if time.Now().After(deadline) {
			t.Fatal("the failing poll never ran")
		}
		time.Sleep(10 * time.Millisecond)
	}
	srv := httptest.NewServer(NewServer(slog.New(slog.DiscardHandler), Deps{Store: state.NewStore(), Health: health}).Handler(SPA(fstest.MapFS{})))
	defer srv.Close()
	for _, path := range []string{"/api/v1/status", "/api/v1/vehicles"} {
		res, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if strings.Contains(string(b), "10.1.2.3") || strings.Contains(string(b), "last_error") {
			t.Errorf("%s leaks the feed error: %s", path, b)
		}
		if !strings.Contains(string(b), `"stale":true`) {
			t.Errorf("%s no longer says the feed is stale: %s", path, b)
		}
	}
}
