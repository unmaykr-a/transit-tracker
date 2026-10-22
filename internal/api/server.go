// Package api wires the HTTP surface: REST under /api/v1, health, and the embedded web UI.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/unmaykr-a/transit-tracker/internal/buildinfo"
	"github.com/unmaykr-a/transit-tracker/internal/departures"
	"github.com/unmaykr-a/transit-tracker/internal/ingest"
	"github.com/unmaykr-a/transit-tracker/internal/state"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
)

type Server struct {
	log     *slog.Logger
	started time.Time
	store   *state.Store
	health  *ingest.Health
	// originPatterns are extra hosts allowed to open /ws cross-origin
	// (same-origin is always allowed).
	originPatterns []string
	static         func() *staticdata.Index
	staticStatus   func() staticdata.Status
	departures     *departures.Service
}

// Deps are the server's collaborators.
type Deps struct {
	Store          *state.Store
	Health         *ingest.Health
	OriginPatterns []string
	Static         func() *staticdata.Index
	StaticStatus   func() staticdata.Status
	Departures     *departures.Service
}

func NewServer(log *slog.Logger, d Deps) *Server {
	if d.Store == nil {
		d.Store = state.NewStore()
	}
	if d.Health == nil {
		d.Health = ingest.NewHealth()
	}
	return &Server{log: log, started: time.Now(), store: d.Store, health: d.Health, originPatterns: d.OriginPatterns,
		static: d.Static, staticStatus: d.StaticStatus, departures: d.Departures}
}

// Handler returns the root handler. web is the built UI (see Dist).
func (s *Server) Handler(web http.Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(s.requestLog)

	r.Get("/healthz", s.healthz)
	r.Get("/ws", s.websocket)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/version", s.version)
		r.Get("/status", s.status)
		r.Get("/vehicles", s.vehicles)
		r.Get("/vehicles/stream", s.vehicleStream)
		r.Get("/vehicles/{id}", func(w http.ResponseWriter, r *http.Request) { s.vehicle(w, r, chi.URLParam(r, "id")) })
		s.mountStatic(r)
		r.NotFound(func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		})
	})
	r.Handle("/*", web)
	return r
}

type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	UptimeSec int64  `json:"uptime_s"`
}

func (s *Server) info() versionInfo {
	return versionInfo{buildinfo.Version, buildinfo.Commit, int64(time.Since(s.started).Seconds())}
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Status string `json:"status"`
		versionInfo
	}{"ok", s.info()})
}

func (s *Server) version(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.info())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	writeJSONBody(w, status, v)
}

// writeJSONBody writes v without touching caching headers.
func writeJSONBody(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)
		if r.URL.Path == "/healthz" {
			return // polled every 30 s by Docker; keep logs readable
		}
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start))
	})
}
