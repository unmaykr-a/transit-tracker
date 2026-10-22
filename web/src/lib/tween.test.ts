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

  it("clamps odd intervals", () => {
    const base = nextTween(undefined, at(59.4, 24.7, "2026-10-22T09:00:00Z"), 0, true);
    expect(nextTween(base, at(59.401, 24.7, "2026-10-22T09:05:00Z"), 0, true).duration).toBe(15_000);
    expect(nextTween(base, at(59.401, 24.7, "2026-10-22T09:00:00.2Z"), 0, true).duration).toBe(1_000);
  });
});
