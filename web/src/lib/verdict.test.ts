import { describe, expect, it } from "vitest";

import { judge, minutesUntilSigned } from "./verdict";

const now = Date.parse("2026-10-22T09:00:00Z");
const at = (min: number, sec = 0) => new Date(now + min * 60_000 + sec * 1000).toISOString();

describe("judge (same table as the Go dashboard)", () => {
  it.each([
    [20, 0, 6, 1, "neutral", 13],
    [12, 0, 6, 1, "neutral", 5],
    [11, 0, 6, 1, "green", 4],
    [8, 0, 6, 1, "green", 1],
    [7, 0, 6, 1, "amber", 0],
    [7, 59, 6, 1, "amber", 0],
    [6, 0, 6, 1, "red", -1],
    [0, -30, 0, 0, "red", -1],
  ])("in %im%is, walk %i, buffer %i → %s %i", (m, s, walk, buffer, level, slack) => {
    expect(judge(at(m, s), now, walk, buffer)).toEqual({ level, slack });
  });

  it("floors countdown minutes", () => {
    expect(minutesUntilSigned(at(4, 59), now)).toBe(4);
    expect(minutesUntilSigned(at(0, -1), now)).toBe(-1);
  });
});
