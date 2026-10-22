// Package config loads runtime configuration from TRAM_* environment variables.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr     string // listen address, TRAM_ADDR
	DataDir  string // SQLite and caches live here, TRAM_DATA_DIR
	BaseURL  string // public URL behind the reverse proxy, TRAM_BASE_URL
	LogLevel slog.Level

	// FakeFeeds replays recorded fixtures instead of calling upstream, TRAM_FAKE_FEEDS=1.
	FakeFeeds bool
	// Contact goes into the upstream User-Agent so operators can reach us, TRAM_CONTACT.
	Contact string
	Feeds   FeedConfig
}

// FeedConfig holds upstream URLs and poll intervals. Admin-editable overrides arrive with settings (M3).
type FeedConfig struct {
	GPSURL        string        // TRAM_GPS_URL
	GPSInterval   time.Duration // TRAM_GPS_INTERVAL, default 5s (feed updates every ~10 s)
	ElronURL      string        // TRAM_ELRON_URL
	ElronInterval time.Duration // TRAM_ELRON_INTERVAL, default 10s
	SiriURL       string        // TRAM_SIRI_URL, live departures per stop
	OTPURL        string        // TRAM_OTP_URL, peatus.ee OpenTripPlanner GraphQL
	StopsURL      string        // TRAM_STOPS_URL, Tallinn stops.txt
	ElronGTFSURL  string        // TRAM_ELRON_GTFS_URL
}

const repoURL = "https://github.com/unmaykr-a/transit-tracker"

// Load reads configuration from the process environment.
func Load() (Config, error) { return load(os.Getenv) }

func load(getenv func(string) string) (Config, error) {
	c := Config{
		Addr:    or(getenv("TRAM_ADDR"), ":8080"),
		DataDir: or(getenv("TRAM_DATA_DIR"), "./data"),
		BaseURL: strings.TrimRight(getenv("TRAM_BASE_URL"), "/"),
	}
	c.Contact = or(getenv("TRAM_CONTACT"), or(c.BaseURL, repoURL))
	c.Feeds = FeedConfig{
		GPSURL:       or(getenv("TRAM_GPS_URL"), "https://transport.tallinn.ee/gps.txt"),
		ElronURL:     or(getenv("TRAM_ELRON_URL"), "https://elron.ee/map_data.json"),
		SiriURL:      or(getenv("TRAM_SIRI_URL"), "https://transport.tallinn.ee/siri-stop-departures.php"),
		OTPURL:       or(getenv("TRAM_OTP_URL"), "https://api.peatus.ee/routing/v1/routers/estonia/index/graphql"),
		StopsURL:     or(getenv("TRAM_STOPS_URL"), "https://transport.tallinn.ee/data/stops.txt"),
		ElronGTFSURL: or(getenv("TRAM_ELRON_GTFS_URL"), "https://eu-gtfs.remix.com/elron.zip"),
	}
	var err error
	if c.FakeFeeds, err = parseBool(getenv("TRAM_FAKE_FEEDS")); err != nil {
		return c, fmt.Errorf("TRAM_FAKE_FEEDS: %w", err)
	}
	// Never poll faster than the upstream updates (PLAN.md, licensing note).
	if c.Feeds.GPSInterval, err = duration(getenv, "TRAM_GPS_INTERVAL", 5*time.Second, 2*time.Second); err != nil {
		return c, err
	}
	if c.Feeds.ElronInterval, err = duration(getenv, "TRAM_ELRON_INTERVAL", 10*time.Second, 5*time.Second); err != nil {
		return c, err
	}
	if _, _, err := net.SplitHostPort(c.Addr); err != nil {
		return c, fmt.Errorf("TRAM_ADDR %q: %w", c.Addr, err)
	}
	if lvl := getenv("TRAM_LOG_LEVEL"); lvl != "" {
		if err := c.LogLevel.UnmarshalText([]byte(lvl)); err != nil {
			return c, fmt.Errorf("TRAM_LOG_LEVEL %q: %w", lvl, err)
		}
	}
	return c, nil
}

// HealthURL is the loopback URL the healthcheck subcommand probes.
func (c Config) HealthURL() string {
	_, port, _ := net.SplitHostPort(c.Addr)
	return "http://127.0.0.1:" + port + "/healthz"
}

// UserAgent identifies us to upstream feeds.
func (c Config) UserAgent(version string) string {
	return "Tram/" + version + " (+" + c.Contact + ")"
}

func parseBool(s string) (bool, error) {
	if s == "" {
		return false, nil
	}
	return strconv.ParseBool(s)
}

func duration(getenv func(string) string, key string, def, minimum time.Duration) (time.Duration, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", key, v, err)
	}
	if d < minimum {
		return 0, fmt.Errorf("%s %q: must be at least %s", key, v, minimum)
	}
	return d, nil
}

func or(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
