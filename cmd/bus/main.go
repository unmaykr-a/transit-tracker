// Command bus serves the Tallinn transit tracker.
//
//	bus              run the server
//	bus healthcheck  probe /healthz on the local server (for Docker HEALTHCHECK)
//	bus version      print version and exit
//	bus eta-check    compare predicted arrivals with SIRI on live data
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"
	_ "time/tzdata" // distroless has no zoneinfo; Europe/Tallinn must always resolve

	"github.com/unmaykr-a/transit-tracker/internal/api"
	"github.com/unmaykr-a/transit-tracker/internal/auth"
	"github.com/unmaykr-a/transit-tracker/internal/buildinfo"
	"github.com/unmaykr-a/transit-tracker/internal/config"
	"github.com/unmaykr-a/transit-tracker/internal/dashboard"
	"github.com/unmaykr-a/transit-tracker/internal/departures"
	"github.com/unmaykr-a/transit-tracker/internal/eta"
	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/ingest"
	"github.com/unmaykr-a/transit-tracker/internal/settings"
	"github.com/unmaykr-a/transit-tracker/internal/state"
	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
	"github.com/unmaykr-a/transit-tracker/internal/store"
	"github.com/unmaykr-a/transit-tracker/internal/walk"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(2)
	}
	cmd := ""
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "":
		err = serve(cfg)
	case "healthcheck":
		err = healthcheck(cfg)
	case "version":
		fmt.Println(buildinfo.Version, buildinfo.Commit)
	case "eta-check":
		err = etaCheck(cfg, os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q (want: healthcheck, version, eta-check)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func serve(cfg config.Config) error {
	log := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tallinn, err := time.LoadLocation("Europe/Tallinn")
	if err != nil {
		return err
	}
	db, err := store.Open(ctx, cfg.DataDir, log)
	if err != nil {
		return err
	}
	defer db.Close()

	accounts := auth.NewService(db)
	if seeded, err := accounts.SeedAdmin(ctx, cfg.AdminUser, cfg.AdminPassword); err != nil {
		return err
	} else if seeded {
		log.Info("created admin from BUS_ADMIN_USER", "username", cfg.AdminUser)
	}
	set, err := settings.New(ctx, db, defaultSettings(cfg))
	if err != nil {
		return err
	}
	cur := set.Get // admin edits apply on the next poll / request

	httpClient := &http.Client{Timeout: 60 * time.Second}
	var importer *staticdata.Importer
	if cfg.FakeFeeds {
		log.Warn("BUS_FAKE_FEEDS is on: replaying recorded fixtures, not live data")
		httpClient.Transport = feeds.FakeTransport{Lines: func(stopID string) []feeds.FakeLine {
			return fakeLines(importer.Index(), stopID)
		}}
	}
	fetcher := feeds.NewFetcher(httpClient, "")
	fetcher.UserAgent = func() string { return "Bus/" + buildinfo.Version + " (+" + cur().UserAgentContact + ")" }
	otp := &feeds.OTP{URL: func() string { return cur().OTPURL }, Fetcher: fetcher}
	tracker := eta.NewTracker()
	importer = &staticdata.Importer{
		OnSwap:  func(ix *staticdata.Index) { tracker.SetGeometry(eta.BuildGeometry(ix)) },
		Fetcher: fetcher, OTP: otp, AgencyID: feeds.TallinnAgencyID, Store: db, Log: log, Location: tallinn,
		CountyAgencyPrefix: countyPrefix(cfg.County),
		TartuAgencyPrefix:  tartuPrefix(cfg.Tartu),
		StopsURL:           func() string { return cur().StopsURL },
		ElronGTFSURL:       func() string { return cur().ElronGTFSURL },
	}
	go importer.Run(ctx)
	countyTrips := &ingest.CountyTrips{URL: func() string { return cur().CountyTripsURL }, Fetcher: fetcher}
	vehicles := state.NewStore()
	deps := &departures.Service{Fetcher: fetcher, OTP: otp, SiriURL: func() string { return cur().SiriURL }, Index: importer.Index, Location: tallinn,
		Live: func(trip string, st *staticdata.Stop) (time.Time, bool) {
			return liveTime(countyTrips, vehicles, tracker, trip, st)
		}}

	health := ingest.NewHealth()
	gps := &ingest.GPSPoller{URL: func() string { return cur().GPSURL }, Fetcher: fetcher, Store: vehicles, Annotate: tracker.Annotate}
	elron := &ingest.ElronPoller{URL: func() string { return cur().ElronURL }, Fetcher: fetcher, Store: vehicles, Location: tallinn, Annotate: tracker.MatchTrains}
	// Live feeds retry at least every minute, so the map recovers soon after the upstream does.
	go health.Run(ctx, log, ingest.Feed{Name: "gps", Interval: func() time.Duration { return time.Duration(cur().GPSIntervalS) * time.Second }, Poll: gps.Poll, MaxBackoff: time.Minute})
	go health.Run(ctx, log, ingest.Feed{Name: "elron", Interval: func() time.Duration { return time.Duration(cur().ElronIntervalS) * time.Second }, Poll: elron.Poll, MaxBackoff: time.Minute})
	if cfg.County {
		// Live Harju county buses: positions as often as the feed changes (~5 s), predicted times less often.
		county := &ingest.CountyPoller{URL: func() string { return cur().CountyVehiclesURL }, Fetcher: fetcher, Store: vehicles,
			Known: func(line string) bool { return isLine(importer.Index(), line, "county") }, Annotate: tracker.AnnotateCounty}
		go health.Run(ctx, log, ingest.Feed{Name: "county", Interval: func() time.Duration { return 5 * time.Second }, Poll: county.Poll, MaxBackoff: time.Minute})
		go health.Run(ctx, log, ingest.Feed{Name: "county-trips", Interval: func() time.Duration { return 20 * time.Second }, Poll: countyTrips.Poll, MaxBackoff: 2 * time.Minute})
	}
	if cfg.Tartu && !cfg.FakeFeeds {
		// Tartu's buses: one WebSocket held open; Poll hands the latest set to the store.
		tartu := &ingest.TartuStream{URL: func() string { return cur().TartuLiveURL }, UserAgent: fetcher.UserAgent, Store: vehicles, Log: log,
			Known: func(line string) bool { return isLine(importer.Index(), line, "tartu") }, Annotate: tracker.AnnotateTartu}
		defer tartu.Close()
		go health.Run(ctx, log, ingest.Feed{Name: "tartu", Interval: func() time.Duration { return 5 * time.Second }, Poll: tartu.Poll, MaxBackoff: time.Minute})
	}
	go cleanupSessions(ctx, log, db)
	go prewarmSaved(ctx, log, db, importer.Index, deps)
	walker := walk.NewService([]walk.Provider{
		walk.OTP{Client: otp},
		walk.Transitous{URL: func() string { return cur().TransitousURL }, Fetcher: fetcher},
		walk.Haversine{},
	}, func() []string {
		if cfg.FakeFeeds {
			return []string{walk.ProviderHaversine} // offline
		}
		return cur().WalkProviders
	})
	board := &dashboard.Builder{Index: importer.Index, Departures: deps, Walk: walker,
		Times: func(p *staticdata.Pattern) []float64 { return tracker.PatternTimes(importer.Index(), p) }}

	var sso *auth.OIDC
	if cfg.OIDC.Enabled() {
		sso = auth.NewOIDC(cfg.OIDC, cfg.BaseURL+"/api/v1/auth/oidc/callback")
		log.Info("single sign-on enabled", "issuer", cfg.OIDC.Issuer, "admin_group", cfg.OIDC.AdminGroup)
	}
	srv := &http.Server{
		Addr: cfg.Addr,
		Handler: api.NewServer(log, api.Deps{
			OIDC: sso, OIDCConfig: cfg.OIDC,
			Store: vehicles, Health: health, Static: importer.Index, StaticStatus: importer.Status, Departures: deps,
			Auth: accounts, Settings: set, DB: db, DataDir: cfg.DataDir, TrustedProxies: cfg.TrustedProxies,
			RefreshStatic: importer.Import, Dashboard: board, Tracker: tracker,
			Geocode: func(ctx context.Context, q string) ([]feeds.Address, error) {
				return geocode(ctx, fetcher, cur().GeocoderURL, q)
			},
			MetricsMode: cfg.Metrics, MetricsToken: cfg.MetricsToken,
		}).Handler(api.SPA(api.Dist())),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	// Live streams (WebSocket, SSE) run until their request context ends;
	// Shutdown does not cancel those, so cancel them all when it starts.
	baseCtx, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	srv.BaseContext = func(net.Listener) context.Context { return baseCtx }
	srv.RegisterOnShutdown(cancelRequests)
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("listening", "addr", cfg.Addr, "version", buildinfo.Version, "commit", buildinfo.Commit)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	// Within Docker's 10 s grace period.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Warn("shutdown did not finish cleanly", "err", err)
	}
	return nil
}

// isLine reports whether a bus line is a route of the static data from source (county, tartu).
func isLine(ix *staticdata.Index, line, source string) bool {
	if ix == nil {
		return false
	}
	r, ok := ix.Route(staticdata.RouteID(feeds.ModeBus, line))
	return ok && r.Source == source
}

// liveTime is a live predicted time for a county or Tartu timetable trip at a
// stop: the county feed's prediction, else the tracker's ETA for the bus
// running that trip (Tartu reports which trip each bus runs, not when it arrives).
func liveTime(county *ingest.CountyTrips, vehicles *state.Store, tracker *eta.Tracker, trip string, st *staticdata.Stop) (time.Time, bool) {
	if st.Code != "" {
		if at, ok := county.Expected(trip, st.Code); ok {
			return at, true
		}
	}
	key := ingest.TripKey(trip)
	_, all := vehicles.Snapshot()
	for _, v := range all {
		if v.TripID == "" || ingest.TripKey(v.TripID) != key {
			continue
		}
		t, ok := tracker.Trip(v, time.Now(), 80)
		if !ok {
			return time.Time{}, false
		}
		for _, a := range t.Ahead {
			if a.StopID == st.ID {
				return a.ETA, true
			}
		}
		return time.Time{}, false
	}
	return time.Time{}, false
}

// defaultSettings are the environment-derived values admin settings override.
func defaultSettings(cfg config.Config) settings.Settings {
	return settings.Settings{
		InstanceName: "Bus", SignupMode: "invite", PublicMap: true, DefaultTheme: "system",
		MapStyleLight: "https://tiles.openfreemap.org/styles/positron",
		MapStyleDark:  "https://tiles.openfreemap.org/styles/dark",
		MapCenter:     [2]float64{24.7536, 59.437}, MapZoom: 13,
		GPSURL: cfg.Feeds.GPSURL, GPSIntervalS: int(cfg.Feeds.GPSInterval / time.Second),
		ElronURL: cfg.Feeds.ElronURL, ElronIntervalS: int(cfg.Feeds.ElronInterval / time.Second),
		SiriURL: cfg.Feeds.SiriURL, OTPURL: cfg.Feeds.OTPURL, StopsURL: cfg.Feeds.StopsURL, ElronGTFSURL: cfg.Feeds.ElronGTFSURL,
		CountyVehiclesURL: cfg.Feeds.CountyVehiclesURL, CountyTripsURL: cfg.Feeds.CountyTripsURL, TartuLiveURL: cfg.Feeds.TartuLiveURL,
		UserAgentContact: cfg.Contact,
		WalkProviders:    []string{walk.ProviderOTP, walk.ProviderTransitous, walk.ProviderHaversine},
		TransitousURL:    "https://api.transitous.org/api/v5/plan",
		GeocoderURL:      feeds.DefaultInADSURL,
	}
}

// prewarmSaved keeps live departures for every saved boarding stop in the
// cache, so dashboards and Home Assistant answer instantly (PLAN.md: every 20 s).
func prewarmSaved(ctx context.Context, log *slog.Logger, db *store.Store, ix func() *staticdata.Index, deps *departures.Service) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		ids, err := db.Q.ListBoardStops(ctx)
		if err != nil {
			log.Warn("pre-warm: listing saved stops", "err", err)
			continue
		}
		index := ix()
		// Saved trips board at any platform of their origin stop, and of each stop they change at.
		areas, err := db.Q.ListTripOrigins(ctx)
		if err != nil {
			areas = nil
		}
		if lists, err := db.Q.ListTripTransfers(ctx); err == nil {
			for _, l := range lists {
				for _, t := range dashboard.ParseTransfers(l) {
					areas = append(areas, t.Board)
				}
			}
		}
		for _, a := range slices.Compact(slices.Sorted(slices.Values(areas))) {
			if area, ok := index.Area(a); ok {
				ids = append(ids, area.StopIDs...)
			}
		}
		for _, id := range ids {
			if st, ok := index.Stop(id); ok && st.SiriID != "" {
				_, _ = deps.Siri(ctx, st.SiriID)
			}
		}
	}
}

// cleanupSessions deletes expired sessions once a day.
func cleanupSessions(ctx context.Context, log *slog.Logger, db *store.Store) {
	for {
		if err := db.Q.DeleteExpiredSessions(ctx, time.Now().Unix()); err != nil && ctx.Err() == nil {
			log.Warn("cleaning up sessions", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(24 * time.Hour):
		}
	}
}

func healthcheck(cfg config.Config) error {
	client := http.Client{Timeout: 3 * time.Second}
	res, err := client.Get(cfg.HealthURL())
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: %s", res.Status)
	}
	return nil
}

// fakeLines feeds synthetic departures from the static index in fake-feed mode.
func fakeLines(ix *staticdata.Index, stopID string) []feeds.FakeLine {
	var out []feeds.FakeLine
	for _, l := range ix.Lines(stopID) {
		for _, h := range l.Headsigns {
			out = append(out, feeds.FakeLine{Mode: l.Mode, Line: l.ShortName, Headsign: h})
		}
	}
	return out
}

// tartuPrefix selects Tartu's city bus operators when Tartu is on.
func tartuPrefix(on bool) string {
	if on {
		return feeds.TartuCityAgencyPrefix
	}
	return ""
}

// countyPrefix selects the Harju county operators when county buses are on.
func countyPrefix(on bool) string {
	if on {
		return feeds.HarjuCountyAgencyPrefix
	}
	return ""
}

// geocode searches Estonian addresses (In-ADS), skipping apartments.
func geocode(ctx context.Context, f *feeds.Fetcher, base, q string) ([]feeds.Address, error) {
	u := base + "?" + url.Values{"address": {q}, "results": {"8"}, "appartment": {"0"}}.Encode()
	res, err := f.Get(ctx, u, false)
	if err != nil {
		return nil, err
	}
	out, _, err := feeds.ParseInADS(bytes.NewReader(res.Body))
	return out, err
}
