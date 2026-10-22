package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/unmaykr-a/transit-tracker/internal/auth"
)

const (
	oidcCookie   = "bus_oidc"
	oidcFlowTTL  = 10 * time.Minute
	oidcBasePath = "/api/v1/auth/oidc"
)

func (s *Server) mountOIDC(r chi.Router) {
	if s.oidc == nil {
		return
	}
	r.Get("/auth/oidc/login", s.handleOIDCLogin)
	r.Get("/auth/oidc/callback", s.handleOIDCCallback)
}

// safeNext keeps redirects on this site: a path, never "//host" or a URL.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") || strings.ContainsAny(next, "\r\n") {
		return "/"
	}
	return next
}

type signedFlow struct {
	auth.Flow
	Exp int64 `json:"e"`
}

func signFlow(f auth.Flow, now time.Time) string {
	b, _ := json.Marshal(signedFlow{f, now.Add(oidcFlowTTL).Unix()})
	payload := base64.RawURLEncoding.EncodeToString(b)
	m := hmac.New(sha256.New, auth.FlowKey)
	m.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func readFlow(v string, now time.Time) (auth.Flow, error) {
	payload, sig, ok := strings.Cut(v, ".")
	if !ok {
		return auth.Flow{}, errors.New("bad flow cookie")
	}
	m := hmac.New(sha256.New, auth.FlowKey)
	m.Write([]byte(payload))
	want := m.Sum(nil)
	got, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil || !hmac.Equal(got, want) {
		return auth.Flow{}, errors.New("bad flow cookie signature")
	}
	b, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return auth.Flow{}, err
	}
	var f signedFlow
	if err := json.Unmarshal(b, &f); err != nil {
		return auth.Flow{}, err
	}
	if now.Unix() > f.Exp {
		return auth.Flow{}, errors.New("sign-in took too long; start again")
	}
	return f.Flow, nil
}

func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	target, flow, err := s.oidc.Start(r.Context(), safeNext(r.URL.Query().Get("next")))
	if err != nil {
		s.log.Error("oidc start", "err", err)
		http.Redirect(w, r, "/login?error=sso_unavailable", http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: oidcCookie, Value: signFlow(flow, time.Now()), Path: oidcBasePath, HttpOnly: true,
		// Lax: the provider sends the user back with a top-level GET.
		SameSite: http.SameSiteLaxMode, Secure: meta(r).HTTPS, MaxAge: int(oidcFlowTTL.Seconds()),
	})
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	fail := func(code string, err error) {
		if err != nil {
			s.log.Warn("oidc sign-in failed", "err", err)
		}
		http.Redirect(w, r, "/login?error="+url.QueryEscape(code), http.StatusFound)
	}
	http.SetCookie(w, &http.Cookie{Name: oidcCookie, Path: oidcBasePath, MaxAge: -1, HttpOnly: true, Secure: meta(r).HTTPS, SameSite: http.SameSiteLaxMode})
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		fail("sso_denied", errors.New(e+": "+q.Get("error_description")))
		return
	}
	c, err := r.Cookie(oidcCookie)
	if err != nil {
		fail("sso_expired", err)
		return
	}
	flow, err := readFlow(c.Value, time.Now())
	if err != nil {
		fail("sso_expired", err)
		return
	}
	claims, err := s.oidc.Finish(r.Context(), flow, q.Get("state"), q.Get("code"))
	if err != nil {
		fail("sso_failed", err)
		return
	}
	u, err := s.auth.SignInOIDC(r.Context(), s.oidcConfig, claims)
	switch {
	case errors.Is(err, auth.ErrOIDCNoAccount):
		fail("sso_no_account", err)
		return
	case errors.Is(err, auth.ErrOIDCDisabled):
		fail("sso_disabled", err)
		return
	case err != nil:
		fail("sso_failed", err)
		return
	}
	token, _, err := s.auth.StartSession(r.Context(), u, meta(r).IP, r.UserAgent())
	if err != nil {
		fail("sso_failed", err)
		return
	}
	s.setSessionCookie(w, r, token)
	http.Redirect(w, r, safeNext(flow.Next), http.StatusFound)
}
