package auth

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/store"
	"github.com/unmaykr-a/transit-tracker/internal/store/db"
)

func TestPasswordHashing(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("hash = %s", h)
	}
	h2, _ := HashPassword("correct horse battery staple")
	if h == h2 {
		t.Error("salts must differ")
	}
	if ok, err := VerifyPassword(h, "correct horse battery staple"); !ok || err != nil {
		t.Errorf("verify correct: %v %v", ok, err)
	}
	if ok, _ := VerifyPassword(h, "wrong"); ok {
		t.Error("wrong password verified")
	}
	for _, bad := range []string{"", "$bcrypt$x", "$argon2id$v=19$m=0,t=2,p=1$AA$AA", "$argon2id$v=19$m=1,t=1,p=1$!!$AA"} {
		if _, err := VerifyPassword(bad, "x"); err == nil {
			t.Errorf("malformed hash %q accepted", bad)
		}
	}
}

func TestTokens(t *testing.T) {
	a, b := NewToken("tram_"), NewToken("tram_")
	if a == b || !strings.HasPrefix(a, "tram_") || len(a) < 40 {
		t.Fatalf("tokens %q %q", a, b)
	}
	if HashToken(a) == HashToken(b) || len(HashToken(a)) != 64 {
		t.Error("token hashes")
	}
}

func TestLimiter(t *testing.T) {
	now := time.Unix(0, 0)
	l := NewLimiter(5, 15*time.Minute)
	l.Now = func() time.Time { return now }
	for range 4 {
		l.Fail("ip:1", "user:a")
	}
	if l.Blocked("ip:1") {
		t.Fatal("blocked after 4")
	}
	l.Fail("ip:1", "user:a")
	if !l.Blocked("ip:2", "user:a") || !l.Blocked("ip:1", "user:b") {
		t.Fatal("either key at the limit must block")
	}
	now = now.Add(16 * time.Minute)
	if l.Blocked("ip:1", "user:a") {
		t.Fatal("window did not slide")
	}
	l.Fail("ip:1")
	l.Reset("ip:1")
	if l.Blocked("ip:1") {
		t.Fatal("reset")
	}
}

func newService(t *testing.T) (*Service, *time.Time) {
	t.Helper()
	st, err := store.Open(context.Background(), t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	s := NewService(st)
	s.Now = func() time.Time { return now }
	s.Limiter.Now = s.Now
	return s, &now
}

func TestSetupLoginSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	s, now := newService(t)
	if need, _ := s.NeedsSetup(ctx); !need {
		t.Fatal("fresh db needs setup")
	}
	var ve *ValidationError
	if _, err := s.Setup(ctx, "x", "", "longenough"); !errors.As(err, &ve) {
		t.Fatalf("short username: %v", err)
	}
	if _, err := s.Setup(ctx, "andri", "", "short"); !errors.As(err, &ve) {
		t.Fatalf("short password: %v", err)
	}
	admin, err := s.Setup(ctx, "andri", "a@example.com", "hunter2hunter2")
	if err != nil || admin.Role != RoleAdmin {
		t.Fatalf("setup: %+v %v", admin, err)
	}
	if _, err := s.Setup(ctx, "other", "", "hunter2hunter2"); !errors.Is(err, ErrSetupDone) {
		t.Fatalf("second setup: %v", err)
	}
	if seeded, err := s.SeedAdmin(ctx, "env", "envpassword"); seeded || err != nil {
		t.Fatalf("seed after setup: %v %v", seeded, err)
	}

	// Usernames are case-insensitive.
	token, p, err := s.Login(ctx, "ANDRI", "hunter2hunter2", "10.0.0.1", "test")
	if err != nil || p.User.ID != admin.ID || p.CSRF == "" {
		t.Fatalf("login: %v", err)
	}
	got, err := s.Authenticate(ctx, token, "10.0.0.1")
	if err != nil || got == nil || got.User.Username != "andri" || got.CSRF != p.CSRF {
		t.Fatalf("authenticate: %+v %v", got, err)
	}
	if got, _ := s.Authenticate(ctx, "bogus", ""); got != nil {
		t.Fatal("bogus token authenticated")
	}

	// Sliding expiry: use within 30 days keeps the session alive.
	*now = now.Add(29 * 24 * time.Hour)
	if got, _ := s.Authenticate(ctx, token, ""); got == nil {
		t.Fatal("session expired too early")
	}
	*now = now.Add(29 * 24 * time.Hour)
	if got, _ := s.Authenticate(ctx, token, ""); got == nil {
		t.Fatal("touch did not extend the session")
	}
	*now = now.Add(31 * 24 * time.Hour)
	if got, _ := s.Authenticate(ctx, token, ""); got != nil {
		t.Fatal("idle session must expire")
	}

	token, _, _ = s.Login(ctx, "andri", "hunter2hunter2", "10.0.0.1", "test")
	if err := s.Logout(ctx, token); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Authenticate(ctx, token, ""); got != nil {
		t.Fatal("logged-out session still valid")
	}
}

func TestLoginFailuresAndDisabled(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	u, _ := s.Setup(ctx, "andri", "", "hunter2hunter2")
	for range 5 {
		if _, _, err := s.Login(ctx, "andri", "wrong", "1.1.1.1", ""); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("want invalid credentials, got %v", err)
		}
	}
	if _, _, err := s.Login(ctx, "andri", "hunter2hunter2", "2.2.2.2", ""); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("username limit: %v", err)
	}
	if _, _, err := s.Login(ctx, "nobody", "x", "1.1.1.1", ""); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("ip limit: %v", err)
	}
	s.Limiter = NewLimiter(5, 15*time.Minute)
	token, _, err := s.Login(ctx, "andri", "hunter2hunter2", "3.3.3.3", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Store.Q.UpdateUserDisabled(ctx, db.UpdateUserDisabledParams{Disabled: 1, ID: u.ID})
	if got, _ := s.Authenticate(ctx, token, ""); got != nil {
		t.Fatal("disabled user still authenticated")
	}
	if _, _, err := s.Login(ctx, "andri", "hunter2hunter2", "3.3.3.3", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled login: %v", err)
	}
}

func TestInvitesAndSignup(t *testing.T) {
	ctx := context.Background()
	s, now := newService(t)
	admin, _ := s.Setup(ctx, "andri", "", "hunter2hunter2")
	if _, err := s.Signup(ctx, "invite", "", "bob", "", "hunter2hunter2"); !errors.Is(err, ErrSignupClosed) {
		t.Fatalf("signup without invite: %v", err)
	}
	token, inv, err := s.CreateInvite(ctx, admin.ID, RoleUser, 48*time.Hour)
	if err != nil || inv.Role != RoleUser {
		t.Fatal(err)
	}
	if role, err := s.CheckInvite(ctx, token); role != RoleUser || err != nil {
		t.Fatalf("check invite: %q %v", role, err)
	}
	bob, err := s.Signup(ctx, "off", token, "bob", "b@example.com", "hunter2hunter2")
	if err != nil || bob.Role != RoleUser {
		t.Fatalf("invited signup: %+v %v", bob, err)
	}
	if _, err := s.Signup(ctx, "off", token, "carol", "", "hunter2hunter2"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("reused invite: %v", err)
	}
	if _, err := s.Signup(ctx, "open", "", "Bob", "", "hunter2hunter2"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("duplicate username: %v", err)
	}
	if u, err := s.Signup(ctx, "open", "", "carol", "", "hunter2hunter2"); err != nil || u.Role != RoleUser {
		t.Fatalf("open signup: %v", err)
	}
	expiring, _, _ := s.CreateInvite(ctx, admin.ID, RoleAdmin, time.Hour)
	*now = now.Add(2 * time.Hour)
	if _, err := s.CheckInvite(ctx, expiring); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("expired invite: %v", err)
	}
	if _, _, err := s.CreateInvite(ctx, admin.ID, "root", time.Hour); err == nil {
		t.Error("bad role accepted")
	}
}

func TestChangePasswordEndsOtherSessions(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	_, _ = s.Setup(ctx, "andri", "", "hunter2hunter2")
	t1, p1, _ := s.Login(ctx, "andri", "hunter2hunter2", "1", "")
	t2, _, _ := s.Login(ctx, "andri", "hunter2hunter2", "2", "")
	if err := s.ChangePassword(ctx, p1, "wrong", "newpassword1"); err == nil {
		t.Fatal("wrong current password accepted")
	}
	if err := s.ChangePassword(ctx, p1, "hunter2hunter2", "newpassword1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Authenticate(ctx, t1, ""); got == nil {
		t.Error("current session must survive")
	}
	if got, _ := s.Authenticate(ctx, t2, ""); got != nil {
		t.Error("other sessions must end")
	}
	if _, _, err := s.Login(ctx, "andri", "newpassword1", "3", ""); err != nil {
		t.Errorf("login with new password: %v", err)
	}
}

func TestGuardLastAdmin(t *testing.T) {
	ctx := context.Background()
	s, _ := newService(t)
	admin, _ := s.Setup(ctx, "andri", "", "hunter2hunter2")
	if err := s.GuardLastAdmin(ctx, s.Store.Q, admin, RoleUser, false); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demote last admin: %v", err)
	}
	if err := s.GuardLastAdmin(ctx, s.Store.Q, admin, RoleAdmin, true); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("disable last admin: %v", err)
	}
	if _, err := s.CreateUser(ctx, s.Store.Q, "second", "", "hunter2hunter2", RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if err := s.GuardLastAdmin(ctx, s.Store.Q, admin, RoleUser, false); err != nil {
		t.Fatalf("with two admins: %v", err)
	}
}
