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

const sessionCookie = "bus_session"

type ctxKey int

const metaKey ctxKey = 0

// requestMeta is per-request information derived by the middleware.
type requestMeta struct {
	IP    string
	HTTPS bool
	// UntrustedProxy: forwarding headers came from a peer not in
	// BUS_TRUSTED_PROXIES, so IP is that proxy, not the client.
	UntrustedProxy bool
	Principal      *auth.Principal
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
// believed from BUS_TRUSTED_PROXIES (e.g. Nginx Proxy Manager's network).
func (s *Server) clientMeta(r *http.Request) *requestMeta {
	m := &requestMeta{HTTPS: r.TLS != nil}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	m.IP = host
	remote, err := netip.ParseAddr(host)
	if err != nil || !s.trusted(remote) {
		if r.Header.Get("X-Forwarded-For") != "" || r.Header.Get("Forwarded") != "" || r.Header.Get("X-Real-IP") != "" {
			m.UntrustedProxy = true
			s.warnProxyOnce.Do(func() {
				s.log.Warn("requests arrive through a proxy that is not trusted: set BUS_TRUSTED_PROXIES to its network, "+
					"or every visitor shares the proxy's address (login rate limits, private /metrics)", "proxy", host)
			})
		}
		return m
	}
	if strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		m.HTTPS = true
	}
	// Walk X-Forwarded-For from the right, skipping our own proxies.
	// All header lines: a proxy may append its own line rather than extend the client's.
	hops := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
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
		// Only bus_ bearers are API tokens: others (BUS_METRICS_TOKEN for /metrics) are checked where they apply.
		if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && s.auth != nil && strings.HasPrefix(strings.TrimSpace(bearer), auth.TokenPrefix) {
			// API tokens (Home Assistant, scripts): read-only, never admin.
			p, err := s.auth.AuthenticateToken(r.Context(), strings.TrimSpace(bearer), m.IP)
			if err != nil {
				s.log.Error("token lookup", "err", err)
			}
			if p == nil {
				writeError(w, http.StatusUnauthorized, "invalid API token", "")
				return
			}
			if !isSafe(r.Method) {
				writeError(w, http.StatusForbidden, "API tokens are read-only", "")
				return
			}
			m.Principal = p
		} else if s.auth != nil {
			if c, err := r.Cookie(sessionCookie); err == nil {
				p, err := s.auth.Authenticate(r.Context(), c.Value, m.IP)
				if err != nil {
					s.log.Error("session lookup", "err", err)
				}
				m.Principal = p
				if p != nil && p.Extended {
					// Sliding expiry: the cookie would otherwise lapse 30 days after login, however active.
					s.setSessionCookie(w, r.WithContext(context.WithValue(r.Context(), metaKey, m)), c.Value)
				}
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
		return settings.Settings{PublicMap: true, SignupMode: "off", InstanceName: "Bus"}
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
		if !p.IsAdmin() || p.ViaToken() {
			writeError(w, http.StatusForbidden, "admin only", "")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireSession rejects API tokens: account security pages need a login.
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal(r).ViaToken() {
			writeError(w, http.StatusForbidden, "not available to API tokens", "")
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
	case errors.Is(err, auth.ErrSetupDone), errors.Is(err, auth.ErrLastAdmin), errors.Is(err, auth.ErrLastSignIn):
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
