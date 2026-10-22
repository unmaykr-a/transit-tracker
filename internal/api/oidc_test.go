package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-jose/go-jose/v4"

	"github.com/unmaykr-a/transit-tracker/internal/auth"
	"github.com/unmaykr-a/transit-tracker/internal/config"
	"github.com/unmaykr-a/transit-tracker/internal/ingest"
	"github.com/unmaykr-a/transit-tracker/internal/settings"
	"github.com/unmaykr-a/transit-tracker/internal/store"
)

// fakeIdP is a minimal OpenID provider: discovery, JWKS and a token endpoint
// that checks the PKCE verifier and returns an RS256-signed ID token.
type fakeIdP struct {
	t      *testing.T
	srv    *httptest.Server
	key    *rsa.PrivateKey
	mu     sync.Mutex
	codes  map[string]idpGrant // code → grant
	client string
}

type idpGrant struct {
	claims    map[string]any
	challenge string
}

func newFakeIdP(t *testing.T, clientID string) *fakeIdP {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{t: t, key: key, codes: map[string]idpGrant{}, client: clientID}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": f.srv.URL, "authorization_endpoint": f.srv.URL + "/authorize", "token_endpoint": f.srv.URL + "/token",
			"jwks_uri": f.srv.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		f.mu.Lock()
		g, ok := f.codes[r.PostForm.Get("code")]
		delete(f.codes, r.PostForm.Get("code"))
		f.mu.Unlock()
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 300, "id_token": f.sign(g.claims)})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeIdP) sign(claims map[string]any) string {
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	if err != nil {
		f.t.Fatal(err)
	}
	b, _ := json.Marshal(claims)
	obj, err := signer.Sign(b)
	if err != nil {
		f.t.Fatal(err)
	}
	s, _ := obj.CompactSerialize()
	return s
}

// authorize plays the user approving the sign-in: it reads the authorization
// request and returns the callback URL our server would be sent to.
func (f *fakeIdP) authorize(authURL, callback string, claims map[string]any) string {
	u, err := url.Parse(authURL)
	if err != nil || !strings.HasPrefix(authURL, f.srv.URL+"/authorize") {
		f.t.Fatalf("not an authorization URL: %q", authURL)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || q.Get("client_id") != f.client || !strings.Contains(q.Get("scope"), "openid") {
		f.t.Fatalf("authorization request %v", q)
	}
	full := map[string]any{"iss": f.srv.URL, "aud": f.client, "exp": time.Now().Add(5 * time.Minute).Unix(), "iat": time.Now().Unix(), "nonce": q.Get("nonce")}
	for k, v := range claims {
		full[k] = v
	}
	code := auth.NewToken("")
	f.mu.Lock()
	f.codes[code] = idpGrant{claims: full, challenge: q.Get("code_challenge")}
	f.mu.Unlock()
	return callback + "?state=" + url.QueryEscape(q.Get("state")) + "&code=" + code
}

func oidcServer(t *testing.T, autoCreate bool) (*httptest.Server, *fakeIdP, func() *client) {
	t.Helper()
	ctx := context.Background()
	idp := newFakeIdP(t, "bus-app")
	dir := t.TempDir()
	st, err := store.Open(ctx, dir, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	set, _ := settings.New(ctx, st, testSettings())
	cfg := config.OIDCConfig{Issuer: idp.srv.URL, ClientID: "bus-app", ClientSecret: "s3cret", AdminGroup: "bus-admins", Name: "Authentik", AutoCreate: autoCreate}
	// The redirect URL needs the server's address, so start it first and set the handler after.
	var handler http.Handler
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { handler.ServeHTTP(w, r) }))
	handler = NewServer(slog.New(slog.DiscardHandler), Deps{
		Health: ingest.NewHealth(), Auth: auth.NewService(st), Settings: set, DB: st, DataDir: dir,
		OIDC: auth.NewOIDC(cfg, srv.URL+"/api/v1/auth/oidc/callback"), OIDCConfig: cfg,
	}).Handler(SPA(fstest.MapFS{}))
	t.Cleanup(srv.Close)
	newClient := func() *client {
		jar, _ := cookiejar.New(nil)
		return &client{t: t, base: srv.URL, http: &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	}
	return srv, idp, newClient
}

// signIn runs the browser side of the flow and returns where the user lands.
func signIn(t *testing.T, c *client, idp *fakeIdP, next string, claims map[string]any) string {
	t.Helper()
	res, err := c.http.Get(c.base + "/api/v1/auth/oidc/login?next=" + url.QueryEscape(next))
	if err != nil || res.StatusCode != http.StatusFound {
		t.Fatalf("login start: %v %v", err, res)
	}
	cb := idp.authorize(res.Header.Get("Location"), c.base+"/api/v1/auth/oidc/callback", claims)
	res, err = c.http.Get(cb)
	if err != nil || res.StatusCode != http.StatusFound {
		t.Fatalf("callback: %v %v", err, res)
	}
	return res.Header.Get("Location")
}

func TestOIDCSignIn(t *testing.T) {
	_, idp, newClient := oidcServer(t, true)
	admin := newClient()
	if code, _ := admin.do("POST", "/api/v1/auth/setup", map[string]string{"username": "andri", "password": "hunter2hunter2"}); code != 200 {
		t.Fatal("setup")
	}
	if _, s := newClient().do("GET", "/api/v1/session", nil); s["sso"].(map[string]any)["name"] != "Authentik" {
		t.Errorf("session sso = %v", s["sso"])
	}

	// First sign-in creates the user from preferred_username, admin via the group claim.
	c := newClient()
	claims := map[string]any{"sub": "u-123", "preferred_username": "Mari Maasikas", "email": "mari@example.com", "email_verified": true, "groups": []string{"bus-admins"}}
	if to := signIn(t, c, idp, "/map", claims); to != "/map" {
		t.Errorf("landed on %q", to)
	}
	_, me := c.do("GET", "/api/v1/me", nil)
	u := me["user"].(map[string]any)
	if u["username"] != "Mari-Maasikas" || u["role"] != "admin" || u["email"] != "mari@example.com" || u["has_password"] != false {
		t.Errorf("created user = %v", u)
	}

	// Same subject again, without the group: same account, demoted (another admin exists).
	c2 := newClient()
	claims["groups"] = []string{"staff"}
	signIn(t, c2, idp, "//evil.example/", claims)
	_, me = c2.do("GET", "/api/v1/me", nil)
	if u2 := me["user"].(map[string]any); u2["id"] != u["id"] || u2["role"] != "user" {
		t.Errorf("second sign-in = %v", u2)
	}
	// A username clash gets a suffix; open redirects are refused.
	c3 := newClient()
	if to := signIn(t, c3, idp, "//evil.example/", map[string]any{"sub": "u-999", "preferred_username": "Mari Maasikas"}); to != "/" {
		t.Errorf("open redirect allowed: %q", to)
	}
	if _, me := c3.do("GET", "/api/v1/me", nil); me["user"].(map[string]any)["username"] != "Mari-Maasikas-2" {
		t.Errorf("clash = %v", me["user"])
	}

	// SSO accounts cannot use the password form until they set a password.
	if code, _ := newClient().do("POST", "/api/v1/auth/login", map[string]string{"username": "Mari-Maasikas", "password": "anything-at-all"}); code != 401 {
		t.Errorf("password login for SSO account = %d", code)
	}
	if code, _ := c2.do("POST", "/api/v1/me/password", map[string]string{"current_password": "", "new_password": "a-new-password-1"}); code/100 != 2 {
		t.Errorf("SSO user setting a password = %d", code)
	}
	if code, _ := newClient().do("POST", "/api/v1/auth/login", map[string]string{"username": "Mari-Maasikas", "password": "a-new-password-1"}); code != 200 {
		t.Errorf("password login after setting one = %d", code)
	}
}

func TestOIDCFailures(t *testing.T) {
	_, idp, newClient := oidcServer(t, false)
	c := newClient()
	// Unknown user with auto-creation off.
	if to := signIn(t, c, idp, "/", map[string]any{"sub": "nobody"}); to != "/login?error=sso_no_account" {
		t.Errorf("no account → %q", to)
	}
	// A tampered state, a missing flow cookie and a provider error.
	res, _ := c.http.Get(c.base + "/api/v1/auth/oidc/login")
	cb := idp.authorize(res.Header.Get("Location"), c.base+"/api/v1/auth/oidc/callback", map[string]any{"sub": "x"})
	res, _ = c.http.Get(strings.Replace(cb, "state=", "state=x", 1))
	if loc := res.Header.Get("Location"); loc != "/login?error=sso_failed" {
		t.Errorf("bad state → %q", loc)
	}
	res, _ = newClient().http.Get(cb)
	if loc := res.Header.Get("Location"); loc != "/login?error=sso_expired" {
		t.Errorf("no cookie → %q", loc)
	}
	res, _ = c.http.Get(c.base + "/api/v1/auth/oidc/callback?error=access_denied")
	if loc := res.Header.Get("Location"); loc != "/login?error=sso_denied" {
		t.Errorf("denied → %q", loc)
	}
}

func TestFlowCookie(t *testing.T) {
	now := time.Now()
	v := signFlow(auth.Flow{State: "s", Nonce: "n", Verifier: "v", Next: "/x"}, now)
	if f, err := readFlow(v, now); err != nil || f.Next != "/x" {
		t.Fatalf("round trip: %v %v", f, err)
	}
	if _, err := readFlow(v, now.Add(11*time.Minute)); err == nil {
		t.Error("expired flow accepted")
	}
	if _, err := readFlow(strings.Replace(v, ".", "x.", 1), now); err == nil {
		t.Error("tampered flow accepted")
	}
	for in, want := range map[string]string{
		"/map?x=1": "/map?x=1", "//evil": "/", "https://evil": "/", "/\\evil": "/", "": "/",
		"/\t/evil.com": "/", "/\n/evil.com": "/", "/a\\b": "/", "/stops/a1?p=x#y": "/stops/a1?p=x#y",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q", in, got)
		}
	}
}

// startLink runs the browser side of linking for a signed-in client and returns where it lands.
func startLink(t *testing.T, c *client, idp *fakeIdP, claims map[string]any) string {
	t.Helper()
	code, out := c.do("POST", "/api/v1/me/identities/oidc", nil)
	if code != 200 {
		t.Fatalf("link start = %d %v", code, out)
	}
	res, err := c.http.Get(idp.authorize(out["url"].(string), c.base+"/api/v1/auth/oidc/callback", claims))
	if err != nil || res.StatusCode != http.StatusFound {
		t.Fatalf("link callback: %v %v", err, res)
	}
	return res.Header.Get("Location")
}

func TestOIDCLinkExistingAccount(t *testing.T) {
	_, idp, newClient := oidcServer(t, true)
	admin := newClient()
	if code, _ := admin.do("POST", "/api/v1/auth/setup", map[string]string{"username": "andri", "password": "hunter2hunter2"}); code != 200 {
		t.Fatal("setup")
	}
	adminID := func() any { _, me := admin.do("GET", "/api/v1/me", nil); return me["user"].(map[string]any)["id"] }()

	// The admin links their provider account; it is not in the admin group, so they are warned.
	andri := map[string]any{"sub": "andri-sso", "preferred_username": "andri.m", "email": "andri@example.com", "groups": []string{"staff"}}
	if to := startLink(t, admin, idp, andri); to != "/settings?sso=linked&warn=admin_group" {
		t.Errorf("link landed on %q", to)
	}
	if to := startLink(t, admin, idp, andri); to != "/settings?sso=linked&warn=admin_group" { // linking again is harmless
		t.Errorf("relink landed on %q", to)
	}
	_, ids := admin.do("GET", "/api/v1/me/identities", nil)
	if l := ids["identities"].([]any); len(l) != 1 || l[0].(map[string]any)["email"] != "andri@example.com" {
		t.Errorf("identities = %v", ids)
	}

	// Single sign-on now opens the existing admin account (still admin: it is the last one).
	c := newClient()
	if to := signIn(t, c, idp, "/admin/users", andri); to != "/admin/users" {
		t.Errorf("sign-in landed on %q", to)
	}
	if _, me := c.do("GET", "/api/v1/me", nil); me["user"].(map[string]any)["id"] != adminID || me["user"].(map[string]any)["role"] != "admin" {
		t.Errorf("signed in as %v", me["user"])
	}

	// An identity that already has its own account cannot be linked to another.
	mari := map[string]any{"sub": "mari-sso", "preferred_username": "mari"}
	m := newClient()
	signIn(t, m, idp, "/", mari)
	m.do("GET", "/api/v1/me", nil) // picks up the CSRF token
	if to := startLink(t, admin, idp, mari); to != "/settings?error=sso_link_taken" {
		t.Errorf("taken identity → %q", to)
	}

	// The link only completes in the browser session that started it.
	code, out := admin.do("POST", "/api/v1/me/identities/oidc", nil)
	if code != 200 {
		t.Fatal(code)
	}
	cb := idp.authorize(out["url"].(string), admin.base+"/api/v1/auth/oidc/callback", map[string]any{"sub": "intruder"})
	other := newClient()
	u, _ := url.Parse(admin.base + "/api/v1/auth/oidc/callback")
	for _, ck := range admin.http.Jar.Cookies(u) {
		if ck.Name == oidcCookie { // the flow cookie, but not the session
			other.http.Jar.SetCookies(u, []*http.Cookie{ck})
		}
	}
	res, _ := other.http.Get(cb)
	if loc := res.Header.Get("Location"); loc != "/settings?error=sso_link_session" {
		t.Errorf("foreign session → %q", loc)
	}

	// Anonymous requests cannot start a link (API tokens are refused by requireSession too).
	if code, _ := newClient().do("POST", "/api/v1/me/identities/oidc", nil); code != 401 {
		t.Errorf("anonymous link = %d", code)
	}

	// Unlinking: fine with a password; refused for the only sign-in of an SSO-only account.
	if code, _ := admin.do("POST", "/api/v1/me/identities/unlink", map[string]string{"issuer": idp.srv.URL, "subject": "andri-sso"}); code != 204 {
		t.Errorf("unlink = %d", code)
	}
	if code, _ := admin.do("POST", "/api/v1/me/identities/unlink", map[string]string{"issuer": idp.srv.URL, "subject": "mari-sso"}); code != 404 {
		t.Errorf("unlinking someone else's identity = %d", code)
	}
	if code, _ := m.do("POST", "/api/v1/me/identities/unlink", map[string]string{"issuer": idp.srv.URL, "subject": "mari-sso"}); code != 409 {
		t.Errorf("unlinking the only sign-in = %d", code)
	}
	if c2 := newClient(); signIn(t, c2, idp, "/", andri) == "/" {
		if _, me := c2.do("GET", "/api/v1/me", nil); me["user"].(map[string]any)["id"] == adminID {
			t.Error("unlinked identity still signs in to the admin account")
		}
	}
}
