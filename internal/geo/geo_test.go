package geo

import (
	"math"
	"testing"
)

func TestDistanceAndBearing(t *testing.T) {
	// Balti jaam → Viru väljak, about 1.25 km roughly east-south-east.
	d := DistanceM(59.4402, 24.7372, 59.4370, 24.7536)
	if math.Abs(d-1000) > 100 {
		t.Errorf("distance = %.0f m", d)
	}
	b := BearingDeg(59.4402, 24.7372, 59.4370, 24.7536)
	if b < 100 || b > 125 {
		t.Errorf("bearing = %.1f", b)
	}
	if BearingDeg(59, 24, 60, 24) != 0 {
		t.Error("due north must be 0")
	}
	if got := BearingDeg(59, 24, 58, 24); math.Abs(got-180) > 1e-9 {
		t.Errorf("due south = %v", got)
	}
	if DistanceM(59, 24, 59, 24) != 0 {
		t.Error("zero distance")
	}
}
