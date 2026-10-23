package departures

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
)

var tallinn, _ = time.LoadLocation("Europe/Tallinn")

// 09:20:00 local on 2026-10-22.
var now = time.Date(2026, 10, 22, 9, 20, 0, 0, tallinn)

func TestSecOfDay(t *testing.T) {
	for _, tt := range []struct {
		now  time.Time
		sec  int
		want time.Time
	}{
		{now, 9*3600 + 25*60, time.Date(2026, 10, 22, 9, 25, 0, 0, tallinn)},
		// 00:10 with a trip from yesterday's service day (sec > 86400).
		{time.Date(2026, 10, 23, 0, 10, 0, 0, tallinn), 86400 + 15*60, time.Date(2026, 10, 23, 0, 15, 0, 0, tallinn)},
		// 23:55 asking about 00:05 tomorrow.
		{time.Date(2026, 10, 22, 23, 55, 0, 0, tallinn), 5 * 60, time.Date(2026, 10, 23, 0, 5, 0, 0, tallinn)},
	} {
		if got := secOfDay(tt.sec, tt.now, tallinn); !got.Equal(tt.want) {
			t.Errorf("secOfDay(%d, %v) = %v, want %v", tt.sec, tt.now, got, tt.want)
		}
	}
}

type upstream struct {
	srv       *httptest.Server
	siriCalls atomic.Int32
	otpCalls  atomic.Int32
	otpIDs    []any
}

func newUpstream(t *testing.T) *upstream {
	u := &upstream{}
	sec := now.Hour()*3600 + now.Minute()*60
	u.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/siri" {
			u.siriCalls.Add(1)
			fmt.Fprintf(w, "Transport,RouteNum,ExpectedTimeInSeconds,ScheduleTimeInSeconds,%d,version20201024\n", sec)
			if r.URL.Query().Get("stopid") == "1301" {
				fmt.Fprintf(w, "stop,1301\ntram,4,%d,%d,Lennujaam,420,Z\nbus,17,%d,%d,Väike-Õismäe,120,Z\nbus,5,%d,%d,Late,99999,Z\n",
					sec+420, sec+360, sec+120, sec+120, sec+5*3600, sec+5*3600)
			}
			return
		}
		u.otpCalls.Add(1)
		var req struct {
			Variables map[string]any `json:"variables"`
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &req)
		u.otpIDs = req.Variables["ids"].([]any)
		serviceDay := time.Date(2026, 10, 22, 0, 0, 0, 0, tallinn).Unix()
		var stops []map[string]any
		for _, id := range u.otpIDs {
			st := map[string]any{"gtfsId": id, "stoptimesWithoutPatterns": []any{
				map[string]any{"scheduledDeparture": sec + 600, "realtimeDeparture": sec + 600, "realtime": false, "serviceDay": serviceDay, "headsign": nil,
					"trip": map[string]any{"route": map[string]any{"shortName": "R12", "longName": "Tallinn - Keila", "mode": "RAIL"}}},
				map[string]any{"scheduledDeparture": sec + 300, "realtimeDeparture": sec + 300, "realtime": false, "serviceDay": serviceDay, "headsign": "Kopli",
					"trip": map[string]any{"route": map[string]any{"shortName": "72", "mode": "BUS"}}},
			}}
			if id == "estonia:1301" { // a city platform county bus 120 also calls at
				st["stoptimesWithoutPatterns"] = append(st["stoptimesWithoutPatterns"].([]any),
					map[string]any{"scheduledDeparture": sec + 480, "realtimeDeparture": sec + 480, "realtime": false, "serviceDay": serviceDay, "headsign": "Jüri",
						"trip": map[string]any{"gtfsId": "estonia:ATL-Fr-harjumaa120-A-OB-1600-abc", "route": map[string]any{"shortName": "120", "longName": "Tallinn - Jüri", "mode": "BUS"}}})
			}
			stops = append(stops, st)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"stops": stops}})
	}))
	t.Cleanup(u.srv.Close)
	return u
}

func testService(u *upstream) *Service {
	f := feeds.NewFetcher(u.srv.Client(), "test")
	ix := staticdata.NewIndex(staticdata.Dataset{Routes: []staticdata.Route{
		{ID: "tram:T4", Mode: feeds.ModeTram, ShortName: "T4"},
		{ID: "bus:17", Mode: feeds.ModeBus, ShortName: "17"},
		{ID: "trolley:72", Mode: feeds.ModeTrolley, ShortName: "72"},
		{ID: "train:R12", Mode: feeds.ModeTrain, ShortName: "R12"},
	}})
	return &Service{
		Fetcher: f, OTP: &feeds.OTP{URL: feeds.Const(u.srv.URL + "/otp"), Fetcher: f}, SiriURL: feeds.Const(u.srv.URL + "/siri"),
		Index: func() *staticdata.Index { return ix }, Location: tallinn, Now: func() time.Time { return now },
	}
}

func TestForStops(t *testing.T) {
	u := newUpstream(t)
	s := testService(u)
	city := &staticdata.Stop{ID: "1301", SiriID: "1301", Platform: "A", Modes: []feeds.Mode{feeds.ModeTram, feeds.ModeBus}}
	quiet := &staticdata.Stop{ID: "1300", SiriID: "1300", Platform: "B", Modes: []feeds.Mode{feeds.ModeBus}}
	rail := &staticdata.Stop{ID: "10137", Platform: "C", Modes: []feeds.Mode{feeds.ModeTrain}}

	deps, err := s.ForStops(context.Background(), []*staticdata.Stop{city, quiet, rail}, 20, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range deps {
		got = append(got, fmt.Sprintf("%s/%s %s→%s +%dm rt=%v", d.Platform, d.Source, d.Line, d.Headsign, int(d.ExpectedAt.Sub(now).Minutes()), d.Realtime))
	}
	want := []string{
		"A/siri 17→Väike-Õismäe +2m rt=true",
		"B/otp 72→Kopli +5m rt=false",
		"C/otp 72→Kopli +5m rt=false",
		"A/siri T4→Lennujaam +7m rt=true",
		"B/otp R12→Keila +10m rt=false",
		"C/otp R12→Keila +10m rt=false",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("departures:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if d := deps[3]; d.RouteID != "tram:T4" || d.DelayS != 60 || !d.LowFloor || d.Mode != feeds.ModeTram {
		t.Errorf("tram departure = %+v", d)
	}
	if d := deps[1]; d.Mode != feeds.ModeTrolley || d.RouteID != "trolley:72" {
		t.Errorf("OTP BUS 72 should map to the trolleybus route: %+v", d)
	}
	if d := deps[4]; d.Mode != feeds.ModeTrain || d.RouteID != "train:R12" {
		t.Errorf("rail departure = %+v", d)
	}
	// Only the quiet city stop and the rail stop went to OTP, in one request.
	if u.otpCalls.Load() != 1 || len(u.otpIDs) != 2 {
		t.Errorf("otp calls=%d ids=%v", u.otpCalls.Load(), u.otpIDs)
	}

	// Second call within the TTL is served from cache.
	if _, err := s.ForStops(context.Background(), []*staticdata.Stop{city, quiet, rail}, 3, 2*time.Hour); err != nil {
		t.Fatal(err)
	}
	if u.siriCalls.Load() != 2 || u.otpCalls.Load() != 1 {
		t.Errorf("cache miss: siri=%d otp=%d", u.siriCalls.Load(), u.otpCalls.Load())
	}
}

func TestDropArrivals(t *testing.T) {
	deps := []Departure{{Headsign: "Balti jaam"}, {Headsign: "Kopli"}, {Headsign: "TALLINN"}}
	got := DropArrivals(deps, []string{"Balti jaam", "Tallinn"})
	if len(got) != 1 || got[0].Headsign != "Kopli" {
		t.Fatalf("got %+v", got)
	}
}

func TestCountyBusesJoinCityBoards(t *testing.T) {
	u := newUpstream(t)
	s := testService(u)
	city := &staticdata.Stop{ID: "1301", SiriID: "1301", Code: "10801-1", Platform: "A", Modes: []feeds.Mode{feeds.ModeTram, feeds.ModeBus}}
	ix := staticdata.NewIndex(staticdata.Dataset{
		Stops: []staticdata.Stop{*city},
		Routes: []staticdata.Route{
			{ID: "tram:T4", Mode: feeds.ModeTram, ShortName: "T4", Source: "tallinn"},
			{ID: "bus:17", Mode: feeds.ModeBus, ShortName: "17", Source: "tallinn"},
			{ID: "bus:120", Mode: feeds.ModeBus, ShortName: "120", Source: "county"},
		},
		Patterns: []staticdata.Pattern{{ID: "p120", RouteID: "bus:120", StopIDs: []string{"1301", "9999"}}},
	})
	s.Index = func() *staticdata.Index { return ix }

	deps, err := s.ForStops(context.Background(), []*staticdata.Stop{city}, 20, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range deps {
		got = append(got, fmt.Sprintf("%s %s %s +%dm", d.Source, d.RouteID, d.Line, int(d.ExpectedAt.Sub(now).Minutes())))
	}
	// SIRI's live city rows, plus only the county route from the timetable
	// (OTP's city rows for this platform would duplicate SIRI's).
	want := []string{"siri bus:17 17 +2m", "siri tram:T4 T4 +7m", "otp bus:120 120 +8m"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("board:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if u.siriCalls.Load() != 1 || u.otpCalls.Load() != 1 {
		t.Errorf("siri=%d otp=%d calls", u.siriCalls.Load(), u.otpCalls.Load())
	}

	// With the county feed, the county row carries its bus's live time (by trip and stop code).
	s.CountyLive = func(trip, code string) (time.Time, bool) {
		if trip == "estonia:ATL-Fr-harjumaa120-A-OB-1600-abc" && code == "10801-1" {
			return now.Add(11 * time.Minute), true
		}
		return time.Time{}, false
	}
	deps, _ = s.ForStops(context.Background(), []*staticdata.Stop{city}, 20, 2*time.Hour)
	d := deps[len(deps)-1]
	if d.Line != "120" || !d.Realtime || d.DelayS != 180 || !d.ExpectedAt.Equal(now.Add(11*time.Minute)) {
		t.Errorf("county row with the live feed: %+v", d)
	}
}

// On the day the clocks go back, "seconds since midnight" and the wall clock
// differ by an hour; departures follow the board's own server clock either way.
func TestSiriTimesOnClockChangeDay(t *testing.T) {
	now := time.Date(2026, 10, 25, 10, 0, 0, 0, tallinn) // EET, an hour after the change
	s := &Service{Location: tallinn, Index: func() *staticdata.Index { return nil }}
	st := &staticdata.Stop{ID: "1"}
	for name, server := range map[string]int{"wall clock": 10 * 3600, "elapsed": 11 * 3600} {
		b := feeds.SiriBoard{ServerSecOfDay: server, Departures: []feeds.SiriDeparture{{Mode: feeds.ModeBus, Line: "17", ExpectedSec: server + 600, ScheduledSec: server + 540}}}
		got := s.fromSiri(st, b, now)
		if !got[0].ExpectedAt.Equal(now.Add(10*time.Minute)) || !got[0].ScheduledAt.Equal(now.Add(9*time.Minute)) {
			t.Errorf("%s: expected %v scheduled %v, want %v", name, got[0].ExpectedAt, got[0].ScheduledAt, now.Add(10*time.Minute))
		}
	}
	// Around midnight a row may count from the next day (small) or past 86400.
	late := time.Date(2026, 10, 22, 23, 58, 0, 0, tallinn)
	for _, sec := range []int{120, 86520} {
		b := feeds.SiriBoard{ServerSecOfDay: 86280, Departures: []feeds.SiriDeparture{{Mode: feeds.ModeBus, Line: "17", ExpectedSec: sec, ScheduledSec: sec}}}
		if got := s.fromSiri(st, b, late)[0].ExpectedAt; !got.Equal(late.Add(4 * time.Minute)) {
			t.Errorf("row %d: %v, want %v", sec, got, late.Add(4*time.Minute))
		}
	}
}
