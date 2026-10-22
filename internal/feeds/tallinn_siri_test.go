package feeds

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParseSiriFixtures(t *testing.T) {
	b, skipped, err := ParseSiri(openFixture(t, "tallinn_siri_1300.txt"))
	if err != nil || skipped != 0 {
		t.Fatalf("err=%v skipped=%d", err, skipped)
	}
	want := SiriBoard{StopID: "1300", ServerSecOfDay: 33788, Departures: []SiriDeparture{
		{Mode: ModeBus, Line: "55", ExpectedSec: 34573, ScheduledSec: 34573, Destination: "P. Pinna", SecondsUntil: 785, LowFloor: true},
		{Mode: ModeBus, Line: "55", ExpectedSec: 36193, ScheduledSec: 36193, Destination: "P. Pinna", SecondsUntil: 2405, LowFloor: true},
	}}
	if !reflect.DeepEqual(b, want) {
		t.Errorf("got %+v\nwant %+v", b, want)
	}

	empty, _, err := ParseSiri(openFixture(t, "tallinn_siri_empty.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if empty.StopID != "" || len(empty.Departures) != 0 || empty.ServerSecOfDay != 33683 {
		t.Errorf("empty board = %+v", empty)
	}
}

func TestParseSiriRows(t *testing.T) {
	const header = "Transport,RouteNum,ExpectedTimeInSeconds,ScheduleTimeInSeconds,3600,version20201024\n"
	in := header + "stop,42\n" +
		"tram,T4,3700,3640,Tondi,100,Z\n" +
		"trol,3,90000,90000,Kaubamaja, Viru,86400,false\n" + // after midnight, comma in destination
		"bus,17A,1,2,Ülemiste,x,Z\n" + // bad seconds-until
		"ship,1,1,1,X,1,Z\n" // unknown mode
	b, skipped, err := ParseSiri(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if skipped != 2 || b.StopID != "42" || len(b.Departures) != 2 {
		t.Fatalf("board=%+v skipped=%d", b, skipped)
	}
	if d := b.Departures[0]; d.Mode != ModeTram || d.Line != "T4" || d.DelaySec() != 60 || !d.LowFloor {
		t.Errorf("tram row = %+v", d)
	}
	if d := b.Departures[1]; d.Mode != ModeTrolley || d.Destination != "Kaubamaja, Viru" || d.ExpectedSec != 90000 || d.LowFloor {
		t.Errorf("trolley row = %+v", d)
	}
}

func TestParseSiriBadHeader(t *testing.T) {
	for _, in := range []string{"", "<html>error</html>", "bus,55,1,1,X,1,Z"} {
		if _, _, err := ParseSiri(strings.NewReader(in)); !errors.Is(err, ErrSiriHeader) {
			t.Errorf("ParseSiri(%q) err = %v, want ErrSiriHeader", in, err)
		}
	}
}
