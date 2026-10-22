> **Note:** the app was renamed from the working name "Tram" to **Bus** at 1.0.0 (environment variables `BUS_*`,
> Home Assistant domain `bus`). This plan keeps the original wording.

# Tallinn Transit Tracker — Build Plan

Oct 22, 2026 · @Andri meister

## Overview and goals

Build a self-hosted, multi-user Docker app (working name **Tram**; rename freely) that shows live Tallinn city buses, trams, trolleybuses and Elron trains, tells each user whether they can make their next departure from where they are, and exposes the same data to Home Assistant as sensors and a Lovelace card. It runs as one container on a Raspberry Pi 5 (arm64) and on amd64, behind a reverse proxy.

**Must have (v1)**

- Live map of every vehicle, smoothly animated, with filters by mode and line.
- Search by line number (shows route, stops, live vehicles) and by stop name (shows platforms, lines serving it, next departures).
- Per-user saved "My lines": a line + direction + boarding stop (+ optional alighting stop), shown on a home dashboard with live countdowns.
- "Can I make it?" verdict per departure using walking time from the user's current location to the stop.
- Admin area: users, roles, invites, data-source settings, polling intervals, HA API tokens, health/status.
- Home Assistant integration (custom component via HACS) + custom Lovelace card that picks from the user's saved lines.
- Modern, minimal, mobile-first UI; installable as a PWA; light and dark themes; Estonian and English.

**Non-goals (v1)**

- Full A-to-B journey planner with transfers (can be added later via the national OpenTripPlanner API).
- Ticketing, push notifications to phones outside HA, Harju county (regional) buses on the map. County buses are a stretch goal, see Data sources.
- arm/v7 builds.

## Data sources

All needed data is public and keyless; the backend polls each source once and fans out to every client, so upstream load stays constant no matter how many users there are. Claude Code must verify each endpoint live in milestone 1 before building on it, because these are unofficial feeds without SLAs.

| Source | What it gives | Format / cadence | Use in app |
| --- | --- | --- | --- |
| `https://transport.tallinn.ee/gps.txt` | Live positions of all TLT buses, trams, trolleybuses | CSV: `type,line,lon*1e6,lat*1e6,,heading,vehicleId,lowFloor,?,destination`; type 1 = trolley, 2 = bus, 3 = tram; rewritten every \~10 s; speed column always empty | Map vehicles, ETA estimation |
| `https://transport.tallinn.ee/siri-stop-departures.php?stopid=<siriId>` | Live expected vs scheduled departures per platform | CSV lines `type,line,expectedSec,scheduledSec,destination,secondsUntil,lowFloor` | Departure boards, My lines countdowns |
| `https://transport.tallinn.ee/data/stops.txt`, `routes.txt` | City stops (with SIRI ids) and route/stop sequences | CSV, changes rarely | Stop search, line pages, platform letters |
| `https://www.elron.ee/map_data.json` | Live Elron trains: trip no., route, position, heading, speed, delay (min), last stop | JSON, poll \~10 s | Trains on map, train delays |
| `https://www.elron.ee/stops_data.json` | 141 Elron stations | JSON | Station search |
| Elron GTFS `https://eu-gtfs.remix.com/elron.zip` | Static rail timetable, shapes, colours | GTFS, refresh daily | Station boards, line shapes |
| Peatus.ee OpenTripPlanner GraphQL `https://api.peatus.ee/routing/v1/routers/estonia/index/graphql` | National stops, routes, stop-times with realtime, trip plans incl. walking legs | GraphQL POST | Line shapes/patterns, train station departures, walking routes |
| Transitous (MOTIS) | Open routing, geocoding, stop times | JSON API | Fallback for walking time and geocoding |
| Ridango WebSocket behind iil.pilet.ee | Harju county buses, full fleet every \~3 s | WebSocket | Stretch goal only |

**Reference projects to read, not copy blindly**

- [tahaxerel/live-tallinn-transport](https://github.com/tahaxerel/live-tallinn-transport): zero-dependency Node proxy + MapLibre page. Most useful parts: the exact feed formats, the dead-reckoning that animates vehicles between 10 s fixes, projecting vehicle GPS onto a trip's shape to find "N stops away", and the leave-now verdict. No license is shown, so reimplement ideas rather than copying code.
- [estonian-transport-mcp](https://pypi.org/project/estonian-transport-mcp/): documents the peatus.ee GraphQL queries for stop search, departures, nearby stops, route patterns and trip stops. Good template for the OTP client.

Licensing note: check the Tallinn open-data terms and the peatus.ee terms, cache aggressively, set a descriptive `User-Agent` with a contact URL, and never poll more often than the feed updates.

## Architecture and stack

One Go binary serves the API, the WebSocket stream and the embedded React build; SQLite holds users and settings; live state lives in memory. This keeps the image small (\~30 MB), the Pi's RAM use low, and the same language split as Silt (Go backend, TypeScript UI).

| Layer | Choice | Why |
| --- | --- | --- |
| Backend | Go 1.23+, `chi` router, `nhooyr.io/websocket` (or `coder/websocket`), `sqlc` + `modernc.org/sqlite` (pure Go, no CGO) | Static binary, trivial multi-arch builds |
| Migrations | `goose` embedded | Runs on startup |
| Frontend | React 19 + TypeScript + Vite, TanStack Query + Router, Tailwind CSS v4, shadcn/ui (Radix) | Fast, minimal, accessible components |
| Map | MapLibre GL JS + OpenFreeMap vector tiles (positron / dark-matter style), vehicles as a GeoJSON symbol layer | No API key, smooth with 400+ vehicles |
| Realtime to clients | WebSocket `/ws`, server pushes compact vehicle diffs every feed tick; SSE fallback | One upstream poll, many viewers |
| Auth | Session cookie (HttpOnly, SameSite=Lax) + argon2id passwords; optional OIDC (Authentik) | Fits existing Authentik at auth.taali.ee |
| HA | Python custom component (`custom_components/tram`) + Lovelace card in TypeScript/Lit, shipped as separate HACS repos or one monorepo with two release artifacts | Native HA sensors + UI card |
| Packaging | Multi-stage Dockerfile, distroless/static runtime, `linux/arm64` + `linux/amd64` via buildx, GHCR | Pi 5 + x86 |

**Repo layout**

```
tram/
  cmd/tram/            main.go (flags, config, wiring)
  internal/
    feeds/             tallinn_gps.go, tallinn_siri.go, elron.go, otp.go, gtfs.go
    state/             in-memory vehicle store, ring buffer of recent fixes
    eta/               shape projection, ETA, dead reckoning
    walk/              walking-time providers (OTP, Transitous, haversine)
    api/               REST handlers, ws hub, auth middleware
    store/             sqlc queries, migrations
    staticdata/        stop/route/shape import + refresh jobs
  web/                 Vite React app (built into internal/api/dist via go:embed)
  ha/
    custom_components/tram/
    card/              tram-card (Lit, Rollup)
  deploy/              docker-compose.yml, .env.example
  docs/
```

The diagram below shows how the pieces connect.

&#91;embedded content: system architecture · 4 feeds, 1 container, 4 clients\]

Upstream load stays at one request per feed interval regardless of user count; the Lovelace card reads HA entities, never the API directly.

## Data model

SQLite stores people, preferences and imported static transit data; live vehicles and departures are never persisted, only cached in memory with timestamps.

| Table | Key columns | Notes |
| --- | --- | --- |
| `users` | id, username, email, password\_hash, role (`admin`/`user`), locale, theme, created\_at, disabled | First run creates the admin from env or a setup wizard |
| `sessions` | id, user\_id, expires\_at, user\_agent, ip | Revocable from admin and profile |
| `invites` | token\_hash, role, expires\_at, created\_by, used\_by | Open signup off by default |
| `api_tokens` | id, user\_id, name, token\_hash, scopes (`read`), last\_used\_at | Used by HA; shown once on creation |
| `saved_lines` | id, user\_id, mode, line, direction\_id/headsign, board\_stop\_id, alight\_stop\_id (nullable), label, sort, notify\_threshold\_min | Core of "My lines" and the HA card |
| `saved_places` | id, user\_id, label (Home, Work), lat, lon | Fallback origin when GPS is off; used by HA |
| `user_prefs` | user\_id, walk\_speed\_kmh (default 4.8), buffer\_min (default 1), default\_map\_layers | Drives the can-I-make-it verdict |
| `settings` | key, value (JSON) | Admin-editable: poll intervals, feed URLs, map style URL, walking provider, signup mode, OIDC config |
| `stops` | id, siri\_id, gtfs\_id, name, platform\_code, lat, lon, modes, source | Imported from Tallinn stops.txt + Elron GTFS/stops\_data |
| `routes` | id, mode, short\_name, long\_name, color, source | Tallinn routes.txt + Elron GTFS |
| `patterns` | id, route\_id, direction, headsign, stop\_ids (ordered JSON), shape (encoded polyline) | Needed for projecting vehicles onto a line |
| `audit_log` | at, actor\_id, action, target, detail | Admin actions |

Static data refreshes nightly (03:30 Europe/Tallinn) and on demand from admin; the import is atomic (load into staging tables, swap).

## Backend: ingestion, prediction, API

The backend runs one goroutine per feed, merges everything into a single normalized vehicle model, and serves departures on demand with short caches.

**Ingestion workers**

1. `gps` poller: every 5 s (feed updates \~10 s), `If-Modified-Since`/ETag, parse CSV, skip unchanged snapshot. Keep the last 5 fixes per vehicle to derive speed and bearing.
2. `elron` poller: every 10 s, normalize trains into the same model (`mode=train`, `delay_min`, `last_stop`).
3. `siri` fetcher: on demand per stop, cache 15 s, coalesce concurrent requests (singleflight). Pre-warm stops that appear in any user's saved lines every 20 s so dashboards and HA are instant.
4. `trains departures`: peatus.ee OTP `stoptimesWithoutPatterns` for Elron stations, cache 30 s.
5. `staticdata` job: nightly import (see Data model).
6. Every worker uses exponential backoff with jitter, records last success/error for the admin health page, and marks data stale after 60 s so the UI can show a "feed delayed" banner.

Normalized vehicle: `{id, mode, line, lat, lon, bearing, speed_mps, destination, low_floor, delay_s?, pattern_id?, next_stop_id?, updated_at}`.

**Prediction (`internal/eta`)**

- Match each vehicle to a pattern: same line, destination/headsign match, then nearest shape by projected distance and heading agreement.
- Project the GPS point onto the pattern shape to get distance-along-route; derive next stop and stops remaining.
- Live departure times come from SIRI where available (authoritative). For a stop with no SIRI data (trains, gaps), estimate ETA = remaining distance / smoothed speed, blended with scheduled segment times.
- For the map, send positions plus speed and bearing; the client dead-reckons along the shape between ticks (pattern taken from the reference project).

**REST API (`/api/v1`, JSON)**

| Endpoint | Purpose |
| --- | --- |
| `GET /vehicles?mode=&line=` | Snapshot of live vehicles |
| `WS /ws` | Subscribe; messages `snapshot`, `diff`, `stale`; client can send `{filter: {lines:[...]}}` |
| `GET /stops/search?q=` | Fuzzy stop search (diacritic-insensitive: "ulemiste" finds "Ülemiste") |
| `GET /stops/{id}` | Stop + platforms + lines serving each platform |
| `GET /stops/{id}/departures?limit=` | Merged live + scheduled departures |
| `GET /lines/search?q=` | Lines by number/name across modes |
| `GET /lines/{mode}/{line}` | Patterns, ordered stops, shape, live vehicles |
| `GET /walk?from=lat,lon&to=stopId` | Walking time and polyline |
| `GET /me`, `PATCH /me` | Profile and prefs |
| `GET/POST/PATCH/DELETE /me/lines` | Saved lines CRUD + reorder |
| `GET /me/dashboard?lat=&lon=` | Saved lines with next 3 departures and verdict each, one call |
| `GET/POST/DELETE /me/tokens` | API tokens for HA |
| `GET /ha/lines`, `GET /ha/lines/{id}` | Token-auth, compact payload for HA |
| `/admin/*` | Users, invites, settings, feed health, static data refresh, audit log |
| `GET /healthz`, `GET /metrics` | Docker healthcheck; Prometheus for the existing Grafana |

Generate an OpenAPI spec (e.g. `huma` or hand-written `openapi.yaml`) and a typed TS client for the frontend.

## Frontend UX

The app opens on "My lines", not the map, because the daily question is "when is my tram and can I make it"; the map is one tap away.

**Screens**

1. **Home / My lines**: one card per saved line, e.g. `Tram 4 → Tondi · from Hobujaama B`. Big countdown for the next departure, then two more. A coloured verdict chip: green "Leave in 3 min", amber "Leave now", red "Too tight, next in 9". Walking time shown as "6 min walk". Pull to refresh; live updates via WS. Drag to reorder.
2. **Map**: full-screen MapLibre. Vehicle markers = rounded pill with line number, coloured by mode (tram, bus, trolley, train), small heading wedge. Filter chips (modes, "My lines only"). Tap vehicle → bottom sheet: line, destination, delay, next stops with ETAs, highlight its route shape. Tap stop → departures sheet. User location as a blue dot with accuracy ring. Cluster stops below zoom 14.
3. **Search** (single bar, top of map and its own tab): typing `4` or `17A` returns lines; typing letters returns stops. Line page: direction toggle, ordered stop list with live vehicle positions inline ("• 2 stops away"), shape on a mini map, "Save this line" button. Stop page: platform tabs (A, B…) with the destinations each serves, merged departure list, lines serving the stop as chips.
4. **Add saved line flow**: pick line → direction → boarding stop (list ordered along the route, nearest highlighted) → optional alighting stop → label. Max 3 taps after search.
5. **Settings / profile**: walk speed, buffer minutes, saved places (Home, Work), theme, language, API tokens for HA, active sessions.
6. **Admin** (role-gated, separate section): see Auth and admin.

**Design rules**

- Minimal: neutral greys, one accent per mode (tram red, bus teal, trolley blue, train orange, close to Tallinn's own colours), Inter or Geist font, 8 px grid, generous whitespace, no emoji markers.
- Mobile-first with bottom tab bar (Home, Map, Search, Settings); desktop gets a left rail and a split map + list view.
- Light/dark follow system, switchable. Respect `prefers-reduced-motion` (no vehicle tweening).
- PWA: manifest, service worker caching the shell and static stop data, "Add to home screen". Geolocation needs HTTPS, which the reverse proxy provides.
- i18n with `i18next`, Estonian + English, default from browser.
- Accessibility: all actions keyboard reachable, AA contrast, countdowns announced politely via `aria-live`.

## Can I make it?

The verdict compares walking time plus a buffer with the time until departure, and walking time comes from a real pedestrian route, falling back to a straight-line estimate.

**Walking time providers** (admin picks the order in settings)

1. peatus.ee OTP `plan` query with `transportModes: [WALK]`: real footpaths, no key. Default.
2. Transitous / MOTIS walking route: fallback.
3. Optional self-hosted OSRM or Valhalla (foot profile, Estonia extract \~100 MB) for full independence.
4. Haversine × 1.3 detour factor ÷ user walk speed: last resort, labelled "approx."

Cache walk results by (origin rounded to \~25 m grid, stop id) for 10 min to avoid hammering providers as the user's GPS jitters.

**Verdict** with `slack = minutes_until_departure − walk_min − buffer_min`:

| Slack | Chip | Text |
| --- | --- | --- |
| ≥ 5 | neutral | "Leave in {slack} min" |
| 1 to 4 | green | "Leave in {slack} min" |
| 0 | amber | "Leave now" |
| < 0 | red | "Too tight, next in {n} min" (and the next catchable departure is highlighted) |

**Location**

- Browser: `watchPosition` while the app is visible; send position only in the dashboard request, never stored server-side.
- No permission or desktop: use the user's chosen saved place (Home/Work) as origin.
- HA: origin is an HA `person`/`device_tracker` or `zone.home` entity, passed by the integration (see HA section).
- Use live departure time (SIRI expected) when present, otherwise scheduled; show a small "scheduled" tag in that case.

## Auth and admin

Local accounts with sessions are the default, OIDC is an optional add-on, and the map can optionally be public while personal features always require login.

**Auth**

- First start: if no users exist, `/setup` wizard creates the admin (or `TRAM_ADMIN_USER` / `TRAM_ADMIN_PASSWORD` env seeds it).
- argon2id hashing, login rate limiting (5 attempts / 15 min per IP + user), CSRF token on state-changing requests, secure cookies when behind HTTPS (`TRAM_TRUSTED_PROXIES` to read `X-Forwarded-*` from NPM).
- Optional OIDC (generic; tested with Authentik): auto-create users on first login, map a group claim to the admin role.
- API tokens: random 32 bytes, stored hashed, prefixed `tram_` for easy secret scanning, read-only scope.

**Admin pages**

| Page | Contents |
| --- | --- |
| Users | List, create, disable, reset password, change role, revoke sessions, delete (with their saved lines) |
| Invites | Create invite link (role, expiry), list/revoke |
| General settings | Instance name, signup mode (off / invite / open), public map on/off, default locale/theme, map style URL, default map centre and zoom |
| Data sources | Feed URLs (editable, with defaults), poll intervals, walking provider order, User-Agent contact, "Refresh static data now" |
| Feed health | Per source: last success, last error, latency, vehicles count, stale flag; small sparkline of vehicle count over last hour |
| API tokens | All tokens across users with last-used, revoke |
| Audit log | Admin actions, filterable |
| About | Version, build commit, uptime, DB size, backup/export button (SQLite online backup to download) |

## Home Assistant integration

A HACS custom integration turns each saved line into sensors, and a custom Lovelace card renders them; the card's editor lists the user's saved lines so they pick from a dropdown instead of typing ids.

**Integration `custom_components/tram`**

- Config flow (UI only, no YAML): base URL (e.g. `https://tram.taali.ee`), API token, verify SSL. Validates with `GET /api/v1/me`.
- Options flow: choose which saved lines to import (multi-select from `/ha/lines`), update interval (default 30 s, min 15 s), origin entity for walking time (`zone.home`, a `person.*` or `device_tracker.*`).
- `DataUpdateCoordinator` polls `GET /ha/lines?origin=lat,lon` once per interval for all lines (one request, not one per line).
- One HA device per saved line, entities:
  - `sensor.tram_<label>_next` (timestamp device class) with attributes: line, mode, headsign, stop name, platform, minutes, delay\_s, realtime (bool), next\_departures (list of 3), walk\_min, verdict, leave\_in\_min.
  - `sensor.tram_<label>_leave_in` (minutes) for automations like "notify me 2 min before I must leave".
  - `binary_sensor.tram_<label>_catchable`.
- Diagnostics download with token redacted; translations et/en; `iot_class: cloud_polling` (local\_polling if the server is on LAN).
- Later: push via WebSocket instead of polling.

**Card `tram-card`** (Lit + TypeScript, Rollup bundle, HACS frontend repo)

- Visual editor: dropdown of the user's saved lines (read from the integration's entities), multi-select, toggles for show walk time, show verdict, number of departures (1 to 5), compact vs. full layout.
- Rendering: same look as the web app card (line pill in mode colour, headsign, big countdown, verdict chip), adapts to HA theme variables, works in sections and masonry dashboards, tap opens the app's line page.
- YAML example:

```yaml
type: custom:tram-card
entities:
  - sensor.tram_work_tram_4_next
  - sensor.tram_bus_17_next
departures: 3
show_walk: true
```

The integration and card live in the monorepo under `ha/` and are published as two GitHub releases so HACS can install each.

## Deployment

Ship one multi-arch image to GHCR and a compose file that follows the homelab conventions: `/opt/tram/docker-compose.yml`, `container_name: tram`, bind-mounted data, secrets in `.env`, behind Nginx Proxy Manager at `tram.taali.ee`. Port 8095 is free in the current service map.

```yaml
# /opt/tram/docker-compose.yml
services:
  tram:
    image: ghcr.io/<you>/tram:latest
    container_name: tram
    restart: unless-stopped
    user: "1000:1000"
    ports:
      - "8095:8080"
    env_file: .env
    environment:
      TZ: Europe/Tallinn
      TRAM_DATA_DIR: /data
      TRAM_BASE_URL: https://tram.taali.ee
      TRAM_TRUSTED_PROXIES: 172.16.0.0/12,192.168.0.0/16
    volumes:
      - /opt/tram/data:/data
    healthcheck:
      test: ["CMD", "/tram", "healthcheck"]
      interval: 30s
      timeout: 5s
      retries: 3
```

- `.env.example`: `TRAM_ADMIN_USER`, `TRAM_ADMIN_PASSWORD`, `TRAM_SESSION_SECRET`, optional `TRAM_OIDC_*`.
- Dockerfile: stage 1 `node:22-alpine` builds `web/`; stage 2 `golang:1.23` builds with `CGO_ENABLED=0` and embeds the web build; stage 3 `gcr.io/distroless/static:nonroot`. Target image < 40 MB, idle RAM < 60 MB.
- GitHub Actions: lint + test on PR; on tag build `linux/amd64,linux/arm64` with buildx + QEMU, push to GHCR, attach SBOM; separate workflows release the HA integration zip and the card bundle.
- NPM: proxy host `tram.taali.ee` → `<pi>:8095`, enable WebSockets support (needed for `/ws`), Let's Encrypt or Cloudflare origin cert.
- Backups: `/opt/tram/data/tram.db` is picked up by the existing `backup-docker-configs` job if data is included, or use the admin export.
- Observability: `/metrics` scraped by the existing Grafana/Prometheus; add a small dashboard JSON in `deploy/`.

## Milestones

Build in eight vertical slices; each ends with something runnable in Docker and its acceptance check passing before the next starts.

1. **M0 Spike and skeleton**: hit every feed in the Data sources table, save sample responses as test fixtures in `internal/feeds/testdata/`, write `docs/feeds.md` with confirmed formats. Repo skeleton, Go server serving a Vite hello page, Dockerfile, compose, CI. *Done when:* `docker compose up` serves the page on arm64 and amd64.
2. **M1 Live map**: gps + elron pollers, in-memory store, `/vehicles`, `/ws`, MapLibre map with animated markers and mode filters, stale banner. *Done when:* 300+ vehicles move smoothly on a phone, CPU on the Pi < 5 %.
3. **M2 Static data and search**: stop/route/pattern import, stop and line search, line page with ordered stops and shape, stop page with platforms and SIRI departures, train station boards from OTP. *Done when:* searching "Balti jaam" shows city platforms and Elron departures.
4. **M3 Accounts and admin**: setup wizard, login, sessions, invites, users page, settings, feed health, audit log. *Done when:* a second user can be invited and the admin can disable them.
5. **M4 My lines and verdict**: saved lines CRUD and add-line flow, `/me/dashboard`, walking providers, verdict chips, saved places, PWA. *Done when:* Home screen shows correct countdown and verdict against SIRI for 3 saved lines.
6. **M5 Vehicle matching and ETA**: pattern matching, "N stops away", vehicle sheet with next-stop ETAs, dead reckoning along shapes. *Done when:* predicted ETAs are within ±1 min of SIRI for 80 % of a sampled hour.
7. **M6 Home Assistant**: API tokens, `/ha/*`, custom integration with config/options flow, sensors, card with visual editor. *Done when:* card shows a saved line in HA and an automation triggers on `leave_in` = 2.
8. **M7 Polish and release**: i18n et/en, dark mode, accessibility pass, Prometheus metrics, docs (README, screenshots, HACS instructions), v1.0.0 tag.

## Testing, risks and open questions

**Testing**

- Go: table-driven unit tests for every feed parser against recorded fixtures (including malformed lines, empty feed, Estonian characters), ETA projection tests on a known shape, verdict boundaries.
- Integration: `httptest` fake upstreams; a `TRAM_FAKE_FEEDS=1` mode that replays fixtures so the UI can be developed offline and at night when few vehicles run.
- Frontend: Vitest + Testing Library for components, Playwright for the add-line flow, login, and map smoke test.
- HA: `pytest-homeassistant-custom-component` for config flow and coordinator.

**Risks**

| Risk | Mitigation |
| --- | --- |
| Unofficial feeds change format or disappear | Parsers isolated per source, fixtures, health page alerts, feed URLs editable in admin |
| Rate limiting / blocking by upstream | One shared poller, caching, singleflight, polite User-Agent, backoff |
| SIRI ids vs GTFS ids mismatch between sources | Map by name + coordinates within 30 m at import, store both ids, log unmatched stops for review |
| Geolocation blocked over HTTP | Require HTTPS via NPM; saved-place fallback |
| Pi resource use with many WS clients | Diff-based messages, gzip/permessage-deflate, per-client filters |
| Train positions sparse between stations | Show delay from Elron feed, interpolate along GTFS shape |

**Open questions for Andri**

- [ ] Final app name and domain (placeholder `tram.taali.ee`).
- [ ] Public map without login, or everything behind auth?
- [ ] Use Authentik OIDC from day one or add later?
- [ ] Include Harju county buses (Ridango socket) in v1 or later?
- [ ] Open-source license (MIT vs AGPL) and whether to publish on GitHub.

## Kickoff prompt for Claude Code

Export this doc as Markdown into the repo as `docs/PLAN.md`, then paste the prompt below into Claude Code in the empty repo.

```markdown
You are building "Tram", a self-hosted Tallinn public transport tracker. The full spec is in docs/PLAN.md — read it completely before writing code and treat it as the source of truth.

Working rules:
- Work milestone by milestone (M0 → M7). Start with M0 only.
- Enter plan mode first: propose the concrete file list and steps for the current milestone, wait for my OK, then implement.
- In M0, fetch every upstream feed listed in "Data sources", save real samples as fixtures, and document the confirmed formats in docs/feeds.md. If any endpoint differs from the plan, stop and tell me before building on it.
- Stack is fixed: Go (chi, coder/websocket, sqlc, modernc sqlite, goose), React + TS + Vite + Tailwind v4 + shadcn/ui + MapLibre, HA integration in Python, card in Lit. No CGO. Must build for linux/arm64 and linux/amd64.
- Keep a CLAUDE.md with build/test commands and conventions; update it as the project grows.
- Every milestone ends with: tests passing, `docker compose up` working, its "Done when" check verified, a short summary of what changed, and a git commit.
- Never commit secrets; use .env.example.
- Ask me before adding any dependency not named in the plan.
```

Suggested follow-up prompts: "Continue with M1 per docs/PLAN.md", and after each milestone "Review the milestone against PLAN.md and list gaps before we move on".
