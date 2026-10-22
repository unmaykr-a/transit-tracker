package feeds

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// GTFS holds the parts of a static GTFS feed the app needs (Elron rail).
type GTFS struct {
	Stops     map[string]GTFSStop
	Routes    []GTFSRoute
	Trips     []GTFSTrip
	StopTimes map[string][]string // trip_id → stop_ids in stop_sequence order
	Shapes    map[string][][2]float64
}

type GTFSStop struct {
	ID, Code, Name, Alias, Area string
	Lat, Lon                    float64
}

type GTFSRoute struct {
	ID, ShortName, LongName, Color, TextColor string
	Type                                      int
}

type GTFSTrip struct {
	ID, RouteID, ShapeID, Headsign, ShortName string
	DirectionID                               int
}

// ReadGTFSZip reads a GTFS zip archive.
func ReadGTFSZip(b []byte) (*GTFS, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, fmt.Errorf("gtfs: %w", err)
	}
	return ReadGTFS(zr)
}

// ReadGTFS reads routes, trips, stop_times, stops and shapes from fsys.
func ReadGTFS(fsys fs.FS) (*GTFS, error) {
	g := &GTFS{Stops: map[string]GTFSStop{}, StopTimes: map[string][]string{}, Shapes: map[string][][2]float64{}}

	if err := eachRow(fsys, "routes.txt", func(r row) {
		t, _ := strconv.Atoi(r.get("route_type"))
		g.Routes = append(g.Routes, GTFSRoute{ID: r.get("route_id"), ShortName: r.get("route_short_name"), LongName: r.get("route_long_name"), Color: r.get("route_color"), TextColor: r.get("route_text_color"), Type: t})
	}); err != nil {
		return nil, err
	}
	if err := eachRow(fsys, "trips.txt", func(r row) {
		d, _ := strconv.Atoi(r.get("direction_id"))
		g.Trips = append(g.Trips, GTFSTrip{ID: r.get("trip_id"), RouteID: r.get("route_id"), ShapeID: r.get("shape_id"), Headsign: r.get("trip_headsign"), ShortName: r.get("trip_short_name"), DirectionID: d})
	}); err != nil {
		return nil, err
	}
	type seqStop struct {
		seq  int
		stop string
	}
	seqs := map[string][]seqStop{}
	if err := eachRow(fsys, "stop_times.txt", func(r row) {
		n, _ := strconv.Atoi(r.get("stop_sequence"))
		seqs[r.get("trip_id")] = append(seqs[r.get("trip_id")], seqStop{n, r.get("stop_id")})
	}); err != nil {
		return nil, err
	}
	for trip, ss := range seqs {
		sort.Slice(ss, func(i, j int) bool { return ss[i].seq < ss[j].seq })
		ids := make([]string, len(ss))
		for i, s := range ss {
			ids[i] = s.stop
		}
		g.StopTimes[trip] = ids
	}
	if err := eachRow(fsys, "stops.txt", func(r row) {
		lat, _ := strconv.ParseFloat(r.get("stop_lat"), 64)
		lon, _ := strconv.ParseFloat(r.get("stop_lon"), 64)
		g.Stops[r.get("stop_id")] = GTFSStop{ID: r.get("stop_id"), Code: r.get("stop_code"), Name: r.get("stop_name"), Alias: r.get("alias"), Area: r.get("stop_area"), Lat: lat, Lon: lon}
	}); err != nil {
		return nil, err
	}
	type pt struct {
		seq      int
		lon, lat float64
	}
	pts := map[string][]pt{}
	err := eachRow(fsys, "shapes.txt", func(r row) {
		n, _ := strconv.Atoi(r.get("shape_pt_sequence"))
		lat, _ := strconv.ParseFloat(r.get("shape_pt_lat"), 64)
		lon, _ := strconv.ParseFloat(r.get("shape_pt_lon"), 64)
		pts[r.get("shape_id")] = append(pts[r.get("shape_id")], pt{n, lon, lat})
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) { // shapes.txt is optional in GTFS
		return nil, err
	}
	for id, ps := range pts {
		sort.Slice(ps, func(i, j int) bool { return ps[i].seq < ps[j].seq })
		line := make([][2]float64, len(ps))
		for i, p := range ps {
			line[i] = [2]float64{p.lon, p.lat}
		}
		g.Shapes[id] = line
	}
	return g, nil
}

type row struct {
	idx    map[string]int
	fields []string
}

func (r row) get(col string) string {
	if i, ok := r.idx[col]; ok && i < len(r.fields) {
		return strings.TrimSpace(r.fields[i])
	}
	return ""
}

func eachRow(fsys fs.FS, name string, fn func(row)) error {
	f, err := fsys.Open(name)
	if err != nil {
		return fmt.Errorf("gtfs %s: %w", name, err)
	}
	defer f.Close()
	cr := csv.NewReader(f)
	cr.FieldsPerRecord = -1
	cr.ReuseRecord = true
	cr.LazyQuotes = true
	header, err := cr.Read()
	if err != nil {
		return fmt.Errorf("gtfs %s: %w", name, err)
	}
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.TrimPrefix(strings.TrimSpace(h), "\ufeff")] = i
	}
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("gtfs %s: %w", name, err)
		}
		fn(row{idx, rec})
	}
}
