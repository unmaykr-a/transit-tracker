// Command tram serves the Tallinn transit tracker.
//
//	tram              run the server
//	tram healthcheck  probe /healthz on the local server (for Docker HEALTHCHECK)
//	tram version      print version and exit
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
	"github.com/unmaykr-a/transit-tracker/internal/buildinfo"
	"github.com/unmaykr-a/transit-tracker/internal/config"
	"github.com/unmaykr-a/transit-tracker/internal/feeds"
	"github.com/unmaykr-a/transit-tracker/internal/ingest"
	"github.com/unmaykr-a/transit-tracker/internal/state"
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
	default:
		err = fmt.Errorf("unknown command %q (want: healthcheck, version)", cmd)
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
	httpClient := &http.Client{Timeout: 20 * time.Second}
	if cfg.FakeFeeds {
		log.Warn("TRAM_FAKE_FEEDS is on: replaying recorded fixtures, not live data")
		httpClient.Transport = feeds.FakeTransport{}
	}
	fetcher := feeds.NewFetcher(httpClient, cfg.UserAgent(buildinfo.Version))
	store := state.NewStore()
	health := ingest.NewHealth()
	gps := &ingest.GPSPoller{URL: cfg.Feeds.GPSURL, Fetcher: fetcher, Store: store}
	elron := &ingest.ElronPoller{URL: cfg.Feeds.ElronURL, Fetcher: fetcher, Store: store, Location: tallinn}
	go health.Run(ctx, log, ingest.Feed{Name: "gps", Interval: cfg.Feeds.GPSInterval, Poll: gps.Poll})
	go health.Run(ctx, log, ingest.Feed{Name: "elron", Interval: cfg.Feeds.ElronInterval, Poll: elron.Poll})

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           api.NewServer(log, api.Deps{Store: store, Health: health}).Handler(api.SPA(api.Dist())),
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
