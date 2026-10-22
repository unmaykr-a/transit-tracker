package feeds

import (
	"archive/zip"
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"math"
	"net/http"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed testdata/tallinn_gps.txt testdata/elron_map_data.json testdata/elron_stops_data.json testdata/tallinn_siri_1300.txt
//go:embed testdata/tallinn_stops.txt testdata/otp_agency_routes.json testdata/elron_gtfs
var fakeFS embed.FS

// FakeLine is a line calling at a stop, used to synthesise departures.
type FakeLine struct {
	Mode     Mode
	Line     string
	Headsign string
}

// FakeTransport answers upstream requests from the recorded fixtures
// (BUS_FAKE_FEEDS=1), so the UI can be developed offline and at night.
// Vehicles move back and forth along their recorded heading so animation is
// visible; snapshots change every 10 s like the real feed. Departures are
// synthesised from Lines (wired to the static index) at fixed headways.
type FakeTransport struct {
	Now   func() time.Time
	Lines func(stopID string) []FakeLine
}

func (f FakeTransport) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func (f FakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tick := f.now().Truncate(10 * time.Second)
	var body []byte
	var err error
	switch name := path.Base(req.URL.Path); name {
	case "gps.txt":
		body, err = fakeGPS(tick)
	case "map_data.json":
		body, err = fakeElron(tick)
	case "stops_data.json":
		body, err = fakeFS.ReadFile("testdata/elron_stops_data.json")
	case "siri-stop-departures.php":
		if f.Lines == nil {
			body, err = fakeFS.ReadFile("testdata/tallinn_siri_1300.txt")
		} else {
			body = f.fakeSiri(req.URL.Query().Get("stopid"))
		}
		return respond(req, http.StatusOK, body, time.Time{}), nil
	case "stops.txt":
		body, err = fakeFS.ReadFile("testdata/tallinn_stops.txt")
	case "elron.zip":
		body, err = zipFS("testdata/elron_gtfs")
	case "graphql":
		return f.fakeOTP(req)
	default:
		return respond(req, http.StatusNotFound, []byte("no fixture for "+req.URL.Path), time.Time{}), nil
	}
	if err != nil {
		return nil, err
	}
	if ims, perr := http.ParseTime(req.Header.Get("If-Modified-Since")); perr == nil && !tick.After(ims) {
		return respond(req, http.StatusNotModified, nil, tick), nil
	}
	return respond(req, http.StatusOK, body, tick), nil
}

func respond(req *http.Request, status int, body []byte, lastModified time.Time) *http.Response {
	h := http.Header{}
	if !lastModified.IsZero() {
		h.Set("Last-Modified", lastModified.UTC().Format(http.TimeFormat))
	}
	return &http.Response{
		StatusCode: status, Status: fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Proto: "HTTP/1.1", ProtoMajor: 1, ProtoMinor: 1,
		Header: h, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: req,
	}
}

// offset moves a point dist metres along heading.
func offset(lat, lon, headingDeg, dist float64) (float64, float64) {
	h := headingDeg * math.Pi / 180
	dLat := dist * math.Cos(h) / 111320
	dLon := dist * math.Sin(h) / (111320 * math.Cos(lat*math.Pi/180))
	return lat + dLat, lon + dLon
}

// pingPong returns a signed distance oscillating between 0 and amplitude with the given period.
func pingPong(t time.Time, seed uint32, period time.Duration, amplitude float64) float64 {
	p := float64((t.UnixMilli() + int64(seed)*997) % period.Milliseconds())
	x := p / float64(period.Milliseconds()) // 0..1
	if x > 0.5 {
		x = 1 - x
	}
	return 2 * x * amplitude
}

func seedOf(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

func fakeGPS(tick time.Time) ([]byte, error) {
	raw, err := fakeFS.ReadFile("testdata/tallinn_gps.txt")
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		f := strings.Split(line, ",")
		if len(f) < 10 {
			continue
		}
		seed := seedOf(f[0] + f[6])
		lon, _ := strconv.Atoi(f[2])
		lat, _ := strconv.Atoi(f[3])
		heading, _ := strconv.ParseFloat(f[5], 64)
		if seed%5 != 0 { // every fifth vehicle stands still, like at a stop
			nlat, nlon := offset(float64(lat)/1e6, float64(lon)/1e6, heading, pingPong(tick, seed, 4*time.Minute, 600))
			f[2], f[3] = strconv.Itoa(int(nlon*1e6)), strconv.Itoa(int(nlat*1e6))
		}
		out.WriteString(strings.Join(f, ","))
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

func fakeElron(tick time.Time) ([]byte, error) {
	raw, err := fakeFS.ReadFile("testdata/elron_map_data.json")
	if err != nil {
		return nil, err
	}
	var doc struct {
		Status int                 `json:"status"`
		Data   []map[string]string `json:"data"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation("Europe/Tallinn")
	if err != nil {
		return nil, err
	}
	for _, t := range doc.Data {
		lat, _ := strconv.ParseFloat(t["latitude"], 64)
		lon, _ := strconv.ParseFloat(t["longitude"], 64)
		heading, _ := strconv.ParseFloat(t["rongi_suund"], 64)
		nlat, nlon := offset(lat, lon, heading, pingPong(tick, seedOf(t["reis"]), 6*time.Minute, 5000))
		t["latitude"], t["longitude"] = strconv.FormatFloat(nlat, 'f', 6, 64), strconv.FormatFloat(nlon, 'f', 6, 64)
		t["asukoha_uuendus"] = tick.In(loc).Format(time.DateTime)
	}
	return json.Marshal(doc)
}

func zipFS(dir string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	entries, err := fs.ReadDir(fakeFS, dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		b, err := fakeFS.ReadFile(dir + "/" + e.Name())
		if err != nil {
			return nil, err
		}
		w, err := zw.Create(e.Name())
		if err != nil {
			return nil, err
		}
		if _, err := w.Write(b); err != nil {
			return nil, err
		}
	}
	err = zw.Close()
	return buf.Bytes(), err
}

type fakeDep struct {
	line                FakeLine
	scheduled, expected time.Time
}

// fakeDepartures gives each line at a stop a fixed headway (6–15 min) and a
// small, stable delay, for the next hour.
func (f FakeTransport) fakeDepartures(stopID string) []fakeDep {
	now := f.now()
	var out []fakeDep
	for _, l := range f.Lines(stopID) {
		seed := seedOf(stopID + l.Line + l.Headsign)
		headway := time.Duration(6+seed%10) * time.Minute
		delay := time.Duration(seed%150) * time.Second
		first := now.Truncate(headway).Add(time.Duration(seed%uint32(headway.Seconds())) * time.Second)
		for t := first.Add(-headway); t.Before(now.Add(time.Hour)); t = t.Add(headway) {
			if t.Add(delay).After(now) {
				out = append(out, fakeDep{l, t, t.Add(delay)})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].expected.Before(out[j].expected) })
	return out
}

func secOfDay(t time.Time, midnight time.Time) int { return int(t.Sub(midnight).Seconds()) }

func (f FakeTransport) fakeSiri(stopID string) []byte {
	loc, _ := time.LoadLocation("Europe/Tallinn")
	now := f.now().In(loc)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	var b strings.Builder
	fmt.Fprintf(&b, "Transport,RouteNum,ExpectedTimeInSeconds,ScheduleTimeInSeconds,%d,version20201024\n", secOfDay(now, midnight))
	deps := f.fakeDepartures(stopID)
	if len(deps) == 0 {
		return []byte(b.String())
	}
	fmt.Fprintf(&b, "stop,%s\n", stopID)
	words := map[Mode]string{ModeBus: "bus", ModeTram: "tram", ModeTrolley: "trol"}
	for _, d := range deps {
		word, ok := words[d.line.Mode]
		if !ok {
			continue // trains are not in SIRI
		}
		fmt.Fprintf(&b, "%s,%s,%d,%d,%s,%d,Z\n", word, d.line.Line, secOfDay(d.expected, midnight), secOfDay(d.scheduled, midnight),
			d.line.Headsign, int(d.expected.Sub(now).Seconds()))
	}
	return []byte(b.String())
}

func (f FakeTransport) fakeOTP(req *http.Request) (*http.Response, error) {
	raw, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}
	var q struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	_ = json.Unmarshal(raw, &q)
	switch {
	case strings.Contains(q.Query, "agencies"):
		// No county operators in the fake feed: offline mode is city and rail only.
		return respond(req, http.StatusOK, []byte(`{"data":{"agencies":[]}}`), time.Time{}), nil
	case strings.Contains(q.Query, "agency("):
		b, err := fakeFS.ReadFile("testdata/otp_agency_routes.json")
		if err != nil {
			return nil, err
		}
		return respond(req, http.StatusOK, b, time.Time{}), nil
	case strings.Contains(q.Query, "stoptimesWithoutPatterns") && f.Lines != nil:
		ids, _ := q.Variables["ids"].([]any)
		loc, _ := time.LoadLocation("Europe/Tallinn")
		now := f.now().In(loc)
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		otpModes := map[Mode]string{ModeBus: "BUS", ModeTrolley: "BUS", ModeTram: "TRAM", ModeTrain: "RAIL"}
		var stops []map[string]any
		for _, raw := range ids {
			id, _ := raw.(string)
			var times []map[string]any
			for _, d := range f.fakeDepartures(strings.TrimPrefix(id, "estonia:")) {
				times = append(times, map[string]any{
					"scheduledDeparture": secOfDay(d.scheduled, midnight), "realtimeDeparture": secOfDay(d.scheduled, midnight),
					"realtime": false, "serviceDay": midnight.Unix(), "headsign": d.line.Headsign,
					"trip": map[string]any{"route": map[string]any{"shortName": d.line.Line, "mode": otpModes[d.line.Mode]}},
				})
			}
			stops = append(stops, map[string]any{"gtfsId": id, "stoptimesWithoutPatterns": times})
		}
		b, err := json.Marshal(map[string]any{"data": map[string]any{"stops": stops}})
		if err != nil {
			return nil, err
		}
		return respond(req, http.StatusOK, b, time.Time{}), nil
	}
	return respond(req, http.StatusOK, []byte(`{"errors":[{"message":"fake OTP: unsupported query"}]}`), time.Time{}), nil
}
