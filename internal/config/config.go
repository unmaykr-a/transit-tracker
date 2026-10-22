// Package config loads runtime configuration from TRAM_* environment variables.
package config

import (
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
)

type Config struct {
	Addr     string // listen address, TRAM_ADDR
	DataDir  string // SQLite and caches live here, TRAM_DATA_DIR
	BaseURL  string // public URL behind the reverse proxy, TRAM_BASE_URL
	LogLevel slog.Level
}

// Load reads configuration from the process environment.
func Load() (Config, error) { return load(os.Getenv) }

func load(getenv func(string) string) (Config, error) {
	c := Config{
		Addr:    or(getenv("TRAM_ADDR"), ":8080"),
		DataDir: or(getenv("TRAM_DATA_DIR"), "./data"),
		BaseURL: strings.TrimRight(getenv("TRAM_BASE_URL"), "/"),
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

func or(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
