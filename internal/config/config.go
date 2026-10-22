// Package config loads runtime configuration from BUS_* environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr     string // listen address, BUS_ADDR
	DataDir  string // SQLite and caches live here, BUS_DATA_DIR
	BaseURL  string // public URL behind the reverse proxy, BUS_BASE_URL
	LogLevel slog.Level

	// FakeFeeds replays recorded fixtures instead of calling upstream, BUS_FAKE_FEEDS=1.
	FakeFeeds bool
	// Contact goes into the upstream User-Agent so operators can reach us, BUS_CONTACT.
	Contact string
	Feeds   FeedConfig

	// TrustedProxies may set X-Forwarded-For/-Proto, BUS_TRUSTED_PROXIES (comma-separated CIDRs).
	TrustedProxies []netip.Prefix
	// AdminUser/AdminPassword seed the first admin on an empty database.
	AdminUser, AdminPassword string
	// Metrics controls GET /metrics: "private" (loopback and private networks
	// only; the default), "public" or "off". MetricsToken, when set, is required
	// as a bearer token instead of the network check.
	Metrics, MetricsToken string
	// OIDC enables single sign-on when Issuer is set (BUS_OIDC_*). Needs BaseURL
	// for the redirect URL: <BaseURL>/api/v1/auth/oidc/callback.
	OIDC OIDCConfig
}

// OIDCConfig is a generic OpenID Connect provider (tested with Authentik).
type OIDCConfig struct {
	Issuer, ClientID, ClientSecret string
	// AdminGroup, if set, makes members of this group (the "groups" claim) admins
	// and removes admin from users who leave it.
	AdminGroup string
	// Name labels the login button ("Sign in with <Name>").
	Name string
	// AutoCreate creates a local user on first sign-in (default true).
	AutoCreate bool
}

// Enabled reports whether single sign-on is configured.
func (o OIDCConfig) Enabled() bool { return o.Issuer != "" }

// FeedConfig holds upstream URLs and poll intervals. Admin-editable overrides arrive with settings (M3).
type FeedConfig struct {
	GPSURL        string        // BUS_GPS_URL
	GPSInterval   time.Duration // BUS_GPS_INTERVAL, default 5s (feed updates every ~10 s)
	ElronURL      string        // BUS_ELRON_URL
	ElronInterval time.Duration // BUS_ELRON_INTERVAL, default 10s
	SiriURL       string        // BUS_SIRI_URL, live departures per stop
	OTPURL        string        // BUS_OTP_URL, peatus.ee OpenTripPlanner GraphQL
	StopsURL      string        // BUS_STOPS_URL, Tallinn stops.txt
	ElronGTFSURL  string        // BUS_ELRON_GTFS_URL
}

const repoURL = "https://github.com/unmaykr-a/transit-tracker"

// Load reads configuration from the process environment.
func Load() (Config, error) { return load(os.Getenv) }

func load(getenv func(string) string) (Config, error) {
	c := Config{
		Addr:    or(getenv("BUS_ADDR"), ":8080"),
		DataDir: or(getenv("BUS_DATA_DIR"), "./data"),
		BaseURL: strings.TrimRight(getenv("BUS_BASE_URL"), "/"),
	}
	c.Contact = or(getenv("BUS_CONTACT"), or(c.BaseURL, repoURL))
	c.Feeds = FeedConfig{
		GPSURL:       or(getenv("BUS_GPS_URL"), "https://transport.tallinn.ee/gps.txt"),
		ElronURL:     or(getenv("BUS_ELRON_URL"), "https://elron.ee/map_data.json"),
		SiriURL:      or(getenv("BUS_SIRI_URL"), "https://transport.tallinn.ee/siri-stop-departures.php"),
		OTPURL:       or(getenv("BUS_OTP_URL"), "https://api.peatus.ee/routing/v1/routers/estonia/index/graphql"),
		StopsURL:     or(getenv("BUS_STOPS_URL"), "https://transport.tallinn.ee/data/stops.txt"),
		ElronGTFSURL: or(getenv("BUS_ELRON_GTFS_URL"), "https://eu-gtfs.remix.com/elron.zip"),
	}
	var err error
	if c.FakeFeeds, err = parseBool(getenv("BUS_FAKE_FEEDS")); err != nil {
		return c, fmt.Errorf("BUS_FAKE_FEEDS: %w", err)
	}
	// Never poll faster than the upstream updates (PLAN.md, licensing note).
	if c.Feeds.GPSInterval, err = duration(getenv, "BUS_GPS_INTERVAL", 5*time.Second, 2*time.Second); err != nil {
		return c, err
	}
	if c.Feeds.ElronInterval, err = duration(getenv, "BUS_ELRON_INTERVAL", 10*time.Second, 5*time.Second); err != nil {
		return c, err
	}
	for _, p := range strings.Split(getenv("BUS_TRUSTED_PROXIES"), ",") {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(p)
		if err != nil {
			addr, aerr := netip.ParseAddr(p)
			if aerr != nil {
				return c, fmt.Errorf("BUS_TRUSTED_PROXIES %q: %w", p, err)
			}
			prefix = netip.PrefixFrom(addr, addr.BitLen())
		}
		c.TrustedProxies = append(c.TrustedProxies, prefix.Masked())
	}
	c.AdminUser, c.AdminPassword = getenv("BUS_ADMIN_USER"), getenv("BUS_ADMIN_PASSWORD")
	c.Metrics, c.MetricsToken = strings.ToLower(strings.TrimSpace(getenv("BUS_METRICS"))), getenv("BUS_METRICS_TOKEN")
	c.OIDC = OIDCConfig{
		Issuer: strings.TrimRight(getenv("BUS_OIDC_ISSUER"), "/"), ClientID: getenv("BUS_OIDC_CLIENT_ID"),
		ClientSecret: getenv("BUS_OIDC_CLIENT_SECRET"), AdminGroup: getenv("BUS_OIDC_ADMIN_GROUP"),
		Name: or(getenv("BUS_OIDC_NAME"), "single sign-on"), AutoCreate: true,
	}
	if c.OIDC.AutoCreate, err = parseBoolDefault(getenv("BUS_OIDC_AUTO_CREATE"), true); err != nil {
		return c, fmt.Errorf("BUS_OIDC_AUTO_CREATE: %w", err)
	}
	if c.OIDC.Enabled() {
		switch {
		case c.OIDC.ClientID == "":
			return c, errors.New("BUS_OIDC_CLIENT_ID is required with BUS_OIDC_ISSUER")
		case c.BaseURL == "":
			return c, errors.New("BUS_BASE_URL is required with BUS_OIDC_ISSUER (it forms the redirect URL)")
		}
	}
	switch c.Metrics {
	case "":
		c.Metrics = "private"
	case "private", "public", "off":
	default:
		return c, fmt.Errorf("BUS_METRICS %q: use private, public or off", c.Metrics)
	}
	if _, _, err := net.SplitHostPort(c.Addr); err != nil {
		return c, fmt.Errorf("BUS_ADDR %q: %w", c.Addr, err)
	}
	if lvl := getenv("BUS_LOG_LEVEL"); lvl != "" {
		if err := c.LogLevel.UnmarshalText([]byte(lvl)); err != nil {
			return c, fmt.Errorf("BUS_LOG_LEVEL %q: %w", lvl, err)
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
	return "Bus/" + version + " (+" + c.Contact + ")"
}

func parseBoolDefault(s string, def bool) (bool, error) {
	if s == "" {
		return def, nil
	}
	return strconv.ParseBool(s)
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
