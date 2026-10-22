package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/dashboard"
	"github.com/unmaykr-a/transit-tracker/internal/departures"
)

func TestAPITokensAndHALines(t *testing.T) {
	c, im := fullServer(t)
	ix := im.Index()
	var board string
	for _, p := range ix.Patterns("tram:T4") {
		if p.Direction == 0 {
			board = p.StopIDs[3]
		}
	}
	if code, out := c.do("POST", "/api/v1/me/lines", map[string]any{"route_id": "tram:T4", "direction": 0, "board_stop_id": board, "label": "Work"}); code != 201 {
		t.Fatalf("save line: %d %v", code, out)
	}

	if code, _ := c.do("POST", "/api/v1/me/tokens", map[string]string{"name": " "}); code != 400 {
		t.Errorf("blank token name = %d", code)
	}
	code, out := c.do("POST", "/api/v1/me/tokens", map[string]string{"name": "Home Assistant"})
	if code != 201 {
		t.Fatalf("create token: %d %v", code, out)
	}
	token := out["token"].(string)
	info := out["info"].(map[string]any)
	if !strings.HasPrefix(token, "bus_") || !strings.HasPrefix(token, info["prefix"].(string)) || info["last_used_at"] != nil {
		t.Errorf("token %q info %v", token, info)
	}

	// A bearer client without cookies.
	ha := &client{t: t, base: c.base, http: &http.Client{}}
	bearer := []string{"Authorization", "Bearer " + token}
	if code, me := ha.do("GET", "/api/v1/me", nil, bearer...); code != 200 || me["username"] != "andri" && me["user"] == nil {
		t.Fatalf("GET /me with token: %d %v", code, me)
	}
	st, _ := ix.Stop(board)
	q := "/api/v1/ha/lines?departures=4&origin=" + strconv.FormatFloat(st.Lat, 'f', 6, 64) + "," + strconv.FormatFloat(st.Lon, 'f', 6, 64)
	code, res := ha.do("GET", q, nil, bearer...)
	if code != 200 || res["has_origin"] != true {
		t.Fatalf("ha/lines: %d %v", code, res)
	}
	lines := res["lines"].([]any)
	if len(lines) != 1 {
		t.Fatalf("lines = %v", lines)
	}
	l := lines[0].(map[string]any)
	if l["label"] != "Work" || l["line"] != "T4" || l["mode"] != "tram" || l["stop_id"] != board || l["catchable"] != true ||
		l["next"] == nil || l["walk_min"] == nil || l["leave_in_min"] == nil || !strings.HasPrefix(l["path"].(string), "/lines/tram/T4?stop=") {
		t.Errorf("line = %v", l)
	}
	if deps := l["next_departures"].([]any); len(deps) != 4 {
		t.Errorf("next_departures = %d, want 4", len(deps))
	}
	if next := l["next"].(map[string]any); next["time"] != l["next_departures"].([]any)[0].(map[string]any)["time"] {
		t.Errorf("next %v is not the first listed departure", next)
	}
	// Without an origin there is no verdict.
	if _, res := ha.do("GET", "/api/v1/ha/lines", nil, bearer...); res["lines"].([]any)[0].(map[string]any)["catchable"] != nil {
		t.Errorf("catchable without origin: %v", res)
	}

	// Tokens are read-only, never admin, and cannot manage tokens or sessions.
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/v1/me/lines"}, {"GET", "/api/v1/admin/users"}, {"GET", "/api/v1/me/tokens"},
		{"GET", "/api/v1/me/sessions"}, {"POST", "/api/v1/me/tokens"},
	} {
		if code, _ := ha.do(tc.method, tc.path, map[string]any{}, bearer...); code != 403 {
			t.Errorf("%s %s with token = %d, want 403", tc.method, tc.path, code)
		}
	}
	if code, _ := ha.do("GET", "/api/v1/me", nil, "Authorization", "Bearer bus_nope"); code != 401 {
		t.Errorf("bad token = %d", code)
	}

	// Last use is recorded; the owner and the admin see the token.
	_, mine := c.do("GET", "/api/v1/me/tokens", nil)
	tok := mine["tokens"].([]any)[0].(map[string]any)
	if tok["last_used_at"] == nil || tok["name"] != "Home Assistant" {
		t.Errorf("my tokens = %v", mine)
	}
	_, all := c.do("GET", "/api/v1/admin/tokens", nil)
	if a := all["tokens"].([]any); len(a) != 1 || a[0].(map[string]any)["username"] != "andri" {
		t.Errorf("admin tokens = %v", all)
	}
	id := jsonNum(tok["id"])
	if code, _ := c.do("DELETE", "/api/v1/admin/tokens/"+id, nil); code != 204 {
		t.Errorf("admin revoke = %d", code)
	}
	if code, _ := ha.do("GET", "/api/v1/me", nil, bearer...); code != 401 {
		t.Errorf("revoked token = %d", code)
	}
	if code, _ := c.do("DELETE", "/api/v1/me/tokens/"+id, nil); code != 404 {
		t.Errorf("revoke twice = %d", code)
	}
	_, audit := c.do("GET", "/api/v1/admin/audit", nil)
	if !strings.Contains(jsonString(audit), "token.revoke") {
		t.Errorf("audit missing token.revoke: %v", audit)
	}
}

func TestHAViewSkipsMissedDepartures(t *testing.T) {
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	dep := func(min int, level dashboard.Level, slack int) dashboard.DepartureVerdict {
		return dashboard.DepartureVerdict{Departure: departures.Departure{ExpectedAt: now.Add(time.Duration(min) * time.Minute), Realtime: true},
			Verdict: dashboard.Verdict{Level: level, SlackMin: slack}}
	}
	e := dashboard.Entry{
		Line:          dashboard.Line{ID: 1, ShortName: "4", Mode: "tram"},
		Walk:          &dashboard.Walk{Minutes: 6},
		Departures:    []dashboard.DepartureVerdict{dep(2, dashboard.LevelRed, -5), dep(9, dashboard.LevelGreen, 2), dep(17, dashboard.LevelNeutral, 10), dep(25, dashboard.LevelNeutral, 18)},
		NextCatchable: 1,
	}
	h := haView(e, true, 2, now)
	if h.Next == nil || h.Next.Minutes != 9 || *h.LeaveInMin != 2 || h.Verdict != "green" || !*h.Catchable || len(h.Departures) != 2 || h.Departures[0].Minutes != 9 {
		t.Errorf("view = %+v", h)
	}
	if h.Label != "4" {
		t.Errorf("label falls back to the line: %q", h.Label)
	}
	e.NextCatchable = -1
	h = haView(e, true, 3, now)
	if h.Next == nil || h.Next.Minutes != 2 || *h.Catchable || *h.LeaveInMin != -5 || h.Verdict != "red" {
		t.Errorf("nothing catchable: %+v", h)
	}
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
