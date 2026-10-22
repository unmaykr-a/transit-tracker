# Bus — notes for Claude Code

Self-hosted Tallinn transit tracker. **`docs/PLAN.md` is the spec and source of truth**; `docs/feeds.md`
records the upstream formats as actually observed (it wins over PLAN.md where they disagree).
Work milestone by milestone (M0 → M7); each ends with tests green, `docker compose up` working, its
"Done when" check verified, and a commit.

## Layout

- `cmd/bus` — entrypoint: `bus` (serve), `bus healthcheck`, `bus version`, `bus eta-check` (ETA accuracy vs
  SIRI; `-record file` saves the GPS/SIRI input, `-replay file` re-evaluates it offline — use replay to compare
  model changes on identical data).
- `internal/api` — chi router, `/healthz`, `/api/v1/*`, SPA serving of the embedded web build (`dist/`).
- `internal/config` — `BUS_*` env config. `internal/buildinfo` — version vars set via ldflags.
- `internal/feeds` — pure parsers for upstream feeds, the polite conditional-GET `Fetcher`, and
  `FakeTransport` (BUS_FAKE_FEEDS=1 replays fixtures with moving vehicles). Fixtures in `testdata/`.
- `internal/ingest` — one polling loop per feed with backoff + jitter; `Health` tracks last success/error,
  latency, count, and marks a feed stale after 60 s without fresh data. GPS/Elron pollers live here.
- `internal/state` — in-memory live vehicle store (last 5 fixes per vehicle → speed/bearing), versioned,
  with coalescing change notifications. `internal/geo` — haversine/bearing.
- `internal/store` — SQLite (modernc, pure Go) with goose migrations (`store/migrations`) and sqlc queries
  (`store/queries` → generated `store/db`, do not edit). Regenerate with `sqlc generate` (sqlc v1.31+).
- `internal/staticdata` — nightly import (03:30 Tallinn, on start if missing/older than 20 h or built by another
  `BuildVersion`): OTP city routes + Tallinn stops.txt (names, SIRI ids) + Elron GTFS (rail) + Harju county bus
  routes (`BUS_COUNTY`, every OTP agency `estonia:harjumaa*`; route source `county`, `bus:<short>` ids, county-only
  stops have no SIRI id; timetable only, not given to the ETA tracker). Builds stops,
  stop areas (platform letters A, B…), routes (`<mode>:<short>`), patterns; saved in one transaction and served
  from an immutable in-memory `Index` (diacritic-insensitive search via `Fold`). Bump `BuildVersion` whenever
  `Build` output changes for the same input.
- `internal/departures` — SIRI (15 s cache) for city platforms, OTP stoptimes (30 s cache) for rail, county-only and
  city platforms without live rows; city platforms county buses also serve get SIRI rows plus the county rows from OTP; `internal/cache.TTL` coalesces concurrent requests and serves stale on error.
- `internal/auth` — argon2id passwords, cookie sessions (`bus_session`, HttpOnly, SameSite=Lax, Secure behind
  HTTPS; 30-day sliding expiry; only the token's sha256 is stored), invites, login rate limit (5 per 15 min per IP
  and per username), last-admin guard. `internal/settings` — admin-editable settings (one JSON row per key over env
  defaults; `null` resets). Pollers, fetcher User-Agent and upstream URLs read settings through getter funcs
  (`func() string` / `feeds.Const`) so edits apply without a restart.
- API middleware order: `withSession` (client IP/scheme from `BUS_TRUSTED_PROXIES` only) → `csrfGuard` (unsafe
  methods: same-origin `Origin`, and `X-CSRF-Token` = session CSRF when logged in) → `publicGate` (map/search/stops
  need login when `public_map` is off) / `requireUser` / `requireAdmin`. Audit every admin action via `s.audit`.
- Account API: `GET /session` (bootstrap: public settings, setup_required, user, csrf), `/auth/{setup,login,logout,
  signup}`, `/auth/invites/{token}`, `/me`, `/me/password`, `/me/sessions`. Admin API under `/admin/*`: users,
  invites, settings, health (with per-minute history), static/refresh, audit, about, backup (`VACUUM INTO`).
- My lines: `saved_lines`, `saved_places`, `user_prefs` tables. `internal/walk` (OTP plan → Transitous → haversine
  ×1.3, order from settings, 10-min cache per ~25 m origin cell + stop; distance → minutes at the user's speed).
  `internal/dashboard` builds cards: departures of the line from its boarding platform (whole board, then filtered by
  route, saved headsign preferred) + walk + `Judge` (slack = min until departure − walk − buffer; ≥5 neutral, 1–4
  green, 0 amber, <0 red). The web mirrors `Judge` in `src/lib/verdict.ts` — keep them identical. Saved boarding
  stops are SIRI-pre-warmed every 20 s. API: `/me/lines` (+ `/reorder`), `/me/places`, `/me/prefs`,
  `/me/dashboard?lat=&lon=|place=` (location is never stored).
  Saved trips (`saved_lines.kind = 'trip'`, `from_area_id` → `to_area_id`): `Index.Connections` finds every
  (route, boarding platform) whose pattern visits a platform of From before one of To; the card merges departures of
  all those platforms (filtered to those routes/headsigns), each judged with its own platform's walk (`walks`).
  `GET /trips?from=&to=` previews the lines; the web page is `/trips/new?from=`.
- PWA: `web/public/manifest.webmanifest`, `sw.js` (cache-first hashed assets, network-first shell, SWR for
  `/api/v1/stops`), icons rendered from `icon.svg`. Registered only in production builds.
- Static API: `/api/v1/search?q=`, `/stops/search`, `/lines/search`, `/stops` (all platforms, ETag),
  `/stops/{areaOrPlatformId}`, `/stops/{id}/departures?platform=&limit=`, `/lines/{mode}/{line}`.
- Live API: `GET /api/v1/vehicles?mode=&line=`, `GET /api/v1/vehicles/{id}`, `GET /api/v1/status`,
  `WS /ws` (messages `snapshot`, `diff` with `upsert`/`move`/`remove`, `stale`; client sends
  `{"filter":{"modes":[],"lines":[]}}`), SSE fallback `GET /api/v1/vehicles/stream`. Diffs are computed per
  client against what it last received, so slow clients self-heal. `move` tuples are
  `[id, lat, lon, bearing, speed_mps, updated_at_ms, dist_along_m]`.
- `internal/eta` — vehicle → pattern matching and ETAs. `Tracker.Annotate` runs inside the GPS poller before the
  store: projects each fix onto the line's pattern shapes (equirectangular, `Shape.Project`), scores offset + heading
  disagreement (soft, weaker when stationary) − headsign/stickiness bonuses, sets `pattern_id`, `next_stop_id`,
  `dist_along_m`. Progress feeds travel paces per stop pair (shared by all lines on that street, decaying) and mean
  dwell per stop; terminus layover is ignored. ETAs sum the paces ahead plus dwell at intermediate stops (route speed
  until a segment has data), scaled by a self-calibrated factor per mode and horizon band learned from the tracker's
  own probes against later GPS arrivals (`calibrate`, half strength; `bus_eta_scale` metric). API: `/vehicles/{id}/trip` (stops ahead with ETAs),
  `/patterns/{id}` and `/patterns?route=` (encoded polylines, ETag). The map tweens markers along the shape
  (`lib/shapes.ts`, `lib/tween.ts`) using `dist_along_m` (7th `move` tuple field).
- `web/` — Vite + React 19 + TS + Tailwind v4 + shadcn/ui, TanStack Router (code-based routes in
  `src/router.tsx`) + Query, i18next (`src/locales/{en,et}.json` — every UI string goes in both), MapLibre.
  `src/lib/liveClient.ts` is the single shared WS/SSE connection; `src/map/VehicleMap.tsx` animates markers
  outside React (tweens in `src/lib/tween.ts`). Builds to `web/dist`, copied into `internal/api/dist`
  (gitignored except `.gitkeep`) and embedded via `go:embed`.
- Single sign-on (`internal/auth/oidc.go`, `internal/api/oidc.go`): OpenID Connect code flow with PKCE via go-oidc;
  state/nonce/verifier/next travel in an HMAC-signed 10-minute cookie (`bus_oidc`, per-process key). Identities are
  `user_identities` (issuer + subject → user); users are auto-created (`CreateUserNoPassword`, empty password hash =
  no local password), the admin role follows `BUS_OIDC_ADMIN_GROUP` (last admin never demoted). Errors redirect to
  `/login?error=sso_*`. Tests use an in-process fake IdP (`oidc_test.go`); e2e runs `web/e2e/fake-idp.mjs` (approves as
  the sub set with `GET /as?sub=`). Linking: `POST /me/identities/oidc` (session + CSRF) returns the provider URL and
  puts the user id in the flow (`Flow.Link`); the callback links only if the same user's session finishes it, then
  redirects to `/settings?sso=linked` (`&warn=admin_group`, or `?error=sso_link_taken|sso_link_session`).
  `GET /me/identities`, `POST /me/identities/unlink` (refused for the only sign-in of a password-less account).
- API tokens (`api_tokens`, `internal/auth/tokens.go`): `Authorization: Bearer bus_…`, only the sha256 stored.
  Token requests are **read-only** (unsafe methods → 403), never admin, and cannot see/manage tokens or sessions
  (`requireSession`). `/me/tokens` (session only), `/admin/tokens`. `GET /api/v1/ha/lines?origin=lat,lon&departures=1..5`
  is the Home Assistant feed: every saved line flattened, `next` = first departure still catchable from the origin.
- Home Assistant: `custom_components/bus` lives at the repository root (HACS requires it there; CI runs `hacs/action`
  and `hassfest`), its tests and the card under `ha/`. The integration (config flow URL+token validated via `/me`, reauth on 401/403,
  options: lines/interval/origin entity, one coordinator request per interval, one device per line with
  `sensor.bus_<label>_next|_leave_in` and `binary_sensor.bus_<label>_catchable`, diagnostics redact the token) with
  pytest (`pytest-homeassistant-custom-component`, run from `ha/`, `pythonpath = [".."]`); `ha/card/` — `bus-card` (Lit + TS + Rollup, editor
  built on `<ha-form>`; `src/model.ts` mirrors the verdict table — keep it identical to `dashboard.Judge`).
  Releases attach `bus.zip` and `bus-card.js` (`hacs.json` uses `zip_release`).
- Metrics: `internal/metrics` (tiny Prometheus text-format registry, no client library); `internal/api/metrics.go`
  registers families — counters/histograms on the request path, everything else collected at scrape time. Request
  metrics are labelled by chi route pattern, never raw paths. `/metrics` access: `BUS_METRICS` (private | public |
  off) or `BUS_METRICS_TOKEN`.
- Accessibility: `web/e2e/a11y.spec.ts` runs axe (WCAG 2.1 AA) on every page in light and dark plus a keyboard-only
  flow; new pages go into its lists. Text on mode colours must keep ≥ 4.5:1 contrast with white.
- `deploy/` — homelab compose (bind mount, reverse proxy) + `.env.example` + `grafana-dashboard.json`. Root
  `compose.yaml` runs the published GHCR image (the normal way to run Bus); `compose.dev.yaml` builds from the checkout.
  Releases: the Release workflow (`workflow_dispatch` with a version, or a `v*` tag) publishes the image
  (`latest`, `X.Y`, `X.Y.Z`), `bus.zip` and `bus-card.js`.

## Commands

```sh
make web          # npm ci + vite build, copy into internal/api/dist
make build        # static binary → bin/bus (CGO_ENABLED=0)
make test         # go test -race ./... and vitest
make lint         # gofmt, go vet, eslint, tsc
make run          # go run ./cmd/bus  (UI: cd web && npm run dev → proxies /api, /ws to :8080)
BUS_FAKE_FEEDS=1 make run       # offline / night-time development with replayed fixtures
cd web && npm run e2e            # Playwright vs ../bin/bus with fake feeds, a fresh temp DB and a seeded
                                 # admin (see playwright.config.ts). Run `make web build` first; set
                                 # PW_CHROMIUM_PATH to use a preinstalled Chromium
docker compose up -d             # published image on http://localhost:8095
docker compose -f compose.dev.yaml up --build   # image built from this checkout
make docker       # buildx linux/amd64 + linux/arm64
sqlc generate     # after editing internal/store/queries or migrations
cd ha && python -m pytest        # HA integration (venv with requirements_test.txt; ruff check/format too)
cd ha/card && npm test && npm run build   # Lovelace card → ha/card/dist/bus-card.js
```

Go toolchain is pinned by `go.mod` (`go 1.27`); older local Go auto-downloads it (GOTOOLCHAIN=auto).

## Conventions

- No CGO, ever: the image is `distroless/static` and must build for amd64 + arm64.
- Parsers in `internal/feeds` take an `io.Reader`, never do I/O, skip+count malformed rows instead of failing
  the snapshot, and get table-driven tests plus a test against a real fixture.
- Join keys between sources (see docs/feeds.md): Tallinn `ID` = OTP `code` = Elron GTFS `stop_code`;
  Tallinn `SiriID` = OTP gtfs id number. Trams are `T1`..`T6` everywhere. Vehicle ids are `<mode>-<number>`.
- Frontend routes with params: `/stops/$id` (`?p=` platform), `/lines/$mode/$line`, `/map?stop=&lines=`,
  `/search?q=`. Search params are parsed as JSON by the router: numeric-looking values arrive as numbers.
- Times are always shown 24-hour in Europe/Tallinn (`lib/format.ts`).
- Frontend: no `Date.now()`/random in render (React compiler lint); use `useNow()`. Live data in React goes
  through throttled `useLiveState()`; per-frame work stays out of React. `globalThis.__tramMap` exposes the
  MapLibre instance for e2e tests.
- `time/tzdata` is linked into the binary; always use `Europe/Tallinn` explicitly for feed times.
- Polite upstream use: one shared poller per feed, descriptive User-Agent, never poll faster than the feed updates.
- Ask before adding dependencies not named in PLAN.md. npm installs use `--save-exact`; prefer releases ≥ 2 weeks old.
- Never commit secrets; config examples go in `.env.example`.
