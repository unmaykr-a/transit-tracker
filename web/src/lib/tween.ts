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
}

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

export function positionAt(t: Tween, now: number): [number, number] {
  if (t.duration <= 0 || now >= t.start + t.duration) return [t.toLat, t.toLon];
  if (now <= t.start) return [t.fromLat, t.fromLon];
  const k = (now - t.start) / t.duration;
  return [t.fromLat + (t.toLat - t.fromLat) * k, t.fromLon + (t.toLon - t.fromLon) * k];
}

export function isMoving(t: Tween, now: number): boolean {
  return t.duration > 0 && now < t.start + t.duration && (t.fromLat !== t.toLat || t.fromLon !== t.toLon);
}

export function approxDistanceM(lat1: number, lon1: number, lat2: number, lon2: number): number {
  const kx = 111_320 * Math.cos(((lat1 + lat2) / 2) * (Math.PI / 180));
  return Math.hypot((lat2 - lat1) * 110_574, (lon2 - lon1) * kx);
}

/** Returns the tween for a new fix of v, continuing from where prev is drawn at now. */
export function nextTween(prev: Tween | undefined, v: Vehicle, now: number, animate: boolean): Tween {
  const fixAt = Date.parse(v.updated_at) || now;
  if (!prev || !animate) {
    return { fromLat: v.lat, fromLon: v.lon, toLat: v.lat, toLon: v.lon, start: now, duration: 0, lastFixAt: fixAt };
  }
  if (prev.toLat === v.lat && prev.toLon === v.lon) {
    return { ...prev, lastFixAt: fixAt };
  }
  const [lat, lon] = positionAt(prev, now);
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
  };
}
