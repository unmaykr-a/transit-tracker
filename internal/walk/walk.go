// Package walk estimates walking time from a point to a stop. Real pedestrian
// routes come from peatus.ee OTP or Transitous; a straight-line estimate is
// the last resort and is marked approximate.
package walk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/cache"
	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/geo"
)

// Point is a WGS84 coordinate.
type Point struct{ Lat, Lon float64 }

// Route is a provider's answer: the walking distance along footpaths.
type Route struct {
	DistanceM float64
	Provider  string
	Approx    bool // straight-line estimate, not a real route
}

// Provider computes a walking route.
type Provider interface {
	Name() string
	Route(ctx context.Context, from, to Point) (Route, error)
}

// Provider names, used in the admin "walking provider order" setting.
const (
	ProviderOTP        = "otp"
	ProviderTransitous = "transitous"
	ProviderHaversine  = "haversine"
)

// DetourFactor turns a straight line into a typical city walking distance.
const DetourFactor = 1.3

// Haversine is the last-resort provider.
type Haversine struct{}

func (Haversine) Name() string { return ProviderHaversine }
func (Haversine) Route(_ context.Context, from, to Point) (Route, error) {
	return Route{DistanceM: geo.DistanceM(from.Lat, from.Lon, to.Lat, to.Lon) * DetourFactor, Provider: ProviderHaversine, Approx: true}, nil
}

// OTP asks peatus.ee OpenTripPlanner for a walk-only itinerary.
type OTP struct{ Client *feeds.OTP }

func (OTP) Name() string { return ProviderOTP }

const otpWalkQuery = `query($fromLat: Float!, $fromLon: Float!, $toLat: Float!, $toLon: Float!) {
  plan(from: {lat: $fromLat, lon: $fromLon}, to: {lat: $toLat, lon: $toLon}, transportModes: [{mode: WALK}], numItineraries: 1) {
    itineraries { walkDistance duration }
  }
}`

func (o OTP) Route(ctx context.Context, from, to Point) (Route, error) {
	var d struct {
		Plan struct {
			Itineraries []struct {
				WalkDistance float64 `json:"walkDistance"`
			} `json:"itineraries"`
		} `json:"plan"`
	}
	vars := map[string]any{"fromLat": from.Lat, "fromLon": from.Lon, "toLat": to.Lat, "toLon": to.Lon}
	if err := o.Client.Query(ctx, otpWalkQuery, vars, &d); err != nil {
		return Route{}, err
	}
	if len(d.Plan.Itineraries) == 0 || d.Plan.Itineraries[0].WalkDistance <= 0 {
		return Route{}, errors.New("otp: no walking route")
	}
	return Route{DistanceM: d.Plan.Itineraries[0].WalkDistance, Provider: ProviderOTP}, nil
}

// Transitous asks the Transitous (MOTIS) router for a direct walk.
type Transitous struct {
	URL     func() string // e.g. https://api.transitous.org/api/v5/plan
	Fetcher *feeds.Fetcher
}

func (Transitous) Name() string { return ProviderTransitous }

func (t Transitous) Route(ctx context.Context, from, to Point) (Route, error) {
	q := url.Values{}
	q.Set("fromPlace", fmt.Sprintf("%.6f,%.6f", from.Lat, from.Lon))
	q.Set("toPlace", fmt.Sprintf("%.6f,%.6f", to.Lat, to.Lon))
	q.Set("directModes", "WALK")
	q.Set("transitModes", "WALK")
	q.Set("maxDirectTime", "7200")
	res, err := t.Fetcher.Get(ctx, t.URL()+"?"+q.Encode(), false)
	if err != nil {
		return Route{}, err
	}
	var d struct {
		Direct []struct {
			Legs []struct {
				Distance float64 `json:"distance"`
			} `json:"legs"`
		} `json:"direct"`
	}
	if err := json.Unmarshal(res.Body, &d); err != nil {
		return Route{}, fmt.Errorf("transitous: %w", err)
	}
	if len(d.Direct) == 0 {
		return Route{}, errors.New("transitous: no walking route")
	}
	var dist float64
	for _, l := range d.Direct[0].Legs {
		dist += l.Distance
	}
	if dist <= 0 {
		return Route{}, errors.New("transitous: empty walking route")
	}
	return Route{DistanceM: dist, Provider: ProviderTransitous}, nil
}

// MaxRoutedM is the straight-line distance beyond which walks are not routed.
const MaxRoutedM = 5000

// Service tries providers in the configured order and caches answers per
// (origin rounded to a ~25 m grid, destination) for 10 minutes, so GPS jitter
// does not hammer the providers.
type Service struct {
	Providers map[string]Provider
	Order     func() []string
	cache     cache.TTL[string, Route]
}

func NewService(providers []Provider, order func() []string) *Service {
	m := map[string]Provider{}
	for _, p := range providers {
		m[p.Name()] = p
	}
	return &Service{Providers: m, Order: order, cache: cache.TTL[string, Route]{TTL: 10 * time.Minute, StaleFor: time.Hour}}
}

// gridKey snaps a point to roughly 25 m cells.
func gridKey(p Point) string {
	const cell = 25.0
	latStep := cell / 111_320
	lonStep := cell / (111_320 * math.Cos(p.Lat*math.Pi/180))
	return strconv.FormatInt(int64(math.Floor(p.Lat/latStep)), 36) + "," + strconv.FormatInt(int64(math.Floor(p.Lon/lonStep)), 36)
}

// Route returns the walking route from origin to a stop (destKey identifies it in the cache).
func (s *Service) Route(ctx context.Context, from Point, destKey string, to Point) (Route, error) {
	// Far away, walking time hardly matters and routing it would only cost the
	// upstreams a request per origin: the straight-line estimate does.
	if geo.DistanceM(from.Lat, from.Lon, to.Lat, to.Lon) > MaxRoutedM {
		return Haversine{}.Route(ctx, from, to)
	}
	return s.cache.Get(ctx, gridKey(from)+"→"+destKey, func(ctx context.Context) (Route, error) {
		for _, name := range s.Order() {
			p, ok := s.Providers[name]
			if !ok {
				continue
			}
			pctx, cancel := context.WithTimeout(ctx, 4*time.Second)
			r, err := p.Route(pctx, from, to)
			cancel()
			if err == nil {
				return r, nil
			}
		}
		// Every provider failed (or none is configured): the straight-line
		// estimate keeps the verdict available, marked approximate.
		return Haversine{}.Route(ctx, from, to)
	})
}

// Minutes converts a distance to whole walking minutes at speedKmh (rounded up).
func Minutes(distanceM, speedKmh float64) int {
	if speedKmh <= 0 {
		speedKmh = 4.8
	}
	return int(math.Ceil(distanceM / (speedKmh * 1000 / 60)))
}
