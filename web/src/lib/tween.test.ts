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

  it("clamps odd intervals", () => {
    const base = nextTween(undefined, at(59.4, 24.7, "2026-10-22T09:00:00Z"), 0, true);
    expect(nextTween(base, at(59.401, 24.7, "2026-10-22T09:05:00Z"), 0, true).duration).toBe(15_000);
    expect(nextTween(base, at(59.401, 24.7, "2026-10-22T09:00:00.2Z"), 0, true).duration).toBe(1_000);
  });
});
