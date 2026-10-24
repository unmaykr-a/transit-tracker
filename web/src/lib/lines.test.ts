import { describe, expect, it } from "vitest";

import { displayLine, lineRegion, matchesFilter, matchesLine, parseLineQuery } from "./lines";
import { kindOf, type Kind, type Mode, type Vehicle } from "./types";

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
    const modes = new Set<Kind>(["bus"]);
    expect(matchesFilter(v("bus", "17"), modes, [])).toBe(true);
    expect(matchesFilter(v("tram", "T4"), modes, [])).toBe(false);
    expect(matchesFilter(v("bus", "17"), modes, ["5"])).toBe(false);
  });

  it("keeps Tartu's lines apart from Tallinn's but finds both by number", () => {
    const tartu2 = v("bus", "tartu-2", "2");
    expect(matchesLine(tartu2, ["2"])).toBe(true); // the map filter: bus 2 in either city
    expect(matchesLine(tartu2, ["=2"])).toBe(false); // Tallinn's line page: its own buses only
    expect(matchesLine(tartu2, ["=tartu-2"])).toBe(true);
    expect(matchesLine(v("bus", "2"), ["=2"])).toBe(true);
    expect(displayLine("tartu-2")).toBe("2");
    expect(displayLine("17A")).toBe("17A");
    expect(displayLine("raplamaa-K28")).toBe("K28");
    expect(displayLine("laane-virumaa-18V")).toBe("18V");
    expect(displayLine("intercity-164")).toBe("164");
    expect(lineRegion("tartu-31R")).toBe("tartu");
    expect(lineRegion("laanemaa-13")).toBe("laanemaa");
    expect(lineRegion("31")).toBeUndefined();
  });

  it("filters county buses with their own chip", () => {
    const county = { ...v("bus", "120"), county: true };
    expect(kindOf(county)).toBe("county");
    expect(matchesFilter(county, new Set<Kind>(["bus"]), [])).toBe(false);
    expect(matchesFilter(county, new Set<Kind>(["county"]), [])).toBe(true);
    expect(matchesFilter(v("bus", "17"), new Set<Kind>(["county"]), [])).toBe(false);
  });
});
