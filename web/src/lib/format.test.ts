import { describe, expect, it } from "vitest";

import { clock, countdown, minutesUntil } from "./format";

const t = ((k: string, o?: { count?: number }) => (k === "time.now" ? "now" : `${o?.count} min`)) as never;

describe("format", () => {
  const now = Date.parse("2026-10-22T07:00:00Z");
  it("counts down", () => {
    expect(countdown("2026-10-22T07:00:30Z", now, t)).toBe("now");
    expect(countdown("2026-10-22T07:00:59Z", now, t)).toBe("now");
    expect(countdown("2026-10-22T07:04:50Z", now, t)).toBe("4 min"); // floored, matching the verdict
    expect(countdown("2026-10-22T07:04:10Z", now, t)).toBe("4 min");
    expect(countdown("2026-10-22T07:59:00Z", now, t)).toBe("59 min");
    expect(countdown("2026-10-22T08:30:00Z", now, t, "et")).toBe("11:30"); // Tallinn is UTC+3 in October
  });
  it("floors minutes and never goes negative", () => {
    expect(minutesUntil("2026-10-22T07:04:59Z", now)).toBe(4);
    expect(minutesUntil("2026-10-22T06:59:00Z", now)).toBe(0);
  });
  it("formats clock time in Tallinn", () => {
    expect(clock("2026-10-22T21:05:00Z", "et")).toBe("00:05");
    expect(clock("2026-10-22T12:05:00Z", "en")).toBe("15:05");
  });
});
