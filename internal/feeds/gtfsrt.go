package feeds

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"
	"time"
)

// GTFS-Realtime (protobuf) for the county buses: ÜTK Põhja (Harju county,
// Ridango AVL) publishes VehiclePositions and TripUpdates. Only the fields used
// are decoded, by hand, so there is no protobuf dependency; unknown fields are
// skipped as the wire format allows. Field numbers are from gtfs-realtime.proto.

// RTVehicle is one VehiclePosition.
type RTVehicle struct {
	ID        string // vehicle label (a licence plate for ÜTK Põhja), else its id
	TripID    string
	RouteID   string // "174_9721_1_3"
	Line      string // public short name, "174" (RTLine)
	Region    string // the county in the trip id, "harjumaa" (RTLine)
	StartTime string // scheduled trip start, "16:20:00"
	StartDate string // "20261023"
	Lat, Lon  float64
	Bearing   float64 // -1 when missing
	SpeedMps  float64
	HasSpeed  bool
	StopID    string // the current or next stop: the national stop code ("11903-2")
	At        time.Time
}

// RTTripUpdate is one TripUpdate: predicted times at the stops still ahead.
type RTTripUpdate struct {
	TripID    string
	RouteID   string
	Line      string
	Region    string
	StartTime string
	StartDate string
	Stops     []RTStopTime
}

// RTStopTime is a predicted arrival/departure; zero times are unknown.
type RTStopTime struct {
	StopID    string
	Seq       int
	Arrival   time.Time
	Departure time.Time
	Delay     *int // seconds, when the feed gives it
}

// RTFeed is a decoded GTFS-RT FeedMessage.
type RTFeed struct {
	At       time.Time
	Vehicles []RTVehicle
	Trips    []RTTripUpdate
}

var errProto = errors.New("gtfs-rt: malformed protobuf")

// ParseGTFSRT decodes a FeedMessage. Entities that fail to decode are skipped
// and counted; a broken message envelope is an error.
func ParseGTFSRT(r io.Reader) (RTFeed, int, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return RTFeed{}, 0, err
	}
	var f RTFeed
	skipped := 0
	err = fields(b, func(num int, wt int, v uint64, sub []byte) error {
		switch {
		case num == 1 && wt == 2: // header
			return fields(sub, func(n, wt int, v uint64, _ []byte) error {
				if n == 3 && wt == 0 && v > 0 {
					f.At = time.Unix(int64(v), 0)
				}
				return nil
			})
		case num == 2 && wt == 2: // entity
			if err := entity(sub, &f); err != nil {
				skipped++
			}
		}
		return nil
	})
	if err != nil {
		return RTFeed{}, skipped, fmt.Errorf("gtfs-rt: %w", err)
	}
	return f, skipped, nil
}

func entity(b []byte, f *RTFeed) error {
	return fields(b, func(num, wt int, _ uint64, sub []byte) error {
		if wt != 2 {
			return nil
		}
		switch num {
		case 3:
			t, err := tripUpdate(sub)
			if err != nil {
				return err
			}
			f.Trips = append(f.Trips, t)
		case 4:
			v, err := vehiclePosition(sub)
			if err != nil {
				return err
			}
			if v.ID != "" && (v.Lat != 0 || v.Lon != 0) {
				f.Vehicles = append(f.Vehicles, v)
			}
		}
		return nil
	})
}

type tripDesc struct{ tripID, routeID, start, date string }

func trip(b []byte) (tripDesc, error) {
	var t tripDesc
	err := fields(b, func(num, wt int, _ uint64, sub []byte) error {
		if wt != 2 {
			return nil
		}
		switch num {
		case 1:
			t.tripID = string(sub)
		case 2:
			t.start = string(sub)
		case 3:
			t.date = string(sub)
		case 5:
			t.routeID = string(sub)
		}
		return nil
	})
	return t, err
}

func vehiclePosition(b []byte) (RTVehicle, error) {
	v := RTVehicle{Bearing: -1}
	var vid, label string
	err := fields(b, func(num, wt int, x uint64, sub []byte) error {
		switch {
		case num == 1 && wt == 2:
			t, err := trip(sub)
			if err != nil {
				return err
			}
			v.TripID, v.RouteID, v.StartTime, v.StartDate = t.tripID, t.routeID, t.start, t.date
		case num == 2 && wt == 2:
			return fields(sub, func(n, wt int, x uint64, _ []byte) error {
				f := float64(math.Float32frombits(uint32(x)))
				if wt != 5 {
					return nil
				}
				switch n {
				case 1:
					v.Lat = f
				case 2:
					v.Lon = f
				case 3:
					v.Bearing = f
				case 5:
					v.SpeedMps, v.HasSpeed = f, true
				}
				return nil
			})
		case num == 5 && wt == 0:
			v.At = time.Unix(int64(x), 0)
		case num == 7 && wt == 2:
			v.StopID = string(sub)
		case num == 8 && wt == 2:
			return fields(sub, func(n, wt int, _ uint64, s []byte) error {
				if wt == 2 && n == 1 {
					vid = string(s)
				} else if wt == 2 && n == 2 {
					label = string(s)
				}
				return nil
			})
		}
		return nil
	})
	v.ID = strings.TrimSpace(label)
	if v.ID == "" {
		v.ID = strings.TrimSpace(vid)
	}
	v.Region, v.Line = RTLine(v.RouteID, v.TripID)
	return v, err
}

func tripUpdate(b []byte) (RTTripUpdate, error) {
	var t RTTripUpdate
	err := fields(b, func(num, wt int, _ uint64, sub []byte) error {
		if wt != 2 {
			return nil
		}
		switch num {
		case 1:
			d, err := trip(sub)
			if err != nil {
				return err
			}
			t.TripID, t.RouteID, t.StartTime, t.StartDate = d.tripID, d.routeID, d.start, d.date
		case 2:
			var st RTStopTime
			err := fields(sub, func(n, wt int, x uint64, s []byte) error {
				switch {
				case n == 1 && wt == 0:
					st.Seq = int(x)
				case (n == 2 || n == 3) && wt == 2:
					var at time.Time
					var delay *int
					if err := fields(s, func(n, wt int, x uint64, _ []byte) error {
						if wt == 0 && n == 2 && x > 0 {
							at = time.Unix(int64(x), 0)
						} else if wt == 0 && n == 1 {
							d := int(int32(x)) // int32, not zigzag
							delay = &d
						}
						return nil
					}); err != nil {
						return err
					}
					if n == 2 {
						st.Arrival = at
					} else {
						st.Departure = at
					}
					if delay != nil {
						st.Delay = delay
					}
				case n == 4 && wt == 2:
					st.StopID = string(s)
				}
				return nil
			})
			if err != nil {
				return err
			}
			t.Stops = append(t.Stops, st)
		}
		return nil
	})
	t.Region, t.Line = RTLine(t.RouteID, t.TripID)
	return t, err
}

// rtTripLine finds the county and line in a Ridango trip id:
// "…-Fr-harjumaa174-A-OB-1620-…", "…-Fr-raplamaaK28-…", "…-Fr-harjumaaSV14-…", "…-Fr-laane_viru18V-…".
var rtTripLine = regexp.MustCompile(`-Fr-([a-z_]+?)([A-Z]{0,2}[0-9]+[A-Za-z]?)-`)

// RTLine reads the county ("harjumaa") and public line number ("174") of a
// trip. The line comes from the route id's first part ("174_9721_1_3") when
// there is one, else from the trip id, which also names the county.
func RTLine(routeID, tripID string) (region, line string) {
	if m := rtTripLine.FindStringSubmatch(tripID); m != nil {
		region, line = m[1], m[2]
	}
	if routeID != "" {
		line, _, _ = strings.Cut(routeID, "_")
	}
	return region, line
}

// fields walks one protobuf message, calling fn with each field: varint and
// fixed values in v, length-delimited ones in sub.
func fields(b []byte, fn func(num, wt int, v uint64, sub []byte) error) error {
	for len(b) > 0 {
		key, n := binary.Uvarint(b)
		if n <= 0 {
			return errProto
		}
		b = b[n:]
		num, wt := int(key>>3), int(key&7)
		var v uint64
		var sub []byte
		switch wt {
		case 0:
			v, n = binary.Uvarint(b)
			if n <= 0 {
				return errProto
			}
			b = b[n:]
		case 1:
			if len(b) < 8 {
				return errProto
			}
			v, b = binary.LittleEndian.Uint64(b), b[8:]
		case 2:
			l, n := binary.Uvarint(b)
			if n <= 0 || uint64(len(b)-n) < l {
				return errProto
			}
			sub, b = b[n:n+int(l)], b[n+int(l):]
		case 5:
			if len(b) < 4 {
				return errProto
			}
			v, b = uint64(binary.LittleEndian.Uint32(b)), b[4:]
		default:
			return errProto
		}
		if err := fn(num, wt, v, sub); err != nil {
			return err
		}
	}
	return nil
}
