package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/unmaykr-a/transit-tracker/internal/auth"
	"github.com/unmaykr-a/transit-tracker/internal/settings"
)

const sessionCookie = "tram_session"

type ctxKey int

const metaKey ctxKey = 0

// requestMeta is per-request information derived by the middleware.
type requestMeta struct {
	IP        string
	HTTPS     bool
	Principal *auth.Principal
}

func meta(r *http.Request) *requestMeta {
	if m, ok := r.Context().Value(metaKey).(*requestMeta); ok {
		return m
	}
	return &requestMeta{}
}

func principal(r *http.Request) *auth.Principal { return meta(r).Principal }

func (s *Server) trusted(addr netip.Addr) bool {
	for _, p := range s.trustedProxies {
		if p.Contains(addr.Unmap()) {
			return true
		}
	}
	return false
}

// clientMeta resolves the client IP and scheme. X-Forwarded-* headers are only
// believed from TRAM_TRUSTED_PROXIES (e.g. Nginx Proxy Manager's network).
func (s *Server) clientMeta(r *http.Request) *requestMeta {
	m := &requestMeta{HTTPS: r.TLS != nil}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	m.IP = host
	remote, err := netip.ParseAddr(host)
	if err != nil || !s.trusted(remote) {
		return m
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		m.HTTPS = true
	}
	// Walk X-Forwarded-For from the right, skipping our own proxies.
	hops := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(hops) - 1; i >= 0; i-- {
		a, err := netip.ParseAddr(strings.TrimSpace(hops[i]))
		if err != nil {
			break
		}
		m.IP = a.Unmap().String()
		if !s.trusted(a) {
			break
		}
	}
	return m
}

// withSession attaches request metadata and the logged-in principal, if any.
func (s *Server) withSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := s.clientMeta(r)
		if s.auth != nil {
			if c, err := r.Cookie(sessionCookie); err == nil {
				p, err := s.auth.Authenticate(r.Context(), c.Value, m.IP)
				if err != nil {
					s.log.Error("session lookup", "err", err)
				}
				m.Principal = p
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), metaKey, m)))
	})
}

func isSafe(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

// csrfGuard protects state-changing requests: the Origin (when sent) must be
// this host, and logged-in requests must echo the session's CSRF token.
func (s *Server) csrfGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isSafe(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			u, err := url.Parse(origin)
			if err != nil || !strings.EqualFold(u.Host, r.Host) {
				writeError(w, http.StatusForbidden, "cross-origin request refused", "")
				return
			}
		}
		if p := principal(r); p != nil && p.SessionID != "" && r.Header.Get("X-CSRF-Token") != p.CSRF {
			writeError(w, http.StatusForbidden, "missing or invalid CSRF token", "")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) settingsNow() settings.Settings {
	if s.settings == nil {
		return settings.Settings{PublicMap: true, SignupMode: "off", InstanceName: "Tram"}
	}
	return s.settings.Get()
}

// publicGate lets anonymous visitors use the map, search and stop pages only
// when the public map setting is on.
func (s *Server) publicGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal(r) == nil && !s.settingsNow().PublicMap {
			writeError(w, http.StatusUnauthorized, "login required", "")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal(r) == nil {
			writeError(w, http.StatusUnauthorized, "login required", "")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := principal(r)
		if p == nil {
			writeError(w, http.StatusUnauthorized, "login required", "")
			return
		}
		if !p.IsAdmin() {
			writeError(w, http.StatusForbidden, "admin only", "")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeError(w http.ResponseWriter, status int, msg, field string) {
	body := map[string]string{"error": msg}
	if field != "" {
		body["field"] = field
	}
	writeJSON(w, status, body)
}

// writeErr maps domain errors to HTTP responses; anything unknown is a 500.
func (s *Server) writeErr(w http.ResponseWriter, err error) {
	var ve *auth.ValidationError
	var fe *settings.FieldError
	switch {
	case errors.As(err, &ve):
		writeError(w, http.StatusBadRequest, ve.Error(), ve.Field)
	case errors.As(err, &fe):
		writeError(w, http.StatusBadRequest, fe.Error(), fe.Field)
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeError(w, http.StatusUnauthorized, err.Error(), "")
	case errors.Is(err, auth.ErrRateLimited):
		writeError(w, http.StatusTooManyRequests, err.Error(), "")
	case errors.Is(err, auth.ErrSetupDone), errors.Is(err, auth.ErrLastAdmin):
		writeError(w, http.StatusConflict, err.Error(), "")
	case errors.Is(err, auth.ErrUsernameTaken):
		writeError(w, http.StatusConflict, err.Error(), "username")
	case errors.Is(err, auth.ErrInviteInvalid):
		writeError(w, http.StatusBadRequest, err.Error(), "invite")
	case errors.Is(err, auth.ErrSignupClosed):
		writeError(w, http.StatusForbidden, err.Error(), "")
	case errors.Is(err, errBadJSON):
		writeError(w, http.StatusBadRequest, err.Error(), "")
	default:
		s.log.Error("request failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error", "")
	}
}

var errBadJSON = errors.New("request body must be a JSON object")

func decodeJSON(r *http.Request, v any) error {
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(v); err != nil {
		return errBadJSON
	}
	return nil
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: meta(r).HTTPS, MaxAge: int(s.auth.SessionTTL.Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: meta(r).HTTPS, MaxAge: -1})
}
