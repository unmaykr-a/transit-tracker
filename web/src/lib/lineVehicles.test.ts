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

  it("ignores non-main patterns", () => {
    const shortTurn = pattern("st", "Lennujaam", [59.5], false);
    expect(placeVehicles([shortTurn], [veh("a", 59.5, "Lennujaam")])).toEqual([]);
  });
});
