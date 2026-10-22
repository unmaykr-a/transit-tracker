package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unmaykr-a/transit-tracker/internal/config"
)

// Providers differ on the issuer's trailing slash; either spelling in the config must work.
func TestOIDCIssuerTrailingSlash(t *testing.T) {
	for _, tc := range []struct{ served, configured string }{
		{"/application/o/bus/", "/application/o/bus"}, // Authentik
		{"/realms/home", "/realms/home/"},
		{"/realms/home", "/realms/home"},
	} {
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"issuer": srv.URL + tc.served, "authorization_endpoint": srv.URL + "/authorize",
				"token_endpoint": srv.URL + "/token", "jwks_uri": srv.URL + "/keys",
			})
		}))
		o := NewOIDC(config.OIDCConfig{Issuer: srv.URL + tc.configured, ClientID: "bus"}, "http://bus/cb")
		if _, _, err := o.load(context.Background()); err != nil {
			t.Errorf("served %q, configured %q: %v", tc.served, tc.configured, err)
		}
		srv.Close()
	}
}
