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
)

func newTestServer(t *testing.T, web fstest.MapFS) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(NewServer(slog.New(slog.DiscardHandler)).Handler(SPA(web)))
	t.Cleanup(srv.Close)
	return srv
}

var builtWeb = fstest.MapFS{
	"index.html":           {Data: []byte("<!doctype html><title>Tram</title>")},
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
		{"/", "<title>Tram</title>", "no-cache", 200},
		{"/index.html", "<title>Tram</title>", "no-cache", 200},
		{"/map", "<title>Tram</title>", "no-cache", 200},           // SPA route
		{"/lines/tram/T4", "<title>Tram</title>", "no-cache", 200}, // nested SPA route
		{"/assets/app-abc.js", "console.log(1)", "public, max-age=31536000, immutable", 200},
		{"/manifest.webmanifest", "{}", "no-cache", 200},
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
