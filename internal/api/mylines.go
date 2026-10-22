package api

import (
	"database/sql"
	"errors"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/unmaykr-a/transit-tracker/internal/auth"
	"github.com/unmaykr-a/transit-tracker/internal/dashboard"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
	"github.com/unmaykr-a/transit-tracker/internal/store/db"
	"github.com/unmaykr-a/transit-tracker/internal/walk"
)

// MaxSavedLines keeps dashboards (and the SIRI pre-warm) bounded per user.
const MaxSavedLines = 20

func idParam(r *http.Request) int64 {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return id
}

func invalid(field, msg string) error { return &auth.ValidationError{Field: field, Msg: msg} }

// validateLine checks that board (and alight, if set) are on the route in that
// direction, alight after board, and fills in the direction's headsign.
func validateLine(ix *staticdata.Index, routeID string, direction int, board, alight string) (string, error) {
	if _, ok := ix.Route(routeID); !ok {
		return "", invalid("route_id", "unknown line")
	}
	headsign := ""
	for _, p := range ix.Patterns(routeID) {
		if p.Direction != direction {
			continue
		}
		if headsign == "" {
			headsign = p.Headsign // main pattern of the direction comes first
		}
		bi := slices.Index(p.StopIDs, board)
		if bi < 0 || bi == len(p.StopIDs)-1 {
			continue
		}
		if alight == "" || slices.Index(p.StopIDs[bi+1:], alight) >= 0 {
			return headsign, nil
		}
	}
	if headsign == "" {
		return "", invalid("direction", "the line has no such direction")
	}
	if alight != "" {
		return "", invalid("alight_stop_id", "must come after the boarding stop in this direction")
	}
	return "", invalid("board_stop_id", "the line does not depart from this stop in this direction")
}

// validateTrip checks that both stop areas exist and some line connects them.
func validateTrip(ix *staticdata.Index, from, to string) error {
	if _, ok := ix.Area(from); !ok {
		return invalid("from_area_id", "unknown stop")
	}
	if _, ok := ix.Area(to); !ok {
		return invalid("to_area_id", "unknown stop")
	}
	if from == to {
		return invalid("to_area_id", "choose a different stop")
	}
	if len(ix.Connections(from, to)) == 0 {
		return invalid("to_area_id", "no line goes from the first stop to this one without changing")
	}
	return nil
}

// handleTripPreview lists the lines that connect two stop areas, for the trip form.
func (s *Server) handleTripPreview(w http.ResponseWriter, r *http.Request) {
	from, to := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	ix := s.index()
	if err := validateTrip(ix, from, to); err != nil {
		var ve *auth.ValidationError
		if errors.As(err, &ve) && ve.Field == "to_area_id" && from != to {
			if _, ok := ix.Area(to); ok {
				writeJSON(w, http.StatusOK, map[string]any{"lines": []dashboard.TripLine{}}) // valid stops, nothing direct
				return
			}
		}
		s.writeErr(w, err)
		return
	}
	trip := dashboard.ResolveLine(ix, db.SavedLine{Kind: dashboard.KindTrip, FromAreaID: from, ToAreaID: to})
	writeJSON(w, http.StatusOK, map[string]any{"from": trip.From, "to": trip.To, "lines": trip.Lines})
}

func (s *Server) handleListLines(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Q.ListSavedLines(r.Context(), principal(r).User.ID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	ix := s.index()
	out := make([]dashboard.Line, 0, len(rows))
	for _, sl := range rows {
		out = append(out, dashboard.ResolveLine(ix, sl))
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": out})
}

func (s *Server) handleCreateLine(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind         string `json:"kind"` // "line" (default) or "trip"
		RouteID      string `json:"route_id"`
		Direction    int    `json:"direction"`
		BoardStopID  string `json:"board_stop_id"`
		AlightStopID string `json:"alight_stop_id"`
		FromAreaID   string `json:"from_area_id"`
		ToAreaID     string `json:"to_area_id"`
		Label        string `json:"label"`
	}
	if err := decodeJSON(r, &in); err != nil {
		s.writeErr(w, err)
		return
	}
	p := principal(r)
	if n, err := s.db.Q.CountSavedLines(r.Context(), p.User.ID); err != nil {
		s.writeErr(w, err)
		return
	} else if n >= MaxSavedLines {
		s.writeErr(w, invalid("route_id", "you can save up to 20 lines"))
		return
	}
	if in.Kind == dashboard.KindTrip {
		if err := validateTrip(s.index(), in.FromAreaID, in.ToAreaID); err != nil {
			s.writeErr(w, err)
			return
		}
		sl, err := s.db.Q.CreateSavedTrip(r.Context(), db.CreateSavedTripParams{
			UserID: p.User.ID, FromAreaID: in.FromAreaID, ToAreaID: in.ToAreaID, Label: strings.TrimSpace(in.Label), CreatedAt: time.Now().Unix(),
		})
		if err != nil {
			s.writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"line": dashboard.ResolveLine(s.index(), sl)})
		return
	} else if in.Kind != "" && in.Kind != dashboard.KindLine {
		s.writeErr(w, invalid("kind", "must be line or trip"))
		return
	}
	headsign, err := validateLine(s.index(), in.RouteID, in.Direction, in.BoardStopID, in.AlightStopID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	sl, err := s.db.Q.CreateSavedLine(r.Context(), db.CreateSavedLineParams{
		UserID: p.User.ID, RouteID: in.RouteID, Direction: int64(in.Direction), Headsign: headsign,
		BoardStopID: in.BoardStopID, AlightStopID: in.AlightStopID, Label: strings.TrimSpace(in.Label), CreatedAt: time.Now().Unix(),
	})
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"line": dashboard.ResolveLine(s.index(), sl)})
}

func (s *Server) savedLine(w http.ResponseWriter, r *http.Request) (db.SavedLine, bool) {
	sl, err := s.db.Q.GetSavedLine(r.Context(), db.GetSavedLineParams{ID: idParam(r), UserID: principal(r).User.ID})
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "saved line not found", "")
		return sl, false
	} else if err != nil {
		s.writeErr(w, err)
		return sl, false
	}
	return sl, true
}

func (s *Server) handleUpdateLine(w http.ResponseWriter, r *http.Request) {
	sl, ok := s.savedLine(w, r)
	if !ok {
		return
	}
	var in struct {
		Label        *string `json:"label"`
		AlightStopID *string `json:"alight_stop_id"`
		NotifyMin    *int    `json:"notify_threshold_min"`
	}
	if err := decodeJSON(r, &in); err != nil {
		s.writeErr(w, err)
		return
	}
	if in.Label != nil {
		sl.Label = strings.TrimSpace(*in.Label)
	}
	if in.AlightStopID != nil && sl.Kind == dashboard.KindTrip {
		s.writeErr(w, invalid("alight_stop_id", "a trip already ends at its destination"))
		return
	}
	if in.AlightStopID != nil {
		if _, err := validateLine(s.index(), sl.RouteID, int(sl.Direction), sl.BoardStopID, *in.AlightStopID); err != nil {
			s.writeErr(w, err)
			return
		}
		sl.AlightStopID = *in.AlightStopID
	}
	if in.NotifyMin != nil {
		if *in.NotifyMin < 0 || *in.NotifyMin > 60 {
			s.writeErr(w, invalid("notify_threshold_min", "must be 0–60"))
			return
		}
		sl.NotifyThresholdMin = int64(*in.NotifyMin)
	}
	if err := s.db.Q.UpdateSavedLine(r.Context(), db.UpdateSavedLineParams{Label: sl.Label, AlightStopID: sl.AlightStopID, NotifyThresholdMin: sl.NotifyThresholdMin, ID: sl.ID, UserID: sl.UserID}); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"line": dashboard.ResolveLine(s.index(), sl)})
}

func (s *Server) handleDeleteLine(w http.ResponseWriter, r *http.Request) {
	n, err := s.db.Q.DeleteSavedLine(r.Context(), db.DeleteSavedLineParams{ID: idParam(r), UserID: principal(r).User.ID})
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "saved line not found", "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleReorderLines(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := decodeJSON(r, &in); err != nil {
		s.writeErr(w, err)
		return
	}
	uid := principal(r).User.ID
	err := s.db.Tx(r.Context(), func(q *db.Queries) error {
		rows, err := q.ListSavedLines(r.Context(), uid)
		if err != nil {
			return err
		}
		if len(rows) != len(in.IDs) {
			return invalid("ids", "must list every saved line once")
		}
		for _, sl := range rows {
			if !slices.Contains(in.IDs, sl.ID) {
				return invalid("ids", "must list every saved line once")
			}
		}
		for i, id := range in.IDs {
			if err := q.SetSavedLineSort(r.Context(), db.SetSavedLineSortParams{Sort: int64(i), ID: id, UserID: uid}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.handleListLines(w, r)
}

type placeJSON struct {
	ID    int64   `json:"id"`
	Label string  `json:"label"`
	Lat   float64 `json:"lat"`
	Lon   float64 `json:"lon"`
}

func validatePlace(label string, lat, lon float64) error {
	if l := strings.TrimSpace(label); l == "" || len(l) > 40 {
		return invalid("label", "must be 1–40 characters")
	}
	if math.IsNaN(lat) || lat < -90 || lat > 90 || math.IsNaN(lon) || lon < -180 || lon > 180 || (lat == 0 && lon == 0) {
		return invalid("lat", "invalid coordinates")
	}
	return nil
}

func (s *Server) handleListPlaces(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Q.ListSavedPlaces(r.Context(), principal(r).User.ID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]placeJSON, 0, len(rows))
	for _, p := range rows {
		out = append(out, placeJSON{p.ID, p.Label, p.Lat, p.Lon})
	}
	writeJSON(w, http.StatusOK, map[string]any{"places": out})
}

func (s *Server) handleCreatePlace(w http.ResponseWriter, r *http.Request) {
	var in placeJSON
	if err := decodeJSON(r, &in); err != nil {
		s.writeErr(w, err)
		return
	}
	if err := validatePlace(in.Label, in.Lat, in.Lon); err != nil {
		s.writeErr(w, err)
		return
	}
	p, err := s.db.Q.CreateSavedPlace(r.Context(), db.CreateSavedPlaceParams{UserID: principal(r).User.ID, Label: strings.TrimSpace(in.Label), Lat: in.Lat, Lon: in.Lon})
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"place": placeJSON{p.ID, p.Label, p.Lat, p.Lon}})
}

func (s *Server) handleUpdatePlace(w http.ResponseWriter, r *http.Request) {
	var in placeJSON
	if err := decodeJSON(r, &in); err != nil {
		s.writeErr(w, err)
		return
	}
	if err := validatePlace(in.Label, in.Lat, in.Lon); err != nil {
		s.writeErr(w, err)
		return
	}
	uid := principal(r).User.ID
	if _, err := s.db.Q.GetSavedPlace(r.Context(), db.GetSavedPlaceParams{ID: idParam(r), UserID: uid}); errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "place not found", "")
		return
	}
	if err := s.db.Q.UpdateSavedPlace(r.Context(), db.UpdateSavedPlaceParams{Label: strings.TrimSpace(in.Label), Lat: in.Lat, Lon: in.Lon, ID: idParam(r), UserID: uid}); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"place": placeJSON{idParam(r), strings.TrimSpace(in.Label), in.Lat, in.Lon}})
}

func (s *Server) handleDeletePlace(w http.ResponseWriter, r *http.Request) {
	n, err := s.db.Q.DeleteSavedPlace(r.Context(), db.DeleteSavedPlaceParams{ID: idParam(r), UserID: principal(r).User.ID})
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "place not found", "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) prefs(r *http.Request) (dashboard.Prefs, error) {
	p, err := s.db.Q.GetUserPrefs(r.Context(), principal(r).User.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return dashboard.DefaultPrefs, nil
	}
	return dashboard.Prefs{WalkSpeedKmh: p.WalkSpeedKmh, BufferMin: int(p.BufferMin)}, err
}

func (s *Server) handleGetPrefs(w http.ResponseWriter, r *http.Request) {
	p, err := s.prefs(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleUpdatePrefs(w http.ResponseWriter, r *http.Request) {
	cur, err := s.prefs(r)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	var in struct {
		WalkSpeedKmh *float64 `json:"walk_speed_kmh"`
		BufferMin    *int     `json:"buffer_min"`
	}
	if err := decodeJSON(r, &in); err != nil {
		s.writeErr(w, err)
		return
	}
	if in.WalkSpeedKmh != nil {
		if *in.WalkSpeedKmh < 2 || *in.WalkSpeedKmh > 9 {
			s.writeErr(w, invalid("walk_speed_kmh", "must be 2–9 km/h"))
			return
		}
		cur.WalkSpeedKmh = *in.WalkSpeedKmh
	}
	if in.BufferMin != nil {
		if *in.BufferMin < 0 || *in.BufferMin > 15 {
			s.writeErr(w, invalid("buffer_min", "must be 0–15 minutes"))
			return
		}
		cur.BufferMin = *in.BufferMin
	}
	if err := s.db.Q.UpsertUserPrefs(r.Context(), db.UpsertUserPrefsParams{UserID: principal(r).User.ID, WalkSpeedKmh: cur.WalkSpeedKmh, BufferMin: int64(cur.BufferMin)}); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cur)
}

// origin reads ?lat=&lon= (browser location, never stored) or ?place=<id>.
func (s *Server) origin(r *http.Request) (*walk.Point, error) {
	q := r.URL.Query()
	if place := q.Get("place"); place != "" {
		id, _ := strconv.ParseInt(place, 10, 64)
		p, err := s.db.Q.GetSavedPlace(r.Context(), db.GetSavedPlaceParams{ID: id, UserID: principal(r).User.ID})
		if errors.Is(err, sql.ErrNoRows) {
			return nil, invalid("place", "unknown place")
		}
		if err != nil {
			return nil, err
		}
		return &walk.Point{Lat: p.Lat, Lon: p.Lon}, nil
	}
	latS, lonS := q.Get("lat"), q.Get("lon")
	if o := q.Get("origin"); o != "" { // "lat,lon", as Home Assistant sends it
		latS, lonS, _ = strings.Cut(o, ",")
	}
	if latS == "" && lonS == "" {
		return nil, nil
	}
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(latS), 64)
	lon, err2 := strconv.ParseFloat(strings.TrimSpace(lonS), 64)
	if err1 != nil || err2 != nil || validatePlace("x", lat, lon) != nil {
		return nil, invalid("lat", "invalid coordinates")
	}
	return &walk.Point{Lat: lat, Lon: lon}, nil
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
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
	lines, err := s.db.Q.ListSavedLines(r.Context(), principal(r).User.ID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	entries := []dashboard.Entry{}
	if s.dashboard != nil {
		entries = s.dashboard.Build(r.Context(), lines, origin, prefs)
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries, "prefs": prefs, "has_origin": origin != nil, "server_time": time.Now()})
}

func (s *Server) mountMyLines(r chi.Router) {
	r.Get("/me/lines", s.handleListLines)
	r.Post("/me/lines", s.handleCreateLine)
	r.Post("/me/lines/reorder", s.handleReorderLines)
	r.Patch("/me/lines/{id}", s.handleUpdateLine)
	r.Delete("/me/lines/{id}", s.handleDeleteLine)
	r.Get("/me/places", s.handleListPlaces)
	r.Post("/me/places", s.handleCreatePlace)
	r.Patch("/me/places/{id}", s.handleUpdatePlace)
	r.Delete("/me/places/{id}", s.handleDeletePlace)
	r.Get("/me/prefs", s.handleGetPrefs)
	r.Patch("/me/prefs", s.handleUpdatePrefs)
	r.Get("/me/dashboard", s.handleDashboard)
}
