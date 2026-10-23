import { describe, expect, it } from "vitest";

import { isMoving, nextTween, positionAt } from "./tween";
import type { Vehicle } from "./types";

const at = (lat: number, lon: number, t: string): Vehicle => ({
  id: "x", mode: "tram", line: "T4", label: "T4", lat, lon, bearing: 0, speed_mps: 5,
  destination: "Tondi", low_floor: true, updated_at: t,
});

describe("tween", () => {
  it("starts in place, then glides over the update interval", () => {
    const t0 = 1_000_000;
    const first = nextTween(undefined, at(59.4, 24.7, "2026-10-22T09:00:00Z"), t0, true);
    expect(positionAt(first, t0)).toEqual([59.4, 24.7]);
    expect(isMoving(first, t0)).toBe(false);

    const second = nextTween(first, at(59.401, 24.7, "2026-10-22T09:00:10Z"), t0 + 500, true);
    expect(second.duration).toBe(12_500);
    const [mid] = positionAt(second, t0 + 500 + 6_250);
    expect(mid).toBeCloseTo(59.4005, 6);
    expect(positionAt(second, t0 + 500 + 20_000)).toEqual([59.401, 24.7]);
    expect(isMoving(second, t0 + 600)).toBe(true);
  });

  it("continues from the drawn position when a fix arrives mid-glide", () => {
    const t0 = 0;
    let tw = nextTween(undefined, at(59.4, 24.7, "2026-10-22T09:00:00Z"), t0, true);
    tw = nextTween(tw, at(59.401, 24.7, "2026-10-22T09:00:10Z"), t0, true);
    const tw2 = nextTween(tw, at(59.402, 24.7, "2026-10-22T09:00:20Z"), t0 + 6_250, true);
    expect(tw2.fromLat).toBeCloseTo(59.4005, 6);
  });

  it("teleports on large jumps and when motion is reduced", () => {
    const base = nextTween(undefined, at(59.4, 24.7, "2026-10-22T09:00:00Z"), 0, true);
    expect(nextTween(base, at(59.5, 24.7, "2026-10-22T09:00:10Z"), 0, true).duration).toBe(0);
    expect(nextTween(base, at(59.401, 24.7, "2026-10-22T09:00:10Z"), 0, false).duration).toBe(0);
  });

  it("glides along the pattern shape when both fixes are on it", () => {
    const shape = { pointAt: (a: number): [number, number] => [59.4 + a / 1e5, 24.7] };
    const lookup = (p: string) => (p === "p1" ? shape : undefined);
    const v1 = { ...at(59.4, 24.7, "2026-10-22T09:00:00Z"), pattern_id: "p1", dist_along_m: 0 };
    const v2 = { ...at(59.401, 24.71, "2026-10-22T09:00:10Z"), pattern_id: "p1", dist_along_m: 100 };
    const t1 = nextTween(undefined, v1, 0, true, lookup);
    const t2 = nextTween(t1, v2, 0, true, lookup);
    expect([t2.fromAlong, t2.toAlong]).toEqual([0, 100]);
    // Halfway in time = halfway along the shape (not the straight line, whose lon would move).
    expect(positionAt(t2, t2.duration / 2, lookup)).toEqual([59.4 + 50 / 1e5, 24.7]);
    // Without the shape loaded it falls back to the straight line.
    expect(positionAt(t2, t2.duration / 2)[1]).toBeCloseTo(24.705, 6);
    // Moving backwards (another trip on the same pattern) does not follow the shape.
    const back = nextTween(t2, { ...v2, lat: 59.4001, dist_along_m: 10, updated_at: "2026-10-22T09:00:20Z" }, t2.duration, true, lookup);
    expect(back.fromAlong).toBe(back.toAlong);
  });

  it("dead-reckons a moving vehicle past its (old) fix, never backwards, and stops after a while", () => {
    const shape = { pointAt: (a: number): [number, number] => [59.4 + a / 1e5, 24.7] };
    const lookup = () => shape;
    const along = (t: ReturnType<typeof nextTween>, now: number) => Math.round((positionAt(t, now, lookup)[0] - 59.4) * 1e5);
    const v1 = { ...at(59.4, 24.7, "2026-10-22T09:00:00Z"), pattern_id: "p1", dist_along_m: 0 };
    const t1 = nextTween(undefined, v1, 0, true, lookup);
    // The fix at 100 m is 6 s old on arrival and the bus does 10 m/s: by now it is near 160 m.
    const v2 = { ...v1, lat: 59.401, dist_along_m: 100, speed_mps: 10, updated_at: "2026-10-22T09:00:10Z" };
    const t2 = nextTween(t1, v2, 0, true, lookup, 6_000);
    expect(along(t2, 2_500)).toBe(185); // blended onto the moving target: 100 + 10 m/s × 8.5 s
    expect(along(t2, 60_000)).toBe(200); // capped at AHEAD_MAX_M past the fix
    // The next fix is behind the marker (the bus stopped at a light): a moving one waits…
    const v3 = { ...v2, lat: 59.4015, dist_along_m: 150, speed_mps: 3, updated_at: "2026-10-22T09:00:20Z" };
    const t3 = nextTween(t2, v3, 10_000, true, lookup, 5_000);
    expect(along(t3, 10_500)).toBe(along(t2, 10_000));
    // …a stopped one glides back to the truth.
    const t4 = nextTween(t2, { ...v3, speed_mps: 0 }, 10_000, true, lookup, 5_000);
    expect(along(t4, 10_000 + t4.duration)).toBe(150);
    // Standing still (below walking pace): no dead reckoning.
    expect(nextTween(t1, { ...v2, speed_mps: 1 }, 0, true, lookup, 6_000).speed).toBeUndefined();
  });

  it("draws matched trains on their track, not at the raw GPS", () => {
    const shape = { pointAt: (a: number): [number, number] => [59.4 + a / 1e5, 24.7] };
    const train = { ...at(59.4003, 24.705, "2026-10-22T09:00:00Z"), mode: "train" as const, pattern_id: "r1", dist_along_m: 30 };
    expect(positionAt(nextTween(undefined, train, 0, true), 0, () => shape)).toEqual([59.4 + 30 / 1e5, 24.7]);
    // A bus stays where its GPS says when it stands.
    const bus = { ...train, mode: "bus" as const };
    expect(positionAt(nextTween(undefined, bus, 0, true), 0, () => shape)).toEqual([59.4003, 24.705]);
  });

  it("clamps odd intervals", () => {
    const base = nextTween(undefined, at(59.4, 24.7, "2026-10-22T09:00:00Z"), 0, true);
    expect(nextTween(base, at(59.401, 24.7, "2026-10-22T09:05:00Z"), 0, true).duration).toBe(15_000);
    expect(nextTween(base, at(59.401, 24.7, "2026-10-22T09:00:00.2Z"), 0, true).duration).toBe(1_000);
  });
});
