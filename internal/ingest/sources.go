package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"strings"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/state"
)

// GPSPoller polls transport.tallinn.ee/gps.txt into the store.
type GPSPoller struct {
	URL     string
	Fetcher *feeds.Fetcher
	Store   *state.Store
	Now     func() time.Time

	lastHash [32]byte
}

func (p *GPSPoller) Poll(ctx context.Context) (Result, error) {
	res, err := p.Fetcher.Get(ctx, p.URL, true)
	if err != nil {
		return Result{}, err
	}
	if res.NotModified {
		return Result{Unchanged: true, Latency: res.Latency}, nil
	}
	h := sha256.Sum256(res.Body)
	if h == p.lastHash {
		return Result{Unchanged: true, Latency: res.Latency}, nil
	}
	gps, _, err := feeds.ParseGPS(bytes.NewReader(res.Body))
	if err != nil {
		return Result{}, err
	}
	at := res.LastModified
	now := p.now()
	// A Last-Modified in the future or far past is a clock problem; trust our own clock.
	if at.IsZero() || at.After(now.Add(time.Minute)) || now.Sub(at) > 10*time.Minute {
		at = now
	}
	vs := make([]state.Vehicle, 0, len(gps))
	for _, g := range gps {
		vs = append(vs, state.Vehicle{
			ID:          g.ID,
			Mode:        g.Mode,
			Line:        g.Line,
			Label:       g.Line,
			Lat:         g.Lat,
			Lon:         g.Lon,
			Bearing:     float64(g.Heading),
			Destination: g.Destination,
			LowFloor:    g.LowFloor,
			UpdatedAt:   at,
		})
	}
	p.Store.Replace("gps", vs)
	p.lastHash = h
	return Result{Count: len(vs), DataAt: at, Latency: res.Latency}, nil
}

func (p *GPSPoller) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// ElronPoller polls elron.ee/map_data.json into the store.
type ElronPoller struct {
	URL      string
	Fetcher  *feeds.Fetcher
	Store    *state.Store
	Location *time.Location
}

func (p *ElronPoller) Poll(ctx context.Context) (Result, error) {
	// Elron sends a 14-day max-age and no useful validators; always fetch the body.
	res, err := p.Fetcher.Get(ctx, p.URL, false)
	if err != nil {
		return Result{}, err
	}
	trains, _, err := feeds.ParseElronTrains(bytes.NewReader(res.Body), p.Location)
	if err != nil {
		return Result{}, err
	}
	var newest time.Time
	vs := make([]state.Vehicle, 0, len(trains))
	for _, t := range trains {
		v := TrainVehicle(t)
		if v.UpdatedAt.After(newest) {
			newest = v.UpdatedAt
		}
		vs = append(vs, v)
	}
	p.Store.Replace("elron", vs)
	if newest.IsZero() || len(vs) == 0 {
		newest = time.Now() // no trains running (night) is fresh data, not a stale feed
	}
	return Result{Count: len(vs), DataAt: newest, Latency: res.Latency}, nil
}

// TrainVehicle normalises one Elron train.
func TrainVehicle(t feeds.ElronTrain) state.Vehicle {
	v := state.Vehicle{
		ID:          "train-" + t.Trip,
		Mode:        feeds.ModeTrain,
		Line:        t.Route,
		Label:       t.Trip,
		Lat:         t.Lat,
		Lon:         t.Lon,
		Bearing:     float64(t.Heading),
		SpeedMps:    t.SpeedKmh / 3.6,
		SpeedKnown:  true,
		Destination: TrainDestination(t.Route),
		UpdatedAt:   t.UpdatedAt,
	}
	if t.DelayKnown {
		d := t.DelayMin * 60
		v.DelayS = &d
	}
	if v.UpdatedAt.IsZero() {
		v.UpdatedAt = time.Now()
	}
	return v
}

// TrainDestination takes the terminus from an Elron route name:
// "Tallinn - Tartu" → "Tartu", "Tartu-Tallinn" → "Tallinn".
func TrainDestination(route string) string {
	i := strings.LastIndex(route, "-")
	if i < 0 {
		return strings.TrimSpace(route)
	}
	return strings.TrimSpace(route[i+1:])
}
