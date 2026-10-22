package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/unmaykr-a/transit-tracker/internal/auth"
	"github.com/unmaykr-a/transit-tracker/internal/store"
)

// With accounts on (as in production), BUS_METRICS_TOKEN must still open
// /metrics: only bus_ bearers are treated as API tokens.
func TestMetricsTokenWithAccounts(t *testing.T) {
	st, err := store.Open(context.Background(), t.TempDir(), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	h := NewServer(slog.New(slog.DiscardHandler), Deps{Auth: auth.NewService(st), DB: st, MetricsToken: "s3cret"}).Handler(SPA(fstest.MapFS{}))
	do := func(method, path, bearer string) (int, string) {
		req := httptest.NewRequest(method, path, nil)
		req.RemoteAddr = "203.0.113.9:5000"
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	// Made-up methods share one label instead of creating a series each.
	do("XMETHODA", "/api/v1/version", "")
	code, body := do(http.MethodGet, "/metrics", "s3cret")
	if code != 200 {
		t.Fatalf("metrics with token = %d", code)
	}
	if strings.Contains(body, "XMETHODA") || !strings.Contains(body, `method="OTHER"`) {
		t.Errorf("method label not bounded:\n%s", body)
	}
	if code, _ := do(http.MethodGet, "/metrics", "wrong"); code != 403 {
		t.Errorf("wrong token = %d", code)
	}
	if code, _ := do(http.MethodGet, "/api/v1/me", "bus_nope"); code != 401 {
		t.Errorf("unknown API token = %d", code)
	}
}
