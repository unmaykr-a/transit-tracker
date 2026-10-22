// Command tram serves the Tallinn transit tracker.
//
//	tram              run the server
//	tram healthcheck  probe /healthz on the local server (for Docker HEALTHCHECK)
//	tram version      print version and exit
//	tram eta-check    compare predicted arrivals with SIRI on live data
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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
		log.Info("created admin from TRAM_ADMIN_USER", "username", cfg.AdminUser)
	}
	set, err := settings.New(ctx, db, defaultSettings(cfg))
	if err != nil {
		return err
	}
	cur := set.Get // admin edits apply on the next poll / request

	httpClient := &http.Client{Timeout: 60 * time.Second}
	var importer *staticdata.Importer
	if cfg.FakeFeeds {
		log.Warn("TRAM_FAKE_FEEDS is on: replaying recorded fixtures, not live data")
		httpClient.Transport = feeds.FakeTransport{Lines: func(stopID string) []feeds.FakeLine {
			return fakeLines(importer.Index(), stopID)
		}}
	}
	fetcher := feeds.NewFetcher(httpClient, "")
	fetcher.UserAgent = func() string { return "Tram/" + buildinfo.Version + " (+" + cur().UserAgentContact + ")" }
	otp := &feeds.OTP{URL: func() string { return cur().OTPURL }, Fetcher: fetcher}
	tracker := eta.NewTracker()
	importer = &staticdata.Importer{
		OnSwap:  func(ix *staticdata.Index) { tracker.SetGeometry(eta.BuildGeometry(ix)) },
		Fetcher: fetcher, OTP: otp, AgencyID: feeds.TallinnAgencyID, Store: db, Log: log, Location: tallinn,
		StopsURL:     func() string { return cur().StopsURL },
		ElronGTFSURL: func() string { return cur().ElronGTFSURL },
	}
	go importer.Run(ctx)
	deps := &departures.Service{Fetcher: fetcher, OTP: otp, SiriURL: func() string { return cur().SiriURL }, Index: importer.Index, Location: tallinn}

	vehicles := state.NewStore()
	health := ingest.NewHealth()
	gps := &ingest.GPSPoller{URL: func() string { return cur().GPSURL }, Fetcher: fetcher, Store: vehicles, Annotate: tracker.Annotate}
	elron := &ingest.ElronPoller{URL: func() string { return cur().ElronURL }, Fetcher: fetcher, Store: vehicles, Location: tallinn}
	go health.Run(ctx, log, ingest.Feed{Name: "gps", Interval: func() time.Duration { return time.Duration(cur().GPSIntervalS) * time.Second }, Poll: gps.Poll})
	go health.Run(ctx, log, ingest.Feed{Name: "elron", Interval: func() time.Duration { return time.Duration(cur().ElronIntervalS) * time.Second }, Poll: elron.Poll})
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
	board := &dashboard.Builder{Index: importer.Index, Departures: deps, Walk: walker}

	srv := &http.Server{
		Addr: cfg.Addr,
		Handler: api.NewServer(log, api.Deps{
			Store: vehicles, Health: health, Static: importer.Index, StaticStatus: importer.Status, Departures: deps,
			Auth: accounts, Settings: set, DB: db, DataDir: cfg.DataDir, TrustedProxies: cfg.TrustedProxies,
			RefreshStatic: importer.Import, Dashboard: board, Tracker: tracker,
		}).Handler(api.SPA(api.Dist())),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("listening", "addr", cfg.Addr, "version", buildinfo.Version, "commit", buildinfo.Commit)

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// defaultSettings are the environment-derived values admin settings override.
func defaultSettings(cfg config.Config) settings.Settings {
	return settings.Settings{
		InstanceName: "Tram", SignupMode: "invite", PublicMap: true, DefaultTheme: "system",
		MapStyleLight: "https://tiles.openfreemap.org/styles/positron",
		MapStyleDark:  "https://tiles.openfreemap.org/styles/dark",
		MapCenter:     [2]float64{24.7536, 59.437}, MapZoom: 13,
		GPSURL: cfg.Feeds.GPSURL, GPSIntervalS: int(cfg.Feeds.GPSInterval / time.Second),
		ElronURL: cfg.Feeds.ElronURL, ElronIntervalS: int(cfg.Feeds.ElronInterval / time.Second),
		SiriURL: cfg.Feeds.SiriURL, OTPURL: cfg.Feeds.OTPURL, StopsURL: cfg.Feeds.StopsURL, ElronGTFSURL: cfg.Feeds.ElronGTFSURL,
		UserAgentContact: cfg.Contact,
		WalkProviders:    []string{walk.ProviderOTP, walk.ProviderTransitous, walk.ProviderHaversine},
		TransitousURL:    "https://api.transitous.org/api/v5/plan",
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
