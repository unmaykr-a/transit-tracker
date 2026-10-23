import { describe, expect, it } from "vitest";

import type { Pattern } from "@/api";

import { placeVehicles } from "./lineVehicles";
import type { Vehicle } from "./types";

const stop = (id: string, lat: number) => ({ id, name: id, platform: "", area_id: "a" + id, lat, lon: 24.7 });
const pattern = (id: string, headsign: string, lats: number[], main = true): Pattern => ({
  id, direction: id === "out" ? 0 : 1, headsign, main, shape: [],
  stops: lats.map((lat, i) => stop(`${id}${i}`, lat)),
});
const veh = (id: string, lat: number, destination: string): Vehicle => ({
  id, mode: "tram", line: "T4", label: "T4", lat, lon: 24.7, bearing: 0, speed_mps: 5,
  destination, low_floor: true, updated_at: "2026-10-22T09:00:00Z",
});

// Stops ~300 m apart going north (out) and south (back).
const out = pattern("out", "Lennujaam", [59.4, 59.4027, 59.4054, 59.4081]);
const back = pattern("back", "Tondi", [59.4081, 59.4054, 59.4027, 59.4]);

describe("placeVehicles", () => {
  it("puts a vehicle at a stop when within 60 m", () => {
    const [p] = placeVehicles([out, back], [veh("a", 59.4029, "Lennujaam")]);
    expect(p).toMatchObject({ patternId: "out", stopIndex: 1, atStop: true });
  });

  it("puts a vehicle between stops, heading to the next one", () => {
    // Just past stop 1 towards stop 2 on the outbound pattern.
    const [p] = placeVehicles([out, back], [veh("a", 59.404, "Lennujaam")]);
    expect(p).toMatchObject({ patternId: "out", stopIndex: 2, atStop: false });
    // Same position southbound is between back stops 1 and 2, heading to 2 (lat 59.4027).
    const [q] = placeVehicles([out, back], [veh("b", 59.404, "Tondi")]);
    expect(q).toMatchObject({ patternId: "back", stopIndex: 2, atStop: false });
  });

  it("falls back to the nearest pattern when the destination is unknown", () => {
    const [p] = placeVehicles([out, back], [veh("a", 59.4081, "Depoo")]);
    expect(p?.atStop).toBe(true);
  });

  it("prefers the server's next-stop match", () => {
    // Geometrically nearest to out stop 0, but the server says it is heading to out stop 2.
    const v = { ...veh("a", 59.4, "Lennujaam"), pattern_id: "out-variant", next_stop_id: "out2" };
    const [p] = placeVehicles([out, back], [v]);
    expect(p).toMatchObject({ patternId: "out", stopIndex: 2, atStop: false });
    // Within 60 m of the stop before its next stop: at that stop.
    const atStop = { ...veh("b", 59.4028, "Lennujaam"), next_stop_id: "out2" };
    expect(placeVehicles([out, back], [atStop])[0]).toMatchObject({ stopIndex: 1, atStop: true });
  });

  it("ignores non-main patterns", () => {
    const shortTurn = pattern("st", "Lennujaam", [59.5], false);
    expect(placeVehicles([shortTurn], [veh("a", 59.5, "Lennujaam")])).toEqual([]);
  });

  it("tells how far between the stops a vehicle is, for drawing it on the line", () => {
    const [mid] = placeVehicles([out, back], [veh("a", (59.4027 + 59.4054) / 2, "Lennujaam")]);
    expect(mid!.atStop).toBe(false);
    expect(mid!.progress).toBeGreaterThan(0.4);
    expect(mid!.progress).toBeLessThan(0.6);
    const [at] = placeVehicles([out, back], [veh("b", 59.4029, "Lennujaam")]);
    expect(at!.progress).toBe(1);
  });
});
