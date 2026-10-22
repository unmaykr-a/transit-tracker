package config

import (
	"log/slog"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoadDefaults(t *testing.T) {
	c, err := load(env(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != ":8080" || c.DataDir != "./data" || c.LogLevel != slog.LevelInfo {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	if c.FakeFeeds || c.Feeds.GPSInterval != 5*time.Second || c.Feeds.ElronInterval != 10*time.Second ||
		c.Feeds.GPSURL != "https://transport.tallinn.ee/gps.txt" || c.Feeds.ElronURL != "https://elron.ee/map_data.json" {
		t.Fatalf("unexpected feed defaults: %+v", c)
	}
	if got := c.UserAgent("v1"); got != "Bus/v1 (+https://github.com/unmaykr-a/transit-tracker)" {
		t.Fatalf("UserAgent = %q", got)
	}
	if got := c.HealthURL(); got != "http://127.0.0.1:8080/healthz" {
		t.Fatalf("HealthURL = %q", got)
	}
}

func TestLoadOverrides(t *testing.T) {
	c, err := load(env(map[string]string{
		"BUS_ADDR":            "0.0.0.0:9000",
		"BUS_DATA_DIR":        "/data",
		"BUS_BASE_URL":        "https://bus.taali.ee/",
		"BUS_LOG_LEVEL":       "debug",
		"BUS_FAKE_FEEDS":      "1",
		"BUS_GPS_INTERVAL":    "10s",
		"BUS_TRUSTED_PROXIES": "172.16.0.0/12, 192.168.1.10",
		"BUS_ADMIN_USER":      "admin",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != "0.0.0.0:9000" || c.DataDir != "/data" || c.BaseURL != "https://bus.taali.ee" || c.LogLevel != slog.LevelDebug {
		t.Fatalf("unexpected config: %+v", c)
	}
	if !c.FakeFeeds || c.Feeds.GPSInterval != 10*time.Second || c.UserAgent("v1") != "Bus/v1 (+https://bus.taali.ee)" {
		t.Fatalf("unexpected feed config: %+v", c)
	}
	if len(c.TrustedProxies) != 2 || c.TrustedProxies[1].String() != "192.168.1.10/32" || c.AdminUser != "admin" {
		t.Fatalf("proxies/admin: %v %q", c.TrustedProxies, c.AdminUser)
	}
	if got := c.HealthURL(); got != "http://127.0.0.1:9000/healthz" {
		t.Fatalf("HealthURL = %q", got)
	}
}

func TestLoadInvalid(t *testing.T) {
	for _, m := range []map[string]string{
		{"BUS_ADDR": "8080"},
		{"BUS_LOG_LEVEL": "loud"},
		{"BUS_FAKE_FEEDS": "maybe"},
		{"BUS_GPS_INTERVAL": "1s"}, // faster than upstream updates
		{"BUS_ELRON_INTERVAL": "soon"},
		{"BUS_TRUSTED_PROXIES": "not-a-cidr"},
	} {
		if _, err := load(env(m)); err == nil {
			t.Errorf("load(%v) succeeded, want error", m)
		}
	}
}
