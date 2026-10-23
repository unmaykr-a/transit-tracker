package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/unmaykr-a/transit-tracker/internal/auth"
	"github.com/unmaykr-a/transit-tracker/internal/ingest"
	"github.com/unmaykr-a/transit-tracker/internal/settings"
	"github.com/unmaykr-a/transit-tracker/internal/state"
	"github.com/unmaykr-a/transit-tracker/internal/store"
)

func testSettings() settings.Settings {
	return settings.Settings{
		InstanceName: "Bus", SignupMode: "invite", PublicMap: true, DefaultTheme: "system",
		MapStyleLight: "https://example.com/l", MapStyleDark: "https://example.com/d", MapCenter: [2]float64{24.75, 59.43}, MapZoom: 13,
		GPSURL: "https://example.com/gps", GPSIntervalS: 5, ElronURL: "https://example.com/elron", ElronIntervalS: 10,
		SiriURL: "https://example.com/siri", OTPURL: "https://example.com/otp", StopsURL: "https://example.com/stops",
		ElronGTFSURL: "https://example.com/gtfs", UserAgentContact: "https://example.com",
		WalkProviders: []string{"haversine"}, TransitousURL: "https://example.com/transitous", GeocoderURL: "https://example.com/geocode",
	}
}

type client struct {
	t    *testing.T
	base string
	http *http.Client
	csrf string
}

func (c *client) do(method, path string, body any, headers ...string) (int, map[string]any) {
	c.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	if csrf, ok := out["csrf"].(string); ok {
		c.csrf = csrf
	}
	return res.StatusCode, out
}

func accountServer(t *testing.T, trusted ...netip.Prefix) (*httptest.Server, func() *client, *state.Store) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, dir, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	set, err := settings.New(ctx, st, testSettings())
	if err != nil {
		t.Fatal(err)
	}
	vehicles := state.NewStore()
	srv := httptest.NewServer(NewServer(slog.New(slog.DiscardHandler), Deps{
		Store: vehicles, Health: ingest.NewHealth(), Auth: auth.NewService(st), Settings: set, DB: st, DataDir: dir, TrustedProxies: trusted,
	}).Handler(SPA(fstest.MapFS{})))
	t.Cleanup(srv.Close)
	newClient := func() *client {
		jar, _ := cookiejar.New(nil)
		return &client{t: t, base: srv.URL, http: &http.Client{Jar: jar}}
	}
	return srv, newClient, vehicles
}

func TestSetupLoginAndCSRF(t *testing.T) {
	_, newClient, _ := accountServer(t)
	admin := newClient()
	if code, s := admin.do("GET", "/api/v1/session", nil); code != 200 || s["setup_required"] != true || s["user"] != nil {
		t.Fatalf("fresh session: %d %v", code, s)
	}
	if code, _ := admin.do("GET", "/api/v1/me", nil); code != 401 {
		t.Fatalf("anonymous /me = %d", code)
	}
	if code, out := admin.do("POST", "/api/v1/auth/setup", map[string]string{"username": "x", "password": "hunter2hunter2"}); code != 400 || out["field"] != "username" {
		t.Fatalf("invalid setup: %d %v", code, out)
	}
	code, out := admin.do("POST", "/api/v1/auth/setup", map[string]string{"username": "andri", "password": "hunter2hunter2"})
	if code != 200 || admin.csrf == "" || out["user"].(map[string]any)["role"] != "admin" {
		t.Fatalf("setup: %d %v", code, out)
	}
	if code, _ := newClient().do("POST", "/api/v1/auth/setup", map[string]string{"username": "evil", "password": "hunter2hunter2"}); code != 409 {
		t.Fatalf("second setup = %d", code)
	}
	if code, s := admin.do("GET", "/api/v1/session", nil); code != 200 || s["setup_required"] != false || s["user"] == nil {
		t.Fatalf("session after setup: %v", s)
	}

	// Mutations need the CSRF token and a same-origin Origin.
	csrf := admin.csrf
	admin.csrf = ""
	if code, _ := admin.do("PATCH", "/api/v1/me", map[string]string{"email": "x@y"}); code != 403 {
		t.Fatalf("PATCH without CSRF = %d", code)
	}
	admin.csrf = csrf
	if code, _ := admin.do("PATCH", "/api/v1/me", map[string]string{"email": "a@example.com"}, "Origin", "https://evil.example"); code != 403 {
		t.Fatalf("cross-origin PATCH = %d", code)
	}
	if code, out := admin.do("PATCH", "/api/v1/me", map[string]string{"email": "a@example.com", "locale": "et"}); code != 200 || out["user"].(map[string]any)["locale"] != "et" {
		t.Fatalf("PATCH /me = %d %v", code, out)
	}

	// Logout ends the session; logging in again works.
	if code, _ := admin.do("POST", "/api/v1/auth/logout", nil); code != 204 {
		t.Fatalf("logout = %d", code)
	}
	if code, _ := admin.do("GET", "/api/v1/me", nil); code != 401 {
		t.Fatal("still logged in after logout")
	}
	admin.csrf = ""
	if code, _ := admin.do("POST", "/api/v1/auth/login", map[string]string{"username": "andri", "password": "nope"}); code != 401 {
		t.Fatalf("bad login = %d", code)
	}
	if code, _ := admin.do("POST", "/api/v1/auth/login", map[string]string{"username": "Andri", "password": "hunter2hunter2"}); code != 200 {
		t.Fatalf("login = %d", code)
	}
	for range 5 {
		newClient().do("POST", "/api/v1/auth/login", map[string]string{"username": "andri", "password": "nope"})
	}
	if code, _ := newClient().do("POST", "/api/v1/auth/login", map[string]string{"username": "andri", "password": "hunter2hunter2"}); code != 429 {
		t.Fatalf("rate limit = %d", code)
	}
}

func TestInviteSignupDisableAndAudit(t *testing.T) {
	_, newClient, _ := accountServer(t)
	admin := newClient()
	admin.do("POST", "/api/v1/auth/setup", map[string]string{"username": "andri", "password": "hunter2hunter2"})

	code, out := admin.do("POST", "/api/v1/admin/invites", map[string]any{"role": "user", "expires_in_hours": 24})
	if code != 201 || !strings.HasPrefix(out["path"].(string), "/signup?invite=") {
		t.Fatalf("create invite: %d %v", code, out)
	}
	token := out["token"].(string)

	bob := newClient()
	if code, out := bob.do("GET", "/api/v1/auth/invites/"+token, nil); code != 200 || out["role"] != "user" {
		t.Fatalf("check invite: %d %v", code, out)
	}
	if code, _ := bob.do("POST", "/api/v1/auth/signup", map[string]string{"username": "bob", "password": "hunter2hunter2"}); code != 403 {
		t.Fatalf("signup without invite = %d", code)
	}
	if code, out := bob.do("POST", "/api/v1/auth/signup", map[string]string{"invite": token, "username": "bob", "password": "hunter2hunter2"}); code != 200 {
		t.Fatalf("signup: %d %v", code, out)
	}
	if code, _ := bob.do("GET", "/api/v1/me", nil); code != 200 {
		t.Fatal("bob not logged in after signup")
	}
	if code, _ := bob.do("GET", "/api/v1/admin/users", nil); code != 403 {
		t.Fatalf("non-admin /admin = %d", code)
	}

	_, users := admin.do("GET", "/api/v1/admin/users", nil)
	var bobID string
	for _, u := range users["users"].([]any) {
		if u.(map[string]any)["username"] == "bob" {
			bobID = jsonNum(u.(map[string]any)["id"])
		}
	}
	if code, _ := admin.do("PATCH", "/api/v1/admin/users/"+bobID, map[string]any{"disabled": true}); code != 200 {
		t.Fatalf("disable bob = %d", code)
	}
	if code, _ := bob.do("GET", "/api/v1/me", nil); code != 401 {
		t.Fatal("disabled user still has a session")
	}
	if code, _ := newClient().do("POST", "/api/v1/auth/login", map[string]string{"username": "bob", "password": "hunter2hunter2"}); code != 401 {
		t.Fatal("disabled user can log in")
	}

	// The last admin cannot be demoted or disabled, nor delete themself.
	_, me := admin.do("GET", "/api/v1/me", nil)
	myID := jsonNum(me["user"].(map[string]any)["id"])
	if code, _ := admin.do("PATCH", "/api/v1/admin/users/"+myID, map[string]any{"role": "user"}); code != 409 {
		t.Fatalf("demote last admin = %d", code)
	}
	if code, _ := admin.do("DELETE", "/api/v1/admin/users/"+myID, nil); code != 409 {
		t.Fatalf("delete self = %d", code)
	}
	if code, _ := admin.do("DELETE", "/api/v1/admin/users/"+bobID, nil); code != 204 {
		t.Fatalf("delete bob = %d", code)
	}

	_, audit := admin.do("GET", "/api/v1/admin/audit", nil)
	var actions []string
	for _, e := range audit["entries"].([]any) {
		actions = append(actions, e.(map[string]any)["action"].(string))
	}
	want := "user.delete,user.disable,user.signup,invite.create,setup.complete"
	if strings.Join(actions, ",") != want {
		t.Errorf("audit = %v, want %s", actions, want)
	}
	_, filtered := admin.do("GET", "/api/v1/admin/audit?action=user.signup", nil)
	if n := len(filtered["entries"].([]any)); n != 1 {
		t.Errorf("filtered audit entries = %d", n)
	}
}

func jsonNum(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func TestPublicMapSettingAndAdminSettings(t *testing.T) {
	_, newClient, _ := accountServer(t)
	admin := newClient()
	admin.do("POST", "/api/v1/auth/setup", map[string]string{"username": "andri", "password": "hunter2hunter2"})
	anon := newClient()
	if code, _ := anon.do("GET", "/api/v1/vehicles", nil); code != 200 {
		t.Fatalf("public map on: %d", code)
	}
	if code, out := admin.do("PATCH", "/api/v1/admin/settings", map[string]any{"public_map": false, "instance_name": "Taali"}); code != 200 || out["overrides"].(map[string]any)["public_map"] != true {
		t.Fatalf("update settings: %d %v", code, out)
	}
	if code, _ := anon.do("GET", "/api/v1/vehicles", nil); code != 401 {
		t.Fatalf("public map off, anonymous = %d", code)
	}
	if code, _ := anon.do("GET", "/api/v1/search?q=x", nil); code != 401 {
		t.Fatalf("public map off, anonymous search = %d", code)
	}
	if code, s := anon.do("GET", "/api/v1/session", nil); code != 200 || s["settings"].(map[string]any)["instance_name"] != "Taali" {
		t.Fatalf("session stays public: %d %v", code, s)
	}
	if code, _ := admin.do("GET", "/api/v1/vehicles", nil); code != 200 {
		t.Fatalf("logged in, public map off = %d", code)
	}
	if code, out := admin.do("PATCH", "/api/v1/admin/settings", map[string]any{"gps_interval_s": 1}); code != 400 || out["field"] != "gps_interval_s" {
		t.Fatalf("invalid setting: %d %v", code, out)
	}
	if code, out := admin.do("PATCH", "/api/v1/admin/settings", map[string]any{"public_map": nil}); code != 200 || out["settings"].(map[string]any)["public_map"] != true {
		t.Fatalf("reset setting: %d %v", code, out)
	}
}

func TestTrustedProxyAndBackup(t *testing.T) {
	_, newClient, _ := accountServer(t, netip.MustParsePrefix("127.0.0.1/32"))
	admin := newClient()
	// Behind a trusted proxy that terminated TLS, the session cookie is Secure.
	req, _ := http.NewRequest("POST", admin.base+"/api/v1/auth/setup", strings.NewReader(`{"username":"andri","password":"hunter2hunter2"}`))
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-For", "203.0.113.7, 127.0.0.1")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	var secure bool
	for _, c := range res.Cookies() {
		if c.Name == sessionCookie {
			secure = c.Secure && c.HttpOnly && c.SameSite == http.SameSiteLaxMode
		}
	}
	if !secure {
		t.Fatal("cookie must be Secure, HttpOnly, SameSite=Lax behind an HTTPS proxy")
	}
	admin.do("POST", "/api/v1/auth/login", map[string]string{"username": "andri", "password": "hunter2hunter2"}, "X-Forwarded-For", "198.51.100.9")
	_, sessions := admin.do("GET", "/api/v1/me/sessions", nil)
	ips := []string{}
	for _, s := range sessions["sessions"].([]any) {
		ips = append(ips, s.(map[string]any)["ip"].(string))
	}
	if !strings.Contains(strings.Join(ips, ","), "198.51.100.9") {
		t.Errorf("client IP from trusted proxy not used: %v", ips)
	}

	breq, _ := http.NewRequest("GET", admin.base+"/api/v1/admin/backup", nil)
	bres, err := admin.http.Do(breq)
	if err != nil {
		t.Fatal(err)
	}
	defer bres.Body.Close()
	head := make([]byte, 16)
	_, _ = io.ReadFull(bres.Body, head)
	if bres.StatusCode != 200 || string(head) != "SQLite format 3\x00" || !strings.Contains(bres.Header.Get("Content-Disposition"), "attachment") {
		t.Fatalf("backup: %d %q", bres.StatusCode, head)
	}
	if code, about := admin.do("GET", "/api/v1/admin/about", nil); code != 200 || about["arch"] == "" || about["db_bytes"].(float64) <= 0 {
		t.Fatalf("about: %d %v", code, about)
	}
}

func TestClientMetaIgnoresUntrustedForwarding(t *testing.T) {
	s := NewServer(slog.New(slog.DiscardHandler), Deps{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}})
	for _, tt := range []struct {
		remote, xff, proto string
		wantIP             string
		wantHTTPS          bool
	}{
		{"203.0.113.1:5000", "1.2.3.4", "https", "203.0.113.1", false}, // untrusted peer: headers ignored
		{"10.0.0.2:5000", "1.2.3.4", "https", "1.2.3.4", true},
		{"10.0.0.2:5000", "1.2.3.4, 10.0.0.9", "http", "1.2.3.4", false}, // skip our own proxy chain
		{"10.0.0.2:5000", "", "", "10.0.0.2", false},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tt.remote
		r.Header.Set("X-Forwarded-For", tt.xff)
		r.Header.Set("X-Forwarded-Proto", tt.proto)
		m := s.clientMeta(r)
		if m.IP != tt.wantIP || m.HTTPS != tt.wantHTTPS {
			t.Errorf("%+v → ip=%s https=%v", tt, m.IP, m.HTTPS)
		}
	}
}

func TestHTTPSBaseURL(t *testing.T) {
	// An https:// BUS_BASE_URL makes its host HTTPS even through an untrusted
	// proxy; another host (a LAN address) stays plain.
	s := NewServer(slog.New(slog.DiscardHandler), Deps{BaseURL: "https://bus.example.org"})
	for host, want := range map[string]bool{"bus.example.org": true, "BUS.example.org": true, "192.168.1.5:8095": false} {
		r := httptest.NewRequest("GET", "http://"+host+"/", nil)
		r.RemoteAddr = "203.0.113.1:5000"
		if got := s.clientMeta(r).HTTPS; got != want {
			t.Errorf("host %s: https=%v, want %v", host, got, want)
		}
	}
	plain := NewServer(slog.New(slog.DiscardHandler), Deps{BaseURL: "http://bus.example.org"})
	if plain.clientMeta(httptest.NewRequest("GET", "http://bus.example.org/", nil)).HTTPS {
		t.Error("http:// base URL made requests HTTPS")
	}
	// The session cookie and HSTS follow.
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "http://bus.example.org/healthz", nil)
	s.Handler(http.NotFoundHandler()).ServeHTTP(rec, r)
	if rec.Header().Get("Strict-Transport-Security") == "" {
		t.Error("no HSTS behind an https base URL")
	}
}
