package settings

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"

	"github.com/unmaykr-a/transit-tracker/internal/store"
)

func defaults() Settings {
	return Settings{
		InstanceName: "Bus", SignupMode: "invite", PublicMap: true, DefaultTheme: "system",
		MapStyleLight: "https://tiles.openfreemap.org/styles/positron", MapStyleDark: "https://tiles.openfreemap.org/styles/dark",
		MapCenter: [2]float64{24.7536, 59.437}, MapZoom: 13,
		GPSURL: "https://transport.tallinn.ee/gps.txt", GPSIntervalS: 5, ElronURL: "https://elron.ee/map_data.json", ElronIntervalS: 10,
		SiriURL: "https://transport.tallinn.ee/siri-stop-departures.php", OTPURL: "https://api.peatus.ee/x",
		StopsURL: "https://transport.tallinn.ee/data/stops.txt", ElronGTFSURL: "https://eu-gtfs.remix.com/elron.zip",
		UserAgentContact: "https://github.com/unmaykr-a/transit-tracker",
		WalkProviders:    []string{"otp", "transitous", "haversine"}, TransitousURL: "https://api.transitous.org/api/v5/plan",
	}
}

func raw(m map[string]any) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for k, v := range m {
		b, _ := json.Marshal(v)
		out[k] = b
	}
	return out
}

func TestUpdateValidateAndReset(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, dir, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s, err := New(ctx, st, defaults())
	if err != nil {
		t.Fatal(err)
	}
	var changes int
	s.OnChange(func(Settings) { changes++ })

	got, err := s.Update(ctx, raw(map[string]any{"instance_name": "Taali tram", "public_map": false, "gps_interval_s": 10}))
	if err != nil || got.InstanceName != "Taali tram" || got.PublicMap || got.GPSIntervalS != 10 || changes != 1 {
		t.Fatalf("update: %+v %v", got, err)
	}
	for _, bad := range []map[string]any{
		{"signup_mode": "everyone"},
		{"gps_interval_s": 1},
		{"gps_url": "ftp://x"},
		{"no_such_key": 1},
		{"map_zoom": "very"},
		{"walk_providers": []string{"google"}},
	} {
		var fe *FieldError
		if _, err := s.Update(ctx, raw(bad)); !errors.As(err, &fe) {
			t.Errorf("Update(%v) err = %v, want FieldError", bad, err)
		}
	}
	if s.Get().InstanceName != "Taali tram" || changes != 1 {
		t.Fatal("rejected update changed state")
	}
	ov, _ := s.Overrides(ctx)
	if !ov["public_map"] || ov["signup_mode"] {
		t.Errorf("overrides = %v", ov)
	}

	// Overrides persist across restarts; null resets to the default.
	s2, err := New(ctx, st, defaults())
	if err != nil || s2.Get().InstanceName != "Taali tram" {
		t.Fatalf("reload: %+v %v", s2.Get(), err)
	}
	got, err = s2.Update(ctx, map[string]json.RawMessage{"instance_name": json.RawMessage("null")})
	if err != nil || got.InstanceName != "Bus" || got.PublicMap {
		t.Fatalf("reset: %+v %v", got, err)
	}
	if p := got.Public(); p.InstanceName != "Bus" || p.MapZoom != 13 {
		t.Errorf("public = %+v", p)
	}
}
