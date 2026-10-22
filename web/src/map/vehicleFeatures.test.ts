import { describe, expect, it } from "vitest";

import { nextTween } from "@/lib/tween";
import type { Mode, Vehicle } from "@/lib/types";

import { ALL_MODES, vehicleFeatures } from "./vehicleFeatures";

const v = (id: string, mode: Mode, line: string, lat: number): Vehicle => ({
  id, mode, line, label: line, lat, lon: 24.7, bearing: 45, speed_mps: 3,
  destination: "X", low_floor: false, updated_at: "2026-10-22T09:00:00Z",
});

describe("vehicleFeatures", () => {
  const vehicles = [v("tram-1", "tram", "T4", 59.4), v("bus-1", "bus", "4", 59.41), v("bus-2", "bus", "17", 59.42)];

  it("emits GeoJSON points with display props", () => {
    const fc = vehicleFeatures(vehicles, new Map(), 0, ALL_MODES, []);
    expect(fc.features).toHaveLength(3);
    expect(fc.features[0]).toMatchObject({
      id: "tram-1",
      geometry: { coordinates: [24.7, 59.4] },
      properties: { mode: "tram", text: "T4", bearing: 45, sort: 1 },
    });
  });

  it("filters by mode and line", () => {
    expect(vehicleFeatures(vehicles, new Map(), 0, new Set<Mode>(["bus"]), []).features.map((f) => f.id)).toEqual(["bus-1", "bus-2"]);
    expect(vehicleFeatures(vehicles, new Map(), 0, ALL_MODES, ["4"]).features.map((f) => f.id)).toEqual(["tram-1", "bus-1"]);
  });

  it("draws tweened positions", () => {
    const first = nextTween(undefined, vehicles[0]!, 0, true);
    const moved = { ...vehicles[0]!, lat: 59.401, updated_at: "2026-10-22T09:00:10Z" };
    const tweens = new Map([["tram-1", nextTween(first, moved, 0, true)]]);
    const fc = vehicleFeatures([moved], tweens, 6_250, ALL_MODES, []);
    expect(fc.features[0]!.geometry.coordinates[1]).toBeCloseTo(59.4005, 6);
  });
});
