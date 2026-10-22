// Package dashboard builds the "My lines" view: next departures of each saved
// line with walking time and a can-I-make-it verdict.
package dashboard

import "time"

// Level is the verdict chip colour (PLAN.md, "Can I make it?").
type Level string

const (
	LevelNeutral Level = "neutral" // slack ≥ 5: plenty of time
	LevelGreen   Level = "green"   // slack 1–4: leave soon
	LevelAmber   Level = "amber"   // slack 0: leave now
	LevelRed     Level = "red"     // slack < 0: too tight
	LevelNone    Level = "none"    // no origin, so no walking time
)

// Verdict for one departure.
type Verdict struct {
	Level    Level `json:"level"`
	SlackMin int   `json:"slack_min"` // minutes to spare before leaving; negative = too late
}

// MinutesUntil is whole minutes until t, floored, as a countdown shows it.
func MinutesUntil(t, now time.Time) int {
	d := t.Sub(now)
	if d < 0 {
		return -int((-d + time.Minute - 1) / time.Minute)
	}
	return int(d / time.Minute)
}

// Judge computes slack = minutes until departure − walk − buffer and its level.
func Judge(departure, now time.Time, walkMin, bufferMin int) Verdict {
	slack := MinutesUntil(departure, now) - walkMin - bufferMin
	switch {
	case slack >= 5:
		return Verdict{LevelNeutral, slack}
	case slack >= 1:
		return Verdict{LevelGreen, slack}
	case slack == 0:
		return Verdict{LevelAmber, 0}
	default:
		return Verdict{LevelRed, slack}
	}
}
