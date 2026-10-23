package api

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/state"
)

// Live clients mostly see the same thing: the whole map, or the same mode
// chips. Instead of every client copying the store, diffing and encoding on
// each update (which made 1,800 clients cost seconds of CPU per update), one
// liveFrame per store version holds the vehicles once, and one liveView per
// distinct filter holds its vehicles, its encoded snapshot and its encoded
// diff from the previous frame. A client whose last message was the previous
// frame's view gets those shared bytes; any other client (it missed an update,
// or just changed filters) gets a diff computed against what it last received,
// so slow clients still self-heal.

type liveFrame struct {
	version uint64
	at      time.Time
	all     map[string]state.Vehicle // shared and never modified
	prev    *liveFrame               // the frame before, for shared diffs (its own prev is cut)

	mu    sync.Mutex
	views map[string]*liveView
}

type liveView struct {
	once     sync.Once
	vehicles map[string]state.Vehicle // shared and never modified

	diffOnce sync.Once
	diff     []byte // nil when nothing changed for this view

	snapOnce sync.Once
	snap     []byte
}

type liveHub struct {
	mu  sync.Mutex
	cur *liveFrame
}

// frame returns the frame for the store's current version, building it once.
func (s *Server) frame() *liveFrame {
	h := &s.hub
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cur != nil && h.cur.version == s.store.Version() {
		return h.cur
	}
	version, all := s.store.Snapshot()
	f := &liveFrame{version: version, at: time.Now(), all: all, prev: h.cur, views: map[string]*liveView{}}
	if f.prev != nil {
		f.prev.mu.Lock()
		f.prev.prev = nil // keep one frame of history, not all of them
		f.prev.mu.Unlock()
	}
	h.cur = f
	return f
}

// filterKey names a filter canonically, so equal filters share a view.
func filterKey(f Filter) string {
	if len(f.Modes) == 0 && len(f.Lines) == 0 {
		return ""
	}
	modes := make([]string, len(f.Modes))
	for i, m := range f.Modes {
		modes[i] = string(m)
	}
	lines := make([]string, len(f.Lines))
	for i, l := range f.Lines {
		lines[i] = strings.ToLower(l)
	}
	slices.Sort(modes)
	slices.Sort(lines)
	return strings.Join(slices.Compact(modes), ",") + "|" + strings.Join(slices.Compact(lines), ",")
}

func (f *liveFrame) view(filter Filter) *liveView {
	key := filterKey(filter)
	f.mu.Lock()
	v, ok := f.views[key]
	if !ok {
		v = &liveView{}
		f.views[key] = v
	}
	f.mu.Unlock()
	v.once.Do(func() {
		if key == "" {
			v.vehicles = f.all
			return
		}
		v.vehicles = maps.Clone(f.all)
		maps.DeleteFunc(v.vehicles, func(_ string, x state.Vehicle) bool { return !filter.match(x) })
	})
	return v
}

// sharedDiff is the encoded diff from the previous frame's view of the same
// filter to this one (nil if nothing changed); ok is false unless the client's
// last message was that previous frame (from).
func (f *liveFrame) sharedDiff(filter Filter, from uint64) (b []byte, ok bool) {
	f.mu.Lock()
	prev := f.prev
	f.mu.Unlock()
	if prev == nil || prev.version != from {
		return nil, false
	}
	v := f.view(filter)
	v.diffOnce.Do(func() {
		if msg, changed := diffMessage(prev.view(filter).vehicles, v.vehicles, f.version, f.at); changed {
			v.diff, _ = json.Marshal(msg)
		}
	})
	return v.diff, true
}

// snapshot is the view's encoded snapshot; feeds are those of when it was first asked for.
func (f *liveFrame) snapshot(filter Filter, s *Server) []byte {
	v := f.view(filter)
	v.snapOnce.Do(func() {
		v.snap, _ = json.Marshal(liveMessage{Type: "snapshot", Version: f.version, ServerTime: f.at, Vehicles: sortedVehicles(v.vehicles), Feeds: s.health.Snapshot()})
	})
	return v.snap
}

// diffMessage turns what a client has (sent) into what it should have (cur).
func diffMessage(sent, cur map[string]state.Vehicle, version uint64, at time.Time) (liveMessage, bool) {
	msg := liveMessage{Type: "diff", Version: version, ServerTime: at}
	for id, v := range cur {
		old, ok := sent[id]
		switch {
		case ok && old.Equal(v):
		case ok && positionOnly(old, v):
			msg.Move = append(msg.Move, move{v})
		default:
			msg.Upsert = append(msg.Upsert, v)
		}
	}
	for id := range sent {
		if _, ok := cur[id]; !ok {
			msg.Remove = append(msg.Remove, id)
		}
	}
	if len(msg.Upsert) == 0 && len(msg.Move) == 0 && len(msg.Remove) == 0 {
		return msg, false
	}
	slices.SortFunc(msg.Upsert, func(a, b state.Vehicle) int { return strings.Compare(a.ID, b.ID) })
	slices.SortFunc(msg.Move, func(a, b move) int { return strings.Compare(a.v.ID, b.v.ID) })
	slices.Sort(msg.Remove)
	return msg, true
}
