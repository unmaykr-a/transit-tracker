package eta

import (
	"fmt"
	"io"
	"math"
	"slices"
	"time"
)

// Pair matches SIRI's expected times for one line at one stop with our
// predictions one-to-one, closest pairs first, ignoring pairs further apart
// than tolerance (a different trip, not a prediction error). It returns
// predicted − SIRI for each pair. Unpaired SIRI times are trips we have no
// prediction for (vehicle not yet out, or unmatched): they count against
// coverage, not accuracy.
func Pair(siri, predicted []time.Time, tolerance time.Duration) []time.Duration {
	ms := PairIndex(siri, predicted, tolerance)
	out := make([]time.Duration, len(ms))
	for i, m := range ms {
		out[i] = m.Diff
	}
	return out
}

// Match is one pairing made by PairIndex.
type Match struct {
	Siri, Pred int // indexes into the inputs
	Diff       time.Duration
}

// PairIndex is Pair, keeping which inputs were paired.
func PairIndex(siri, predicted []time.Time, tolerance time.Duration) []Match {
	var cs []Match
	for i, s := range siri {
		for j, p := range predicted {
			if d := p.Sub(s); d.Abs() <= tolerance {
				cs = append(cs, Match{i, j, d})
			}
		}
	}
	slices.SortFunc(cs, func(a, b Match) int { return int(a.Diff.Abs() - b.Diff.Abs()) })
	usedS, usedP := map[int]bool{}, map[int]bool{}
	var out []Match
	for _, c := range cs {
		if usedS[c.Siri] || usedP[c.Pred] {
			continue
		}
		usedS[c.Siri], usedP[c.Pred] = true, true
		out = append(out, c)
	}
	return out
}

// Report summarises prediction errors.
type Report struct {
	Expected int // SIRI live departures considered (for coverage)
	Samples  int
	Within1m float64 // share of |error| ≤ 60 s
	Median   time.Duration
	P90      time.Duration
	Bias     time.Duration // mean signed error; positive = we predict late
}

func Summarise(diffs []time.Duration) Report {
	r := Report{Samples: len(diffs)}
	if len(diffs) == 0 {
		return r
	}
	abs := make([]time.Duration, len(diffs))
	var within int
	var sum time.Duration
	for i, d := range diffs {
		abs[i] = d.Abs()
		if abs[i] <= time.Minute {
			within++
		}
		sum += d
	}
	slices.Sort(abs)
	r.Within1m = float64(within) / float64(len(diffs))
	r.Median = abs[len(abs)/2]
	r.P90 = abs[int(math.Ceil(0.9*float64(len(abs))))-1]
	r.Bias = sum / time.Duration(len(diffs))
	return r
}

func (r Report) Print(w io.Writer, label string) {
	cov := ""
	if r.Expected > 0 {
		cov = fmt.Sprintf("  coverage %5.1f%%", 100*float64(r.Samples)/float64(r.Expected))
	}
	fmt.Fprintf(w, "%-10s samples %5d  within ±1 min %5.1f%%  median |err| %4.0fs  p90 %4.0fs  bias %+4.0fs%s\n",
		label, r.Samples, 100*r.Within1m, r.Median.Seconds(), r.P90.Seconds(), r.Bias.Seconds(), cov)
}
