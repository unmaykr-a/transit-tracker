// Package api wires the HTTP surface: REST under /api/v1, health, and the embedded web UI.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/netip"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/unmaykr-a/transit-tracker/internal/auth"
	"github.com/unmaykr-a/transit-tracker/internal/buildinfo"
	"github.com/unmaykr-a/transit-tracker/internal/cache"
	"github.com/unmaykr-a/transit-tracker/internal/config"
	"github.com/unmaykr-a/transit-tracker/internal/dashboard"
	"github.com/unmaykr-a/transit-tracker/internal/departures"
	"github.com/unmaykr-a/transit-tracker/internal/eta"
	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/ingest"
	"github.com/unmaykr-a/transit-tracker/internal/settings"
	"github.com/unmaykr-a/transit-tracker/internal/state"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
	"github.com/unmaykr-a/transit-tracker/internal/store"
)

type Server struct {
	log     *slog.Logger
	started time.Time
	store   *state.Store
	health  *ingest.Health
	hub     liveHub // shared live frames (livehub.go)
	// originPatterns are extra hosts allowed to open /ws cross-origin
	// (same-origin is always allowed).
	originPatterns []string
	static         func() *staticdata.Index
	staticStatus   func() staticdata.Status
	departures     *departures.Service
	auth           *auth.Service
	settings       *settings.Service
	db             *store.Store
	dataDir        string
	trustedProxies []netip.Prefix
	refreshStatic  func(context.Context) error
	dashboard      *dashboard.Builder
	tracker        *eta.Tracker
	metrics        *serverMetrics
	metricsMode    string
	metricsToken   string
	oidc           *auth.OIDC
	oidcConfig     config.OIDCConfig
	geocode        func(ctx context.Context, q string) ([]feeds.Address, error)
	geocodeCache   cache.TTL[string, []feeds.Address]
	placesMu       sync.Mutex
	placesWindow   time.Time
	placesUsed     int
	refreshMu      sync.Mutex
	refreshRun     *refreshRun
	bodyMu         sync.Mutex
	bodies         map[string]importedBody // encoded static lists, per import (see importBody)
}

// Deps are the server's collaborators.
type Deps struct {
	Store          *state.Store
	Health         *ingest.Health
	OriginPatterns []string
	Static         func() *staticdata.Index
	StaticStatus   func() staticdata.Status
	Departures     *departures.Service
	// Accounts and admin; when Auth is nil the server runs without login (tests).
	Auth           *auth.Service
	Settings       *settings.Service
	DB             *store.Store
	DataDir        string
	TrustedProxies []netip.Prefix
	RefreshStatic  func(context.Context) error
	Dashboard      *dashboard.Builder
	Tracker        *eta.Tracker
	// MetricsMode is BUS_METRICS ("private" when empty); MetricsToken BUS_METRICS_TOKEN.
	MetricsMode, MetricsToken string
	// OIDC enables single sign-on; nil when not configured.
	OIDC       *auth.OIDC
	OIDCConfig config.OIDCConfig
	// Geocode searches addresses (for saved places); nil disables /geocode.
	Geocode func(ctx context.Context, q string) ([]feeds.Address, error)
}

func NewServer(log *slog.Logger, d Deps) *Server {
	if d.Store == nil {
		d.Store = state.NewStore()
	}
	if d.Health == nil {
		d.Health = ingest.NewHealth()
	}
	if d.MetricsMode == "" {
		d.MetricsMode = "private"
	}
	s := &Server{log: log, started: time.Now(), store: d.Store, health: d.Health, originPatterns: d.OriginPatterns,
		static: d.Static, staticStatus: d.StaticStatus, departures: d.Departures,
		auth: d.Auth, settings: d.Settings, db: d.DB, dataDir: d.DataDir, trustedProxies: d.TrustedProxies, refreshStatic: d.RefreshStatic,
		dashboard: d.Dashboard, tracker: d.Tracker, metricsMode: d.MetricsMode, metricsToken: d.MetricsToken,
		oidc: d.OIDC, oidcConfig: d.OIDCConfig, geocode: d.Geocode}
	s.geocodeCache.TTL = time.Hour
	s.initMetrics()
	return s
}

// Handler returns the root handler. web is the built UI (see Dist).
func (s *Server) Handler(web http.Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(s.requestLog)
	r.Use(s.withSession)
	r.Use(s.securityHeaders)

	r.Get("/healthz", s.healthz)
	if s.metricsMode != "off" || s.metricsToken != "" {
		r.Get("/metrics", s.handleMetrics)
	}
	r.With(s.publicGate).Get("/ws", s.websocket)
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.csrfGuard)
		r.Get("/version", s.version)
		s.mountAccount(r)
		s.mountAdmin(r)
		r.Group(func(r chi.Router) {
			// Map, search, stops and lines: public unless the public-map setting is off.
			r.Use(s.publicGate)
			r.Get("/status", s.status)
			r.Get("/vehicles", s.vehicles)
			r.Get("/vehicles/stream", s.vehicleStream)
			r.Get("/vehicles/{id}", func(w http.ResponseWriter, r *http.Request) { s.vehicle(w, r, chi.URLParam(r, "id")) })
			s.mountStatic(r)
		})
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
		s.observeRequest(r, ww.Status(), time.Since(start))
		if r.URL.Path == "/healthz" {
			return // polled every 30 s by Docker; keep logs readable
		}
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path, "status", ww.Status(), "dur", time.Since(start))
	})
}
