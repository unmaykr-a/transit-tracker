// Package staticdata imports stops, routes and patterns (nightly), persists them
// in SQLite, and serves them from an in-memory index for search and line/stop pages.
package staticdata

import (
	"time"

	"github.com/unmaykr-a/transit-tracker/internal/feeds"
)

// Stop is one platform (a pole or a station track) where vehicles call.
type Stop struct {
	ID       string       `json:"id"` // OTP/GTFS id; equals the Tallinn SiriID for city stops
	Code     string       `json:"code"`
	SiriID   string       `json:"-"`
	Name     string       `json:"name"`
	Platform string       `json:"platform"` // derived letter within the area, "" when the area has one stop
	Street   string       `json:"street,omitempty"`
	Lat      float64      `json:"lat"`
	Lon      float64      `json:"lon"`
	Modes    []feeds.Mode `json:"modes"`
	AreaID   string       `json:"area_id"`
	Aliases  []string     `json:"-"`
	Source   string       `json:"-"` // "tallinn" or "elron"
}

// OTPID is the stop id in the peatus.ee OpenTripPlanner API.
func (s Stop) OTPID() string { return "estonia:" + s.ID }

// Area groups the platforms of one named stop ("Balti jaam" with its tram,
// bus and rail platforms). Search results and stop pages are areas.
type Area struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Lat     float64      `json:"lat"`
	Lon     float64      `json:"lon"`
	Modes   []feeds.Mode `json:"modes"`
	StopIDs []string     `json:"-"`
}

// Route is a line, e.g. tram T4 or Elron R12.
type Route struct {
	ID        string     `json:"id"` // "<mode>:<short_name>"
	Mode      feeds.Mode `json:"mode"`
	ShortName string     `json:"short_name"`
	LongName  string     `json:"long_name"`
	Color     string     `json:"color,omitempty"` // "#rrggbb"
	TextColor string     `json:"text_color,omitempty"`
	Source    string     `json:"source"` // tallinn, county, tartu, intercity or elron
	Sort      int        `json:"-"`
	// OTPIDs are the national timetable's ids of this route (county, Tartu and
	// long-distance lines), for departures where numbers repeat across regions.
	OTPIDs []string `json:"-"`
}

// RouteID builds the route id for a mode and short name.
func RouteID(mode feeds.Mode, short string) string { return string(mode) + ":" + short }

// Pattern is one stop sequence of a route in one direction.
type Pattern struct {
	ID        string       `json:"id"`
	RouteID   string       `json:"route_id"`
	Direction int          `json:"direction"`
	Headsign  string       `json:"headsign"`
	StopIDs   []string     `json:"stop_ids"`
	Shape     [][2]float64 `json:"-"` // [lon, lat]
	Trips     int          `json:"-"`
	Sort      int          `json:"-"`
}

// Dataset is one complete import.
type Dataset struct {
	Stops      []Stop
	Areas      []Area
	Routes     []Route
	Patterns   []Pattern
	ImportedAt time.Time
	// BuildVersion is set on loaded datasets; see the BuildVersion constant.
	BuildVersion string
}
