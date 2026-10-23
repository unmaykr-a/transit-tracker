import { describe, expect, it } from "vitest";

import type { TripStop } from "@/api";

import { ridePhase, type Ride } from "./ride";

const stop = (index: number, eta = `2026-10-23T10:${String(index).padStart(2, "0")}:00Z`): TripStop => ({
  stop_id: `s${index}`, index, distance_m: index * 400, eta, name: `Stop ${index}`, platform: "", area_id: `a${index}`,
});
const ride: Ride = { vehicle: "bus-1", pattern: "p", stop: "s7", index: 7, name: "Stop 7" };

describe("ridePhase", () => {
  it("counts the stops before yours, warns one stop ahead and at the next stop", () => {
    expect(ridePhase(ride, "p", [3, 4, 5, 6, 7].map((i) => stop(i)))).toEqual({ kind: "riding", stopsLeft: 4, eta: stop(7).eta });
    expect(ridePhase(ride, "p", [6, 7, 8].map((i) => stop(i)))).toEqual({ kind: "prepare", eta: stop(7).eta });
    expect(ridePhase(ride, "p", [7, 8].map((i) => stop(i)))).toEqual({ kind: "next", eta: stop(7).eta });
    expect(ridePhase(ride, "p", [8, 9].map((i) => stop(i)))).toEqual({ kind: "arrived" });
  });

  it("is unknown without a prediction on the same pattern, and riding when yours is beyond it", () => {
    expect(ridePhase(ride, "p", undefined)).toEqual({ kind: "unknown" });
    expect(ridePhase(ride, "other", [stop(3)])).toEqual({ kind: "unknown" });
    expect(ridePhase(ride, "p", [stop(1), stop(2)])).toEqual({ kind: "riding", stopsLeft: 6, eta: "" });
  });
});
