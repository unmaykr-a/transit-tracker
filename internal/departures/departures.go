// Package departures merges live SIRI predictions (city stops) with OTP
// timetables (rail, and city stops without live data) into one board.
package departures

import (
	"bytes"
	"context"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/cache"
	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
)

// Departure is one upcoming departure from a platform.
type Departure struct {
	StopID      string     `json:"stop_id"`
	Platform    string     `json:"platform"`
	Mode        feeds.Mode `json:"mode"`
	Line        string     `json:"line"`
	RouteID     string     `json:"route_id,omitempty"`
	Headsign    string     `json:"headsign"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	ExpectedAt  time.Time  `json:"expected_at"`
	Realtime    bool       `json:"realtime"` // false: timetable only, show a "scheduled" tag
	DelayS      int        `json:"delay_s"`
	LowFloor    bool       `json:"low_floor"`
	Source      string     `json:"source"` // "siri" or "otp"
}

// Service fetches and caches departures.
type Service struct {
	Fetcher  *feeds.Fetcher
	OTP      *feeds.OTP
	SiriURL  func() string // e.g. https://transport.tallinn.ee/siri-stop-departures.php
	Index    func() *staticdata.Index
	Location *time.Location
	Now      func() time.Time

	initOnce sync.Once
	siri     *cache.TTL[string, feeds.SiriBoard]
	otp      *cache.TTL[string, map[string][]feeds.OTPStopTime]
}

// DefaultSiriURL is the Tallinn live departures endpoint.
const DefaultSiriURL = "https://transport.tallinn.ee/siri-stop-departures.php"

func (s *Service) init() {
	s.initOnce.Do(func() {
		s.siri = &cache.TTL[string, feeds.SiriBoard]{TTL: 15 * time.Second, StaleFor: 2 * time.Minute, Now: s.Now}
		s.otp = &cache.TTL[string, map[string][]feeds.OTPStopTime]{TTL: 30 * time.Second, StaleFor: 5 * time.Minute, Now: s.Now}
	})
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Siri returns the (cached) live board of one SIRI stop.
func (s *Service) Siri(ctx context.Context, siriID string) (feeds.SiriBoard, error) {
	s.init()
	return s.siri.Get(ctx, siriID, func(ctx context.Context) (feeds.SiriBoard, error) {
		res, err := s.Fetcher.Get(ctx, s.SiriURL()+"?stopid="+url.QueryEscape(siriID), false)
		if err != nil {
			return feeds.SiriBoard{}, err
		}
		b, _, err := feeds.ParseSiri(bytes.NewReader(res.Body))
		return b, err
	})
}

func (s *Service) otpTimes(ctx context.Context, ids []string) (map[string][]feeds.OTPStopTime, error) {
	s.init()
	ids = slices.Clone(ids)
	sort.Strings(ids)
	return s.otp.Get(ctx, strings.Join(ids, ","), func(ctx context.Context) (map[string][]feeds.OTPStopTime, error) {
		return s.OTP.StopTimes(ctx, ids, 12, 4*3600)
	})
}

// ForStops returns the next departures from the given platforms, soonest first.
// SIRI is authoritative where it has rows; platforms without live rows (rail,
// gaps in the city feed, SIRI down) fall back to the OTP timetable.
func (s *Service) ForStops(ctx context.Context, stops []*staticdata.Stop, limit int, horizon time.Duration) ([]Departure, error) {
	now := s.now()
	type result struct {
		deps []Departure
		otp  *staticdata.Stop // set when this stop needs the timetable
	}
	results := make([]result, len(stops))
	var wg sync.WaitGroup
	for i, st := range stops {
		if st.SiriID == "" || !slices.ContainsFunc(st.Modes, func(m feeds.Mode) bool { return m != feeds.ModeTrain }) {
			results[i].otp = st
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			board, err := s.Siri(ctx, st.SiriID)
			if err != nil || len(board.Departures) == 0 {
				results[i].otp = st
				return
			}
			results[i].deps = s.fromSiri(st, board, now)
		}()
	}
	wg.Wait()

	var all []Departure
	var otpStops []*staticdata.Stop
	for _, r := range results {
		all = append(all, r.deps...)
		if r.otp != nil {
			otpStops = append(otpStops, r.otp)
		}
	}
	var otpErr error
	if len(otpStops) > 0 {
		ids := make([]string, len(otpStops))
		for i, st := range otpStops {
			ids[i] = st.OTPID()
		}
		times, err := s.otpTimes(ctx, ids)
		if err != nil {
			otpErr = err
		}
		for _, st := range otpStops {
			all = append(all, s.fromOTP(st, times[st.OTPID()])...)
		}
	}
	all = slices.DeleteFunc(all, func(d Departure) bool {
		return d.ExpectedAt.Before(now.Add(-30*time.Second)) || d.ExpectedAt.After(now.Add(horizon))
	})
	sort.SliceStable(all, func(i, j int) bool { return all[i].ExpectedAt.Before(all[j].ExpectedAt) })
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	if len(all) == 0 && otpErr != nil {
		return nil, otpErr
	}
	return all, nil
}

// secOfDay converts seconds after local midnight of the service day to a time
// near now. Values past 86400 belong to trips that started before midnight.
func secOfDay(sec int, now time.Time, loc *time.Location) time.Time {
	n := now.In(loc)
	t := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, sec, 0, loc)
	switch d := t.Sub(now); {
	case d < -12*time.Hour:
		t = time.Date(n.Year(), n.Month(), n.Day()+1, 0, 0, sec, 0, loc)
	case d > 12*time.Hour:
		t = time.Date(n.Year(), n.Month(), n.Day()-1, 0, 0, sec, 0, loc)
	}
	return t
}

func (s *Service) routeID(mode feeds.Mode, line string) string {
	ix := s.Index()
	if ix == nil {
		return ""
	}
	if r, ok := ix.Route(staticdata.RouteID(mode, line)); ok {
		return r.ID
	}
	return ""
}

func (s *Service) fromSiri(st *staticdata.Stop, b feeds.SiriBoard, now time.Time) []Departure {
	out := make([]Departure, 0, len(b.Departures))
	for _, d := range b.Departures {
		line := d.Line
		if d.Mode == feeds.ModeTram && !strings.HasPrefix(strings.ToUpper(line), "T") {
			line = "T" + line // match the GPS feed and OTP naming
		}
		out = append(out, Departure{
			StopID: st.ID, Platform: st.Platform, Mode: d.Mode, Line: line, RouteID: s.routeID(d.Mode, line),
			Headsign:    d.Destination,
			ScheduledAt: secOfDay(d.ScheduledSec, now, s.Location),
			ExpectedAt:  secOfDay(d.ExpectedSec, now, s.Location),
			Realtime:    true, DelayS: d.DelaySec(), LowFloor: d.LowFloor, Source: "siri",
		})
	}
	return out
}

func otpMode(m string) feeds.Mode {
	switch m {
	case "RAIL":
		return feeds.ModeTrain
	case "TRAM":
		return feeds.ModeTram
	case "TROLLEYBUS":
		return feeds.ModeTrolley
	default:
		return feeds.ModeBus
	}
}

func (s *Service) fromOTP(st *staticdata.Stop, times []feeds.OTPStopTime) []Departure {
	out := make([]Departure, 0, len(times))
	for _, t := range times {
		mode := otpMode(t.Trip.Route.Mode)
		line := t.Trip.Route.ShortName
		if mode == feeds.ModeBus && s.routeID(feeds.ModeTrolley, line) != "" && s.routeID(feeds.ModeBus, line) == "" {
			mode = feeds.ModeTrolley // OTP files Tallinn trolleybuses under BUS
		}
		headsign := t.Headsign
		if headsign == "" {
			headsign = staticTerminus(t.Trip.Route.LongName)
		}
		base := time.Unix(t.ServiceDay, 0)
		out = append(out, Departure{
			StopID: st.ID, Platform: st.Platform, Mode: mode, Line: line, RouteID: s.routeID(mode, line),
			Headsign:    headsign,
			ScheduledAt: base.Add(time.Duration(t.ScheduledDeparture) * time.Second).In(s.Location),
			ExpectedAt:  base.Add(time.Duration(t.RealtimeDeparture) * time.Second).In(s.Location),
			Realtime:    t.Realtime, DelayS: t.DepartureDelay, Source: "otp",
		})
	}
	return out
}

// staticTerminus takes the last place of "Tallinn - Paldiski" style names.
func staticTerminus(longName string) string {
	if i := strings.LastIndex(longName, " - "); i >= 0 {
		return strings.TrimSpace(longName[i+3:])
	}
	return longName
}

// DropArrivals removes rows whose headsign is the stop itself: SIRI and OTP list
// vehicles terminating here, which are arrivals, not departures.
func DropArrivals(deps []Departure, stopNames []string) []Departure {
	names := map[string]bool{}
	for _, n := range stopNames {
		names[staticdata.Fold(n)] = true
	}
	return slices.DeleteFunc(deps, func(d Departure) bool { return names[staticdata.Fold(d.Headsign)] })
}
