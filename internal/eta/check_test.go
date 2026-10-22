package eta

import (
	"testing"
	"time"
)

func TestPairAndSummarise(t *testing.T) {
	base := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	at := func(s int) time.Time { return base.Add(time.Duration(s) * time.Second) }
	// A missing prediction for the first trip must not shift the pairing of the second.
	d := Pair([]time.Time{at(120), at(600), at(1200)}, []time.Time{at(630), at(1150), at(3000)}, 5*time.Minute)
	if len(d) != 2 || d[0] != 30*time.Second || d[1] != -50*time.Second {
		t.Fatalf("pairs = %v", d)
	}
	r := Summarise([]time.Duration{-30 * time.Second, 30 * time.Second, 90 * time.Second, -10 * time.Second})
	if r.Samples != 4 || r.Within1m != 0.75 || r.Median != 30*time.Second || r.P90 != 90*time.Second || r.Bias != 20*time.Second {
		t.Fatalf("report = %+v", r)
	}
	if Summarise(nil).Samples != 0 {
		t.Error("empty")
	}
}
