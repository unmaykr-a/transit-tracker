package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/unmaykr-a/transit-tracker/internal/auth"
	"github.com/unmaykr-a/transit-tracker/internal/store/db"
)

// tokenView is an API token as listed; the secret itself is only returned on creation.
type tokenView struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	LastUsedIP string     `json:"last_used_ip,omitempty"`
	UserID     int64      `json:"user_id,omitempty"`
	Username   string     `json:"username,omitempty"`
}

func viewToken(t db.ApiToken) tokenView {
	v := tokenView{ID: t.ID, Name: t.Name, Prefix: t.Prefix, CreatedAt: time.Unix(t.CreatedAt, 0).UTC(), LastUsedIP: t.LastUsedIp}
	if t.LastUsedAt.Valid {
		at := time.Unix(t.LastUsedAt.Int64, 0).UTC()
		v.LastUsedAt = &at
	}
	return v
}

func (s *Server) mountTokens(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(s.requireSession)
		r.Get("/me/tokens", s.handleMyTokens)
		r.Post("/me/tokens", s.handleCreateToken)
		r.Delete("/me/tokens/{id}", s.handleRevokeMyToken)
	})
}

func (s *Server) handleMyTokens(w http.ResponseWriter, r *http.Request) {
	ts, err := s.db.Q.ListUserAPITokens(r.Context(), principal(r).User.ID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]tokenView, len(ts))
	for i, t := range ts {
		out[i] = viewToken(t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": out})
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &in); err != nil {
		s.writeErr(w, err)
		return
	}
	token, t, err := s.auth.CreateToken(r.Context(), principal(r).User.ID, in.Name)
	if errors.Is(err, auth.ErrTooManyTokens) {
		writeError(w, http.StatusConflict, err.Error(), "")
		return
	}
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "info": viewToken(t)})
}

func (s *Server) handleRevokeMyToken(w http.ResponseWriter, r *http.Request) {
	n, err := s.db.Q.DeleteUserAPIToken(r.Context(), db.DeleteUserAPITokenParams{ID: idParam(r), UserID: principal(r).User.ID})
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "token not found", "")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminTokens(w http.ResponseWriter, r *http.Request) {
	ts, err := s.db.Q.ListAPITokens(r.Context())
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]tokenView, len(ts))
	for i, t := range ts {
		out[i] = viewToken(db.ApiToken{ID: t.ID, Name: t.Name, Prefix: t.Prefix, CreatedAt: t.CreatedAt, LastUsedAt: t.LastUsedAt, LastUsedIp: t.LastUsedIp})
		out[i].UserID, out[i].Username = t.UserID, t.Username
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": out})
}

func (s *Server) handleAdminRevokeToken(w http.ResponseWriter, r *http.Request) {
	id := idParam(r)
	t, err := s.db.Q.GetAPIToken(r.Context(), id)
	if err == nil {
		_, err = s.db.Q.DeleteAPIToken(r.Context(), id)
	}
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "token not found", "")
			return
		}
		s.writeErr(w, err)
		return
	}
	owner := strconv.FormatInt(t.UserID, 10)
	if u, err := s.db.Q.GetUser(r.Context(), t.UserID); err == nil {
		owner = u.Username
	}
	s.audit(r.Context(), r, "token.revoke", owner, t.Name+" ("+t.Prefix+"…)")
	w.WriteHeader(http.StatusNoContent)
}
