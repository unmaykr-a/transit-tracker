import { describe, expect, it } from "vitest";

import { decodePolyline, Shape } from "./shapes";

describe("shapes", () => {
  it("decodes Google's example polyline", () => {
    expect(decodePolyline("_p~iF~ps|U_ulLnnqC_mqNvxq`@")).toEqual([
      [-120.2, 38.5],
      [-120.95, 40.7],
      [-126.453, 43.252],
    ]);
  });

  it("finds points along an L-shaped route", () => {
    const s = new Shape([
      [24.7, 59.4],
      [24.7, 59.409],
      [24.7176, 59.409],
    ]);
    expect(s.length).toBeGreaterThan(1980);
    expect(s.length).toBeLessThan(2030);
    const [lat, lon] = s.pointAt(500);
    expect(lon).toBeCloseTo(24.7, 6);
    expect(lat).toBeCloseTo(59.4045, 3);
    const [lat2, lon2] = s.pointAt(1500);
    expect(lat2).toBeCloseTo(59.409, 6);
    expect(lon2).toBeGreaterThan(24.7);
    expect(s.pointAt(-1)).toEqual([59.4, 24.7]);
    expect(s.pointAt(1e9)).toEqual([59.409, 24.7176]);
  });
});
