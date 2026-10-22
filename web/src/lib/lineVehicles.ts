import type { Pattern } from "@/api";

import { approxDistanceM } from "./tween";
import type { Vehicle } from "./types";

export interface Placement {
  vehicle: Vehicle;
  patternId: string;
  /** Index of the stop the vehicle is at, or the stop it is heading to. */
  stopIndex: number;
  atStop: boolean;
}

const AT_STOP_M = 60;

function fold(s: string): string {
  return s
    .toLowerCase()
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .replace(/[^\p{L}\p{N}]+/gu, " ")
    .trim();
}

/**
 * Places live vehicles on a line's main patterns. A vehicle goes to the
 * pattern whose headsign matches its destination, else the one with a stop
 * nearest to it. Within the pattern it is either at its nearest stop (within
 * 60 m) or between that stop and whichever neighbour is closer.
 */
export function placeVehicles(patterns: Pattern[], vehicles: Vehicle[]): Placement[] {
  const mains = patterns.filter((p) => p.main && p.stops.length > 0);
  const out: Placement[] = [];
  for (const v of vehicles) {
    const dest = fold(v.destination);
    let candidates = mains.filter((p) => fold(p.headsign) === dest);
    if (candidates.length === 0) candidates = mains;
    let best: { p: Pattern; i: number; d: number } | undefined;
    for (const p of candidates) {
      p.stops.forEach((s, i) => {
        const d = approxDistanceM(v.lat, v.lon, s.lat, s.lon);
        if (!best || d < best.d) best = { p, i, d };
      });
    }
    if (!best) continue;
    const { p, i, d } = best;
    if (d <= AT_STOP_M) {
      out.push({ vehicle: v, patternId: p.id, stopIndex: i, atStop: true });
      continue;
    }
    const dist = (j: number) => {
      const s = p.stops[j];
      return s ? approxDistanceM(v.lat, v.lon, s.lat, s.lon) : Infinity;
    };
    // Closer to the next stop than to the previous one → past stop i, heading to i+1.
    const heading = dist(i + 1) < dist(i - 1) && i + 1 < p.stops.length ? i + 1 : i;
    out.push({ vehicle: v, patternId: p.id, stopIndex: heading, atStop: false });
  }
  return out;
}
