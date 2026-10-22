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
 * Uses the server's pattern match: the vehicle's next stop, found in a main
 * pattern (its own pattern first), and whether it is still at the stop before.
 */
function fromMatch(mains: Pattern[], v: Vehicle): Placement | undefined {
  if (!v.next_stop_id) return undefined;
  const ordered = [...mains.filter((p) => p.id === v.pattern_id), ...mains.filter((p) => p.id !== v.pattern_id && fold(p.headsign) === fold(v.destination))];
  for (const p of ordered) {
    const i = p.stops.findIndex((s) => s.id === v.next_stop_id);
    if (i < 0) continue;
    const prev = p.stops[i - 1];
    if (prev && approxDistanceM(v.lat, v.lon, prev.lat, prev.lon) <= AT_STOP_M) {
      return { vehicle: v, patternId: p.id, stopIndex: i - 1, atStop: true };
    }
    const here = p.stops[i]!;
    return { vehicle: v, patternId: p.id, stopIndex: i, atStop: approxDistanceM(v.lat, v.lon, here.lat, here.lon) <= AT_STOP_M };
  }
  return undefined;
}

/**
 * Places live vehicles on a line's main patterns. The server's match (next
 * stop) wins; otherwise a vehicle goes to the pattern whose headsign matches
 * its destination, else the one with a stop nearest to it, and is either at
 * its nearest stop (within 60 m) or between that stop and the closer neighbour.
 */
export function placeVehicles(patterns: Pattern[], vehicles: Vehicle[]): Placement[] {
  const mains = patterns.filter((p) => p.main && p.stops.length > 0);
  const out: Placement[] = [];
  for (const v of vehicles) {
    const matched = fromMatch(mains, v);
    if (matched) {
      out.push(matched);
      continue;
    }
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
