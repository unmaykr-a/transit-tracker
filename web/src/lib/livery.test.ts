import { describe, expect, it } from "vitest";

import { liveryOf } from "./livery";

describe("liveryOf", () => {
  it("picks the operator's paint scheme", () => {
    expect(liveryOf({ id: "tram-541", mode: "tram", line: "T4" })).toBe("tlt-tram");
    expect(liveryOf({ id: "bus-2081", mode: "bus", line: "3" })).toBe("tlt-bus");
    expect(liveryOf({ id: "trolley-8471", mode: "trolley", line: "72" })).toBe("tlt-trolley");
    expect(liveryOf({ id: "bus-tartu-630TNS", mode: "bus", line: "tartu-2" })).toBe("tartu");
    expect(liveryOf({ id: "bus-659MSN", mode: "bus", line: "174", county: true })).toBe("county");
    expect(liveryOf({ id: "train-3341", mode: "train", line: "Tallinn - Tartu" })).toBe("elron");
  });
});
