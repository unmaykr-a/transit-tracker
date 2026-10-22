package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/unmaykr-a/transit-tracker/internal/auth"
	"github.com/unmaykr-a/transit-tracker/internal/buildinfo"
	"github.com/unmaykr-a/transit-tracker/internal/store/db"
)

func (s *Server) targetUser(w http.ResponseWriter, r *http.Request) (db.User, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err == nil {
		u, err := s.db.Q.GetUser(r.Context(), id)
		if err == nil {
			return u, true
		}
		if !errors.Is(err, sql.ErrNoRows) {
			s.writeErr(w, err)
			return db.User{}, false
		}
	}
	writeError(w, http.StatusNotFound, "user not found", "")
	return db.User{}, false
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.db.Q.ListUsers(r.Context())
	if err != nil {
		s.writeErr(w, err)
		return
	}
	type row struct {
		userJSON
		Sessions int `json:"sessions"`
	}
	counts, err := s.db.Q.CountActiveSessionsByUser(r.Context(), time.Now().Unix())
	if err != nil {
		s.writeErr(w, err)
		return
	}
	sessions := make(map[int64]int, len(counts))
	for _, c := range counts {
		sessions[c.UserID] = int(c.Sessions)
	}
	out := make([]row, 0, len(users))
	for _, u := range users {
		out = append(out, row{toUserJSON(u), sessions[u.ID]})
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": out})
}

func (s *Server) handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username, Email, Password, Role string
	}
	if err := decodeJSON(r, &in); err != nil {
		s.writeErr(w, err)
		return
	}
	u, err := s.auth.CreateUser(r.Context(), s.db.Q, in.Username, in.Email, in.Password, in.Role)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.audit(r.Context(), r, "user.create", u.Username, "role="+u.Role)
	writeJSON(w, http.StatusCreated, map[string]any{"user": toUserJSON(u)})
}

func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	u, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	var in struct {
		Role     *string `json:"role"`
		Disabled *bool   `json:"disabled"`
	}
	if err := decodeJSON(r, &in); err != nil {
		s.writeErr(w, err)
		return
	}
	role, disabled := u.Role, u.Disabled != 0
	if in.Role != nil {
		if err := auth.ValidateRole(*in.Role); err != nil {
			s.writeErr(w, err)
			return
		}
		role = *in.Role
	}
	if in.Disabled != nil {
		disabled = *in.Disabled
	}
	if disabled && u.ID == principal(r).User.ID {
		writeError(w, http.StatusConflict, "you cannot disable your own account", "")
		return
	}
	err := s.db.Tx(r.Context(), func(q *db.Queries) error {
		if err := s.auth.GuardLastAdmin(r.Context(), q, u, role, disabled); err != nil {
			return err
		}
		if err := q.UpdateUserRole(r.Context(), db.UpdateUserRoleParams{Role: role, ID: u.ID}); err != nil {
			return err
		}
		d := int64(0)
		if disabled {
			d = 1
		}
		if err := q.UpdateUserDisabled(r.Context(), db.UpdateUserDisabledParams{Disabled: d, ID: u.ID}); err != nil {
			return err
		}
		if disabled {
			return q.DeleteUserSessions(r.Context(), u.ID) // sign them out everywhere
		}
		return nil
	})
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if role != u.Role {
		s.audit(r.Context(), r, "user.role", u.Username, u.Role+" → "+role)
	}
	if disabled != (u.Disabled != 0) {
		action := "user.enable"
		if disabled {
			action = "user.disable"
		}
		s.audit(r.Context(), r, action, u.Username, "")
	}
	u, _ = s.db.Q.GetUser(r.Context(), u.ID)
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserJSON(u)})
}

func (s *Server) handleAdminResetPassword(w http.ResponseWriter, r *http.Request) {
	u, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	var in struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &in); err != nil {
		s.writeErr(w, err)
		return
	}
	keep := ""
	if u.ID == principal(r).User.ID {
		keep = principal(r).SessionID
	}
	if err := s.auth.SetPassword(r.Context(), u.ID, in.Password, keep); err != nil {
		s.writeErr(w, err)
		return
	}
	// A reset is what an admin does after a compromise: API tokens the intruder
	// may have made go too (the user's own Home Assistant needs a new one).
	n, err := s.db.Q.DeleteAllUserAPITokens(r.Context(), u.ID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.audit(r.Context(), r, "user.password_reset", u.Username, fmt.Sprintf("api tokens revoked: %d", n))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminRevokeSessions(w http.ResponseWriter, r *http.Request) {
	u, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	var err error
	if u.ID == principal(r).User.ID {
		err = s.db.Q.DeleteOtherUserSessions(r.Context(), db.DeleteOtherUserSessionsParams{UserID: u.ID, ID: principal(r).SessionID})
	} else {
		err = s.db.Q.DeleteUserSessions(r.Context(), u.ID)
	}
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.audit(r.Context(), r, "user.sessions_revoke", u.Username, "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	u, ok := s.targetUser(w, r)
	if !ok {
		return
	}
	if u.ID == principal(r).User.ID {
		writeError(w, http.StatusConflict, "you cannot delete your own account", "")
		return
	}
	err := s.db.Tx(r.Context(), func(q *db.Queries) error {
		if err := s.auth.GuardLastAdmin(r.Context(), q, u, "", true); err != nil {
			return err
		}
		return q.DeleteUser(r.Context(), u.ID) // sessions (and later saved lines) cascade
	})
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.audit(r.Context(), r, "user.delete", u.Username, "")
	w.WriteHeader(http.StatusNoContent)
}

type inviteJSON struct {
	ID        int64      `json:"id"`
	Role      string     `json:"role"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedBy    string     `json:"used_by,omitempty"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	Expired   bool       `json:"expired"`
}

func (s *Server) handleAdminInvites(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Q.ListInvites(r.Context())
	if err != nil {
		s.writeErr(w, err)
		return
	}
	now := time.Now().Unix()
	out := make([]inviteJSON, 0, len(rows))
	for _, i := range rows {
		j := inviteJSON{ID: i.ID, Role: i.Role, CreatedBy: i.CreatedByName, CreatedAt: time.Unix(i.CreatedAt, 0).UTC(),
			ExpiresAt: time.Unix(i.ExpiresAt, 0).UTC(), UsedBy: i.UsedByName, Expired: i.ExpiresAt <= now}
		if i.UsedAt.Valid {
			t := time.Unix(i.UsedAt.Int64, 0).UTC()
			j.UsedAt = &t
		}
		out = append(out, j)
	}
	writeJSON(w, http.StatusOK, map[string]any{"invites": out})
}

func (s *Server) handleAdminCreateInvite(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Role           string  `json:"role"`
		ExpiresInHours float64 `json:"expires_in_hours"`
	}
	if err := decodeJSON(r, &in); err != nil {
		s.writeErr(w, err)
		return
	}
	if in.Role == "" {
		in.Role = auth.RoleUser
	}
	if in.ExpiresInHours == 0 {
		in.ExpiresInHours = 7 * 24
	}
	token, inv, err := s.auth.CreateInvite(r.Context(), principal(r).User.ID, in.Role, time.Duration(in.ExpiresInHours*float64(time.Hour)))
	if err != nil {
		s.writeErr(w, err)
		return
	}
	s.audit(r.Context(), r, "invite.create", strconv.FormatInt(inv.ID, 10), "role="+inv.Role)
	writeJSON(w, http.StatusCreated, map[string]any{
		"token": token, "path": "/signup?invite=" + token, // shown once; only the hash is stored
		"invite": inviteJSON{ID: inv.ID, Role: inv.Role, CreatedBy: principal(r).User.Username, CreatedAt: time.Unix(inv.CreatedAt, 0).UTC(), ExpiresAt: time.Unix(inv.ExpiresAt, 0).UTC()},
	})
}

func (s *Server) handleAdminDeleteInvite(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	n, err := s.db.Q.DeleteInvite(r.Context(), id)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "invite not found", "")
		return
	}
	s.audit(r.Context(), r, "invite.revoke", strconv.FormatInt(id, 10), "")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminSettingsBody(ctx context.Context) (map[string]any, error) {
	ov, err := s.settings.Overrides(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"settings": s.settings.Get(), "defaults": s.settings.Defaults(), "overrides": ov}, nil
}

func (s *Server) handleAdminSettings(w http.ResponseWriter, r *http.Request) {
	body, err := s.adminSettingsBody(r.Context())
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, body)
}

func (s *Server) handleAdminUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var patch map[string]json.RawMessage
	if err := decodeJSON(r, &patch); err != nil {
		s.writeErr(w, err)
		return
	}
	if _, err := s.settings.Update(r.Context(), patch); err != nil {
		s.writeErr(w, err)
		return
	}
	keys := make([]string, 0, len(patch))
	for k := range patch {
		keys = append(keys, k)
	}
	b, _ := json.Marshal(keys)
	s.audit(r.Context(), r, "settings.update", "", string(b))
	s.handleAdminSettings(w, r)
}

func (s *Server) handleAdminHealth(w http.ResponseWriter, r *http.Request) {
	statuses := s.health.Snapshot()
	feeds := make([]map[string]any, 0, len(statuses))
	for _, st := range statuses {
		b, _ := json.Marshal(st)
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		m["history"] = s.health.History(st.Name)
		feeds = append(feeds, m)
	}
	_, vehicles := s.store.Snapshot()
	out := map[string]any{"feeds": feeds, "vehicles": len(vehicles), "server_time": time.Now()}
	if s.staticStatus != nil {
		out["static"] = s.staticStatus()
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleAdminRefreshStatic(w http.ResponseWriter, r *http.Request) {
	if s.refreshStatic == nil {
		writeError(w, http.StatusNotImplemented, "static import not configured", "")
		return
	}
	s.audit(r.Context(), r, "static.refresh", "", "")
	// The import outlives this request (a proxy timeout or a closed tab must
	// not abort it), and a second click joins the running import.
	s.refreshMu.Lock()
	run := s.refreshRun
	if run == nil {
		run = &refreshRun{done: make(chan struct{})}
		s.refreshRun = run
		go func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 10*time.Minute)
			defer cancel()
			run.err = s.refreshStatic(ctx)
			s.refreshMu.Lock()
			s.refreshRun = nil
			s.refreshMu.Unlock()
			close(run.done)
		}()
	}
	s.refreshMu.Unlock()
	select {
	case <-run.done:
	case <-r.Context().Done():
		return
	}
	if run.err != nil {
		writeError(w, http.StatusBadGateway, "import failed: "+run.err.Error(), "")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"static": s.staticStatus()})
}

// refreshRun is an admin-started static import in progress.
type refreshRun struct {
	done chan struct{}
	err  error
}

func (s *Server) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	rows, err := s.db.Q.ListAudit(r.Context(), db.ListAuditParams{Action: r.URL.Query().Get("action"), BeforeID: before, Lim: int64(limitParam(r, 50, 500))})
	if err != nil {
		s.writeErr(w, err)
		return
	}
	type entry struct {
		ID     int64     `json:"id"`
		At     time.Time `json:"at"`
		Actor  string    `json:"actor"`
		Action string    `json:"action"`
		Target string    `json:"target"`
		Detail string    `json:"detail"`
	}
	out := make([]entry, 0, len(rows))
	for _, e := range rows {
		out = append(out, entry{e.ID, time.Unix(e.At, 0).UTC(), e.Actor, e.Action, e.Target, e.Detail})
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": out})
}

func fileSize(p string) int64 {
	if st, err := os.Stat(p); err == nil {
		return st.Size()
	}
	return 0
}

func (s *Server) handleAdminAbout(w http.ResponseWriter, r *http.Request) {
	dbPath := filepath.Join(s.dataDir, "bus.db")
	users, _ := s.db.Q.CountUsers(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"version": buildinfo.Version, "commit": buildinfo.Commit, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH,
		"uptime_s": int64(time.Since(s.started).Seconds()), "db_bytes": fileSize(dbPath) + fileSize(dbPath+"-wal"),
		"users": users, "data_dir": s.dataDir,
	})
}

// handleAdminBackup streams a consistent copy of the database (SQLite VACUUM INTO).
func (s *Server) handleAdminBackup(w http.ResponseWriter, r *http.Request) {
	tmp := filepath.Join(s.dataDir, fmt.Sprintf(".backup-%d.db", time.Now().UnixNano()))
	defer os.Remove(tmp)
	if _, err := s.db.DB.ExecContext(r.Context(), "VACUUM INTO ?", tmp); err != nil {
		s.writeErr(w, err)
		return
	}
	f, err := os.Open(tmp)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	defer f.Close()
	s.audit(r.Context(), r, "backup.download", "", "")
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="bus-%s.db"`, time.Now().Format("20060102-150405")))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.Copy(w, f)
}

func (s *Server) mountAdmin(r chi.Router) {
	if s.auth == nil || s.db == nil || s.settings == nil {
		return
	}
	r.Route("/admin", func(r chi.Router) {
		r.Use(s.requireAdmin)
		r.Get("/users", s.handleAdminUsers)
		r.Post("/users", s.handleAdminCreateUser)
		r.Patch("/users/{id}", s.handleAdminUpdateUser)
		r.Post("/users/{id}/password", s.handleAdminResetPassword)
		r.Delete("/users/{id}/sessions", s.handleAdminRevokeSessions)
		r.Delete("/users/{id}", s.handleAdminDeleteUser)
		r.Get("/invites", s.handleAdminInvites)
		r.Post("/invites", s.handleAdminCreateInvite)
		r.Delete("/invites/{id}", s.handleAdminDeleteInvite)
		r.Get("/settings", s.handleAdminSettings)
		r.Patch("/settings", s.handleAdminUpdateSettings)
		r.Get("/health", s.handleAdminHealth)
		r.Post("/static/refresh", s.handleAdminRefreshStatic)
		r.Get("/audit", s.handleAdminAudit)
		r.Get("/about", s.handleAdminAbout)
		r.Get("/backup", s.handleAdminBackup)
		r.Get("/tokens", s.handleAdminTokens)
		r.Delete("/tokens/{id}", s.handleAdminRevokeToken)
	})
}
