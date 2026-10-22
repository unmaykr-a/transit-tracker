import type { Mode, Vehicle } from "@/lib/types";

export interface Health {
  status: string;
  version: string;
  commit: string;
  uptime_s: number;
}

export interface LineChip {
  route_id: string;
  mode: Mode;
  short_name: string;
}

export interface Route {
  id: string;
  mode: Mode;
  short_name: string;
  long_name: string;
  color?: string;
  text_color?: string;
}

export interface AreaHit {
  id: string;
  name: string;
  lat: number;
  lon: number;
  modes: Mode[];
  lines: LineChip[];
}

export interface SearchResult {
  lines: Route[];
  stops: AreaHit[];
}

export interface PlatformLine extends LineChip {
  headsigns: string[];
}

export interface Platform {
  id: string;
  code: string;
  name: string;
  platform: string;
  street?: string;
  lat: number;
  lon: number;
  modes: Mode[];
  area_id: string;
  lines: PlatformLine[];
}

export interface StopArea {
  id: string;
  name: string;
  lat: number;
  lon: number;
  modes: Mode[];
  lines: LineChip[];
  platforms: Platform[];
  focus: string;
}

export interface Departure {
  stop_id: string;
  platform: string;
  mode: Mode;
  line: string;
  route_id?: string;
  headsign: string;
  scheduled_at: string;
  expected_at: string;
  realtime: boolean;
  delay_s: number;
  low_floor: boolean;
  source: "siri" | "otp";
}

export interface PatternStop {
  id: string;
  name: string;
  platform: string;
  area_id: string;
  lat: number;
  lon: number;
}

export interface Pattern {
  id: string;
  direction: number;
  headsign: string;
  main: boolean;
  stops: PatternStop[];
  shape: [number, number][];
}

export interface LineDetail {
  route: Route;
  patterns: Pattern[];
  vehicles: Vehicle[];
}

export interface MapStop {
  id: string;
  area_id: string;
  name: string;
  platform: string;
  lat: number;
  lon: number;
  modes: Mode[];
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

async function getJSON<T>(url: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(url, { signal, headers: { Accept: "application/json" } });
  if (!res.ok) throw new ApiError(res.status, `${url}: ${res.status}`);
  return (await res.json()) as T;
}

export const fetchHealth = (signal?: AbortSignal) => getJSON<Health>("/healthz", signal);

export const fetchSearch = (q: string, signal?: AbortSignal) =>
  getJSON<SearchResult>(`/api/v1/search?q=${encodeURIComponent(q)}&limit=8`, signal);

export const fetchStop = (id: string, signal?: AbortSignal) =>
  getJSON<StopArea>(`/api/v1/stops/${encodeURIComponent(id)}`, signal);

export const fetchDepartures = (id: string, opts: { platform?: string; limit?: number } = {}, signal?: AbortSignal) => {
  const p = new URLSearchParams({ limit: String(opts.limit ?? 30) });
  if (opts.platform) p.set("platform", opts.platform);
  return getJSON<{ departures: Departure[]; server_time: string }>(`/api/v1/stops/${encodeURIComponent(id)}/departures?${p}`, signal);
};

export const fetchLine = (mode: string, line: string, signal?: AbortSignal) =>
  getJSON<LineDetail>(`/api/v1/lines/${encodeURIComponent(mode)}/${encodeURIComponent(line)}`, signal);

export const fetchAllStops = (signal?: AbortSignal) =>
  getJSON<{ imported_at: string; stops: MapStop[] }>("/api/v1/stops", signal);
