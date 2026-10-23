package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/unmaykr-a/transit-tracker/internal/store"
	"github.com/unmaykr-a/transit-tracker/internal/store/db"
)

// Roles.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrRateLimited        = errors.New("too many failed attempts, try again later")
	ErrSetupDone          = errors.New("setup has already been completed")
	ErrUsernameTaken      = errors.New("username is taken")
	ErrInviteInvalid      = errors.New("invite is invalid, used or expired")
	ErrSignupClosed       = errors.New("sign-up is closed")
	ErrLastAdmin          = errors.New("cannot remove the last active admin")
)

// ValidationError is a user-facing input problem.
type ValidationError struct{ Field, Msg string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Msg }

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{2,31}$`)

// ValidateUsername enforces 3–32 characters of letters, digits, '.', '_' and '-'.
func ValidateUsername(u string) error {
	if !usernameRe.MatchString(u) {
		return &ValidationError{"username", "use 3–32 letters, digits, dots, dashes or underscores"}
	}
	return nil
}

// ValidatePassword enforces a minimum length; long passphrases are welcome.
func ValidatePassword(p string) error {
	if n := utf8.RuneCountInString(p); n < 8 || n > 256 {
		return &ValidationError{"password", "use at least 8 characters"}
	}
	return nil
}

// ValidateRole accepts "admin" or "user".
func ValidateRole(r string) error {
	if r != RoleAdmin && r != RoleUser {
		return &ValidationError{"role", "must be admin or user"}
	}
	return nil
}

// Service implements account operations on top of the store.
type Service struct {
	Store      *store.Store
	Limiter    *Limiter
	SessionTTL time.Duration
	Now        func() time.Time
}

func NewService(st *store.Store) *Service {
	return &Service{Store: st, Limiter: NewLimiter(5, 15*time.Minute), SessionTTL: 30 * 24 * time.Hour}
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Principal is the authenticated user of a request.
type Principal struct {
	User      db.User
	SessionID string // hashed session id; empty for API-token requests
	CSRF      string
	TokenID   int64 // API token behind the request; 0 for sessions
	// Extended is set when this request slid the session's expiry forward, so
	// the cookie's lifetime should be renewed to match.
	Extended bool
}

// ViaToken reports whether the request authenticated with an API token.
func (p *Principal) ViaToken() bool { return p != nil && p.TokenID != 0 }

func (p *Principal) IsAdmin() bool { return p != nil && p.User.Role == RoleAdmin }

// NeedsSetup reports whether no user exists yet.
func (s *Service) NeedsSetup(ctx context.Context) (bool, error) {
	n, err := s.Store.Q.CountUsers(ctx)
	return n == 0, err
}

// newUser is a validated user with its password already hashed.
type newUser struct{ username, email, hash, role string }

// prepareUser validates input and hashes the password. Hashing takes tens of
// milliseconds (and may wait for a free slot), so it never runs inside a
// transaction: that held the database write lock for the whole hash, and
// under concurrent sign-ups other writes timed out with SQLITE_BUSY.
func prepareUser(username, email, password, role string) (newUser, error) {
	username = strings.TrimSpace(username)
	if err := ValidateUsername(username); err != nil {
		return newUser{}, err
	}
	if err := ValidatePassword(password); err != nil {
		return newUser{}, err
	}
	if err := ValidateRole(role); err != nil {
		return newUser{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return newUser{}, err
	}
	return newUser{username, strings.TrimSpace(email), hash, role}, nil
}

func (s *Service) insertUser(ctx context.Context, q *db.Queries, n newUser) (db.User, error) {
	if _, err := q.GetUserByUsername(ctx, n.username); err == nil {
		return db.User{}, ErrUsernameTaken
	} else if !errors.Is(err, sql.ErrNoRows) {
		return db.User{}, err
	}
	return q.CreateUser(ctx, db.CreateUserParams{Username: n.username, Email: n.email, PasswordHash: n.hash, Role: n.role, CreatedAt: s.now().Unix()})
}

// CreateUser validates input and inserts a user (q must not be inside a transaction: the password is hashed first).
func (s *Service) CreateUser(ctx context.Context, q *db.Queries, username, email, password, role string) (db.User, error) {
	if err := s.usernameFree(ctx, q, username); err != nil {
		return db.User{}, err
	}
	n, err := prepareUser(username, email, password, role)
	if err != nil {
		return db.User{}, err
	}
	return s.insertUser(ctx, q, n)
}

// usernameFree is a cheap early check, so a taken name costs no password hash (insertUser checks again).
func (s *Service) usernameFree(ctx context.Context, q *db.Queries, username string) error {
	if _, err := q.GetUserByUsername(ctx, strings.TrimSpace(username)); err == nil {
		return ErrUsernameTaken
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	return nil
}

// Setup creates the first admin; it fails once any user exists.
func (s *Service) Setup(ctx context.Context, username, email, password string) (db.User, error) {
	if done, err := s.NeedsSetup(ctx); err != nil {
		return db.User{}, err
	} else if !done {
		return db.User{}, ErrSetupDone
	}
	n, err := prepareUser(username, email, password, RoleAdmin)
	if err != nil {
		return db.User{}, err
	}
	var u db.User
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		count, err := q.CountUsers(ctx)
		if err != nil {
			return err
		}
		if count > 0 {
			return ErrSetupDone
		}
		u, err = s.insertUser(ctx, q, n)
		return err
	})
	return u, err
}

// Signup creates a user through an invite or, when open sign-up is on, without one.
func (s *Service) Signup(ctx context.Context, mode, inviteToken, username, email, password string) (db.User, error) {
	// Check the invite before spending a password hash on the request; the
	// transaction checks it again and claims it atomically.
	role := RoleUser
	if inviteToken != "" {
		r, err := s.CheckInvite(ctx, inviteToken)
		if err != nil {
			return db.User{}, err
		}
		role = r
	} else if mode != "open" {
		return db.User{}, ErrSignupClosed
	}
	if err := s.usernameFree(ctx, s.Store.Q, username); err != nil {
		return db.User{}, err
	}
	n, err := prepareUser(username, email, password, role)
	if err != nil {
		return db.User{}, err
	}
	var u db.User
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		var invite *db.Invite
		if inviteToken != "" {
			inv, err := q.GetInviteByHash(ctx, HashToken(inviteToken))
			if errors.Is(err, sql.ErrNoRows) || (err == nil && (inv.UsedAt.Valid || inv.ExpiresAt <= s.now().Unix() || inv.Role != n.role)) {
				return ErrInviteInvalid
			} else if err != nil {
				return err
			}
			invite = &inv
		}
		var err error
		if u, err = s.insertUser(ctx, q, n); err != nil {
			return err
		}
		if invite != nil {
			n, err := q.UseInvite(ctx, db.UseInviteParams{UsedBy: sql.NullInt64{Int64: u.ID, Valid: true}, UsedAt: sql.NullInt64{Int64: s.now().Unix(), Valid: true}, ID: invite.ID, ExpiresAt: s.now().Unix()})
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrInviteInvalid // used concurrently
			}
		}
		return nil
	})
	return u, err
}

// Login checks credentials (rate limited per IP and per username) and starts a session.
func (s *Service) Login(ctx context.Context, username, password, ip, userAgent string) (string, *Principal, error) {
	keys := limitKeys(ip, username)
	if !s.Limiter.Reserve(keys...) { // counts this attempt as a failure until it succeeds
		return "", nil, ErrRateLimited
	}
	u, err := s.Store.Q.GetUserByUsername(ctx, strings.TrimSpace(username))
	if errors.Is(err, sql.ErrNoRows) {
		_, _ = VerifyPassword(dummyHash, password)
		return "", nil, ErrInvalidCredentials
	} else if err != nil {
		return "", nil, err
	}
	if u.PasswordHash == "" { // single sign-on account without a local password
		_, _ = VerifyPassword(dummyHash, password)
		return "", nil, ErrInvalidCredentials
	}
	ok, err := VerifyPassword(u.PasswordHash, password)
	if err != nil {
		return "", nil, err
	}
	if !ok || u.Disabled != 0 {
		return "", nil, ErrInvalidCredentials
	}
	s.Limiter.Reset(keys...)
	return s.StartSession(ctx, u, ip, userAgent)
}

// StartSession creates a session for u and returns the cookie token.
func (s *Service) StartSession(ctx context.Context, u db.User, ip, userAgent string) (string, *Principal, error) {
	token := NewToken("")
	csrf := NewToken("")
	now := s.now()
	id := HashToken(token)
	if len(userAgent) > 300 {
		userAgent = userAgent[:300]
	}
	err := s.Store.Q.CreateSession(ctx, db.CreateSessionParams{
		ID: id, UserID: u.ID, Csrf: csrf, CreatedAt: now.Unix(), LastSeenAt: now.Unix(),
		ExpiresAt: now.Add(s.SessionTTL).Unix(), UserAgent: userAgent, Ip: ip,
	})
	if err != nil {
		return "", nil, err
	}
	return token, &Principal{User: u, SessionID: id, CSRF: csrf}, nil
}

// touchEvery limits session writes to one per few minutes per session.
const touchEvery = 5 * time.Minute

// Authenticate resolves a session cookie token; nil means not logged in.
// Sessions slide: each use (at most every few minutes) extends the expiry.
func (s *Service) Authenticate(ctx context.Context, token, ip string) (*Principal, error) {
	if token == "" {
		return nil, nil
	}
	row, err := s.Store.Q.GetSessionWithUser(ctx, HashToken(token))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	now := s.now()
	if row.Session.ExpiresAt <= now.Unix() || row.User.Disabled != 0 {
		return nil, nil
	}
	extended := false
	if now.Unix()-row.Session.LastSeenAt >= int64(touchEvery.Seconds()) {
		if err := s.Store.Q.TouchSession(ctx, db.TouchSessionParams{LastSeenAt: now.Unix(), ExpiresAt: now.Add(s.SessionTTL).Unix(), Ip: ip, ID: row.Session.ID}); err != nil {
			return nil, err
		}
		extended = true
	}
	return &Principal{User: row.User, SessionID: row.Session.ID, CSRF: row.Session.Csrf, Extended: extended}, nil
}

// Logout ends the session behind token.
func (s *Service) Logout(ctx context.Context, token string) error {
	return s.Store.Q.DeleteSession(ctx, HashToken(token))
}

// ChangePassword verifies the current password (if the account has one), sets
// a new one and signs out the user's other sessions.
func (s *Service) ChangePassword(ctx context.Context, p *Principal, current, next string) error {
	if p.User.PasswordHash == "" { // single sign-on account adding a local password
		return s.SetPassword(ctx, p.User.ID, next, p.SessionID)
	}
	ok, err := VerifyPassword(p.User.PasswordHash, current)
	if err != nil {
		return err
	}
	if !ok {
		return &ValidationError{"current_password", "is incorrect"}
	}
	return s.SetPassword(ctx, p.User.ID, next, p.SessionID)
}

// SetPassword replaces a user's password and ends their sessions except keepSession.
func (s *Service) SetPassword(ctx context.Context, userID int64, password, keepSession string) error {
	if err := ValidatePassword(password); err != nil {
		return err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return s.Store.Tx(ctx, func(q *db.Queries) error {
		if err := q.UpdateUserPassword(ctx, db.UpdateUserPasswordParams{PasswordHash: hash, ID: userID}); err != nil {
			return err
		}
		return q.DeleteOtherUserSessions(ctx, db.DeleteOtherUserSessionsParams{UserID: userID, ID: keepSession})
	})
}

// CreateInvite returns the plaintext token (shown once) and the stored invite.
func (s *Service) CreateInvite(ctx context.Context, createdBy int64, role string, ttl time.Duration) (string, db.Invite, error) {
	if err := ValidateRole(role); err != nil {
		return "", db.Invite{}, err
	}
	if ttl <= 0 || ttl > 90*24*time.Hour {
		return "", db.Invite{}, &ValidationError{"expires_in_hours", "must be between 1 hour and 90 days"}
	}
	token := NewToken("")
	now := s.now()
	inv, err := s.Store.Q.CreateInvite(ctx, db.CreateInviteParams{
		TokenHash: HashToken(token), Role: role, CreatedBy: sql.NullInt64{Int64: createdBy, Valid: true},
		CreatedAt: now.Unix(), ExpiresAt: now.Add(ttl).Unix(),
	})
	return token, inv, err
}

// CheckInvite reports whether an invite token is usable and for which role.
func (s *Service) CheckInvite(ctx context.Context, token string) (string, error) {
	inv, err := s.Store.Q.GetInviteByHash(ctx, HashToken(token))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (inv.UsedAt.Valid || inv.ExpiresAt <= s.now().Unix())) {
		return "", ErrInviteInvalid
	}
	return inv.Role, err
}

// GuardLastAdmin fails when changing user would leave no active admin.
func (s *Service) GuardLastAdmin(ctx context.Context, q *db.Queries, target db.User, newRole string, disable bool) error {
	if target.Role != RoleAdmin || target.Disabled != 0 {
		return nil
	}
	if newRole == RoleAdmin && !disable {
		return nil
	}
	n, err := q.CountActiveAdmins(ctx)
	if err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastAdmin
	}
	return nil
}

// SeedAdmin creates the first admin from environment variables when there are no users.
func (s *Service) SeedAdmin(ctx context.Context, username, password string) (bool, error) {
	if username == "" || password == "" {
		return false, nil
	}
	_, err := s.Setup(ctx, username, "", password)
	if errors.Is(err, ErrSetupDone) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("seeding admin from BUS_ADMIN_USER: %w", err)
	}
	return true, nil
}
