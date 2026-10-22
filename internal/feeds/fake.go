package feeds

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

//go:embed testdata/tallinn_gps.txt testdata/elron_map_data.json testdata/elron_stops_data.json testdata/tallinn_siri_1300.txt
var fakeFS embed.FS

// FakeTransport answers upstream requests from the recorded fixtures
// (TRAM_FAKE_FEEDS=1), so the UI can be developed offline and at night.
// Vehicles move back and forth along their recorded heading so animation is
// visible; snapshots change every 10 s like the real feed.
type FakeTransport struct {
	Now func() time.Time
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
		body, err = fakeFS.ReadFile("testdata/tallinn_siri_1300.txt")
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
