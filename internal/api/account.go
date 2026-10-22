package api

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/unmaykr-a/transit-tracker/internal/auth"
	"github.com/unmaykr-a/transit-tracker/internal/buildinfo"
	"github.com/unmaykr-a/transit-tracker/internal/store/db"
)

type userJSON struct {
	ID        int64     `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Locale    string    `json:"locale"`
	Theme     string    `json:"theme"`
	Disabled  bool      `json:"disabled"`
	CreatedAt time.Time `json:"created_at"`
}

func toUserJSON(u db.User) userJSON {
	return userJSON{u.ID, u.Username, u.Email, u.Role, u.Locale, u.Theme, u.Disabled != 0, time.Unix(u.CreatedAt, 0).UTC()}
}

// audit records an admin or security-relevant action. Failures are logged, never fatal.
func (s *Server) audit(ctx context.Context, r *http.Request, action, target, detail string) {
	if s.db == nil {
		return
	}
	p := principal(r)
	params := db.InsertAuditParams{At: time.Now().Unix(), Action: action, Target: target, Detail: detail}
	if p != nil {
		params.ActorID.Int64, params.ActorID.Valid = p.User.ID, true
		params.Actor = p.User.Username
	}
	if err := s.db.Q.InsertAudit(ctx, params); err != nil {
		s.log.Error("audit", "action", action, "err", err)
	}
}

// handleSession is the app's bootstrap call: public settings, whether setup is
// needed, and the current user with their CSRF token.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"settings": s.settingsNow().Public(), "version": buildinfo.Version, "user": nil, "setup_required": false}
	if s.auth != nil {
		need, err := s.auth.NeedsSetup(r.Context())
		if err != nil {
			s.writeErr(w, err)
			return
		}
		out["setup_required"] = need
	}
	if p := principal(r); p != nil {
		out["user"] = toUserJSON(p.User)
		out["csrf"] = p.CSRF
	}
	writeJSON(w, http.StatusOK, out)
}

type credentials struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Invite   string `json:"invite"`
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request, u db.User) {
	token, p, err := s.auth.StartSession(r.Context(), u, meta(r).IP, r.UserAgent())
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.setSessionCookie(w, r, token)
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserJSON(p.User), "csrf": p.CSRF})
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := decodeJSON(r, &c); err != nil {
		s.writeErr(w, err)
		return
	}
	u, err := s.auth.Setup(r.Context(), c.Username, c.Email, c.Password)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.audit(r.Context(), r, "setup.complete", u.Username, "")
	s.startSession(w, r, u)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := decodeJSON(r, &c); err != nil {
		s.writeErr(w, err)
		return
	}
	token, p, err := s.auth.Login(r.Context(), c.Username, c.Password, meta(r).IP, r.UserAgent())
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.setSessionCookie(w, r, token)
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserJSON(p.User), "csrf": p.CSRF})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		if err := s.auth.Logout(r.Context(), c.Value); err != nil {
			s.writeErr(w, err)
			return
		}
	}
	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleInviteCheck(w http.ResponseWriter, r *http.Request) {
	role, err := s.auth.CheckInvite(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"role": role})
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	var c credentials
	if err := decodeJSON(r, &c); err != nil {
		s.writeErr(w, err)
		return
	}
	mode := s.settingsNow().SignupMode
	if mode == "off" && c.Invite == "" {
		s.writeErr(w, auth.ErrSignupClosed)
		return
	}
	u, err := s.auth.Signup(r.Context(), mode, c.Invite, c.Username, c.Email, c.Password)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	how := "open sign-up"
	if c.Invite != "" {
		how = "invite"
	}
	s.audit(r.Context(), r, "user.signup", u.Username, how)
	s.startSession(w, r, u)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserJSON(principal(r).User), "csrf": principal(r).CSRF})
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	var in struct {
		Email  *string `json:"email"`
		Locale *string `json:"locale"`
		Theme  *string `json:"theme"`
	}
	if err := decodeJSON(r, &in); err != nil {
		s.writeErr(w, err)
		return
	}
	u := p.User
	if in.Email != nil {
		u.Email = *in.Email
	}
	if in.Locale != nil {
		if !slices.Contains([]string{"", "et", "en"}, *in.Locale) {
			s.writeErr(w, &auth.ValidationError{Field: "locale", Msg: "must be empty, et or en"})
			return
		}
		u.Locale = *in.Locale
	}
	if in.Theme != nil {
		if !slices.Contains([]string{"", "system", "light", "dark"}, *in.Theme) {
			s.writeErr(w, &auth.ValidationError{Field: "theme", Msg: "must be system, light or dark"})
			return
		}
		u.Theme = *in.Theme
	}
	if err := s.db.Q.UpdateUserProfile(r.Context(), db.UpdateUserProfileParams{Email: u.Email, Locale: u.Locale, Theme: u.Theme, ID: u.ID}); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserJSON(u)})
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if err := decodeJSON(r, &in); err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.auth.ChangePassword(r.Context(), principal(r), in.Current, in.New); err != nil {
		s.writeErr(w, err)
		return
	}
	s.audit(r.Context(), r, "user.password_change", principal(r).User.Username, "")
	w.WriteHeader(http.StatusNoContent)
}

type sessionJSON struct {
	ID         string    `json:"id"`
	Current    bool      `json:"current"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	UserAgent  string    `json:"user_agent"`
	IP         string    `json:"ip"`
}

func (s *Server) listSessions(ctx context.Context, userID int64, current string) ([]sessionJSON, error) {
	rows, err := s.db.Q.ListUserSessions(ctx, db.ListUserSessionsParams{UserID: userID, ExpiresAt: time.Now().Unix()})
	if err != nil {
		return nil, err
	}
	out := make([]sessionJSON, 0, len(rows))
	for _, r := range rows {
		out = append(out, sessionJSON{r.ID, r.ID == current, time.Unix(r.CreatedAt, 0).UTC(), time.Unix(r.LastSeenAt, 0).UTC(), r.UserAgent, r.Ip})
	}
	return out, nil
}

func (s *Server) handleMySessions(w http.ResponseWriter, r *http.Request) {
	p := principal(r)
	out, err := s.listSessions(r.Context(), p.User.ID, p.SessionID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": out})
}

func (s *Server) handleRevokeMySession(w http.ResponseWriter, r *http.Request) {
	n, err := s.db.Q.DeleteUserSession(r.Context(), db.DeleteUserSessionParams{ID: chi.URLParam(r, "id"), UserID: principal(r).User.ID})
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "session not found", "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) mountAccount(r chi.Router) {
	r.Get("/session", s.handleSession)
	if s.auth == nil {
		return
	}
	r.Post("/auth/setup", s.handleSetup)
	r.Post("/auth/login", s.handleLogin)
	r.Post("/auth/logout", s.handleLogout)
	r.Post("/auth/signup", s.handleSignup)
	r.Get("/auth/invites/{token}", s.handleInviteCheck)
	r.Group(func(r chi.Router) {
		r.Use(s.requireUser)
		r.Get("/me", s.handleMe)
		r.Patch("/me", s.handleUpdateMe)
		r.Post("/me/password", s.handleChangePassword)
		r.With(s.requireSession).Get("/me/sessions", s.handleMySessions)
		r.Delete("/me/sessions/{id}", s.handleRevokeMySession)
		s.mountTokens(r)
		s.mountMyLines(r)
		s.mountHA(r)
	})
}
