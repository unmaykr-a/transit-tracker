package feeds

import (
	"io"
	"strconv"
	"strings"
)

// GPSVehicle is one row of transport.tallinn.ee/gps.txt.
type GPSVehicle struct {
	ID          string // "<mode>-<number>"; vehicle numbers repeat across modes
	Mode        Mode
	Line        string // as published: trams are "T1".."T6", buses "17A"
	Lat, Lon    float64
	Heading     int // degrees 0..359, -1 when unknown
	LowFloor    bool
	Destination string
}

var gpsModes = map[string]Mode{"1": ModeTrolley, "2": ModeBus, "3": ModeTram}

// ParseGPS parses gps.txt. Malformed rows are skipped and counted rather than
// failing the whole snapshot; err is only set when reading fails.
func ParseGPS(r io.Reader) (vehicles []GPSVehicle, skipped int, err error) {
	err = lines(r, func(line string) {
		v, ok := parseGPSLine(line)
		if !ok {
			skipped++
			return
		}
		vehicles = append(vehicles, v)
	})
	return vehicles, skipped, err
}

func parseGPSLine(row string) (GPSVehicle, bool) {
	f := strings.Split(row, ",")
	if len(f) < 10 {
		return GPSVehicle{}, false
	}
	mode, ok := gpsModes[f[0]]
	if !ok {
		return GPSVehicle{}, false
	}
	lon, err1 := strconv.Atoi(f[2])
	lat, err2 := strconv.Atoi(f[3])
	num := strings.TrimSpace(f[6])
	line := strings.TrimSpace(f[1])
	if err1 != nil || err2 != nil || lat == 0 || lon == 0 || num == "" || line == "" {
		return GPSVehicle{}, false
	}
	heading := -1
	if h, err := strconv.Atoi(f[5]); err == nil && h >= 0 && h < 360 {
		heading = h
	}
	return GPSVehicle{
		ID:          string(mode) + "-" + num,
		Mode:        mode,
		Line:        line,
		Lat:         float64(lat) / 1e6,
		Lon:         float64(lon) / 1e6,
		Heading:     heading,
		LowFloor:    f[7] == "Z",
		Destination: strings.TrimSpace(strings.Join(f[9:], ",")),
	}, true
}
