package api

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/unmaykr-a/transit-tracker/internal/dashboard"
)

// haDeparture is one departure as Home Assistant sensors expose it.
type haDeparture struct {
	Time       time.Time `json:"time"`
	Minutes    int       `json:"minutes"`
	DelayS     int       `json:"delay_s"`
	Realtime   bool      `json:"realtime"`
	Headsign   string    `json:"headsign"`
	Line       string    `json:"line"`
	Mode       string    `json:"mode"`
	Platform   string    `json:"platform"`
	Verdict    string    `json:"verdict"`
	LeaveInMin *int      `json:"leave_in_min"` // nil without an origin
}

// haLine is one saved line, flattened for the integration's sensors. The
// "next" departure is the first one still catchable from the origin (or
// simply the first without one), so leave_in_min counts down to 0 and goes
// to the following departure instead of turning negative.
type haLine struct {
	ID          int64         `json:"id"`
	Kind        string        `json:"kind"` // "line" or "trip"
	Label       string        `json:"label"`
	Line        string        `json:"line"`
	Mode        string        `json:"mode"`
	RouteID     string        `json:"route_id"`
	Headsign    string        `json:"headsign"`
	StopID      string        `json:"stop_id"`
	StopName    string        `json:"stop_name"`
	Platform    string        `json:"platform"`
	Path        string        `json:"path"` // web app page for the line, relative to the base URL
	Next        *haDeparture  `json:"next"`
	Departures  []haDeparture `json:"next_departures"`
	WalkMin     *int          `json:"walk_min"`
	WalkApprox  bool          `json:"walk_approx"`
	Verdict     string        `json:"verdict"`
	LeaveInMin  *int          `json:"leave_in_min"`
	Catchable   *bool         `json:"catchable"` // nil without an origin
	Unavailable bool          `json:"unavailable"`
	Lines       []string      `json:"lines,omitempty"`     // a trip's connecting lines
	WalkFrom    string        `json:"walk_from,omitempty"` // the line's own starting place, when walks start there
	Error       string        `json:"error,omitempty"`
}

func (s *Server) mountHA(r chi.Router) {
	r.Get("/ha/lines", s.handleHALines)
}

// handleHALines serves every saved line in one response, so Home Assistant
// polls once per interval rather than once per line. ?origin=lat,lon (or
// lat=&lon=, place=) adds walking time and verdicts; ?departures=1..5.
func (s *Server) handleHALines(w http.ResponseWriter, r *http.Request) {
	origin, err := s.origin(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	prefs, err := s.prefs(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	n := dashboard.Shown
	if v, err := strconv.Atoi(r.URL.Query().Get("departures")); err == nil {
		n = min(max(v, 1), 5)
	}
	lines, err := s.db.Q.ListSavedLines(r.Context(), principal(r).User.ID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	places, err := s.userPlaces(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	var entries []dashboard.Entry
	if s.dashboard != nil {
		// Extra departures so n remain after skipping ones that can no longer be caught.
		entries = s.dashboard.BuildN(r.Context(), lines, origin, places, prefs, n+10)
	}
	now := time.Now()
	out := make([]haLine, 0, len(entries))
	for _, e := range entries {
		out = append(out, haView(e, origin != nil || e.WalkFrom != "", n, now))
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": out, "has_origin": origin != nil, "server_time": now})
}

func haView(e dashboard.Entry, hasOrigin bool, n int, now time.Time) haLine {
	l := e.Line
	h := haLine{ID: l.ID, Kind: l.Kind, Label: l.Label, Line: l.ShortName, Mode: string(l.Mode), RouteID: l.RouteID, Headsign: l.Headsign,
		Departures: []haDeparture{}, Verdict: string(dashboard.LevelNone), Unavailable: l.Unavailable, Error: e.Error, WalkFrom: e.WalkFrom}
	if h.Label == "" {
		h.Label = l.ShortName
	}
	if l.Board != nil {
		h.StopID, h.StopName, h.Platform = l.Board.ID, l.Board.Name, l.Board.Platform
	}
	if l.ShortName != "" {
		h.Path = "/lines/" + url.PathEscape(string(l.Mode)) + "/" + url.PathEscape(l.ShortName)
		if l.Board != nil {
			h.Path += "?stop=" + url.QueryEscape(l.Board.ID)
		}
	}
	if e.Walk != nil {
		h.WalkMin, h.WalkApprox = &e.Walk.Minutes, e.Walk.Approx
	}
	for _, d := range e.Departures {
		hd := haDeparture{Time: d.ExpectedAt, Minutes: dashboard.MinutesUntil(d.ExpectedAt, now), DelayS: d.DelayS,
			Realtime: d.Realtime, Headsign: d.Headsign, Line: d.Line, Mode: string(d.Mode), Platform: d.Platform, Verdict: string(d.Verdict.Level)}
		if hasOrigin && e.Walk != nil {
			slack := d.Verdict.SlackMin
			hd.LeaveInMin = &slack
		}
		h.Departures = append(h.Departures, hd)
	}
	next := 0
	if hasOrigin && e.Walk != nil {
		catchable := e.NextCatchable >= 0
		h.Catchable = &catchable
		if catchable {
			next = e.NextCatchable
		}
	}
	if next >= 0 && next < len(h.Departures) {
		d := h.Departures[next]
		h.Next, h.Verdict, h.LeaveInMin = &d, d.Verdict, d.LeaveInMin
		// The list starts at the next departure: missed ones are no use on a wall display.
		h.Departures = h.Departures[next:]
	}
	if len(h.Departures) > n {
		h.Departures = h.Departures[:n]
	}
	if l.Kind == dashboard.KindTrip {
		tripView(&h, l)
	}
	return h
}

// tripView fills the line-shaped fields for a trip: the line, mode and
// platform are those of the next departure, whichever line it is.
func tripView(h *haLine, l dashboard.Line) {
	for _, tl := range l.Lines {
		if !slices.Contains(h.Lines, tl.ShortName) {
			h.Lines = append(h.Lines, tl.ShortName)
		}
	}
	if l.From != nil {
		h.StopID, h.StopName, h.Path = l.From.ID, l.From.Name, "/stops/"+url.PathEscape(l.From.ID)
	}
	if h.Label == "" && l.From != nil && l.To != nil {
		h.Label = l.From.Name + " → " + l.To.Name
	}
	h.Line, h.Mode, h.Platform = strings.Join(h.Lines, " · "), "", ""
	if len(l.Lines) > 0 {
		h.Mode = string(l.Lines[0].Mode)
	}
	if h.Next != nil {
		h.Line, h.Mode, h.Platform = h.Next.Line, h.Next.Mode, h.Next.Platform
	}
}
