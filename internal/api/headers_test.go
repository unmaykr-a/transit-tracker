package api

import (
	"net/http"
	"net/netip"
	"strings"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	_, newClient, _ := accountServer(t, netip.MustParsePrefix("127.0.0.1/32"))
	c := newClient()
	get := func(path, proto string) http.Header {
		req, _ := http.NewRequest("GET", c.base+path, nil)
		if proto != "" {
			req.Header.Set("X-Forwarded-Proto", proto)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.Header
	}
	for _, path := range []string{"/", "/api/v1/session", "/healthz"} {
		h := get(path, "")
		csp := h.Get("Content-Security-Policy")
		if !strings.Contains(csp, "script-src 'self';") || !strings.Contains(csp, "frame-ancestors 'none'") {
			t.Errorf("%s: CSP = %q", path, csp)
		}
		if h.Get("X-Content-Type-Options") != "nosniff" || h.Get("X-Frame-Options") != "DENY" || h.Get("Referrer-Policy") == "" {
			t.Errorf("%s: headers = %v", path, h)
		}
		if h.Get("Strict-Transport-Security") != "" {
			t.Errorf("%s: HSTS over plain HTTP", path)
		}
	}
	if get("/", "https").Get("Strict-Transport-Security") == "" {
		t.Error("no HSTS behind an HTTPS proxy")
	}
}
