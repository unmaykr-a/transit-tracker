package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/buildinfo"
	"github.com/unmaykr-a/transit-tracker/internal/config"
	"github.com/unmaykr-a/transit-tracker/internal/departures"
	"github.com/unmaykr-a/transit-tracker/internal/eta"
	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/ingest"
	"github.com/unmaykr-a/transit-tracker/internal/state"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
	"github.com/unmaykr-a/transit-tracker/internal/store"
)

// etaCheck measures predicted arrivals against SIRI live predictions
// (PLAN.md M5: within ±1 min for 80 % of a sampled hour). It runs the live GPS
// pipeline in-process, so it can run next to a server or on its own.
func etaCheck(cfg config.Config, args []string) error {
	fs := flag.NewFlagSet("eta-check", flag.ExitOnError)
	duration := fs.Duration("duration", time.Hour, "how long to sample")
	warmup := fs.Duration("warmup", 4*time.Minute, "learn vehicle speeds before sampling")
	stops := fs.Int("stops", 20, "stops sampled per round (the busiest city stops)")
	interval := fs.Duration("interval", time.Minute, "time between sampling rounds")
	minAhead := fs.Duration("min-ahead", 2*time.Minute, "ignore departures sooner than this")
	maxAhead := fs.Duration("max-ahead", 20*time.Minute, "ignore departures later than this")
	record := fs.String("record", "", "also write the GPS and SIRI input to this file (JSON lines)")
	replay := fs.String("replay", "", "evaluate a recording offline instead of sampling live")
	_ = fs.Parse(args)

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	tallinn, err := time.LoadLocation("Europe/Tallinn")
	if err != nil {
		return err
	}
	db, err := store.Open(ctx, cfg.DataDir, log)
	if err != nil {
		return err
	}
	defer db.Close()
	fetcher := feeds.NewFetcher(&http.Client{Timeout: 30 * time.Second}, cfg.UserAgent(buildinfo.Version)+" eta-check")
	otp := &feeds.OTP{URL: feeds.Const(cfg.Feeds.OTPURL), Fetcher: fetcher}
	im := &staticdata.Importer{Fetcher: fetcher, OTP: otp, StopsURL: feeds.Const(cfg.Feeds.StopsURL), ElronGTFSURL: feeds.Const(cfg.Feeds.ElronGTFSURL),
		AgencyID: feeds.TallinnAgencyID, Store: db, Log: log, Location: tallinn}
	if err := im.LoadSaved(ctx); err != nil {
		return err
	}
	if im.Index().Empty() {
		fmt.Println("importing static data…")
		if err := im.Import(ctx); err != nil {
			return err
		}
	}
	ix := im.Index()
	tracker := eta.NewTracker()
	tracker.SetGeometry(eta.BuildGeometry(ix))
	ev := &evaluator{loc: tallinn, minAhead: *minAhead, maxAhead: *maxAhead, byMode: map[string][]time.Duration{},
		byAhead: map[string][]time.Duration{}, expected: map[string]int{}}

	if *replay != "" {
		return replayRecording(ctx, *replay, *warmup, ix, tracker, ev)
	}

	gpsFetcher := fetcher
	var rec *recorder
	if *record != "" {
		f, err := os.Create(*record)
		if err != nil {
			return err
		}
		defer f.Close()
		rec = &recorder{enc: json.NewEncoder(f)}
		gpsFetcher = feeds.NewFetcher(&http.Client{Timeout: 30 * time.Second, Transport: rec.transport(http.DefaultTransport)}, cfg.UserAgent(buildinfo.Version)+" eta-check")
	}
	vehicles := state.NewStore()
	gps := &ingest.GPSPoller{URL: feeds.Const(cfg.Feeds.GPSURL), Fetcher: gpsFetcher, Store: vehicles, Annotate: tracker.Annotate}
	go ingest.NewHealth().Run(ctx, log, ingest.Feed{Name: "gps", Interval: feeds.Const(cfg.Feeds.GPSInterval), Poll: gps.Poll})
	deps := &departures.Service{Fetcher: fetcher, OTP: otp, SiriURL: feeds.Const(cfg.Feeds.SiriURL), Index: im.Index, Location: tallinn}

	sample := busiestStops(ix, *stops)
	fmt.Printf("warming up for %s, then sampling %d stops every %s for %s\n", *warmup, len(sample), *interval, *duration)
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(*warmup):
	}
	deadline := time.Now().Add(*duration)
	for round := 1; time.Now().Before(deadline); round++ {
		now := time.Now()
		_, cur := vehicles.Snapshot()
		for _, st := range sample {
			board, err := deps.Siri(ctx, st.SiriID)
			if err != nil {
				continue
			}
			rec.write(recEvent{Kind: "siri", At: now, Stop: st.ID, Board: &board})
			ev.add(tracker, st, board, cur, now)
		}
		ev.round(os.Stdout, round)
		select {
		case <-ctx.Done():
			deadline = time.Now()
		case <-time.After(*interval):
		}
	}
	ev.result(os.Stdout)
	return nil
}

// busiestStops returns the n city platforms served by the most lines.
func busiestStops(ix *staticdata.Index, n int) []*staticdata.Stop {
	var out []*staticdata.Stop
	for _, st := range ix.Stops() {
		if st.SiriID != "" && st.Source == "tallinn" && len(ix.Lines(st.ID)) > 0 {
			out = append(out, st)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return len(ix.Lines(out[i].ID)) > len(ix.Lines(out[j].ID)) })
	return out[:min(n, len(out))]
}

// evaluator accumulates prediction errors against SIRI.
type evaluator struct {
	loc                *time.Location
	minAhead, maxAhead time.Duration
	byMode, byAhead    map[string][]time.Duration
	expected           map[string]int
	all                []time.Duration
	// pending pairs waiting for the vehicle to reach the stop (replay only):
	// the arrival seen in the GPS feed grades both our prediction and SIRI's.
	pending  []pendingArrival
	observed map[string][]time.Duration // "ours"/"siri" by lead bucket → predicted − observed
}

type pendingArrival struct {
	vehicle, pattern string
	stop             int
	made, ours, siri time.Time
}

// add compares one stop's SIRI board with our predictions at now.
func (e *evaluator) add(tracker *eta.Tracker, st *staticdata.Stop, board feeds.SiriBoard, cur map[string]state.Vehicle, now time.Time) {
	byRoute := map[string][]time.Time{}
	modeOf := map[string]string{}
	for _, d := range board.Departures {
		line := d.Line
		if d.Mode == feeds.ModeTram && line != "" && line[0] != 'T' {
			line = "T" + line
		}
		exp := secOfDayNear(d.ExpectedSec, now, e.loc)
		if ahead := exp.Sub(now); ahead < e.minAhead || ahead > e.maxAhead || exp.Equal(secOfDayNear(d.ScheduledSec, now, e.loc)) {
			continue // only live predictions (expected differs from schedule) inside the window
		}
		rid := staticdata.RouteID(d.Mode, line)
		byRoute[rid] = append(byRoute[rid], exp)
		modeOf[rid] = string(d.Mode)
	}
	for rid, siri := range byRoute {
		preds := tracker.Predictions(cur, rid, st.ID, now)
		pred := make([]time.Time, len(preds))
		for i, p := range preds {
			pred[i] = p.ETA
		}
		m := modeOf[rid]
		e.expected[m] += len(siri)
		e.expected["all"] += len(siri)
		for _, mt := range eta.PairIndex(siri, pred, 5*time.Minute) {
			e.byMode[m] = append(e.byMode[m], mt.Diff)
			e.all = append(e.all, mt.Diff)
			bucket := "≤5 min"
			switch ahead := siri[mt.Siri].Sub(now); {
			case ahead > 10*time.Minute:
				bucket = "10–20 min"
			case ahead > 5*time.Minute:
				bucket = "5–10 min"
			}
			e.byAhead[bucket] = append(e.byAhead[bucket], mt.Diff)
			if e.observed != nil {
				p := preds[mt.Pred]
				e.pending = append(e.pending, pendingArrival{p.VehicleID, p.PatternID, p.StopIndex, now, p.ETA, siri[mt.Siri]})
			}
			if p := preds[mt.Pred]; p.AtStart {
				e.byAhead["at terminus"] = append(e.byAhead["at terminus"], mt.Diff)
			} else {
				e.byAhead["en route"] = append(e.byAhead["en route"], mt.Diff)
			}
		}
	}
}

// resolve grades pending predictions whose vehicle has now passed the stop.
func (e *evaluator) resolve(g *eta.Geometry, vs map[string]state.Vehicle) {
	keep := e.pending[:0]
	for _, p := range e.pending {
		v, ok := vs[p.vehicle]
		pg, okG := g.Pattern(p.pattern)
		switch {
		case !ok || !okG || v.PatternID != p.pattern || v.UpdatedAt.Sub(p.made) > 30*time.Minute:
			// vehicle gone or switched pattern: no observation
		case v.DistAlongM >= pg.StopAlong[p.stop]-15:
			bucket := "≤5 min"
			switch ahead := p.siri.Sub(p.made); {
			case ahead > 10*time.Minute:
				bucket = "10–20 min"
			case ahead > 5*time.Minute:
				bucket = "5–10 min"
			}
			e.observed["ours "+bucket] = append(e.observed["ours "+bucket], p.ours.Sub(v.UpdatedAt))
			e.observed["siri "+bucket] = append(e.observed["siri "+bucket], p.siri.Sub(v.UpdatedAt))
			e.observed["ours all"] = append(e.observed["ours all"], p.ours.Sub(v.UpdatedAt))
			e.observed["siri all"] = append(e.observed["siri all"], p.siri.Sub(v.UpdatedAt))
		default:
			keep = append(keep, p)
		}
	}
	e.pending = keep
}

func (e *evaluator) round(w io.Writer, n int) {
	r := eta.Summarise(e.all)
	r.Expected = e.expected["all"]
	r.Print(w, fmt.Sprintf("round %d", n))
}

func (e *evaluator) result(w io.Writer) {
	fmt.Fprintln(w, "\nresult")
	modes := make([]string, 0, len(e.byMode))
	for m := range e.byMode {
		modes = append(modes, m)
	}
	slices.Sort(modes)
	for _, m := range modes {
		r := eta.Summarise(e.byMode[m])
		r.Expected = e.expected[m]
		r.Print(w, m)
	}
	for _, b := range []string{"≤5 min", "5–10 min", "10–20 min", "at terminus", "en route"} {
		eta.Summarise(e.byAhead[b]).Print(w, b)
	}
	r := eta.Summarise(e.all)
	r.Expected = e.expected["all"]
	r.Print(w, "all")
	if len(e.observed) > 0 {
		fmt.Fprintln(w, "\nagainst arrivals observed in the GPS feed (predicted − observed)")
		for _, b := range []string{"≤5 min", "5–10 min", "10–20 min", "all"} {
			eta.Summarise(e.observed["ours "+b]).Print(w, "ours "+b)
			eta.Summarise(e.observed["siri "+b]).Print(w, "siri "+b)
		}
		fmt.Fprintln(w)
	}
	if r.Within1m >= 0.8 {
		fmt.Fprintln(w, "PASS: at least 80 % of predictions within ±1 min of SIRI")
	} else {
		fmt.Fprintln(w, "BELOW TARGET: fewer than 80 % within ±1 min of SIRI")
	}
}

// recEvent is one line of a recording: a GPS file as fetched, or a SIRI board
// as sampled. Replaying a recording re-runs the model on identical input, so
// changes to matching and ETAs can be compared without waiting for live data.
type recEvent struct {
	Kind         string           `json:"kind"` // "gps" or "siri"
	At           time.Time        `json:"at"`
	LastModified string           `json:"lm,omitempty"`
	Body         string           `json:"body,omitempty"`
	Stop         string           `json:"stop,omitempty"`
	Board        *feeds.SiriBoard `json:"board,omitempty"`
}

type recorder struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func (r *recorder) write(e recEvent) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	_ = r.enc.Encode(e)
}

// transport records every successful response passing through next.
func (r *recorder) transport(next http.RoundTripper) http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		res, err := next.RoundTrip(req)
		if err != nil || res.StatusCode != http.StatusOK {
			return res, err
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return nil, err
		}
		r.write(recEvent{Kind: "gps", At: time.Now(), LastModified: res.Header.Get("Last-Modified"), Body: string(body)})
		res.Body = io.NopCloser(bytes.NewReader(body))
		return res, nil
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// replayRecording runs the tracker over a recording in recorded order and time.
func replayRecording(ctx context.Context, path string, warmup time.Duration, ix *staticdata.Index, tracker *eta.Tracker, ev *evaluator) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	var cur recEvent
	clock := time.Time{}
	ev.observed = map[string][]time.Duration{}
	fetcher := feeds.NewFetcher(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		h := http.Header{}
		h.Set("Last-Modified", cur.LastModified)
		return &http.Response{StatusCode: http.StatusOK, Header: h, Body: io.NopCloser(strings.NewReader(cur.Body))}, nil
	})}, "replay")
	vehicles := state.NewStore()
	gps := &ingest.GPSPoller{URL: feeds.Const("http://replay/gps.txt"), Fetcher: fetcher, Store: vehicles,
		Now: func() time.Time { return clock }, Annotate: tracker.Annotate}
	dec := json.NewDecoder(bufio.NewReaderSize(f, 1<<20))
	var start time.Time
	for {
		cur = recEvent{}
		if err := dec.Decode(&cur); err == io.EOF || errors.Is(err, io.ErrUnexpectedEOF) {
			break // a recording still being written may end mid-line
		} else if err != nil {
			return err
		}
		if start.IsZero() {
			start = cur.At
		}
		clock = cur.At
		switch cur.Kind {
		case "gps":
			if _, err := gps.Poll(ctx); err != nil {
				return err
			}
			if g := tracker.Geometry(); g != nil && len(ev.pending) > 0 {
				_, vs := vehicles.Snapshot()
				ev.resolve(g, vs)
			}
		case "siri":
			st, ok := ix.Stop(cur.Stop)
			if !ok || cur.Board == nil || cur.At.Sub(start) < warmup {
				continue
			}
			_, vs := vehicles.Snapshot()
			ev.add(tracker, st, *cur.Board, vs, cur.At)
		}
	}
	ev.result(os.Stdout)
	return nil
}

// secOfDayNear converts SIRI seconds-of-day to a time near now.
func secOfDayNear(sec int, now time.Time, loc *time.Location) time.Time {
	n := now.In(loc)
	t := time.Date(n.Year(), n.Month(), n.Day(), 0, 0, sec, 0, loc)
	if t.Sub(now) < -12*time.Hour {
		t = t.AddDate(0, 0, 1)
	}
	return t
}
