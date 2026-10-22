package feeds

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetcherConditionalGet(t *testing.T) {
	var gotUA, gotINM string
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		gotUA, gotINM = r.UserAgent(), r.Header.Get("If-None-Match")
		if gotINM == `W/"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `W/"v1"`)
		w.Header().Set("Last-Modified", "Thu, 22 Oct 2026 06:23:02 GMT")
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	f := NewFetcher(srv.Client(), "Bus/test")
	ctx := context.Background()
	r1, err := f.Get(ctx, srv.URL, true)
	if err != nil || string(r1.Body) != "payload" || r1.NotModified || r1.LastModified.IsZero() {
		t.Fatalf("first get: %+v %v", r1, err)
	}
	if gotUA != "Bus/test" || gotINM != "" {
		t.Errorf("ua=%q inm=%q", gotUA, gotINM)
	}
	r2, err := f.Get(ctx, srv.URL, true)
	if err != nil || !r2.NotModified || r2.Body != nil {
		t.Fatalf("second get: %+v %v", r2, err)
	}
	// Unconditional requests always get the body.
	r3, err := f.Get(ctx, srv.URL, false)
	if err != nil || string(r3.Body) != "payload" || gotINM != "" {
		t.Fatalf("unconditional get: %+v %v inm=%q", r3, err, gotINM)
	}
	if hits != 3 {
		t.Errorf("hits = %d", hits)
	}
}

func TestFetcherErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/big" {
			_, _ = w.Write([]byte(strings.Repeat("x", 100)))
			return
		}
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer srv.Close()
	f := NewFetcher(srv.Client(), "Bus/test")
	if _, err := f.Get(context.Background(), srv.URL+"/x", true); err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("want 502 error, got %v", err)
	}
	f.MaxBytes = 10
	if _, err := f.Get(context.Background(), srv.URL+"/big", false); err == nil {
		t.Error("want size error")
	}
}
