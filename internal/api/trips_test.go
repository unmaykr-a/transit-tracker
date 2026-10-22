package api

import (
	"slices"
	"strconv"
	"testing"

	"github.com/unmaykr-a/transit-tracker/internal/staticdata"
)

func TestSavedTrips(t *testing.T) {
	c, im := fullServer(t)
	ix := im.Index()
	// A trip along tram T4: from its third to its seventh stop (as stop areas).
	var t4 *staticdata.Pattern
	for _, p := range ix.Patterns("tram:T4") {
		if p.Direction == 0 {
			t4 = p
			break
		}
	}
	area := func(stopID string) string { s, _ := ix.Stop(stopID); return s.AreaID }
	from, to := area(t4.StopIDs[2]), area(t4.StopIDs[6])

	// Preview: the connecting lines, T4 among them, boarding in the origin area.
	code, prev := c.do("GET", "/api/v1/trips?from="+from+"&to="+to, nil)
	if code != 200 {
		t.Fatalf("preview: %d %v", code, prev)
	}
	lines := prev["lines"].([]any)
	var routes []string
	for _, l := range lines {
		routes = append(routes, l.(map[string]any)["route_id"].(string))
	}
	if !slices.Contains(routes, "tram:T4") {
		t.Fatalf("preview routes = %v", routes)
	}
	if code, _ := c.do("GET", "/api/v1/trips?from="+from+"&to="+from, nil); code != 400 {
		t.Errorf("same stop preview = %d", code)
	}

	// Validation.
	for _, bad := range []map[string]any{
		{"kind": "trip", "from_area_id": "nope", "to_area_id": to},
		{"kind": "trip", "from_area_id": from, "to_area_id": from},
		{"kind": "bike", "route_id": "tram:T4"},
	} {
		if code, out := c.do("POST", "/api/v1/me/lines", bad); code != 400 {
			t.Errorf("POST %v = %d %v", bad, code, out)
		}
	}

	code, out := c.do("POST", "/api/v1/me/lines", map[string]any{"kind": "trip", "from_area_id": from, "to_area_id": to, "label": "Commute"})
	if code != 201 {
		t.Fatalf("save trip: %d %v", code, out)
	}
	trip := out["line"].(map[string]any)
	if trip["kind"] != "trip" || trip["from"].(map[string]any)["id"] != from || trip["to"].(map[string]any)["id"] != to || len(trip["lines"].([]any)) != len(lines) {
		t.Fatalf("saved trip = %v", trip)
	}
	id := jsonNum(trip["id"])
	if code, _ := c.do("PATCH", "/api/v1/me/lines/"+id, map[string]any{"alight_stop_id": t4.StopIDs[6]}); code != 400 {
		t.Errorf("alight change on a trip = %d", code)
	}

	// Dashboard from the first boarding platform: departures only of connecting
	// routes, from their own platforms, each with a verdict.
	board, _ := ix.Stop(t4.StopIDs[2])
	q := "lat=" + strconv.FormatFloat(board.Lat, 'f', 6, 64) + "&lon=" + strconv.FormatFloat(board.Lon, 'f', 6, 64)
	_, dash := c.do("GET", "/api/v1/me/dashboard?"+q, nil)
	e := dash["entries"].([]any)[0].(map[string]any)
	deps := e["departures"].([]any)
	if len(deps) < 3 || e["walk"] == nil {
		t.Fatalf("trip entry = %v", e)
	}
	conns := ix.Connections(from, to)
	for _, d := range deps {
		d := d.(map[string]any)
		ok := slices.ContainsFunc(conns, func(c staticdata.Connection) bool {
			return c.RouteID == d["route_id"] && c.BoardStopID == d["stop_id"]
		})
		if !ok || d["verdict"].(map[string]any)["level"] == "none" {
			t.Errorf("departure %v is not a connection or has no verdict", d)
		}
	}

	// Home Assistant: kind, label, the next departure's line, all connecting lines.
	_, ha := c.do("GET", "/api/v1/ha/lines?origin="+strconv.FormatFloat(board.Lat, 'f', 6, 64)+","+strconv.FormatFloat(board.Lon, 'f', 6, 64), nil)
	h := ha["lines"].([]any)[0].(map[string]any)
	next := h["next"].(map[string]any)
	if h["kind"] != "trip" || h["label"] != "Commute" || h["line"] != next["line"] || next["line"] == "" || len(h["lines"].([]any)) == 0 || h["stop_id"] != from {
		t.Errorf("ha trip = %v", h)
	}
	// Trips and lines share one ordered list.
	_, l2 := c.do("POST", "/api/v1/me/lines", map[string]any{"route_id": "tram:T4", "direction": 0, "board_stop_id": t4.StopIDs[2]})
	lid := jsonNum(l2["line"].(map[string]any)["id"])
	if code, _ := c.do("POST", "/api/v1/me/lines/reorder", map[string]any{"ids": []any{toInt(lid), toInt(id)}}); code/100 != 2 {
		t.Errorf("reorder = %d", code)
	}
	_, list := c.do("GET", "/api/v1/me/lines", nil)
	if ls := list["lines"].([]any); len(ls) != 2 || ls[0].(map[string]any)["kind"] != "line" || ls[1].(map[string]any)["kind"] != "trip" {
		t.Errorf("list after reorder = %v", list)
	}

	// A usual starting place: used when the request carries no location.
	_, pl := c.do("POST", "/api/v1/me/places", map[string]any{"label": "Home", "lat": board.Lat + 0.002, "lon": board.Lon})
	place := toInt(jsonNum(pl["place"].(map[string]any)["id"]))
	if code, out := c.do("PATCH", "/api/v1/me/lines/"+strconv.FormatInt(toInt(id), 10), map[string]any{"origin_place_id": 999999}); code != 400 {
		t.Errorf("unknown place = %d %v", code, out)
	}
	if code, out := c.do("PATCH", "/api/v1/me/lines/"+strconv.FormatInt(toInt(id), 10), map[string]any{"origin_place_id": place, "label": "To work"}); code != 200 {
		t.Fatalf("set start place = %d %v", code, out)
	}
	_, dash = c.do("GET", "/api/v1/me/dashboard", nil)
	var tripEntry map[string]any
	for _, e := range dash["entries"].([]any) {
		if e.(map[string]any)["line"].(map[string]any)["kind"] == "trip" {
			tripEntry = e.(map[string]any)
		}
	}
	if tripEntry["walk_from"] != "Home" || tripEntry["walk"] == nil || tripEntry["line"].(map[string]any)["label"] != "To work" {
		t.Errorf("trip without a location = walk_from %v walk %v", tripEntry["walk_from"], tripEntry["walk"])
	}
	_, ha = c.do("GET", "/api/v1/ha/lines", nil)
	for _, h := range ha["lines"].([]any) {
		if h := h.(map[string]any); h["kind"] == "trip" && (h["walk_from"] != "Home" || h["walk_min"] == nil) {
			t.Errorf("ha trip without a location = %v", h)
		}
	}

	// Editing the trip's stops; lines have no from/to; null clears the place.
	if code, out := c.do("PATCH", "/api/v1/me/lines/"+strconv.FormatInt(toInt(id), 10), map[string]any{"from_area_id": to, "to_area_id": from}); code != 200 && code != 400 {
		t.Errorf("reverse trip = %d %v", code, out)
	}
	if code, _ := c.do("PATCH", "/api/v1/me/lines/"+strconv.FormatInt(toInt(id), 10), map[string]any{"to_area_id": "nope"}); code != 400 {
		t.Errorf("bad destination = %d", code)
	}
	if code, _ := c.do("PATCH", "/api/v1/me/lines/"+strconv.FormatInt(toInt(lid), 10), map[string]any{"from_area_id": from}); code != 400 {
		t.Errorf("from on a line = %d", code)
	}
	_, out = c.do("PATCH", "/api/v1/me/lines/"+strconv.FormatInt(toInt(id), 10), map[string]any{"origin_place_id": nil})
	if l := out["line"].(map[string]any); l["origin_place_id"] != nil {
		t.Errorf("cleared place = %v", l["origin_place_id"])
	}
	// Deleting a place clears it from lines that start there.
	c.do("PATCH", "/api/v1/me/lines/"+strconv.FormatInt(toInt(lid), 10), map[string]any{"origin_place_id": place})
	c.do("DELETE", "/api/v1/me/places/"+strconv.FormatInt(place, 10), nil)
	_, list = c.do("GET", "/api/v1/me/lines", nil)
	for _, l := range list["lines"].([]any) {
		if l.(map[string]any)["origin_place_id"] != nil {
			t.Errorf("deleted place still referenced: %v", l)
		}
	}
}
