// Command loadtest puts a running Bus server under the load of many simultaneous
// users: live-map clients on WebSocket and SSE, plus people browsing stops,
// lines, search and departures. It reports latency percentiles per endpoint,
// errors, how late live updates arrive, and the server's memory, goroutines and
// CPU (with -pid, on the same machine).
//
//	go run ./tools/loadtest -url http://127.0.0.1:8080 -ws 400 -sse 100 -browsers 50 -d 60s
//
// Run it against BUS_FAKE_FEEDS=1 (never against the public upstream feeds:
// departures and walks pass through to them).
package main

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

type stats struct {
	mu   sync.Mutex
	lat  map[string][]time.Duration
	errs map[string]int
}

func (s *stats) add(name string, d time.Duration, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.errs[name+": "+short(err)]++
		return
	}
	s.lat[name] = append(s.lat[name], d)
}

func short(err error) string {
	m := err.Error()
	if len(m) > 80 {
		m = m[:80]
	}
	return m
}

func pct(ds []time.Duration, p float64) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	return ds[min(len(ds)-1, int(float64(len(ds))*p))]
}

type liveStats struct {
	clients, connected, failed atomic.Int64
	messages, bytes            atomic.Int64
	snapshots                  atomic.Int64
	mu                         sync.Mutex
	delay                      []time.Duration // receive time − server_time of diffs
	maxGap                     time.Duration   // longest silence on any client
}

func (l *liveStats) msg(raw []byte, last *time.Time) {
	now := time.Now()
	l.messages.Add(1)
	l.bytes.Add(int64(len(raw)))
	var m struct {
		Type       string    `json:"type"`
		ServerTime time.Time `json:"server_time"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if m.Type == "snapshot" {
		l.snapshots.Add(1)
	} else if m.Type == "diff" {
		l.delay = append(l.delay, now.Sub(m.ServerTime))
	}
	if !last.IsZero() && now.Sub(*last) > l.maxGap {
		l.maxGap = now.Sub(*last)
	}
	*last = now
}

func main() {
	base := flag.String("url", "http://127.0.0.1:8080", "server base URL")
	nWS := flag.Int("ws", 400, "WebSocket live-map clients")
	nSSE := flag.Int("sse", 100, "SSE live-map clients")
	nBrowse := flag.Int("browsers", 50, "concurrent users browsing pages (each waits 0.5–2 s between requests)")
	dur := flag.Duration("d", 60*time.Second, "test duration")
	ramp := flag.Duration("ramp", 10*time.Second, "time to open all live clients")
	pid := flag.Int("pid", 0, "server process id, for CPU usage (Linux)")
	cookie := flag.String("cookie", "", "bus_session cookie, when the map is not public")
	nUsers := flag.Int("users", 0, "signed-in users: each signs up (needs -admin), saves 3 lines and a place, then loads its dashboard every ~20 s and its Home Assistant feed every ~30 s")
	adminCreds := flag.String("admin", "", "admin username:password, to invite the -users")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *dur)
	defer cancel()
	tr := &http.Transport{MaxIdleConns: 2000, MaxIdleConnsPerHost: 2000, IdleConnTimeout: 30 * time.Second}
	hc := &http.Client{Transport: tr, Timeout: 30 * time.Second}
	st := &stats{lat: map[string][]time.Duration{}, errs: map[string]int{}}
	live := &liveStats{}

	get := func(ctx context.Context, path string) ([]byte, int, error) {
		req, _ := http.NewRequestWithContext(ctx, "GET", *base+path, nil)
		if *cookie != "" {
			req.AddCookie(&http.Cookie{Name: "bus_session", Value: *cookie})
		}
		res, err := hc.Do(req)
		if err != nil {
			return nil, 0, err
		}
		defer res.Body.Close()
		b, err := io.ReadAll(res.Body)
		return b, res.StatusCode, err
	}

	// Real ids to browse.
	var stops []string
	if b, _, err := get(ctx, "/api/v1/stops"); err == nil {
		var d struct {
			Stops []struct {
				ID string `json:"id"`
			} `json:"stops"`
		}
		_ = json.Unmarshal(b, &d)
		for _, s := range d.Stops {
			stops = append(stops, s.ID)
		}
	}
	if len(stops) == 0 {
		fmt.Fprintln(os.Stderr, "no stops: is the server up and the map public (or -cookie set)?")
		os.Exit(1)
	}
	lines := []string{"tram/T4", "tram/T1", "bus/1", "bus/8", "bus/40", "trolleybus/84", "bus/66", "train/R14"}
	queries := []string{"balti", "viru", "kalev", "hobujaama", "pärnu", "kopli", "T4", "17", "lennujaam", "ülemiste"}

	before := sample(*base, hc, *pid)
	var wg sync.WaitGroup

	// Live-map clients, opened evenly over the ramp.
	total := *nWS + *nSSE
	for i := range total {
		wg.Go(func() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(float64(*ramp) * float64(i) / float64(max(1, total)))):
			}
			live.clients.Add(1)
			var err error
			if i < *nWS {
				err = wsClient(ctx, *base, *cookie, live, i)
			} else {
				err = sseClient(ctx, *base, *cookie, hc, live)
			}
			if err != nil && ctx.Err() == nil {
				live.failed.Add(1)
				st.add("live", 0, err)
			}
		})
	}

	// People browsing: open a stop, its departures, a line, search…
	for range *nBrowse {
		wg.Go(func() {
			r := rand.New(rand.NewPCG(rand.Uint64(), 1))
			for ctx.Err() == nil {
				var name, path string
				switch r.IntN(10) {
				case 0, 1, 2:
					id := stops[r.IntN(len(stops))]
					name, path = "departures", "/api/v1/stops/"+id+"/departures?limit=40"
				case 3, 4:
					name, path = "stop", "/api/v1/stops/"+stops[r.IntN(len(stops))]
				case 5:
					name, path = "line", "/api/v1/lines/"+lines[r.IntN(len(lines))]
				case 6:
					name, path = "search", "/api/v1/search?q="+url.QueryEscape(queries[r.IntN(len(queries))])
				case 7:
					name, path = "stops(all)", "/api/v1/stops"
				case 8:
					name, path = "patterns", "/api/v1/patterns"
				default:
					name, path = "vehicles", "/api/v1/vehicles?mode=tram"
				}
				t0 := time.Now()
				_, code, err := get(ctx, path)
				if err == nil && code >= 500 {
					err = fmt.Errorf("HTTP %d", code)
				}
				if ctx.Err() != nil {
					return
				}
				st.add(name, time.Since(t0), err)
				select {
				case <-ctx.Done():
				case <-time.After(500*time.Millisecond + time.Duration(r.IntN(1500))*time.Millisecond):
				}
			}
		})
	}

	// Signed-in users with saved lines: the Home page and Home Assistant.
	if *nUsers > 0 {
		accounts, err := makeUsers(ctx, *base, *adminCreds, *nUsers, tr)
		if err != nil {
			fmt.Fprintln(os.Stderr, "users:", err)
			os.Exit(1)
		}
		for i, a := range accounts {
			wg.Go(func() {
				r := rand.New(rand.NewPCG(uint64(i), 2))
				// Real people do not all open the app in the same millisecond.
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Duration(r.Int64N(int64(20 * time.Second)))):
				}
				nextHA := time.Now()
				for ctx.Err() == nil {
					lat, lon := 59.43+r.Float64()*0.02, 24.72+r.Float64()*0.05
					t0 := time.Now()
					code, err := a.get(ctx, fmt.Sprintf("/api/v1/me/dashboard?lat=%.5f&lon=%.5f", lat, lon), "")
					if err == nil && code != 200 {
						err = fmt.Errorf("HTTP %d", code)
					}
					if ctx.Err() != nil {
						return
					}
					st.add("dashboard", time.Since(t0), err)
					if time.Now().After(nextHA) {
						t0 = time.Now()
						code, err = a.get(ctx, fmt.Sprintf("/api/v1/ha/lines?origin=%.5f,%.5f&departures=3", lat, lon), a.token)
						if err == nil && code != 200 {
							err = fmt.Errorf("HTTP %d", code)
						}
						if ctx.Err() == nil {
							st.add("ha/lines", time.Since(t0), err)
						}
						nextHA = time.Now().Add(30 * time.Second)
					}
					select {
					case <-ctx.Done():
					case <-time.After(15*time.Second + time.Duration(r.IntN(10_000))*time.Millisecond):
					}
				}
			})
		}
	}

	// Watch the server while it runs.
	var peakHeap, peakGor float64
	var peakLive float64
	mon := time.NewTicker(2 * time.Second)
	go func() {
		for range mon.C {
			s := sample(*base, hc, 0)
			peakHeap, peakGor, peakLive = max(peakHeap, s.heap), max(peakGor, s.goroutines), max(peakLive, s.live)
		}
	}()
	t0 := time.Now()
	<-ctx.Done()
	mon.Stop()
	elapsed := time.Since(t0)
	after := sample(*base, hc, *pid)
	wg.Wait()

	fmt.Printf("\n%d WS + %d SSE live clients, %d browsing users, %s\n\n", *nWS, *nSSE, *nBrowse, elapsed.Round(time.Second))
	fmt.Printf("%-12s %7s %9s %9s %9s %9s\n", "endpoint", "count", "p50", "p95", "p99", "max")
	names := slices.Sorted(func(yield func(string) bool) {
		for k := range st.lat {
			if !yield(k) {
				return
			}
		}
	})
	for _, n := range names {
		ds := st.lat[n]
		slices.Sort(ds)
		fmt.Printf("%-12s %7d %9s %9s %9s %9s\n", n, len(ds), r(pct(ds, .5)), r(pct(ds, .95)), r(pct(ds, .99)), r(ds[len(ds)-1]))
	}
	slices.Sort(live.delay)
	fmt.Printf("\nlive: %d opened, %d connected, %d failed, %d snapshots, %d messages (%.1f MB)\n",
		live.clients.Load(), live.connected.Load(), live.failed.Load(), live.snapshots.Load(), live.messages.Load(), float64(live.bytes.Load())/1e6)
	fmt.Printf("live diff delay: p50 %s  p95 %s  p99 %s  max %s; longest silence on a client %s\n",
		r(pct(live.delay, .5)), r(pct(live.delay, .95)), r(pct(live.delay, .99)), r(pct(live.delay, 1)), r(live.maxGap))
	fmt.Printf("server: peak heap %.0f MB, peak goroutines %.0f, peak live clients %.0f\n", peakHeap/1e6, peakGor, peakLive)
	if *pid > 0 && after.cpu > before.cpu {
		fmt.Printf("server CPU: %.0f%% of one core on average\n", 100*(after.cpu-before.cpu)/elapsed.Seconds())
	}
	if len(st.errs) > 0 {
		fmt.Println("\nerrors:")
		for k, n := range st.errs {
			fmt.Printf("  %5d  %s\n", n, k)
		}
	}
}

func r(d time.Duration) string {
	if d < time.Millisecond {
		return d.Round(time.Microsecond).String()
	}
	return d.Round(100 * time.Microsecond).String()
}

func wsClient(ctx context.Context, base, cookie string, live *liveStats, i int) error {
	u := "ws" + strings.TrimPrefix(base, "http") + "/ws"
	h := http.Header{"Origin": {base}}
	if cookie != "" {
		h.Set("Cookie", "bus_session="+cookie)
	}
	c, _, err := websocket.Dial(ctx, u, &websocket.DialOptions{HTTPHeader: h, CompressionMode: websocket.CompressionNoContextTakeover})
	if err != nil {
		return err
	}
	defer c.CloseNow()
	c.SetReadLimit(8 << 20)
	live.connected.Add(1)
	// Some clients filter, like the map with mode chips or a line page.
	if i%5 == 0 {
		_ = c.Write(ctx, websocket.MessageText, []byte(`{"filter":{"modes":["tram"]}}`))
	}
	var last time.Time
	for {
		_, b, err := c.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		live.msg(b, &last)
	}
}

func sseClient(ctx context.Context, base, cookie string, hc *http.Client, live *liveStats) error {
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/api/v1/vehicles/stream", nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "bus_session", Value: cookie})
	}
	c := *hc
	c.Timeout = 0
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("SSE HTTP %d", res.StatusCode)
	}
	live.connected.Add(1)
	sc := bufio.NewScanner(res.Body)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	var last time.Time
	for sc.Scan() {
		if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
			live.msg([]byte(data), &last)
		}
	}
	if ctx.Err() != nil {
		return nil
	}
	return sc.Err()
}

type snap struct{ heap, goroutines, live, cpu float64 }

// sample reads the server's /metrics and, with a pid, its CPU seconds from /proc.
func sample(base string, hc *http.Client, pid int) snap {
	var s snap
	if res, err := hc.Get(base + "/metrics"); err == nil {
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			f := strings.Fields(sc.Text())
			if len(f) < 2 {
				continue
			}
			v, _ := strconv.ParseFloat(f[len(f)-1], 64)
			switch {
			case f[0] == "go_memstats_heap_alloc_bytes":
				s.heap = v
			case f[0] == "go_goroutines":
				s.goroutines = v
			case strings.HasPrefix(f[0], "bus_live_clients"):
				s.live += v
			}
		}
		res.Body.Close()
	}
	if pid > 0 {
		if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); err == nil {
			// Fields after the command name: utime is 14th, stime 15th overall (clock ticks, 100/s).
			if i := strings.LastIndexByte(string(b), ')'); i > 0 {
				f := strings.Fields(string(b)[i+2:])
				if len(f) > 13 {
					u, _ := strconv.ParseFloat(f[11], 64)
					k, _ := strconv.ParseFloat(f[12], 64)
					s.cpu = (u + k) / 100
				}
			}
		}
	}
	return s
}

type account struct {
	base  string
	hc    *http.Client
	csrf  string
	token string
}

func (a *account) do(ctx context.Context, method, path string, body any, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, _ := http.NewRequestWithContext(ctx, method, a.base+path, rd)
	req.Header.Set("Origin", a.base)
	req.Header.Set("Content-Type", "application/json")
	if a.csrf != "" {
		req.Header.Set("X-CSRF-Token", a.csrf)
	}
	res, err := a.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if out != nil {
		_ = json.NewDecoder(res.Body).Decode(out)
	} else {
		_, _ = io.Copy(io.Discard, res.Body)
	}
	return res.StatusCode, nil
}

func (a *account) get(ctx context.Context, path, bearer string) (int, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", a.base+path, nil)
	c := a.hc
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
		c = &http.Client{Transport: a.hc.Transport, Timeout: a.hc.Timeout} // no cookies
	}
	res, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	_, _ = io.Copy(io.Discard, res.Body)
	return res.StatusCode, nil
}

func newAccount(base string, tr http.RoundTripper) *account {
	jar, _ := cookiejar.New(nil)
	return &account{base: base, hc: &http.Client{Transport: tr, Jar: jar, Timeout: 30 * time.Second}}
}

func (a *account) refreshCSRF(ctx context.Context) error {
	var s struct {
		CSRF string `json:"csrf"`
	}
	if _, err := a.do(ctx, "GET", "/api/v1/session", nil, &s); err != nil {
		return err
	}
	a.csrf = s.CSRF
	return nil
}

// makeUsers signs up n users through admin invites and gives each saved lines, a place and an API token.
func makeUsers(ctx context.Context, base, creds string, n int, tr http.RoundTripper) ([]*account, error) {
	user, pass, ok := strings.Cut(creds, ":")
	if !ok {
		return nil, fmt.Errorf("-admin user:password is required with -users")
	}
	admin := newAccount(base, tr)
	if code, err := admin.do(ctx, "POST", "/api/v1/auth/login", map[string]string{"username": user, "password": pass}, nil); err != nil || code != 200 {
		return nil, fmt.Errorf("admin login: %d %v", code, err)
	}
	if err := admin.refreshCSRF(ctx); err != nil {
		return nil, err
	}
	var lines []struct {
		Route struct {
			ID string `json:"id"`
		} `json:"route"`
		Patterns []struct {
			Main      bool `json:"main"`
			Direction int  `json:"direction"`
			Stops     []struct {
				ID string `json:"id"`
			} `json:"stops"`
		} `json:"patterns"`
	}
	for _, l := range []string{"tram/T4", "tram/T1", "bus/1", "bus/8", "bus/40", "trolleybus/84"} {
		var d struct {
			Route struct {
				ID string `json:"id"`
			} `json:"route"`
			Patterns []struct {
				Main      bool `json:"main"`
				Direction int  `json:"direction"`
				Stops     []struct {
					ID string `json:"id"`
				} `json:"stops"`
			} `json:"patterns"`
		}
		if code, _ := admin.do(ctx, "GET", "/api/v1/lines/"+l, nil, &d); code == 200 {
			lines = append(lines, d)
		}
	}
	run := time.Now().UnixMilli() % 1_000_000
	out := make([]*account, 0, n)
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8) // sign-ups hash passwords: do not stampede
	for i := range n {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			var inv struct {
				Token string `json:"token"`
			}
			if code, err := admin.do(ctx, "POST", "/api/v1/admin/invites", map[string]any{}, &inv); err != nil || inv.Token == "" {
				mu.Lock()
				firstErr = cmp.Or(firstErr, fmt.Errorf("invite: %d %v", code, err))
				mu.Unlock()
				return
			}
			a := newAccount(base, tr)
			name := fmt.Sprintf("load-%d-%d", run, i)
			if code, err := a.do(ctx, "POST", "/api/v1/auth/signup", map[string]string{"username": name, "password": "load-test-password", "invite": inv.Token}, nil); err != nil || code != 200 {
				mu.Lock()
				firstErr = cmp.Or(firstErr, fmt.Errorf("signup: %d %v", code, err))
				mu.Unlock()
				return
			}
			if err := a.refreshCSRF(ctx); err != nil {
				return
			}
			for k := range 3 {
				l := lines[(i+k)%len(lines)]
				for _, p := range l.Patterns {
					if p.Main && len(p.Stops) > 3 {
						_, _ = a.do(ctx, "POST", "/api/v1/me/lines", map[string]any{"route_id": l.Route.ID, "direction": p.Direction, "board_stop_id": p.Stops[1+i%(len(p.Stops)-2)].ID}, nil)
						break
					}
				}
			}
			_, _ = a.do(ctx, "POST", "/api/v1/me/places", map[string]any{"label": "Home", "lat": 59.437, "lon": 24.745}, nil)
			var tok struct {
				Token string `json:"token"`
			}
			_, _ = a.do(ctx, "POST", "/api/v1/me/tokens", map[string]string{"name": "Home Assistant"}, &tok)
			a.token = tok.Token
			mu.Lock()
			out = append(out, a)
			mu.Unlock()
		})
	}
	wg.Wait()
	if len(out) == 0 {
		return nil, firstErr
	}
	fmt.Printf("%d users ready (%v)\n", len(out), firstErr)
	return out, nil
}
