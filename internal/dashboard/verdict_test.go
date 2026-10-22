package dashboard

import (
	"testing"
	"time"
)

func TestJudgeBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		inMin, walk, buffer int
		extraSec            int
		want                Verdict
	}{
		{20, 6, 1, 0, Verdict{LevelNeutral, 13}},
		{12, 6, 1, 0, Verdict{LevelNeutral, 5}}, // boundary: 5 is neutral
		{11, 6, 1, 0, Verdict{LevelGreen, 4}},
		{8, 6, 1, 0, Verdict{LevelGreen, 1}},  // boundary: 1 is green
		{7, 6, 1, 0, Verdict{LevelAmber, 0}},  // exactly enough: leave now
		{7, 6, 1, 59, Verdict{LevelAmber, 0}}, // countdowns floor: 7 min 59 s is "7 min"
		{6, 6, 1, 0, Verdict{LevelRed, -1}},
		{0, 0, 0, 30, Verdict{LevelAmber, 0}}, // departing within the minute, already there
		{0, 0, 0, -30, Verdict{LevelRed, -1}}, // already gone
	} {
		dep := now.Add(time.Duration(tt.inMin)*time.Minute + time.Duration(tt.extraSec)*time.Second)
		if got := Judge(dep, now, tt.walk, tt.buffer); got != tt.want {
			t.Errorf("in %dm%ds walk %d buffer %d → %+v, want %+v", tt.inMin, tt.extraSec, tt.walk, tt.buffer, got, tt.want)
		}
	}
}
