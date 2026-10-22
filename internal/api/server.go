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
)

type Server struct {
	log     *slog.Logger
	started time.Time
}

func NewServer(log *slog.Logger) *Server {
	return &Server{log: log, started: time.Now()}
}

// Handler returns the root handler. web is the built UI (see Dist).
func (s *Server) Handler(web http.Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(s.requestLog)

	r.Get("/healthz", s.health)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/version", s.version)
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

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
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
