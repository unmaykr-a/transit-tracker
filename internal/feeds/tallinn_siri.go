package feeds

import (
	"errors"
	"io"
	"strconv"
	"strings"
)

// SiriBoard is the response of siri-stop-departures.php for one stop.
type SiriBoard struct {
	StopID string // SiriID echoed by the "stop," row; empty when there are no departures
	// ServerSecOfDay is the upstream clock as seconds since local midnight.
	ServerSecOfDay int
	Departures     []SiriDeparture
}

// SiriDeparture times are seconds since local midnight of the service day and
// may exceed 86400 for trips after midnight.
type SiriDeparture struct {
	Mode         Mode
	Line         string
	ExpectedSec  int
	ScheduledSec int
	Destination  string
	SecondsUntil int
	LowFloor     bool
}

// DelaySec is expected minus scheduled; negative means early.
func (d SiriDeparture) DelaySec() int { return d.ExpectedSec - d.ScheduledSec }

// ErrSiriHeader means the response did not start with the expected header row,
// i.e. the upstream format changed or an error page came back.
var ErrSiriHeader = errors.New("siri: missing Transport header row")

var siriModes = map[string]Mode{"bus": ModeBus, "tram": ModeTram, "trol": ModeTrolley}

// ParseSiri parses one stop's departures. Malformed departure rows are skipped and counted.
func ParseSiri(r io.Reader) (b SiriBoard, skipped int, err error) {
	sawHeader := false
	err = lines(r, func(line string) {
		f := strings.Split(line, ",")
		switch {
		case !sawHeader:
			if f[0] != "Transport" || len(f) < 5 {
				return
			}
			sawHeader = true
			b.ServerSecOfDay, _ = strconv.Atoi(f[4])
		case f[0] == "stop" && len(f) >= 2:
			b.StopID = strings.TrimSpace(f[1])
		default:
			d, ok := parseSiriLine(f)
			if !ok {
				skipped++
				return
			}
			b.Departures = append(b.Departures, d)
		}
	})
	if err == nil && !sawHeader {
		err = ErrSiriHeader
	}
	return b, skipped, err
}

func parseSiriLine(f []string) (SiriDeparture, bool) {
	if len(f) < 7 {
		return SiriDeparture{}, false
	}
	mode, ok := siriModes[f[0]]
	if !ok {
		return SiriDeparture{}, false
	}
	exp, err1 := strconv.Atoi(f[2])
	sch, err2 := strconv.Atoi(f[3])
	until, err3 := strconv.Atoi(f[len(f)-2])
	if err1 != nil || err2 != nil || err3 != nil || f[1] == "" {
		return SiriDeparture{}, false
	}
	return SiriDeparture{
		Mode:         mode,
		Line:         strings.TrimSpace(f[1]),
		ExpectedSec:  exp,
		ScheduledSec: sch,
		// Destinations may contain commas; everything between the times and the trailing two fields.
		Destination:  strings.TrimSpace(strings.Join(f[4:len(f)-2], ",")),
		SecondsUntil: until,
		LowFloor:     f[len(f)-1] == "Z",
	}, true
}
