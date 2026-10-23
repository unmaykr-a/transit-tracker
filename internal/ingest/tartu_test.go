package ingest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/unmaykr-a/transit-tracker/internal/state"
)

// A status as Ridango sends it (recorded 2026-10-23, Tartu line 2).
const tartuStatusJSON = `{"deviceId":"40317","regionId":32,"locationId":"630TNS","updateTime":"2026-10-23T16:34:27+03:00",
 "trip":{"tripId":"2_Nõlvaku - Kannikese - Lõunakeskus_B>A_e68c0a506df02abaa60a5120766ecac4|Talv_26/27-Fr-3ef88e812ba19553a78abfe309c76563-A-B--1618-6e6481c","directionId":0,"routeShortName":"2","startTime":"16:18:00","startDate":"20261023"},
 "position":{"speed":4.5,"bearing":230,"latitude":58.377594,"longitude":26.727865,"occupancyStatus":null}}`

func TestTartuVehicle(t *testing.T) {
	var s tartuStatus
	if err := json.Unmarshal([]byte(tartuStatusJSON), &s); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 23, 13, 35, 0, 0, time.UTC)
	v, ok := TartuVehicle(s, now)
	if !ok || v.ID != "bus-tartu-630TNS" || v.Line != "tartu-2" || v.Label != "2" || v.SpeedMps != 4.5 || !v.SpeedKnown ||
		v.TripID != "Talv_26/27-Fr-3ef88e812ba19553a78abfe309c76563-A-B--1618-6e6481c" || !v.UpdatedAt.Equal(time.Date(2026, 10, 23, 13, 34, 27, 0, time.UTC)) {
		t.Fatalf("vehicle = %+v", v)
	}
	// The timetable's trip id is the same trip.
	if TripKey("estonia:Talv_26/27-Fr-3ef88e812ba19553a78abfe309c76563-A-B--1618-6e6481c") != TripKey(v.TripID) {
		t.Error("trip keys differ")
	}
	s.Trip = nil
	if _, ok := TartuVehicle(s, now); ok {
		t.Error("a status without a trip (out of service) became a vehicle")
	}
}

func TestTartuStream(t *testing.T) {
	subscribed := make(chan map[string]any, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		_, b, err := c.Read(r.Context())
		if err != nil {
			return
		}
		var sub map[string]any
		_ = json.Unmarshal(b, &sub)
		subscribed <- sub
		_ = c.Write(r.Context(), websocket.MessageText, []byte(`{"connectionEstablished":true,"errorMessage":null}`))
		other := strings.Replace(strings.Replace(tartuStatusJSON, `"630TNS"`, `"999XYZ"`, 1), `"routeShortName":"2"`, `"routeShortName":"99"`, 1)
		_ = c.Write(r.Context(), websocket.MessageText, []byte("["+tartuStatusJSON+","+other+"]"))
		<-r.Context().Done()
	}))
	defer srv.Close()
	now := time.Date(2026, 10, 23, 13, 35, 0, 0, time.UTC)
	store := state.NewStore()
	ts := &TartuStream{
		URL: func() string { return "ws" + strings.TrimPrefix(srv.URL, "http") }, Store: store,
		Known: func(line string) bool { return line == "tartu-2" }, Now: func() time.Time { return now },
	}
	defer ts.Close()
	if _, err := ts.Poll(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case sub := <-subscribed:
		if sub["regionId"].(float64) != TartuRegion {
			t.Errorf("subscription %v", sub)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no subscription")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		res, err := ts.Poll(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if res.Count > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no vehicles from the stream")
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, vs := store.Snapshot()
	if len(vs) != 1 || vs["bus-tartu-630TNS"].Line != "tartu-2" {
		t.Fatalf("stored %v (line 99 is not in the timetable: left out)", vs)
	}
	// A bus not heard of for two minutes is forgotten.
	now = now.Add(3 * time.Minute)
	if res, _ := ts.Poll(context.Background()); res.Count != 0 {
		t.Errorf("%d vehicles after 3 silent minutes", res.Count)
	}
}
