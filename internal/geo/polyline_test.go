package geo

import "testing"

func TestPolylineRoundTrip(t *testing.T) {
	// Google's documented example.
	pts, err := DecodePolyline("_p~iF~ps|U_ulLnnqC_mqNvxq`@")
	if err != nil {
		t.Fatal(err)
	}
	want := [][2]float64{{-120.2, 38.5}, {-120.95, 40.7}, {-126.453, 43.252}}
	if len(pts) != 3 {
		t.Fatalf("pts = %v", pts)
	}
	for i := range want {
		if pts[i] != want[i] {
			t.Errorf("pt %d = %v, want %v", i, pts[i], want[i])
		}
	}
	if got := EncodePolyline(want); got != "_p~iF~ps|U_ulLnnqC_mqNvxq`@" {
		t.Errorf("encode = %q", got)
	}
	if _, err := DecodePolyline("_p~iF~ps|"); err == nil {
		t.Error("truncated polyline must fail")
	}
}
