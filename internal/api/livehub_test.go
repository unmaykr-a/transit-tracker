package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/ingest"
	"github.com/unmaykr-a/transit-tracker/internal/state"
)

func TestFramesShareDiffsPerFilter(t *testing.T) {
	store := state.NewStore()
	store.Replace("gps", []state.Vehicle{veh("tram-1", feeds.ModeTram, "T4", 59.40), veh("bus-1", feeds.ModeBus, "17", 59.41)})
	s := NewServer(slog.New(slog.DiscardHandler), Deps{Store: store, Health: ingest.NewHealth()})
	f1 := s.frame()
	if s.frame() != f1 {
		t.Fatal("same version should reuse the frame")
	}
	trams := Filter{Modes: []feeds.Mode{feeds.ModeTram}}
	if len(f1.view(trams).vehicles) != 1 || len(f1.view(Filter{}).vehicles) != 2 {
		t.Fatalf("views: %v %v", f1.view(trams).vehicles, f1.view(Filter{}).vehicles)
	}
	if f1.view(Filter{Lines: []string{"t4"}, Modes: []feeds.Mode{"tram", "tram"}}) != f1.view(Filter{Modes: []feeds.Mode{"tram"}, Lines: []string{"T4"}}) {
		t.Error("equal filters should share a view")
	}

	moved := veh("tram-1", feeds.ModeTram, "T4", 59.405)
	store.Replace("gps", []state.Vehicle{moved, veh("bus-1", feeds.ModeBus, "17", 59.41)})
	f2 := s.frame()
	a, ok := f2.sharedDiff(Filter{}, f1.version)
	b, _ := f2.sharedDiff(Filter{}, f1.version)
	if !ok || &a[0] != &b[0] {
		t.Fatal("clients that kept up should get the same encoded diff")
	}
	var m wireMessage
	if err := json.Unmarshal(a, &m); err != nil || m.moved() != "tram-1" || len(m.Upsert)+len(m.Remove) != 0 {
		t.Fatalf("diff = %s", a)
	}
	// Buses did not change: nothing for a bus-only client.
	if d, ok := f2.sharedDiff(Filter{Modes: []feeds.Mode{feeds.ModeBus}}, f1.version); !ok || d != nil {
		t.Errorf("bus diff = %s", d)
	}
	// A client that missed a frame gets no shared diff (it computes its own).
	store.Replace("gps", []state.Vehicle{moved})
	f3 := s.frame()
	if _, ok := f3.sharedDiff(Filter{}, f1.version); ok {
		t.Error("a diff from two frames back must not be shared")
	}
	if f2.prev != nil {
		t.Error("frames keep only one step of history")
	}
}

// Many clients on one server all end up with the same state, whether they
// kept up (shared diffs) or joined late (snapshot) or filter.
func TestManyLiveClientsConverge(t *testing.T) {
	srv, store := liveServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var conns []*websocket.Conn
	for range 20 {
		c, _, err := websocket.Dial(ctx, "ws"+srv.URL[4:]+"/ws", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer c.CloseNow()
		if m := readMsg(t, ctx, c); m.Type != "snapshot" || len(m.Vehicles) != 3 {
			t.Fatalf("snapshot: %+v", m)
		}
		conns = append(conns, c)
	}
	store.Replace("gps", []state.Vehicle{veh("tram-1", feeds.ModeTram, "T4", 59.43), veh("bus-1", feeds.ModeBus, "17", 59.41)})
	var first []byte
	for i, c := range conns {
		_, b, err := c.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = b
		} else if !bytes.Equal(b, first) {
			t.Fatalf("client %d got %s, client 0 got %s", i, b, first)
		}
	}
	var m wireMessage
	_ = json.Unmarshal(first, &m)
	if m.moved() != "tram-1" || len(m.Remove) != 1 || m.Remove[0] != "bus-2" {
		t.Errorf("diff = %s", first)
	}
}
