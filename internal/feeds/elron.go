package feeds

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// ElronTrain is one train from elron.ee/map_data.json.
type ElronTrain struct {
	Trip       string // "reis", the public trip number
	Route      string // "liin", e.g. "Tallinn - Tartu" (a name, not a GTFS short name)
	StartTime  string // scheduled trip start, "HH:MM"
	EndTime    string
	SpeedKmh   float64
	Lat, Lon   float64
	Heading    int // -1 when unknown
	DelayMin   int
	DelayKnown bool
	Notice     string // "lisateade"
	Reason     string // "pohjus_teade"
	Status     string // "reisi_staatus", e.g. "plaaniline"
	LastStop   string
	UpdatedAt  time.Time // zero when missing or unparsable
}

// ElronStation is one entry of elron.ee/stops_data.json.
type ElronStation struct {
	Name     string
	Lat, Lon float64
	Notice   string
}

// flexString accepts a JSON string or number; Elron sends everything as strings
// today, but numbers would be the obvious change.
type flexString string

func (s *flexString) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*s = ""
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = flexString(v)
		return nil
	}
	*s = flexString(b)
	return nil
}

func (s flexString) str() string { return strings.TrimSpace(string(s)) }

func (s flexString) float() (float64, bool) {
	v, err := strconv.ParseFloat(s.str(), 64)
	return v, err == nil
}

type elronEnvelope[T any] struct {
	Status json.Number `json:"status"`
	Data   []T         `json:"data"`
}

type elronTrainJSON struct {
	Reis, Liin, ReisiAlgusAeg, ReisiLoppAeg, Kiirus, Latitude, Longitude, RongiSuund,
	ErinevusPlaanist, Lisateade, PohjusTeade, AsukohaUuendus, ReisiStaatus, ViimanePeatus flexString
}

// Elron uses snake_case keys; map them explicitly instead of tagging every field.
func (t *elronTrainJSON) UnmarshalJSON(b []byte) error {
	var m map[string]flexString
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	*t = elronTrainJSON{
		Reis: m["reis"], Liin: m["liin"], ReisiAlgusAeg: m["reisi_algus_aeg"], ReisiLoppAeg: m["reisi_lopp_aeg"],
		Kiirus: m["kiirus"], Latitude: m["latitude"], Longitude: m["longitude"], RongiSuund: m["rongi_suund"],
		ErinevusPlaanist: m["erinevus_plaanist"], Lisateade: m["lisateade"], PohjusTeade: m["pohjus_teade"],
		AsukohaUuendus: m["asukoha_uuendus"], ReisiStaatus: m["reisi_staatus"], ViimanePeatus: m["viimane_peatus"],
	}
	return nil
}

func decodeElron[T any](r io.Reader) ([]T, error) {
	var env elronEnvelope[T]
	if err := json.NewDecoder(r).Decode(&env); err != nil {
		return nil, fmt.Errorf("elron: %w", err)
	}
	if env.Status.String() != "1" {
		return nil, fmt.Errorf("elron: status %q", env.Status)
	}
	return env.Data, nil
}

// ParseElronTrains parses map_data.json. Timestamps are interpreted in loc
// (Europe/Tallinn). Trains without a usable position are skipped and counted.
func ParseElronTrains(r io.Reader, loc *time.Location) (trains []ElronTrain, skipped int, err error) {
	raw, err := decodeElron[elronTrainJSON](r)
	if err != nil {
		return nil, 0, err
	}
	for _, t := range raw {
		lat, ok1 := t.Latitude.float()
		lon, ok2 := t.Longitude.float()
		if !ok1 || !ok2 || lat == 0 || lon == 0 || t.Reis.str() == "" {
			skipped++
			continue
		}
		tr := ElronTrain{
			Trip:      t.Reis.str(),
			Route:     t.Liin.str(),
			StartTime: t.ReisiAlgusAeg.str(),
			EndTime:   t.ReisiLoppAeg.str(),
			Lat:       lat,
			Lon:       lon,
			Heading:   -1,
			Notice:    cleanNotice(t.Lisateade.str()),
			Reason:    t.PohjusTeade.str(),
			Status:    t.ReisiStaatus.str(),
			LastStop:  t.ViimanePeatus.str(),
		}
		tr.SpeedKmh, _ = t.Kiirus.float()
		if h, ok := t.RongiSuund.float(); ok && h >= 0 && h < 360 {
			tr.Heading = int(h)
		}
		if d, err := strconv.Atoi(t.ErinevusPlaanist.str()); err == nil {
			tr.DelayMin, tr.DelayKnown = d, true
		}
		if ts, err := time.ParseInLocation(time.DateTime, t.AsukohaUuendus.str(), loc); err == nil {
			tr.UpdatedAt = ts
		}
		trains = append(trains, tr)
	}
	return trains, skipped, nil
}

// cleanNotice drops placeholder notices; Elron sends "." when there is nothing to say.
func cleanNotice(s string) string {
	if strings.Trim(s, ". ") == "" {
		return ""
	}
	return s
}

type elronStationJSON struct {
	Peatus    flexString `json:"peatus"`
	Latitude  flexString `json:"latitude"`
	Longitude flexString `json:"longitude"`
	Teade     flexString `json:"teade"`
}

// ParseElronStations parses stops_data.json. Stations without coordinates are skipped and counted.
func ParseElronStations(r io.Reader) (stations []ElronStation, skipped int, err error) {
	raw, err := decodeElron[elronStationJSON](r)
	if err != nil {
		return nil, 0, err
	}
	for _, s := range raw {
		lat, ok1 := s.Latitude.float()
		lon, ok2 := s.Longitude.float()
		if !ok1 || !ok2 || s.Peatus.str() == "" {
			skipped++
			continue
		}
		stations = append(stations, ElronStation{Name: s.Peatus.str(), Lat: lat, Lon: lon, Notice: cleanNotice(s.Teade.str())})
	}
	return stations, skipped, nil
}
