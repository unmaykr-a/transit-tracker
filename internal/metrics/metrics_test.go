package metrics

import (
	"strings"
	"testing"
)

func TestExposition(t *testing.T) {
	r := NewRegistry()
	c := r.Counter("tram_requests_total", "Requests.", "route", "code")
	c.Inc("/a", "200")
	c.Inc("/a", "200")
	c.Add(3, `/b"x`, "500")
	h := r.Histogram("tram_duration_seconds", "Durations.", []float64{0.1, 1}, "route")
	h.Observe(0.05, "/a")
	h.Observe(0.1, "/a") // le is inclusive
	h.Observe(5, "/a")
	r.GaugeFunc("tram_vehicles", "Vehicles.", func() []Sample {
		return []Sample{{Labels: []string{"mode", "tram"}, Value: 46}, {Labels: []string{"mode", "bus"}, Value: 402}}
	})
	var b strings.Builder
	r.Write(&b)
	want := `# HELP tram_requests_total Requests.
# TYPE tram_requests_total counter
tram_requests_total{route="/a",code="200"} 2
tram_requests_total{route="/b\"x",code="500"} 3
# HELP tram_duration_seconds Durations.
# TYPE tram_duration_seconds histogram
tram_duration_seconds_bucket{route="/a",le="0.1"} 2
tram_duration_seconds_bucket{route="/a",le="1"} 2
tram_duration_seconds_bucket{route="/a",le="+Inf"} 3
tram_duration_seconds_sum{route="/a"} 5.15
tram_duration_seconds_count{route="/a"} 3
# HELP tram_vehicles Vehicles.
# TYPE tram_vehicles gauge
tram_vehicles{mode="tram"} 46
tram_vehicles{mode="bus"} 402
`
	if b.String() != want {
		t.Errorf("got\n%s\nwant\n%s", b.String(), want)
	}
}
