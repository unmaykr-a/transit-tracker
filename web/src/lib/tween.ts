import type { Vehicle } from "./types";

/**
 * Vehicles report a fix every ~10 s. Instead of jumping, each marker glides
 * from where it is drawn now to the new fix over roughly one update interval.
 *
 * A fix is already 5–15 s old when it reaches the client, so gliding to it
 * alone draws moving vehicles 50–150 m behind where they are. A vehicle matched
 * to a pattern and moving therefore glides to where it should be by now:
 * the fix moved on along the shape at its speed (dead reckoning, at most
 * AHEAD_MAX_MS and AHEAD_MAX_M past the fix). The marker never runs backwards:
 * if the next fix is behind it (the bus stopped at a light), it waits there.
 */
export interface Tween {
  fromLat: number;
  fromLon: number;
  toLat: number;
  toLon: number;
  start: number;
  duration: number;
  lastFixAt: number;
  /** When both fixes are on the same pattern, glide along its shape instead of a straight line. */
  pattern?: string;
  fromAlong?: number;
  toAlong?: number;
  /** Dead reckoning: speed along the shape (m/s) from fixAt (same clock as start). */
  speed?: number;
  fixAt?: number;
  /** Drawn on the shape even at rest: trains run on rails, and Elron's raw GPS drifts off them. */
  snap?: boolean;
}

/** Resolves a pattern's shape; returns undefined when it is not loaded (straight-line fallback). */
export type ShapeLookup = (pattern: string) => { pointAt(along: number): [number, number] } | undefined;

export const DEFAULT_INTERVAL_MS = 10_000;
/**
 * Fixes reach the client 10 ± 5 s apart (feed every ~10 s, server polls every
 * 5 s). Gliding a bit longer than the interval keeps markers moving until the
 * next fix instead of stalling; a fix that arrives mid-glide continues from
 * the drawn position.
 */
const STRETCH = 1.25;
const MAX_TWEEN_MS = 15_000;
const MIN_TWEEN_MS = 1_000;
/** Jumps longer than this are teleports (GPS glitch, vehicle reappearing), not motion. */
const MAX_TWEEN_DISTANCE_M = 1_500;
/** Dead reckoning stops this long after the fix, or this far past it (a stop, a light). */
const AHEAD_MAX_MS = 12_000;
const AHEAD_MAX_M = 100;
/** Slower than this counts as standing (GPS jitter at a stop). */
const AHEAD_MIN_SPEED = 1.5;
/** With dead reckoning the target already moves; reach its path quickly. */
const AHEAD_BLEND_MS = 2_500;

/** Where along its shape the tween's vehicle should be at now: the fix plus dead reckoning. */
function targetAlong(t: Tween, now: number): number | undefined {
  if (t.toAlong === undefined) return undefined;
  if (!t.speed || t.fixAt === undefined) return t.toAlong;
  const dt = Math.min(Math.max(now - t.fixAt, 0), AHEAD_MAX_MS);
  return t.toAlong + Math.min((t.speed * dt) / 1000, AHEAD_MAX_M);
}

export function positionAt(t: Tween, now: number, shapeOf?: ShapeLookup): [number, number] {
  const shape = t.pattern && shapeOf ? shapeOf(t.pattern) : undefined;
  if ((t.speed || t.snap) && shape && t.fromAlong !== undefined && t.toAlong !== undefined) return shape.pointAt(alongAt(t, now)!);
  if (t.duration <= 0 || now >= t.start + t.duration) return [t.toLat, t.toLon];
  if (now <= t.start) return [t.fromLat, t.fromLon];
  const k = (now - t.start) / t.duration;
  if (shape && t.fromAlong !== undefined && t.toAlong !== undefined) {
    return shape.pointAt(alongAt(t, now)!);
  }
  return [t.fromLat + (t.toLat - t.fromLat) * k, t.fromLon + (t.toLon - t.fromLon) * k];
}

function alongAt(t: Tween, now: number): number | undefined {
  const target = targetAlong(t, now);
  if (t.fromAlong === undefined || target === undefined) return undefined;
  const k = t.duration <= 0 || now >= t.start + t.duration ? 1 : now <= t.start ? 0 : (now - t.start) / t.duration;
  const along = t.fromAlong + (target - t.fromAlong) * k;
  return t.speed ? Math.max(along, t.fromAlong) : along; // dead reckoning never runs backwards
}

export function isMoving(t: Tween, now: number): boolean {
  if (t.speed && t.fixAt !== undefined && now < t.fixAt + AHEAD_MAX_MS) return true;
  return t.duration > 0 && now < t.start + t.duration && (t.fromLat !== t.toLat || t.fromLon !== t.toLon);
}

export function approxDistanceM(lat1: number, lon1: number, lat2: number, lon2: number): number {
  const kx = 111_320 * Math.cos(((lat1 + lat2) / 2) * (Math.PI / 180));
  return Math.hypot((lat2 - lat1) * 110_574, (lon2 - lon1) * kx);
}

/**
 * Returns the tween for a new fix of v, continuing from where prev is drawn at now.
 * ageMs is how old the fix is at now (enables dead reckoning; see the top of this file).
 */
export function nextTween(prev: Tween | undefined, v: Vehicle, now: number, animate: boolean, shapeOf?: ShapeLookup, ageMs?: number): Tween {
  const fixAt = Date.parse(v.updated_at) || now;
  const along = v.pattern_id && v.dist_along_m !== undefined ? { pattern: v.pattern_id, toAlong: v.dist_along_m } : {};
  if (!prev || !animate) {
    return { fromLat: v.lat, fromLon: v.lon, toLat: v.lat, toLon: v.lon, start: now, duration: 0, lastFixAt: fixAt, ...along, fromAlong: along.toAlong, ...snapOf(v) };
  }
  if (!prev.speed && prev.toLat === v.lat && prev.toLon === v.lon) {
    return { ...prev, lastFixAt: fixAt };
  }
  const [lat, lon] = positionAt(prev, now, shapeOf);
  // Glide along the street when the vehicle moved forward on the same pattern.
  const prevAlong = alongAt(prev, now);
  const followShape =
    along.pattern !== undefined &&
    prev.pattern === along.pattern &&
    prevAlong !== undefined &&
    // After dead reckoning the fix may be behind the marker: a moving vehicle then
    // waits for it, a stopped one glides back along the street.
    along.toAlong! >= prevAlong - (prev.speed ? AHEAD_MAX_M + 30 : 5) &&
    along.toAlong! - prevAlong < MAX_TWEEN_DISTANCE_M;
  const interval = fixAt > prev.lastFixAt ? fixAt - prev.lastFixAt : DEFAULT_INTERVAL_MS;
  const far = approxDistanceM(lat, lon, v.lat, v.lon) > MAX_TWEEN_DISTANCE_M;
  // Dead reckoning: only along a known shape, for a vehicle that is moving.
  const ahead =
    followShape && ageMs !== undefined && ageMs >= 0 && ageMs < AHEAD_MAX_MS * 2 && (v.speed_mps ?? 0) >= AHEAD_MIN_SPEED
      ? { speed: Math.min(v.speed_mps!, 25), fixAt: now - ageMs }
      : {};
  return {
    fromLat: lat,
    fromLon: lon,
    toLat: v.lat,
    toLon: v.lon,
    start: now,
    duration: far ? 0 : ahead.speed ? AHEAD_BLEND_MS : Math.min(MAX_TWEEN_MS, Math.max(MIN_TWEEN_MS, interval * STRETCH)),
    lastFixAt: fixAt,
    ...(followShape ? { pattern: along.pattern, fromAlong: prevAlong, toAlong: along.toAlong } : { pattern: along.pattern, fromAlong: along.toAlong, toAlong: along.toAlong }),
    ...ahead,
    ...snapOf(v),
  };
}

const snapOf = (v: Vehicle) => (v.mode === "train" && v.pattern_id && v.dist_along_m !== undefined ? { snap: true } : {});
