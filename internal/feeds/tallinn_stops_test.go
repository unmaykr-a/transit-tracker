package feeds

import (
	"strings"
	"testing"
)

func TestParseTallinnStopsFixture(t *testing.T) {
	stops, skipped, err := ParseTallinnStops(openFixture(t, "tallinn_stops.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(stops) < 3500 || skipped < 50 {
		t.Fatalf("got %d stops, %d skipped", len(stops), skipped)
	}
	by := map[string]TallinnStop{}
	for _, s := range stops {
		by[s.Code] = s
	}
	h := by["12204-1"]
	if h.Name != "Hobujaama" || h.SiriID != "1298" || h.Lat != 59.4375 || h.Lon != 24.757 {
		t.Errorf("Hobujaama = %+v", h)
	}
	// 12206-1 has an empty name and inherits "Hobujaama".
	if got := by["12206-1"]; got.Name != "Hobujaama" || got.SiriID != "1300" {
		t.Errorf("inherited row = %+v", got)
	}
}

func TestParseTallinnStopsInheritance(t *testing.T) {
	in := "\ufeffID;SiriID;Lat;Lng;Stops;Name;Info;Street;Area;City\n" +
		"a1;;5940000;2470000;x;Area;0;0;Kesklinn;Tallinn\n" +
		"100-1;11;5940000;2470000;;Ülemiste;;Peterburi tee;Lasnamäe;Tallinn\n" +
		"100-2;12;5940010;2470010;;;;;;\n" +
		"100-3;13;bad;2470010;;;;;;\n"
	stops, skipped, err := ParseTallinnStops(strings.NewReader(in))
	if err != nil || skipped != 2 || len(stops) != 2 {
		t.Fatalf("stops=%+v skipped=%d err=%v", stops, skipped, err)
	}
	if s := stops[1]; s.Name != "Ülemiste" || s.Street != "Peterburi tee" || s.City != "Tallinn" || s.SiriID != "12" {
		t.Errorf("inheritance failed: %+v", s)
	}
}
