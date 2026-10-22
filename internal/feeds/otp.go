package feeds

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// OTP is a client for the peatus.ee OpenTripPlanner GraphQL API.
type OTP struct {
	URL     func() string
	Fetcher *Fetcher
}

// DefaultOTPURL is the national peatus.ee router.
const DefaultOTPURL = "https://api.peatus.ee/routing/v1/routers/estonia/index/graphql"

// TallinnAgencyID is Tallinna Linnatransport (TLT) in the national feed.
const TallinnAgencyID = "estonia:tallinn_10312960"

type OTPStop struct {
	GtfsID string  `json:"gtfsId"`
	Code   string  `json:"code"`
	Name   string  `json:"name"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
}

type OTPPattern struct {
	Code        string    `json:"code"`
	DirectionID int       `json:"directionId"`
	Headsign    string    `json:"headsign"`
	Stops       []OTPStop `json:"stops"`
	Geometry    *struct {
		Points string `json:"points"`
	} `json:"patternGeometry"`
}

type OTPRoute struct {
	GtfsID    string       `json:"gtfsId"`
	ShortName string       `json:"shortName"`
	LongName  string       `json:"longName"`
	Mode      string       `json:"mode"` // BUS, TRAM, RAIL… (OTP reports trolleybuses as BUS)
	Color     string       `json:"color"`
	TextColor string       `json:"textColor"`
	Patterns  []OTPPattern `json:"patterns"`
}

// CityMode maps an OTP route to our mode. Tallinn's route ids carry the real
// mode ("tallinna-lin_trol_72") because OTP files trolleybuses under BUS.
func (r OTPRoute) CityMode() Mode {
	id := strings.ToLower(r.GtfsID)
	switch {
	case strings.Contains(id, "_trol_"):
		return ModeTrolley
	case strings.Contains(id, "_tram_") || r.Mode == "TRAM":
		return ModeTram
	case r.Mode == "RAIL":
		return ModeTrain
	default:
		return ModeBus
	}
}

type OTPStopTime struct {
	ScheduledDeparture int    `json:"scheduledDeparture"` // seconds after serviceDay
	RealtimeDeparture  int    `json:"realtimeDeparture"`
	DepartureDelay     int    `json:"departureDelay"`
	Realtime           bool   `json:"realtime"`
	ServiceDay         int64  `json:"serviceDay"` // unix seconds of the service day's local noon-minus-12h
	Headsign           string `json:"headsign"`
	Trip               struct {
		GtfsID        string `json:"gtfsId"`
		TripShortName string `json:"tripShortName"`
		Route         struct {
			GtfsID    string `json:"gtfsId"`
			ShortName string `json:"shortName"`
			LongName  string `json:"longName"`
			Mode      string `json:"mode"`
		} `json:"route"`
	} `json:"trip"`
}

type gqlError struct {
	Message string `json:"message"`
}

// Query runs any GraphQL query against the OTP endpoint.
func (o *OTP) Query(ctx context.Context, q string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": q, "variables": vars})
	if err != nil {
		return err
	}
	res, err := o.Fetcher.PostJSON(ctx, o.URL(), body)
	if err != nil {
		return err
	}
	return decodeGraphQL(res.Body, out)
}

func decodeGraphQL(b []byte, out any) error {
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []gqlError      `json:"errors"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return fmt.Errorf("otp: %w", err)
	}
	if len(env.Errors) > 0 {
		return fmt.Errorf("otp: %s", env.Errors[0].Message)
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return errors.New("otp: empty response")
	}
	return json.Unmarshal(env.Data, out)
}

const agencyRoutesQuery = `query($id: String!) { agency(id: $id) { routes {
  gtfsId shortName longName mode color textColor
  patterns { code directionId headsign stops { gtfsId code name lat lon } patternGeometry { points } }
} } }`

// AgencyRoutes returns every route of an agency with its patterns, stops and geometry.
func (o *OTP) AgencyRoutes(ctx context.Context, agencyID string) ([]OTPRoute, error) {
	var d struct {
		Agency *struct {
			Routes []OTPRoute `json:"routes"`
		} `json:"agency"`
	}
	if err := o.Query(ctx, agencyRoutesQuery, map[string]any{"id": agencyID}, &d); err != nil {
		return nil, err
	}
	if d.Agency == nil {
		return nil, fmt.Errorf("otp: agency %s not found", agencyID)
	}
	return d.Agency.Routes, nil
}

// ParseAgencyRoutes decodes a saved AgencyRoutes response (fixtures, fake feeds).
func ParseAgencyRoutes(b []byte) ([]OTPRoute, error) {
	var d struct {
		Agency struct {
			Routes []OTPRoute `json:"routes"`
		} `json:"agency"`
	}
	if err := decodeGraphQL(b, &d); err != nil {
		return nil, err
	}
	return d.Agency.Routes, nil
}

const stopTimesQuery = `query($ids: [String], $n: Int, $range: Int) { stops(ids: $ids) {
  gtfsId
  stoptimesWithoutPatterns(numberOfDepartures: $n, timeRange: $range, omitNonPickups: true) {
    scheduledDeparture realtimeDeparture departureDelay realtime serviceDay headsign
    trip { gtfsId tripShortName route { gtfsId shortName longName mode } }
  }
} }`

// StopTimes returns upcoming departures for several stops (gtfs ids like "estonia:10137") in one request.
func (o *OTP) StopTimes(ctx context.Context, ids []string, n int, rangeSec int) (map[string][]OTPStopTime, error) {
	var d struct {
		Stops []struct {
			GtfsID    string        `json:"gtfsId"`
			StopTimes []OTPStopTime `json:"stoptimesWithoutPatterns"`
		} `json:"stops"`
	}
	if err := o.Query(ctx, stopTimesQuery, map[string]any{"ids": ids, "n": n, "range": rangeSec}, &d); err != nil {
		return nil, err
	}
	out := make(map[string][]OTPStopTime, len(d.Stops))
	for _, s := range d.Stops {
		out[s.GtfsID] = s.StopTimes
	}
	return out, nil
}
