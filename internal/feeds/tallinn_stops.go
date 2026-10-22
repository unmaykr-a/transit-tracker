package feeds

import (
	"io"
	"strconv"
	"strings"
)

// TallinnStop is one platform from transport.tallinn.ee/data/stops.txt.
type TallinnStop struct {
	Code     string // national stop code, e.g. "12204-1" (= OTP code, GTFS stop_code)
	SiriID   string // id for siri-stop-departures.php (= OTP gtfs id number)
	Lat, Lon float64
	Name     string
	Street   string
	Area     string
	City     string
}

// ParseTallinnStops parses the semicolon-separated "Pikas" stops file, where
// an empty Name/Street/Area/City inherits the previous row's value. Stop-area
// rows (ids starting with "a") and rows without coordinates are skipped and counted.
func ParseTallinnStops(r io.Reader) (stops []TallinnStop, skipped int, err error) {
	var prev TallinnStop
	header := true
	err = lines(r, func(line string) {
		if header {
			header = false
			return
		}
		f := strings.Split(line, ";")
		for len(f) < 10 {
			f = append(f, "")
		}
		s := TallinnStop{Code: strings.TrimSpace(f[0]), SiriID: strings.TrimSpace(f[1])}
		s.Name = inherit(f[5], prev.Name)
		s.Street = inherit(f[7], prev.Street)
		s.Area = inherit(f[8], prev.Area)
		s.City = inherit(f[9], prev.City)
		lat, err1 := strconv.Atoi(strings.TrimSpace(f[2]))
		lon, err2 := strconv.Atoi(strings.TrimSpace(f[3]))
		prev = s
		if strings.HasPrefix(s.Code, "a") || s.Code == "" || err1 != nil || err2 != nil || lat == 0 || lon == 0 {
			skipped++
			return
		}
		s.Lat, s.Lon = float64(lat)/1e5, float64(lon)/1e5
		stops = append(stops, s)
	})
	return stops, skipped, err
}

func inherit(v, prev string) string {
	if v = strings.TrimSpace(v); v == "" || v == "0" {
		return prev
	}
	return v
}
