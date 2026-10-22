import { describe, expect, it } from "vitest";

import { applyMessage, emptyLiveState, staleFeeds } from "./liveState";
import type { Vehicle } from "./types";

const v = (id: string, lat = 59.4): Vehicle => ({
  id, mode: "bus", line: "17", label: "17", lat, lon: 24.7, bearing: 0, speed_mps: 0,
  destination: "X", low_floor: true, updated_at: "2026-10-22T09:00:00Z",
});

describe("applyMessage", () => {
  it("replaces everything on snapshot and reports removals", () => {
    let s = applyMessage(emptyLiveState(), { type: "snapshot", version: 1, server_time: "2026-10-22T09:00:00Z", vehicles: [v("a"), v("b")] }).state;
    const r = applyMessage(s, { type: "snapshot", version: 2, server_time: "2026-10-22T09:00:00Z", vehicles: [v("b"), v("c")] });
    s = r.state;
    expect([...s.vehicles.keys()]).toEqual(["b", "c"]);
    expect(r.removed).toEqual(["a"]);
    expect(r.reset).toBe(true);
    expect(s.version).toBe(2);
  });

  it("applies diffs without mutating the previous state", () => {
    const s1 = applyMessage(emptyLiveState(), { type: "snapshot", version: 1, server_time: "2026-10-22T09:00:00Z", vehicles: [v("a"), v("b")] }).state;
    const r = applyMessage(s1, { type: "diff", version: 2, server_time: "2026-10-22T09:00:10Z", upsert: [v("a", 59.5), v("c")], remove: ["b", "zz"] });
    expect(s1.vehicles.size).toBe(2);
    expect(r.state.vehicles.get("a")?.lat).toBe(59.5);
    expect([...r.state.vehicles.keys()].sort()).toEqual(["a", "c"]);
    expect(r.removed).toEqual(["b"]); // unknown ids are ignored
    expect(r.changed.map((x) => x.id)).toEqual(["a", "c"]);
  });

  it("applies compact position-only moves", () => {
    const s1 = applyMessage(emptyLiveState(), { type: "snapshot", version: 1, server_time: "2026-10-22T09:00:00Z", vehicles: [v("a")] }).state;
    const t = Date.parse("2026-10-22T09:00:10Z");
    const r = applyMessage(s1, { type: "diff", version: 2, server_time: "2026-10-22T09:00:10Z", move: [["a", 59.41, 24.71, 90, 5.5, t], ["ghost", 1, 2, 3, 4, t]] });
    const a = r.state.vehicles.get("a")!;
    expect([a.lat, a.lon, a.bearing, a.speed_mps, a.updated_at]).toEqual([59.41, 24.71, 90, 5.5, "2026-10-22T09:00:10.000Z"]);
    expect(a.destination).toBe("X"); // details kept
    expect(r.changed.map((x) => x.id)).toEqual(["a"]);
    expect(r.state.vehicles.has("ghost")).toBe(false);
  });

  it("tracks feeds and clock skew from stale messages", () => {
    const feeds = [{ name: "gps", latency_ms: 1, count: 1, consecutive_failures: 0, stale: true }];
    const r = applyMessage(emptyLiveState(), { type: "stale", version: 0, server_time: "2026-10-22T09:00:05Z", feeds }, Date.parse("2026-10-22T09:00:00Z"));
    expect(staleFeeds(r.state.feeds).map((f) => f.name)).toEqual(["gps"]);
    expect(r.state.clockSkewMs).toBe(5000);
  });
});
