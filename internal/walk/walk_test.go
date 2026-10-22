package walk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
)

var (
	viru      = Point{59.4370, 24.7536}
	baltiJaam = Point{59.4411697, 24.7381854}
)

func fixture(t *testing.T, name string) []byte {
	b, err := os.ReadFile("../feeds/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestProviders(t *testing.T) {
	otpBody, trBody := fixture(t, "otp_plan_walk.json"), fixture(t, "transitous_plan_walk.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/otp") {
			_, _ = w.Write(otpBody)
			return
		}
		if r.URL.Query().Get("directModes") != "WALK" {
			http.Error(w, "bad", 400)
			return
		}
		_, _ = w.Write(trBody)
	}))
	defer srv.Close()
	f := feeds.NewFetcher(srv.Client(), "test")
	ctx := context.Background()

	r, err := OTP{Client: &feeds.OTP{URL: feeds.Const(srv.URL + "/otp"), Fetcher: f}}.Route(ctx, viru, baltiJaam)
	if err != nil || r.DistanceM < 1200 || r.DistanceM > 1250 || r.Approx {
		t.Fatalf("otp: %+v %v", r, err)
	}
	r, err = Transitous{URL: feeds.Const(srv.URL + "/plan"), Fetcher: f}.Route(ctx, viru, baltiJaam)
	if err != nil || r.DistanceM != 1265 || r.Provider != ProviderTransitous {
		t.Fatalf("transitous: %+v %v", r, err)
	}
	r, _ = Haversine{}.Route(ctx, viru, baltiJaam)
	if !r.Approx || r.DistanceM < 1100 || r.DistanceM > 1400 {
		t.Fatalf("haversine: %+v", r)
	}
}

type fakeProvider struct {
	name  string
	err   error
	calls atomic.Int32
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Route(context.Context, Point, Point) (Route, error) {
	f.calls.Add(1)
	if f.err != nil {
		return Route{}, f.err
	}
	return Route{DistanceM: 500, Provider: f.name}, nil
}

func TestServiceOrderFallbackAndCache(t *testing.T) {
	down := &fakeProvider{name: ProviderOTP, err: context.DeadlineExceeded}
	up := &fakeProvider{name: ProviderTransitous}
	order := []string{ProviderOTP, ProviderTransitous}
	s := NewService([]Provider{down, up}, func() []string { return order })
	ctx := context.Background()

	r, err := s.Route(ctx, viru, "stop-1", baltiJaam)
	if err != nil || r.Provider != ProviderTransitous {
		t.Fatalf("fallback: %+v %v", r, err)
	}
	// ~5 m of GPS jitter stays in the same grid cell: cached.
	jitter := Point{viru.Lat + 0.00004, viru.Lon + 0.00004}
	if _, err := s.Route(ctx, jitter, "stop-1", baltiJaam); err != nil {
		t.Fatal(err)
	}
	if up.calls.Load() != 1 || down.calls.Load() != 1 {
		t.Errorf("calls otp=%d transitous=%d, want 1/1", down.calls.Load(), up.calls.Load())
	}
	// Another stop is a separate entry.
	_, _ = s.Route(ctx, viru, "stop-2", baltiJaam)
	if up.calls.Load() != 2 {
		t.Errorf("transitous calls = %d", up.calls.Load())
	}
	// Everything down → approximate straight line, never an error.
	up.err = context.DeadlineExceeded
	r, err = s.Route(ctx, Point{59.40, 24.70}, "stop-3", baltiJaam)
	if err != nil || !r.Approx {
		t.Fatalf("all down: %+v %v", r, err)
	}
}

func TestMinutes(t *testing.T) {
	for _, tt := range []struct {
		d, speed float64
		want     int
	}{{0, 4.8, 0}, {80, 4.8, 1}, {81, 4.8, 2}, {1221, 4.8, 16}, {1221, 6, 13}, {400, 0, 5}} {
		if got := Minutes(tt.d, tt.speed); got != tt.want {
			t.Errorf("Minutes(%v, %v) = %d, want %d", tt.d, tt.speed, got, tt.want)
		}
	}
}
