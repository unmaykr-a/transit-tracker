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
    public field?: string,
  ) {
    super(message);
  }
}

let csrfToken = "";

/** Set from the session bootstrap; sent with every state-changing request. */
export function setCsrfToken(t: string | undefined) {
  csrfToken = t ?? "";
}

async function toError(res: Response, url: string): Promise<ApiError> {
  try {
    const body = (await res.json()) as { error?: string; field?: string };
    return new ApiError(res.status, body.error ?? `${url}: ${res.status}`, body.field);
  } catch {
    return new ApiError(res.status, `${url}: ${res.status}`);
  }
}

async function getJSON<T>(url: string, signal?: AbortSignal): Promise<T> {
  const res = await fetch(url, { signal, headers: { Accept: "application/json" } });
  if (!res.ok) throw await toError(res, url);
  return (await res.json()) as T;
}

/** POST/PATCH/DELETE with JSON and the CSRF header. Returns undefined for 204. */
export async function send<T = unknown>(method: "POST" | "PATCH" | "DELETE", url: string, body?: unknown): Promise<T> {
  const res = await fetch(url, {
    method,
    headers: { "Content-Type": "application/json", Accept: "application/json", "X-CSRF-Token": csrfToken },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) throw await toError(res, url);
  if (res.status === 204) return undefined as T;
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

export type Role = "admin" | "user";

export interface User {
  id: number;
  username: string;
  email: string;
  role: Role;
  locale: string;
  theme: string;
  disabled: boolean;
  created_at: string;
}

export interface PublicSettings {
  instance_name: string;
  signup_mode: "off" | "invite" | "open";
  public_map: boolean;
  default_locale: string;
  default_theme: string;
  map_style_light: string;
  map_style_dark: string;
  map_center: [number, number];
  map_zoom: number;
}

export interface SessionInfo {
  settings: PublicSettings;
  version: string;
  setup_required: boolean;
  user: User | null;
  csrf?: string;
}

export const fetchSession = (signal?: AbortSignal) => getJSON<SessionInfo>("/api/v1/session", signal);

export interface Credentials {
  username: string;
  password: string;
  email?: string;
  invite?: string;
}

export const login = (c: Credentials) => send<{ user: User; csrf: string }>("POST", "/api/v1/auth/login", c);
export const setup = (c: Credentials) => send<{ user: User; csrf: string }>("POST", "/api/v1/auth/setup", c);
export const signup = (c: Credentials) => send<{ user: User; csrf: string }>("POST", "/api/v1/auth/signup", c);
export const logout = () => send("POST", "/api/v1/auth/logout");
export const checkInvite = (token: string) => getJSON<{ role: Role }>(`/api/v1/auth/invites/${encodeURIComponent(token)}`);

export const updateMe = (p: Partial<Pick<User, "email" | "locale" | "theme">>) => send<{ user: User }>("PATCH", "/api/v1/me", p);
export const changePassword = (current_password: string, new_password: string) =>
  send("POST", "/api/v1/me/password", { current_password, new_password });

export interface SessionRow {
  id: string;
  current: boolean;
  created_at: string;
  last_seen_at: string;
  user_agent: string;
  ip: string;
}

export const fetchMySessions = (signal?: AbortSignal) => getJSON<{ sessions: SessionRow[] }>("/api/v1/me/sessions", signal);
export const revokeMySession = (id: string) => send("DELETE", `/api/v1/me/sessions/${id}`);

export interface ApiToken {
  id: number;
  name: string;
  prefix: string;
  created_at: string;
  last_used_at: string | null;
  last_used_ip?: string;
  user_id?: number;
  username?: string;
}

export const tokens = {
  list: (signal?: AbortSignal) => getJSON<{ tokens: ApiToken[] }>("/api/v1/me/tokens", signal),
  create: (name: string) => send<{ token: string; info: ApiToken }>("POST", "/api/v1/me/tokens", { name }),
  revoke: (id: number) => send("DELETE", `/api/v1/me/tokens/${id}`),
};

// Admin

export interface AdminUser extends User {
  sessions: number;
}

export interface Invite {
  id: number;
  role: Role;
  created_by: string;
  created_at: string;
  expires_at: string;
  used_by?: string;
  used_at?: string;
  expired: boolean;
}

export interface AdminSettings extends PublicSettings {
  gps_url: string;
  gps_interval_s: number;
  elron_url: string;
  elron_interval_s: number;
  siri_url: string;
  otp_url: string;
  stops_url: string;
  elron_gtfs_url: string;
  user_agent_contact: string;
  walk_providers: string[];
  transitous_url: string;
}

export interface FeedHealth {
  name: string;
  last_success?: string;
  last_data?: string;
  last_error?: string;
  last_error_at?: string;
  latency_ms: number;
  count: number;
  consecutive_failures: number;
  stale: boolean;
  history: { t: number; count: number }[];
}

export interface StaticStatus {
  imported_at?: string;
  last_attempt?: string;
  last_error?: string;
  duration_ms: number;
  stops: number;
  areas: number;
  routes: number;
  patterns: number;
}

export interface AuditEntry {
  id: number;
  at: string;
  actor: string;
  action: string;
  target: string;
  detail: string;
}

export interface About {
  version: string;
  commit: string;
  go: string;
  os: string;
  arch: string;
  uptime_s: number;
  db_bytes: number;
  users: number;
  data_dir: string;
}

export const admin = {
  users: (signal?: AbortSignal) => getJSON<{ users: AdminUser[] }>("/api/v1/admin/users", signal),
  createUser: (u: { username: string; email: string; password: string; role: Role }) => send<{ user: User }>("POST", "/api/v1/admin/users", u),
  updateUser: (id: number, p: { role?: Role; disabled?: boolean }) => send<{ user: User }>("PATCH", `/api/v1/admin/users/${id}`, p),
  resetPassword: (id: number, password: string) => send("POST", `/api/v1/admin/users/${id}/password`, { password }),
  revokeSessions: (id: number) => send("DELETE", `/api/v1/admin/users/${id}/sessions`),
  deleteUser: (id: number) => send("DELETE", `/api/v1/admin/users/${id}`),
  invites: (signal?: AbortSignal) => getJSON<{ invites: Invite[] }>("/api/v1/admin/invites", signal),
  createInvite: (role: Role, expires_in_hours: number) =>
    send<{ token: string; path: string; invite: Invite }>("POST", "/api/v1/admin/invites", { role, expires_in_hours }),
  deleteInvite: (id: number) => send("DELETE", `/api/v1/admin/invites/${id}`),
  settings: (signal?: AbortSignal) =>
    getJSON<{ settings: AdminSettings; defaults: AdminSettings; overrides: Record<string, boolean> }>("/api/v1/admin/settings", signal),
  updateSettings: (patch: Partial<Record<keyof AdminSettings, unknown>>) =>
    send<{ settings: AdminSettings; defaults: AdminSettings; overrides: Record<string, boolean> }>("PATCH", "/api/v1/admin/settings", patch),
  health: (signal?: AbortSignal) =>
    getJSON<{ feeds: FeedHealth[]; static?: StaticStatus; vehicles: number; server_time: string }>("/api/v1/admin/health", signal),
  refreshStatic: () => send<{ static: StaticStatus }>("POST", "/api/v1/admin/static/refresh"),
  audit: (action: string, before: number, signal?: AbortSignal) =>
    getJSON<{ entries: AuditEntry[] }>(`/api/v1/admin/audit?limit=50&action=${encodeURIComponent(action)}${before ? `&before=${before}` : ""}`, signal),
  about: (signal?: AbortSignal) => getJSON<About>("/api/v1/admin/about", signal),
  tokens: (signal?: AbortSignal) => getJSON<{ tokens: ApiToken[] }>("/api/v1/admin/tokens", signal),
  revokeToken: (id: number) => send("DELETE", `/api/v1/admin/tokens/${id}`),
};

// My lines

export interface StopRef {
  id: string;
  area_id: string;
  name: string;
  platform: string;
  lat: number;
  lon: number;
}

export interface SavedLine {
  id: number;
  route_id: string;
  mode: Mode;
  short_name: string;
  direction: number;
  headsign: string;
  label: string;
  board_stop: StopRef | null;
  alight_stop?: StopRef | null;
  notify_threshold_min: number;
  unavailable?: boolean;
}

export type VerdictLevel = "neutral" | "green" | "amber" | "red" | "none";

export interface DashboardDeparture extends Departure {
  verdict: { level: VerdictLevel; slack_min: number };
}

export interface DashboardEntry {
  line: SavedLine;
  departures: DashboardDeparture[];
  walk?: { minutes: number; distance_m: number; provider: string; approx: boolean };
  next_catchable: number;
  error?: string;
}

export interface Prefs {
  walk_speed_kmh: number;
  buffer_min: number;
}

export interface Place {
  id: number;
  label: string;
  lat: number;
  lon: number;
}

export type Origin = { lat: number; lon: number } | { place: number } | null;

export const fetchDashboard = (origin: Origin, signal?: AbortSignal) => {
  const p = new URLSearchParams();
  if (origin && "place" in origin) p.set("place", String(origin.place));
  else if (origin) {
    p.set("lat", origin.lat.toFixed(5));
    p.set("lon", origin.lon.toFixed(5));
  }
  return getJSON<{ entries: DashboardEntry[]; prefs: Prefs; has_origin: boolean; server_time: string }>(`/api/v1/me/dashboard?${p}`, signal);
};

export const myLines = {
  list: (signal?: AbortSignal) => getJSON<{ lines: SavedLine[] }>("/api/v1/me/lines", signal),
  create: (l: { route_id: string; direction: number; board_stop_id: string; alight_stop_id?: string; label?: string }) =>
    send<{ line: SavedLine }>("POST", "/api/v1/me/lines", l),
  update: (id: number, p: { label?: string; alight_stop_id?: string }) => send<{ line: SavedLine }>("PATCH", `/api/v1/me/lines/${id}`, p),
  remove: (id: number) => send("DELETE", `/api/v1/me/lines/${id}`),
  reorder: (ids: number[]) => send<{ lines: SavedLine[] }>("POST", "/api/v1/me/lines/reorder", { ids }),
};

export const places = {
  list: (signal?: AbortSignal) => getJSON<{ places: Place[] }>("/api/v1/me/places", signal),
  create: (p: Omit<Place, "id">) => send<{ place: Place }>("POST", "/api/v1/me/places", p),
  remove: (id: number) => send("DELETE", `/api/v1/me/places/${id}`),
};

export const prefs = {
  get: (signal?: AbortSignal) => getJSON<Prefs>("/api/v1/me/prefs", signal),
  update: (p: Partial<Prefs>) => send<Prefs>("PATCH", "/api/v1/me/prefs", p),
};

export interface TripStop {
  stop_id: string;
  index: number;
  distance_m: number;
  eta: string;
  name: string;
  platform: string;
  area_id: string;
}

export interface VehicleTrip {
  vehicle: Vehicle;
  headsign?: string;
  trip: { pattern_id: string; along_m: number; next_stop_index: number; speed_mps: number; ahead: TripStop[] } | null;
}

export const fetchTrip = (id: string, signal?: AbortSignal) => getJSON<VehicleTrip>(`/api/v1/vehicles/${encodeURIComponent(id)}/trip?limit=6`, signal);
