// Package settings holds admin-editable configuration, stored as one JSON value
// per key in SQLite. Defaults come from the environment (internal/config), so a
// key without a row follows the environment.
package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/unmaykr-a/transit-tracker/internal/store"
	"github.com/unmaykr-a/transit-tracker/internal/store/db"
)

// Settings is the full, effective configuration. JSON names are the storage keys.
type Settings struct {
	InstanceName  string     `json:"instance_name"`
	SignupMode    string     `json:"signup_mode"` // off | invite | open
	PublicMap     bool       `json:"public_map"`  // map, search and stop pages without login
	DefaultLocale string     `json:"default_locale"`
	DefaultTheme  string     `json:"default_theme"` // system | light | dark
	MapStyleLight string     `json:"map_style_light"`
	MapStyleDark  string     `json:"map_style_dark"`
	MapCenter     [2]float64 `json:"map_center"` // lon, lat
	MapZoom       float64    `json:"map_zoom"`

	GPSURL           string `json:"gps_url"`
	GPSIntervalS     int    `json:"gps_interval_s"`
	ElronURL         string `json:"elron_url"`
	ElronIntervalS   int    `json:"elron_interval_s"`
	SiriURL          string `json:"siri_url"`
	OTPURL           string `json:"otp_url"`
	StopsURL         string `json:"stops_url"`
	ElronGTFSURL     string `json:"elron_gtfs_url"`
	UserAgentContact string `json:"user_agent_contact"`

	// WalkProviders is the order walking routes are tried in; a straight-line
	// estimate is always the last resort.
	WalkProviders []string `json:"walk_providers"`
	TransitousURL string   `json:"transitous_url"`
	// GeocoderURL is the In-ADS address search used when adding saved places.
	GeocoderURL string `json:"geocoder_url"`
}

// Public is what any visitor may read (login page, map defaults).
type Public struct {
	InstanceName  string     `json:"instance_name"`
	SignupMode    string     `json:"signup_mode"`
	PublicMap     bool       `json:"public_map"`
	DefaultLocale string     `json:"default_locale"`
	DefaultTheme  string     `json:"default_theme"`
	MapStyleLight string     `json:"map_style_light"`
	MapStyleDark  string     `json:"map_style_dark"`
	MapCenter     [2]float64 `json:"map_center"`
	MapZoom       float64    `json:"map_zoom"`
}

func (s Settings) Public() Public {
	return Public{s.InstanceName, s.SignupMode, s.PublicMap, s.DefaultLocale, s.DefaultTheme, s.MapStyleLight, s.MapStyleDark, s.MapCenter, s.MapZoom}
}

// FieldError is a user-facing validation problem.
type FieldError struct{ Field, Msg string }

func (e *FieldError) Error() string { return e.Field + ": " + e.Msg }

func checkURL(field, v string) error {
	u, err := url.Parse(v)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return &FieldError{field, "must be an http(s) URL"}
	}
	return nil
}

// Validate checks every field.
func (s Settings) Validate() error {
	if strings.TrimSpace(s.InstanceName) == "" || len(s.InstanceName) > 60 {
		return &FieldError{"instance_name", "must be 1–60 characters"}
	}
	if !slices.Contains([]string{"off", "invite", "open"}, s.SignupMode) {
		return &FieldError{"signup_mode", "must be off, invite or open"}
	}
	if !slices.Contains([]string{"", "et", "en"}, s.DefaultLocale) {
		return &FieldError{"default_locale", "must be empty, et or en"}
	}
	if !slices.Contains([]string{"system", "light", "dark"}, s.DefaultTheme) {
		return &FieldError{"default_theme", "must be system, light or dark"}
	}
	if s.MapCenter[0] < -180 || s.MapCenter[0] > 180 || s.MapCenter[1] < -90 || s.MapCenter[1] > 90 {
		return &FieldError{"map_center", "must be [lon, lat]"}
	}
	if s.MapZoom < 1 || s.MapZoom > 20 {
		return &FieldError{"map_zoom", "must be between 1 and 20"}
	}
	// Never poll faster than the feeds update (PLAN.md, licensing note).
	if s.GPSIntervalS < 2 || s.GPSIntervalS > 300 {
		return &FieldError{"gps_interval_s", "must be between 2 and 300 seconds"}
	}
	if s.ElronIntervalS < 5 || s.ElronIntervalS > 300 {
		return &FieldError{"elron_interval_s", "must be between 5 and 300 seconds"}
	}
	if len(s.WalkProviders) > 3 {
		return &FieldError{"walk_providers", "too many providers"}
	}
	for _, p := range s.WalkProviders {
		if !slices.Contains([]string{"otp", "transitous", "haversine"}, p) {
			return &FieldError{"walk_providers", "unknown provider " + p}
		}
	}
	for field, v := range map[string]string{
		"transitous_url": s.TransitousURL, "geocoder_url": s.GeocoderURL,
		"map_style_light": s.MapStyleLight, "map_style_dark": s.MapStyleDark, "gps_url": s.GPSURL, "elron_url": s.ElronURL,
		"siri_url": s.SiriURL, "otp_url": s.OTPURL, "stops_url": s.StopsURL, "elron_gtfs_url": s.ElronGTFSURL, "user_agent_contact": s.UserAgentContact,
	} {
		if err := checkURL(field, v); err != nil {
			return err
		}
	}
	return nil
}

// Service caches the effective settings and persists overrides.
type Service struct {
	store    *store.Store
	defaults Settings
	cur      atomic.Pointer[Settings]
	mu       sync.Mutex
	onChange []func(Settings)
}

// New loads overrides from the database on top of defaults.
func New(ctx context.Context, st *store.Store, defaults Settings) (*Service, error) {
	if err := defaults.Validate(); err != nil {
		return nil, fmt.Errorf("default settings: %w", err)
	}
	s := &Service{store: st, defaults: defaults}
	if err := s.reload(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

// Get returns the current settings.
func (s *Service) Get() Settings { return *s.cur.Load() }

// Defaults returns the environment defaults (shown next to overrides in the admin UI).
func (s *Service) Defaults() Settings { return s.defaults }

// OnChange registers a callback run after every successful update.
func (s *Service) OnChange(fn func(Settings)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = append(s.onChange, fn)
}

func (s *Service) reload(ctx context.Context) error {
	rows, err := s.store.Q.ListSettings(ctx)
	if err != nil {
		return err
	}
	merged, err := merge(s.defaults, rows)
	if err != nil {
		return err
	}
	s.cur.Store(&merged)
	return nil
}

func merge(base Settings, rows []db.Setting) (Settings, error) {
	m := map[string]json.RawMessage{}
	b, _ := json.Marshal(base)
	if err := json.Unmarshal(b, &m); err != nil {
		return base, err
	}
	for _, r := range rows {
		if _, known := m[r.Key]; known {
			m[r.Key] = json.RawMessage(r.Value)
		}
	}
	b, _ = json.Marshal(m)
	var out Settings
	if err := json.Unmarshal(b, &out); err != nil {
		return base, fmt.Errorf("settings: %w", err)
	}
	return out, nil
}

// Overrides returns which keys are set in the database.
func (s *Service) Overrides(ctx context.Context) (map[string]bool, error) {
	rows, err := s.store.Q.ListSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, r := range rows {
		out[r.Key] = true
	}
	return out, nil
}

// Update applies a JSON patch of keys. A null value removes the override so
// the key follows the environment default again. Unknown keys are rejected.
func (s *Service) Update(ctx context.Context, patch map[string]json.RawMessage) (Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	known := map[string]json.RawMessage{}
	b, _ := json.Marshal(s.defaults)
	_ = json.Unmarshal(b, &known)
	rows, err := s.store.Q.ListSettings(ctx)
	if err != nil {
		return Settings{}, err
	}
	next := map[string]string{}
	for _, r := range rows {
		next[r.Key] = r.Value
	}
	for k, v := range patch {
		if _, ok := known[k]; !ok {
			return Settings{}, &FieldError{k, "unknown setting"}
		}
		if string(v) == "null" {
			delete(next, k)
		} else {
			next[k] = string(v)
		}
	}
	var asRows []db.Setting
	for k, v := range next {
		asRows = append(asRows, db.Setting{Key: k, Value: v})
	}
	merged, err := merge(s.defaults, asRows)
	if err != nil {
		return Settings{}, &FieldError{"settings", "invalid value"}
	}
	if err := merged.Validate(); err != nil {
		return Settings{}, err
	}
	err = s.store.Tx(ctx, func(q *db.Queries) error {
		for k, v := range patch {
			if string(v) == "null" {
				if err := q.DeleteSetting(ctx, k); err != nil {
					return err
				}
				continue
			}
			if err := q.UpsertSetting(ctx, db.UpsertSettingParams{Key: k, Value: string(v)}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Settings{}, err
	}
	s.cur.Store(&merged)
	for _, fn := range s.onChange {
		fn(merged)
	}
	return merged, nil
}
