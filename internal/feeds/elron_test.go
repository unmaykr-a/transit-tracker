package feeds

import (
	"strings"
	"testing"
	"time"
)

func tallinn(t *testing.T) *time.Location {
	loc, err := time.LoadLocation("Europe/Tallinn")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestParseElronTrainsFixture(t *testing.T) {
	loc := tallinn(t)
	trains, skipped, err := ParseElronTrains(openFixture(t, "elron_map_data.json"), loc)
	if err != nil || skipped != 0 {
		t.Fatalf("err=%v skipped=%d", err, skipped)
	}
	if len(trains) != 17 {
		t.Fatalf("got %d trains, want 17", len(trains))
	}
	want := ElronTrain{
		Trip: "10", Route: "Tallinn - Tartu", StartTime: "07:05", EndTime: "09:39", SpeedKmh: 120,
		Lat: 58.636062, Lon: 26.552633, Heading: 147, DelayMin: 1, DelayKnown: true,
		Reason: "liiklushäire tõttu", Status: "plaaniline", LastStop: "Jõgeva",
		UpdatedAt: time.Date(2026, 10, 22, 9, 21, 0, 0, loc),
	}
	if got := trains[0]; got != want {
		t.Errorf("first train = %+v\nwant %+v", got, want)
	}
	unknown := 0
	for _, tr := range trains {
		if !tr.DelayKnown {
			unknown++
		}
	}
	if unknown != 11 {
		t.Errorf("trains without delay = %d, want 11", unknown)
	}
}

func TestParseElronTrainsEdgeCases(t *testing.T) {
	loc := tallinn(t)
	in := `{"status":1,"data":[
		{"reis":"5","liin":"A","latitude":59.4,"longitude":24.7,"kiirus":80,"erinevus_plaanist":-2,"rongi_suund":null,"asukoha_uuendus":"bad"},
		{"reis":"6","latitude":"","longitude":"24.7"},
		{"reis":"","latitude":"59.4","longitude":"24.7"}
	]}`
	trains, skipped, err := ParseElronTrains(strings.NewReader(in), loc)
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 2 || len(trains) != 1 {
		t.Fatalf("trains=%+v skipped=%d", trains, skipped)
	}
	tr := trains[0]
	if tr.Lat != 59.4 || tr.SpeedKmh != 80 || tr.DelayMin != -2 || !tr.DelayKnown || tr.Heading != -1 || !tr.UpdatedAt.IsZero() {
		t.Errorf("numeric fields not handled: %+v", tr)
	}
}

func TestParseElronErrors(t *testing.T) {
	for _, in := range []string{``, `<html>`, `{"status":0,"data":[]}`} {
		if _, _, err := ParseElronTrains(strings.NewReader(in), time.UTC); err == nil {
			t.Errorf("ParseElronTrains(%q) succeeded, want error", in)
		}
		if _, _, err := ParseElronStations(strings.NewReader(in)); err == nil {
			t.Errorf("ParseElronStations(%q) succeeded, want error", in)
		}
	}
}

func TestParseElronNoTrains(t *testing.T) {
	// At night (recorded 2026-10-24 01:10) the feed answers with a message instead of a list.
	trains, skipped, err := ParseElronTrains(strings.NewReader(`{"status":1,"data":{"text":"Pole andmeid"},"src":"cache"}`), time.UTC)
	if err != nil || len(trains) != 0 || skipped != 0 {
		t.Errorf("no-data answer: %v %d %v", trains, skipped, err)
	}
	if _, _, err := ParseElronTrains(strings.NewReader(`{"status":1,"data":"oops"}`), time.UTC); err == nil {
		t.Error("a string as data: want an error")
	}
}

func TestParseElronStationsFixture(t *testing.T) {
	st, skipped, err := ParseElronStations(openFixture(t, "elron_stops_data.json"))
	if err != nil || skipped != 0 {
		t.Fatalf("err=%v skipped=%d", err, skipped)
	}
	if len(st) != 141 {
		t.Fatalf("got %d stations, want 141", len(st))
	}
	if want := (ElronStation{Name: "Aardla", Lat: 58.35763, Lon: 26.71458}); st[0] != want {
		t.Errorf("first station = %+v", st[0])
	}
	found := false
	for _, s := range st {
		if s.Name == "Holvandi" {
			found = s.Notice == "Lühike platvorm!"
		}
	}
	if !found {
		t.Error("Holvandi notice not parsed")
	}
}
