package feeds

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestParseAgencyRoutesFixture(t *testing.T) {
	b, err := os.ReadFile("testdata/otp_agency_routes.json")
	if err != nil {
		t.Fatal(err)
	}
	routes, err := ParseAgencyRoutes(b)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[Mode]int{}
	var t4 *OTPRoute
	for i, r := range routes {
		counts[r.CityMode()]++
		if r.ShortName == "T4" {
			t4 = &routes[i]
		}
	}
	if counts[ModeBus] != 69 || counts[ModeTram] != 6 || counts[ModeTrolley] != 5 {
		t.Errorf("mode counts = %v", counts)
	}
	if t4 == nil || t4.LongName != "Tondi - Lennujaam" || len(t4.Patterns) < 2 {
		t.Fatalf("T4 = %+v", t4)
	}
	p := t4.Patterns[0]
	if p.Geometry == nil || p.Geometry.Points == "" || p.Stops[0].Code == "" || p.Stops[0].Lat == 0 {
		t.Errorf("pattern incomplete: %+v", p)
	}
}

func TestOTPClient(t *testing.T) {
	var gotQuery map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotQuery)
		if r.Header.Get("Content-Type") != "application/json" || r.Method != http.MethodPost {
			http.Error(w, "bad", 400)
			return
		}
		if strings.Contains(gotQuery["query"].(string), "nope") {
			_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
			return
		}
		raw, _ := os.ReadFile("testdata/otp_stoptimes_rail.json")
		// Reshape the single-stop fixture into the multi-stop response.
		var one struct {
			Data struct {
				Stop map[string]any `json:"stop"`
			} `json:"data"`
		}
		_ = json.Unmarshal(raw, &one)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"stops": []any{one.Data.Stop}}})
	}))
	defer srv.Close()
	o := &OTP{URL: srv.URL, Fetcher: NewFetcher(srv.Client(), "test")}
	got, err := o.StopTimes(context.Background(), []string{"estonia:10137"}, 6, 21600)
	if err != nil {
		t.Fatal(err)
	}
	st := got["estonia:10137"]
	if len(st) == 0 || st[0].Trip.Route.ShortName != "E33" || st[0].ServiceDay == 0 || st[0].Trip.Route.Mode != "RAIL" {
		t.Fatalf("stoptimes = %+v", st)
	}
	vars := gotQuery["variables"].(map[string]any)
	if vars["n"].(float64) != 6 || vars["ids"].([]any)[0] != "estonia:10137" {
		t.Errorf("variables = %v", vars)
	}
	if err := o.query(context.Background(), "nope", nil, &struct{}{}); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("graphql error not surfaced: %v", err)
	}
}
