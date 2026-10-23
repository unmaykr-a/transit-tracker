package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/unmaykr-a/transit-tracker/internal/ingest"
)

func newTestServer(t *testing.T, web fstest.MapFS) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(NewServer(slog.New(slog.DiscardHandler), Deps{}).Handler(SPA(web)))
	t.Cleanup(srv.Close)
	return srv
}

var builtWeb = fstest.MapFS{
	"index.html":           {Data: []byte("<!doctype html><title>Bus</title>")},
	"assets/app-abc.js":    {Data: []byte("console.log(1)")},
	"manifest.webmanifest": {Data: []byte("{}")},
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

func TestHealthz(t *testing.T) {
	srv := newTestServer(t, builtWeb)
	res, body := get(t, srv.URL+"/healthz")
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("status %d, content-type %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	var v struct{ Status, Version string }
	if err := json.Unmarshal([]byte(body), &v); err != nil || v.Status != "ok" || v.Version == "" {
		t.Fatalf("body %q (err %v)", body, err)
	}
}

func TestRoutes(t *testing.T) {
	srv := newTestServer(t, builtWeb)
	tests := []struct {
		path, wantBody, wantCache string
		wantStatus                int
	}{
		{"/", "<title>Bus</title>", "no-cache", 200},
		{"/index.html", "<title>Bus</title>", "no-cache", 200},
		{"/map", "<title>Bus</title>", "no-cache", 200},           // SPA route
		{"/lines/tram/T4", "<title>Bus</title>", "no-cache", 200}, // nested SPA route
		{"/assets/app-abc.js", "console.log(1)", "public, max-age=31536000, immutable", 200},
		{"/manifest.webmanifest", "{}", "no-cache", 200}, // content type checked below
		{"/assets/missing.js", "404", "", 404},
		{"/api/v1/version", `"version"`, "no-store", 200},
		{"/api/v1/nope", `"not found"`, "no-store", 404}, // API never falls back to the SPA
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			res, body := get(t, srv.URL+tt.path)
			if res.StatusCode != tt.wantStatus {
				t.Errorf("status = %d, want %d", res.StatusCode, tt.wantStatus)
			}
			if !strings.Contains(body, tt.wantBody) {
				t.Errorf("body %q does not contain %q", body, tt.wantBody)
			}
			if got := res.Header.Get("Cache-Control"); got != tt.wantCache {
				t.Errorf("Cache-Control = %q, want %q", got, tt.wantCache)
			}
		})
	}
}

func TestManifestContentType(t *testing.T) {
	srv := newTestServer(t, builtWeb)
	res, _ := get(t, srv.URL+"/manifest.webmanifest")
	if ct := res.Header.Get("Content-Type"); ct != "application/manifest+json" {
		t.Errorf("manifest content type = %q", ct)
	}
}

func TestUnbuiltWeb(t *testing.T) {
	srv := newTestServer(t, fstest.MapFS{".gitkeep": {}})
	res, body := get(t, srv.URL+"/")
	if res.StatusCode != http.StatusServiceUnavailable || !strings.Contains(body, "make web") {
		t.Fatalf("status %d body %q", res.StatusCode, body)
	}
	if res, _ := get(t, srv.URL+"/healthz"); res.StatusCode != http.StatusOK {
		t.Fatalf("healthz status %d without web build", res.StatusCode)
	}
}

func TestEmbeddedDistCompiles(t *testing.T) {
	// Dist must work with only .gitkeep present (fresh clone, no web build).
	if Dist() == nil {
		t.Fatal("nil dist")
	}
}

func TestMetricsEndpoint(t *testing.T) {
	get := func(srv *Server, remote, auth string) (int, string) {
		req := httptest.NewRequest("GET", "/metrics", nil)
		req.RemoteAddr = remote
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		srv.Handler(http.NotFoundHandler()).ServeHTTP(w, req)
		return w.Code, w.Body.String()
	}
	health := ingest.NewHealth()
	srv := NewServer(slog.New(slog.DiscardHandler), Deps{Health: health})
	// A request first, so the request counter has a series.
	h := srv.Handler(http.NotFoundHandler())
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/healthz", nil))

	code, body := get(srv, "192.168.1.20:5000", "")
	if code != 200 {
		t.Fatalf("private client: %d", code)
	}
	for _, want := range []string{
		`bus_build_info{version="`,
		`bus_http_requests_total{route="/healthz",method="GET",code="200"} 1`,
		`bus_http_request_duration_seconds_bucket{route="/healthz",le="+Inf"} 1`,
		`bus_vehicles{mode="tram",state="all"} 0`,
		`bus_live_clients{transport="websocket"} 0`,
		"# TYPE go_goroutines gauge",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics missing %q", want)
		}
	}
	if code, _ := get(srv, "203.0.113.9:5000", ""); code != 403 {
		t.Errorf("public client on private mode = %d", code)
	}
	// A visitor through a reverse proxy we were not told to trust arrives from
	// the proxy's private address: not a private client.
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.RemoteAddr = "172.18.0.5:41000"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	w := httptest.NewRecorder()
	srv.Handler(http.NotFoundHandler()).ServeHTTP(w, req)
	if w.Code != 403 {
		t.Errorf("visitor via an untrusted proxy = %d", w.Code)
	}
	if code, _ := get(NewServer(slog.New(slog.DiscardHandler), Deps{MetricsMode: "public"}), "203.0.113.9:5000", ""); code != 200 {
		t.Errorf("public mode = %d", code)
	}
	if code, _ := get(NewServer(slog.New(slog.DiscardHandler), Deps{MetricsMode: "off"}), "127.0.0.1:5000", ""); code == 200 {
		t.Error("off mode served metrics")
	}
	tok := NewServer(slog.New(slog.DiscardHandler), Deps{MetricsToken: "s3cret"})
	if code, _ := get(tok, "127.0.0.1:5000", ""); code != 403 {
		t.Errorf("token mode without token = %d", code)
	}
	if code, _ := get(tok, "203.0.113.9:5000", "Bearer s3cret"); code != 200 {
		t.Errorf("token mode with token = %d", code)
	}
}
