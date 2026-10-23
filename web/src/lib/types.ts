export type Mode = "tram" | "bus" | "trolley" | "train";

export const MODES: readonly Mode[] = ["tram", "bus", "trolley", "train"];

export interface Vehicle {
  id: string;
  mode: Mode;
  line: string;
  label: string;
  lat: number;
  lon: number;
  bearing: number; // -1 unknown
  speed_mps: number;
  destination: string;
  low_floor: boolean;
  /** A county bus (from the county feed). */
  county?: boolean;
  delay_s?: number;
  pattern_id?: string;
  next_stop_id?: string;
  /** Metres along pattern_id's shape. */
  dist_along_m?: number;
  updated_at: string;
}

export interface FeedStatus {
  name: string;
  last_success?: string;
  last_data?: string;
  last_error?: string;
  last_error_at?: string;
  latency_ms: number;
  count: number;
  consecutive_failures: number;
  stale: boolean;
}

/** Position-only update: [id, lat, lon, bearing, speed_mps, updated_at_unix_ms, dist_along_m]. */
export type MoveTuple = [string, number, number, number, number, number, number?];

export type LiveMessage =
  | { type: "snapshot"; version: number; server_time: string; vehicles?: Vehicle[]; feeds?: FeedStatus[] }
  | { type: "diff"; version: number; server_time: string; upsert?: Vehicle[]; move?: MoveTuple[]; remove?: string[] }
  | { type: "stale"; version: number; server_time: string; feeds?: FeedStatus[] };
