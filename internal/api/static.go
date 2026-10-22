package api

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/unmaykr-a/transit-tracker/internal/departures"
	"github.com/unmaykr-a/transit-tracker/internal/eta"
	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/geo"
	"github.com/unmaykr-a/transit-tracker/internal/state"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
)

func (s *Server) index() *staticdata.Index {
	if s.static == nil {
		return staticdata.NewIndex(staticdata.Dataset{})
	}
	return s.static()
}

type areaJSON struct {
	*staticdata.Area
	Lines []lineChip `json:"lines"`
}

type lineChip struct {
	RouteID   string     `json:"route_id"`
	Mode      feeds.Mode `json:"mode"`
	ShortName string     `json:"short_name"`
}

// areaLines lists the distinct routes calling anywhere in an area, in route order.
func areaLines(ix *staticdata.Index, a *staticdata.Area) []lineChip {
	seen := map[string]bool{}
	var out []lineChip
	for _, sid := range a.StopIDs {
		for _, l := range ix.Lines(sid) {
			if !seen[l.RouteID] {
				seen[l.RouteID] = true
				out = append(out, lineChip{l.RouteID, l.Mode, l.ShortName})
			}
		}
	}
	slices.SortStableFunc(out, func(x, y lineChip) int {
		rx, _ := ix.Route(x.RouteID)
		ry, _ := ix.Route(y.RouteID)
		return rx.Sort - ry.Sort
	})
	if out == nil {
		out = []lineChip{}
	}
	return out
}

func limitParam(r *http.Request, def, maxN int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 {
		return def
	}
	return min(n, maxN)
}

func (s *Server) searchStops(r *http.Request, limit int) []areaJSON {
	ix := s.index()
	res := ix.SearchAreas(r.URL.Query().Get("q"), limit)
	out := make([]areaJSON, 0, len(res))
	for _, h := range res {
		out = append(out, areaJSON{h.Area, areaLines(ix, h.Area)})
	}
	return out
}

func (s *Server) searchLines(r *http.Request, limit int) []*staticdata.Route {
	out := s.index().SearchRoutes(r.URL.Query().Get("q"), limit)
	if out == nil {
		out = []*staticdata.Route{}
	}
	return out
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	limit := limitParam(r, 8, 50)
	writeJSON(w, http.StatusOK, map[string]any{"lines": s.searchLines(r, limit), "stops": s.searchStops(r, limit)})
}

func (s *Server) handleStopSearch(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"stops": s.searchStops(r, limitParam(r, 20, 100))})
}

func (s *Server) handleLineSearch(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"lines": s.searchLines(r, limitParam(r, 20, 100))})
}

// handleAllStops serves every platform compactly for the map's stop layer. It
// only changes on import, so clients may cache it.
func (s *Server) handleAllStops(w http.ResponseWriter, r *http.Request) {
	ix := s.index()
	etag := `"` + strconv.FormatInt(ix.ImportedAt.Unix(), 36) + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	type stopJSON struct {
		ID       string       `json:"id"`
		AreaID   string       `json:"area_id"`
		Name     string       `json:"name"`
		Platform string       `json:"platform"`
		Lat      float64      `json:"lat"`
		Lon      float64      `json:"lon"`
		Modes    []feeds.Mode `json:"modes"`
	}
	stops := ix.Stops()
	out := make([]stopJSON, 0, len(stops))
	for _, st := range stops {
		out = append(out, stopJSON{st.ID, st.AreaID, st.Name, st.Platform, st.Lat, st.Lon, st.Modes})
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	writeJSONBody(w, http.StatusOK, map[string]any{"imported_at": ix.ImportedAt, "stops": out})
}

type platformJSON struct {
	*staticdata.Stop
	Lines []staticdata.LineRef `json:"lines"`
}

// resolveArea accepts an area id ("a1298") or a platform id ("1298").
func resolveArea(ix *staticdata.Index, id string) (*staticdata.Area, string, bool) {
	if a, ok := ix.Area(id); ok {
		return a, "", true
	}
	if st, ok := ix.Stop(id); ok {
		a, ok := ix.Area(st.AreaID)
		return a, st.ID, ok
	}
	return nil, "", false
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	ix := s.index()
	a, focus, ok := resolveArea(ix, chi.URLParam(r, "id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "stop not found"})
		return
	}
	platforms := make([]platformJSON, 0, len(a.StopIDs))
	for _, sid := range a.StopIDs {
		st, _ := ix.Stop(sid)
		lines := ix.Lines(sid)
		if lines == nil {
			lines = []staticdata.LineRef{}
		}
		platforms = append(platforms, platformJSON{st, lines})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": a.ID, "name": a.Name, "lat": a.Lat, "lon": a.Lon, "modes": a.Modes,
		"lines": areaLines(ix, a), "platforms": platforms, "focus": focus,
	})
}

func (s *Server) handleStopDepartures(w http.ResponseWriter, r *http.Request) {
	ix := s.index()
	a, focus, ok := resolveArea(ix, chi.URLParam(r, "id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "stop not found"})
		return
	}
	if p := r.URL.Query().Get("platform"); p != "" {
		focus = p
	}
	var stops []*staticdata.Stop
	names := []string{a.Name}
	for _, sid := range a.StopIDs {
		st, _ := ix.Stop(sid)
		names = append(names, st.Name)
		if focus == "" || sid == focus {
			stops = append(stops, st)
		}
	}
	if s.departures == nil || len(stops) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"departures": []departures.Departure{}, "server_time": time.Now()})
		return
	}
	// Ask for extra rows: arrivals of terminating lines are dropped below.
	limit := limitParam(r, 30, 100)
	deps, err := s.departures.ForStops(r.Context(), stops, limit*2, 3*time.Hour)
	deps = departures.DropArrivals(deps, names)
	if len(deps) > limit {
		deps = deps[:limit]
	}
	if err != nil {
		s.log.Warn("departures", "area", a.ID, "err", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "departures unavailable"})
		return
	}
	if deps == nil {
		deps = []departures.Departure{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"departures": deps, "server_time": time.Now()})
}

type patternStopJSON struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Platform string  `json:"platform"`
	AreaID   string  `json:"area_id"`
	Lat      float64 `json:"lat"`
	Lon      float64 `json:"lon"`
}

type patternJSON struct {
	ID        string            `json:"id"`
	Direction int               `json:"direction"`
	Headsign  string            `json:"headsign"`
	Main      bool              `json:"main"` // busiest pattern of its direction
	Stops     []patternStopJSON `json:"stops"`
	Shape     [][2]float64      `json:"shape"`
}

func (s *Server) handleLine(w http.ResponseWriter, r *http.Request) {
	ix := s.index()
	mode := feeds.Mode(strings.ToLower(chi.URLParam(r, "mode")))
	route, ok := ix.Route(staticdata.RouteID(mode, chi.URLParam(r, "line")))
	if !ok {
		// Accept "4" for tram T4.
		if mode == feeds.ModeTram {
			route, ok = ix.Route(staticdata.RouteID(mode, "T"+chi.URLParam(r, "line")))
		}
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "line not found"})
			return
		}
	}
	seenDir := map[int]bool{}
	var pats []patternJSON
	for _, p := range ix.Patterns(route.ID) {
		pj := patternJSON{ID: p.ID, Direction: p.Direction, Headsign: p.Headsign, Main: !seenDir[p.Direction], Shape: p.Shape}
		seenDir[p.Direction] = true
		for _, sid := range p.StopIDs {
			if st, ok := ix.Stop(sid); ok {
				pj.Stops = append(pj.Stops, patternStopJSON{st.ID, st.Name, st.Platform, st.AreaID, st.Lat, st.Lon})
			}
		}
		pats = append(pats, pj)
	}
	if pats == nil {
		pats = []patternJSON{}
	}
	_, all := s.store.Snapshot()
	vehicles := []state.Vehicle{}
	for _, v := range sortedVehicles(all) {
		if v.Mode == route.Mode && strings.EqualFold(v.Line, route.ShortName) {
			vehicles = append(vehicles, v)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"route": route, "patterns": pats, "vehicles": vehicles})
}

// handleVehicleTrip returns where a vehicle is on its route and when it reaches the stops ahead.
func (s *Server) handleVehicleTrip(w http.ResponseWriter, r *http.Request) {
	v, ok := s.store.Get(chi.URLParam(r, "id"))
	if !ok {
		writeError(w, http.StatusNotFound, "vehicle not found", "")
		return
	}
	out := map[string]any{"vehicle": v, "trip": nil}
	if s.tracker != nil {
		if trip, ok := s.tracker.Trip(v, time.Now(), limitParam(r, 8, 100)); ok {
			ix := s.index()
			type aheadJSON struct {
				eta.StopETA
				Name     string `json:"name"`
				Platform string `json:"platform"`
				AreaID   string `json:"area_id"`
			}
			ahead := make([]aheadJSON, 0, len(trip.Ahead))
			for _, a := range trip.Ahead {
				aj := aheadJSON{StopETA: a}
				if st, ok := ix.Stop(a.StopID); ok {
					aj.Name, aj.Platform, aj.AreaID = st.Name, st.Platform, st.AreaID
				}
				ahead = append(ahead, aj)
			}
			out["trip"] = map[string]any{"pattern_id": trip.PatternID, "along_m": trip.Along, "next_stop_index": trip.NextStop, "speed_mps": trip.SpeedMps, "ahead": ahead}
			if p := s.patternHeadsign(trip.PatternID); p != "" {
				out["headsign"] = p
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) patternHeadsign(id string) string {
	if s.tracker == nil {
		return ""
	}
	if g := s.tracker.Geometry(); g != nil {
		if pg, ok := g.Pattern(id); ok {
			return pg.Pattern.Headsign
		}
	}
	return ""
}

// handlePattern serves one pattern's shape with each stop's distance along it,
// so clients can animate vehicles along the street instead of in straight lines.
func (s *Server) handlePattern(w http.ResponseWriter, r *http.Request) {
	if s.tracker == nil || s.tracker.Geometry() == nil {
		writeError(w, http.StatusNotFound, "pattern not found", "")
		return
	}
	pg, ok := s.tracker.Geometry().Pattern(chi.URLParam(r, "id"))
	if !ok {
		writeError(w, http.StatusNotFound, "pattern not found", "")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Type", "application/json")
	writeJSONBody(w, http.StatusOK, map[string]any{
		"id": pg.Pattern.ID, "route_id": pg.Pattern.RouteID, "headsign": pg.Pattern.Headsign,
		"shape": pg.Pattern.Shape, "stop_ids": pg.Pattern.StopIDs, "stop_along_m": pg.StopAlong,
	})
}

// handleAllPatterns serves every pattern's shape as an encoded polyline (compact:
// ~200 KB for the whole city), cached until the next import.
func (s *Server) handleAllPatterns(w http.ResponseWriter, r *http.Request) {
	if s.tracker == nil || s.tracker.Geometry() == nil {
		writeJSON(w, http.StatusOK, map[string]any{"patterns": []any{}})
		return
	}
	ix := s.index()
	etag := `"p` + strconv.FormatInt(ix.ImportedAt.Unix(), 36) + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	type patternJSON struct {
		ID       string    `json:"id"`
		RouteID  string    `json:"route_id"`
		Polyline string    `json:"polyline"`
		StopIDs  []string  `json:"stop_ids"`
		Along    []float64 `json:"stop_along_m"`
	}
	g := s.tracker.Geometry()
	var out []patternJSON
	for _, rt := range ix.Routes() {
		for _, p := range ix.Patterns(rt.ID) {
			pg, ok := g.Pattern(p.ID)
			if !ok {
				continue
			}
			along := make([]float64, len(pg.StopAlong))
			for i, a := range pg.StopAlong {
				along[i] = float64(int(a))
			}
			out = append(out, patternJSON{p.ID, p.RouteID, geo.EncodePolyline(p.Shape), p.StopIDs, along})
		}
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Type", "application/json")
	writeJSONBody(w, http.StatusOK, map[string]any{"patterns": out})
}

func (s *Server) mountStatic(r chi.Router) {
	r.Get("/trips", s.handleTripPreview)
	r.Get("/patterns", s.handleAllPatterns)
	r.Get("/vehicles/{id}/trip", s.handleVehicleTrip)
	r.Get("/patterns/{id}", s.handlePattern)
	r.Get("/search", s.handleSearch)
	r.Get("/stops", s.handleAllStops)
	r.Get("/stops/search", s.handleStopSearch)
	r.Get("/stops/{id}", s.handleStop)
	r.Get("/stops/{id}/departures", s.handleStopDepartures)
	r.Get("/lines/search", s.handleLineSearch)
	r.Get("/lines/{mode}/{line}", s.handleLine)
}
