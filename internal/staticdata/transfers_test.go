package staticdata

import (
	"sort"
	"testing"

	"github.com/unmaykr-a/transit-tracker/internal/geo"
)

func stopTimer(p *Pattern) []float64 {
	out := make([]float64, len(p.StopIDs))
	for i := range out {
		out[i] = float64(i) * 90
	}
	return out
}

// Every option is a real trip: each leg has a direct ride, changes on foot stay
// short, there are at most MaxTransfers, and fewer changes come first.
func TestTransferOptionsAreRideable(t *testing.T) {
	ix, _ := fixtureIndex(t)
	ids := make([]string, 0, len(ix.areas))
	for id := range ix.areas {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	checked, withTwo := 0, 0
	for i := 0; i < len(ids) && checked < 25; i += 7 {
		for j := 3; j < len(ids) && checked < 25; j += 53 {
			from, to := ids[i], ids[(i+j)%len(ids)]
			if from == to || len(ix.Connections(from, to)) > 0 {
				continue
			}
			opts := ix.TransferOptions(from, to, stopTimer, 5)
			if len(opts) == 0 {
				continue
			}
			checked++
			for k, o := range opts {
				if len(o.Transfers) == 0 || len(o.Transfers) > MaxTransfers || len(o.RouteIDs) != len(o.Transfers)+1 {
					t.Fatalf("%s→%s: bad option %+v", from, to, o)
				}
				if len(o.Transfers) == 2 {
					withTwo++
				}
				if k > 0 && len(opts[k-1].Transfers) > len(o.Transfers) {
					t.Errorf("%s→%s: more changes listed before fewer", from, to)
				}
				legFrom := from
				for _, tr := range o.Transfers {
					if len(ix.Connections(legFrom, tr.Alight)) == 0 {
						t.Errorf("%s→%s: no ride %s→%s", from, to, legFrom, tr.Alight)
					}
					a, b := ix.areas[tr.Alight], ix.areas[tr.Board]
					if d := geo.DistanceM(a.Lat, a.Lon, b.Lat, b.Lon); d > MaxTransferWalkM {
						t.Errorf("%s→%s: change walks %.0f m", from, to, d)
					}
					legFrom = tr.Board
				}
				if len(ix.Connections(legFrom, to)) == 0 {
					t.Errorf("%s→%s: no last ride from %s", from, to, legFrom)
				}
			}
		}
	}
	if checked < 10 || withTwo == 0 {
		t.Fatalf("checked %d pairs (%d two-change options): fixture too small for this test?", checked, withTwo)
	}
}

func TestTransferOptionsPreferFewerChangesAndDedupeLines(t *testing.T) {
	ix, _ := fixtureIndex(t)
	opts := ix.TransferOptions("a13012", "a2947", stopTimer, 5) // Peterburi tee → Kärmu
	if len(opts) == 0 || len(opts[0].Transfers) != 1 {
		t.Fatalf("want a one-change option first: %+v", opts)
	}
	seen := map[string]bool{}
	for _, o := range opts {
		k := optionKey(o)
		if seen[k] {
			t.Errorf("lines %s offered twice", k)
		}
		seen[k] = true
	}
	if got := ix.TransferOptions("a13012", "a13012", stopTimer, 5); got != nil {
		t.Errorf("same area: %v", got)
	}
	if got := ix.TransferOptions("nope", "a2947", stopTimer, 5); got != nil {
		t.Errorf("unknown area: %v", got)
	}
}
