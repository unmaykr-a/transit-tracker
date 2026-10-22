import { describe, expect, it } from "vitest";

import { matchesFilter, matchesLine, parseLineQuery } from "./lines";
import type { Mode, Vehicle } from "./types";

const v = (mode: Mode, line: string, label = line) =>
  ({ id: "x", mode, line, label, lat: 0, lon: 0, bearing: 0, speed_mps: 0, destination: "", low_floor: false, updated_at: "" }) as Vehicle;

describe("line matching", () => {
  it("parses queries", () => {
    expect(parseLineQuery(" 4, 17A  t2 ")).toEqual(["4", "17a", "t2"]);
    expect(parseLineQuery("")).toEqual([]);
  });

  it("matches trams with or without the T prefix", () => {
    expect(matchesLine(v("tram", "T4"), ["4"])).toBe(true);
    expect(matchesLine(v("tram", "T4"), ["t4"])).toBe(true);
    expect(matchesLine(v("bus", "4"), ["4"])).toBe(true);
    expect(matchesLine(v("bus", "40"), ["4"])).toBe(false);
    expect(matchesLine(v("bus", "T4"), ["4"])).toBe(false);
  });

  it("matches trains by trip number label", () => {
    expect(matchesLine(v("train", "Tallinn - Tartu", "10"), ["10"])).toBe(true);
  });

  it("combines modes and lines", () => {
    const modes = new Set<Mode>(["bus"]);
    expect(matchesFilter(v("bus", "17"), modes, [])).toBe(true);
    expect(matchesFilter(v("tram", "T4"), modes, [])).toBe(false);
    expect(matchesFilter(v("bus", "17"), modes, ["5"])).toBe(false);
  });
});
