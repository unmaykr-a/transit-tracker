import { describe, expect, it } from "vitest";

import { level, minutesUntil, slackNow, view, type Departure } from "./model";

const now = new Date("2026-10-22T09:00:00Z");
const at = (min: number, sec = 0) => new Date(now.getTime() + min * 60000 + sec * 1000).toISOString();
const dep = (min: number, leave: number | null): Departure => ({
  time: at(min),
  minutes: min,
  delay_s: 0,
  realtime: true,
  headsign: "Kopli",
  verdict: "",
  leave_in_min: leave,
});

describe("verdict levels mirror dashboard.Judge", () => {
  it.each([
    [7, "neutral"],
    [5, "neutral"],
    [4, "green"],
    [1, "green"],
    [0, "amber"],
    [-1, "red"],
  ])("slack %i → %s", (slack, want) => expect(level(slack)).toBe(want));
});

describe("countdown", () => {
  it("floors minutes", () => {
    expect(minutesUntil(at(4, 59), now)).toBe(4);
    expect(minutesUntil(at(0, 30), now)).toBe(0);
    expect(minutesUntil(at(-1), now)).toBe(-1);
  });

  it("slack counts down with the departure between polls", () => {
    const d = dep(9, 4); // server: 9 min away, leave in 4
    expect(slackNow(d, now)).toBe(4);
    expect(slackNow(d, new Date(now.getTime() + 3 * 60000))).toBe(1);
    expect(slackNow(dep(9, null), now)).toBeNull();
  });
});

describe("view", () => {
  const attrs = {
    line: "T4",
    mode: "tram" as const,
    headsign: "Kopli",
    stop_name: "Hobujaama",
    platform: "A",
    walk_min: 5,
    next_departures: [dep(3, -2), dep(9, 4), dep(17, 12), dep(25, 20)],
    url: "https://bus.example.com/lines/tram/T4",
  };

  it("skips departures that can no longer be caught", () => {
    const v = view(attrs, now, 2);
    expect(v.departures.map((d) => d.minutes)).toEqual([9, 17]);
    expect(v.departures[0].level).toBe("green");
    expect(v.stop).toBe("Hobujaama · A");
  });

  it("drops departures that have left and keeps red ones when none can be caught", () => {
    const later = new Date(now.getTime() + 10 * 60000);
    expect(view(attrs, later, 5).departures.map((d) => d.minutes)).toEqual([7, 15]);
    const tight = { ...attrs, next_departures: [dep(3, -2), dep(6, -1)] };
    expect(view(tight, now, 3).departures.map((d) => d.level)).toEqual(["red", "red"]);
  });

  it("has no verdict without an origin", () => {
    const v = view({ ...attrs, next_departures: [dep(4, null)] }, now, 3);
    expect(v.departures[0]).toMatchObject({ minutes: 4, level: "none", slack: null });
  });
});
