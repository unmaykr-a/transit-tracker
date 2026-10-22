package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
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

// mountIdentities serves the signed-in user's linked identities (sessions only, never API tokens).
func (s *Server) mountIdentities(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(s.requireSession)
		r.Get("/me/identities", s.handleMyIdentities)
		r.Post("/me/identities/unlink", s.handleUnlinkIdentity)
		if s.oidc != nil {
			r.Post("/me/identities/oidc", s.handleOIDCLinkStart)
		}
	})
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

func (s *Server) setFlowCookie(w http.ResponseWriter, r *http.Request, flow auth.Flow) {
	http.SetCookie(w, &http.Cookie{
		Name: oidcCookie, Value: signFlow(flow, time.Now()), Path: oidcBasePath, HttpOnly: true,
		// Lax: the provider sends the user back with a top-level GET.
		SameSite: http.SameSiteLaxMode, Secure: meta(r).HTTPS, MaxAge: int(oidcFlowTTL.Seconds()),
	})
}

func (s *Server) handleOIDCLogin(w http.ResponseWriter, r *http.Request) {
	target, flow, err := s.oidc.Start(r.Context(), safeNext(r.URL.Query().Get("next")))
	if err != nil {
		s.log.Error("oidc start", "err", err)
		http.Redirect(w, r, "/login?error=sso_unavailable", http.StatusFound)
		return
	}
	s.setFlowCookie(w, r, flow)
	http.Redirect(w, r, target, http.StatusFound)
}

// handleOIDCLinkStart begins linking a provider identity to the signed-in
// user. It is a CSRF-protected POST returning the provider URL, so another
// site cannot start a link; the callback links only for the same session's user.
func (s *Server) handleOIDCLinkStart(w http.ResponseWriter, r *http.Request) {
	target, flow, err := s.oidc.Start(r.Context(), "/settings")
	if err != nil {
		s.log.Error("oidc start", "err", err)
		writeError(w, http.StatusBadGateway, "single sign-on is unavailable", "")
		return
	}
	flow.Link = principal(r).User.ID
	s.setFlowCookie(w, r, flow)
	writeJSON(w, http.StatusOK, map[string]string{"url": target})
}

type identityJSON struct {
	Issuer    string    `json:"issuer"`
	Subject   string    `json:"subject"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	LastLogin time.Time `json:"last_login"`
}

func (s *Server) handleMyIdentities(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Q.ListUserIdentities(r.Context(), principal(r).User.ID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]identityJSON, 0, len(rows))
	for _, i := range rows {
		out = append(out, identityJSON{i.Issuer, i.Subject, i.Email, time.Unix(i.CreatedAt, 0).UTC(), time.Unix(i.LastLogin, 0).UTC()})
	}
	writeJSON(w, http.StatusOK, map[string]any{"identities": out})
}

func (s *Server) handleUnlinkIdentity(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Issuer  string `json:"issuer"`
		Subject string `json:"subject"`
	}
	if err := decodeJSON(r, &in); err != nil {
		s.writeErr(w, err)
		return
	}
	err := s.auth.UnlinkOIDC(r.Context(), principal(r).User, in.Issuer, in.Subject)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "identity not found", "")
		return
	}
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.audit(r.Context(), r, "identity.unlink", principal(r).User.Username, in.Issuer)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	back := "/login" // where failures land: the login page, or settings when linking
	fail := func(code string, err error) {
		if err != nil {
			s.log.Warn("oidc sign-in failed", "err", err)
		}
		http.Redirect(w, r, back+"?error="+url.QueryEscape(code), http.StatusFound)
	}
	http.SetCookie(w, &http.Cookie{Name: oidcCookie, Path: oidcBasePath, MaxAge: -1, HttpOnly: true, Secure: meta(r).HTTPS, SameSite: http.SameSiteLaxMode})
	q := r.URL.Query()
	var flow auth.Flow
	c, err := r.Cookie(oidcCookie)
	if err == nil {
		flow, err = readFlow(c.Value, time.Now())
	}
	if flow.Link != 0 {
		back = "/settings"
	}
	if e := q.Get("error"); e != "" {
		fail("sso_denied", errors.New(e+": "+q.Get("error_description")))
		return
	}
	if err != nil {
		fail("sso_expired", err)
		return
	}
	claims, err := s.oidc.Finish(r.Context(), flow, q.Get("state"), q.Get("code"))
	if err != nil {
		fail("sso_failed", err)
		return
	}
	if flow.Link != 0 {
		s.finishLink(w, r, flow.Link, claims, fail)
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

// finishLink attaches the verified identity to the user who started the link,
// provided this browser is still signed in as them.
func (s *Server) finishLink(w http.ResponseWriter, r *http.Request, userID int64, claims auth.Claims, fail func(string, error)) {
	p := principal(r)
	if p == nil || p.User.ID != userID || p.SessionID == "" {
		fail("sso_link_session", errors.New("link finished without the session that started it"))
		return
	}
	switch err := s.auth.LinkOIDC(r.Context(), userID, claims); {
	case errors.Is(err, auth.ErrOIDCLinkedElsewhere):
		fail("sso_link_taken", err)
		return
	case err != nil:
		fail("sso_failed", err)
		return
	}
	s.audit(r.Context(), r, "identity.link", p.User.Username, claims.Issuer)
	to := "/settings?sso=linked"
	// With an admin group, the role follows it on single sign-on: warn an admin
	// whose provider account is not in it before they are surprised.
	if g := s.oidcConfig.AdminGroup; g != "" && p.User.Role == auth.RoleAdmin && !slices.Contains(claims.Groups, g) {
		to += "&warn=admin_group"
	}
	http.Redirect(w, r, to, http.StatusFound)
}
