package config

import (
	"log/slog"
	"testing"
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
	if got := c.HealthURL(); got != "http://127.0.0.1:8080/healthz" {
		t.Fatalf("HealthURL = %q", got)
	}
}

func TestLoadOverrides(t *testing.T) {
	c, err := load(env(map[string]string{
		"TRAM_ADDR":      "0.0.0.0:9000",
		"TRAM_DATA_DIR":  "/data",
		"TRAM_BASE_URL":  "https://tram.taali.ee/",
		"TRAM_LOG_LEVEL": "debug",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Addr != "0.0.0.0:9000" || c.DataDir != "/data" || c.BaseURL != "https://tram.taali.ee" || c.LogLevel != slog.LevelDebug {
		t.Fatalf("unexpected config: %+v", c)
	}
	if got := c.HealthURL(); got != "http://127.0.0.1:9000/healthz" {
		t.Fatalf("HealthURL = %q", got)
	}
}

func TestLoadInvalid(t *testing.T) {
	for _, m := range []map[string]string{
		{"TRAM_ADDR": "8080"},
		{"TRAM_LOG_LEVEL": "loud"},
	} {
		if _, err := load(env(m)); err == nil {
			t.Errorf("load(%v) succeeded, want error", m)
		}
	}
}
