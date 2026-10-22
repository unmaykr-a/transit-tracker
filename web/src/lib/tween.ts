import type { Vehicle } from "./types";

/**
 * Vehicles report a fix every ~10 s. Instead of jumping, each marker glides
 * from where it is drawn now to the new fix over roughly one update interval.
 * The marker therefore lags reality by up to one interval but never overshoots
 * or cuts corners beyond the straight line between two real fixes.
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

export function positionAt(t: Tween, now: number, shapeOf?: ShapeLookup): [number, number] {
  if (t.duration <= 0 || now >= t.start + t.duration) return [t.toLat, t.toLon];
  if (now <= t.start) return [t.fromLat, t.fromLon];
  const k = (now - t.start) / t.duration;
  const shape = t.pattern && shapeOf ? shapeOf(t.pattern) : undefined;
  if (shape && t.fromAlong !== undefined && t.toAlong !== undefined) {
    return shape.pointAt(t.fromAlong + (t.toAlong - t.fromAlong) * k);
  }
  return [t.fromLat + (t.toLat - t.fromLat) * k, t.fromLon + (t.toLon - t.fromLon) * k];
}

function alongAt(t: Tween, now: number): number | undefined {
  if (t.fromAlong === undefined || t.toAlong === undefined) return undefined;
  if (t.duration <= 0 || now >= t.start + t.duration) return t.toAlong;
  if (now <= t.start) return t.fromAlong;
  return t.fromAlong + ((t.toAlong - t.fromAlong) * (now - t.start)) / t.duration;
}

export function isMoving(t: Tween, now: number): boolean {
  return t.duration > 0 && now < t.start + t.duration && (t.fromLat !== t.toLat || t.fromLon !== t.toLon);
}

export function approxDistanceM(lat1: number, lon1: number, lat2: number, lon2: number): number {
  const kx = 111_320 * Math.cos(((lat1 + lat2) / 2) * (Math.PI / 180));
  return Math.hypot((lat2 - lat1) * 110_574, (lon2 - lon1) * kx);
}

/** Returns the tween for a new fix of v, continuing from where prev is drawn at now. */
export function nextTween(prev: Tween | undefined, v: Vehicle, now: number, animate: boolean, shapeOf?: ShapeLookup): Tween {
  const fixAt = Date.parse(v.updated_at) || now;
  const along = v.pattern_id && v.dist_along_m !== undefined ? { pattern: v.pattern_id, toAlong: v.dist_along_m } : {};
  if (!prev || !animate) {
    return { fromLat: v.lat, fromLon: v.lon, toLat: v.lat, toLon: v.lon, start: now, duration: 0, lastFixAt: fixAt, ...along, fromAlong: along.toAlong };
  }
  if (prev.toLat === v.lat && prev.toLon === v.lon) {
    return { ...prev, lastFixAt: fixAt };
  }
  const [lat, lon] = positionAt(prev, now, shapeOf);
  // Glide along the street when the vehicle moved forward on the same pattern.
  const prevAlong = alongAt(prev, now);
  const followShape =
    along.pattern !== undefined && prev.pattern === along.pattern && prevAlong !== undefined && along.toAlong! >= prevAlong - 5 && along.toAlong! - prevAlong < MAX_TWEEN_DISTANCE_M;
  const interval = fixAt > prev.lastFixAt ? fixAt - prev.lastFixAt : DEFAULT_INTERVAL_MS;
  const far = approxDistanceM(lat, lon, v.lat, v.lon) > MAX_TWEEN_DISTANCE_M;
  return {
    fromLat: lat,
    fromLon: lon,
    toLat: v.lat,
    toLon: v.lon,
    start: now,
    duration: far ? 0 : Math.min(MAX_TWEEN_MS, Math.max(MIN_TWEEN_MS, interval * STRETCH)),
    lastFixAt: fixAt,
    ...(followShape ? { pattern: along.pattern, fromAlong: prevAlong, toAlong: along.toAlong } : { pattern: along.pattern, fromAlong: along.toAlong, toAlong: along.toAlong }),
  };
}
