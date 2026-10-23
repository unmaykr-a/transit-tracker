package dashboard

import (
	"slices"
	"testing"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/departures"
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

func TestExtendWithTimetable(t *testing.T) {
	at := func(min int) time.Time { return time.Date(2026, 10, 23, 9, min, 0, 0, time.UTC) }
	live := []departures.Departure{{RouteID: "bus:17", StopID: "s1", ExpectedAt: at(5)}, {RouteID: "bus:17", StopID: "s1", ExpectedAt: at(20)}}
	tt := []departures.Departure{
		{RouteID: "bus:17", StopID: "s1", ExpectedAt: at(19)}, // already live
		{RouteID: "bus:17", StopID: "s1", ExpectedAt: at(50)}, // beyond the live board
		{RouteID: "bus:3", StopID: "s1", ExpectedAt: at(10)},  // no live rows for this line
	}
	got := extendWithTimetable(live, tt)
	var mins []int
	for _, d := range got {
		mins = append(mins, d.ExpectedAt.Minute())
	}
	if !slices.Equal(mins, []int{5, 10, 20, 50}) {
		t.Errorf("merged = %v", mins)
	}
}
