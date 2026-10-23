package dashboard

import (
	"slices"
	"testing"

	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
)

func TestTransfersFormat(t *testing.T) {
	ts := []staticdata.Transfer{{Alight: "a1", Board: "a1"}, {Alight: "a2", Board: "a3"}}
	s := FormatTransfers(ts)
	if s != "a1,a2>a3" || !slices.Equal(ParseTransfers(s), ts) {
		t.Fatalf("%q → %v", s, ParseTransfers(s))
	}
	if ParseTransfers("") != nil || FormatTransfers(nil) != "" {
		t.Error("no changes")
	}
	legs := LegAreas("f", "t", ts)
	if !slices.Equal(legs, [][2]string{{"f", "a1"}, {"a1", "a2"}, {"a3", "t"}}) {
		t.Errorf("legs = %v", legs)
	}
}
