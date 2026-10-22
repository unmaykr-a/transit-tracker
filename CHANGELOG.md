# Changelog

## Unreleased

- Map: a selected vehicle's route is drawn in its line colour with an outline that stands out on both light and dark
  maps, with its stops marked at every zoom; vehicles of other lines fade while it is selected.

## 1.3.0 — 2026-10-22

- Single sign-on: link an existing account, the admin included, to your provider account under **Settings → Single
  sign-on**, and unlink it there. Linking runs the normal sign-in at the provider and completes only in the session
  that started it; an identity already used by another account is refused, and an admin whose provider account is
  outside `BUS_OIDC_ADMIN_GROUP` is warned. Both actions are in the audit log.
- The e2e tests run against a fake OpenID provider.

## 1.2.0 — 2026-10-22

- **Trips between two stops**: in My lines, *Add a trip* (or *Trip from here* on a stop page) picks a start and an
  end stop and shows every line that rides directly between them. The saved trip's card counts down to the next
  departure of any of them, with walking time and the verdict per boarding platform. Home Assistant sensors work for
  trips too (`kind`, `lines`, and each departure's `line`/`mode`); the card shows the line of each departure.
  Upgrading: just pull the new image; the first start adds three columns to `saved_lines`.
- HACS: the integration moved to `custom_components/bus` at the repository root, so adding
  `https://github.com/unmaykr-a/transit-tracker` as a custom repository (type *Integration*) works. CI now runs HACS's
  own validation and `hassfest`.
- Single sign-on: an issuer URL entered without the provider's trailing slash (or with an extra one) no longer fails
  discovery with "issuer URL … did not match"; Authentik's issuer ends in `/`.

## 1.1.0 — 2026-10-22

Upgrading from 1.0.0: just pull the new image. The first start adds one table (`user_identities`) and re-imports the
static data with county routes (a few seconds); nothing else to do.

- `compose.yaml` now runs the published image (`docker compose up -d`, upgrade with `docker compose pull`); building
  from source moved to `compose.dev.yaml`. README rewritten around running the image, upgrading and backups.

- Vehicle ETAs calibrate themselves: the tracker checks its own predictions against the arrivals it later sees and
  learns a correction per mode and horizon (`bus_eta_scale` metric). On recorded peaks this lifted agreement with
  SIRI from ~59 % to 64–66 % within ±1 min, and predictions now match observed arrivals as well as SIRI does.
- Harju county buses (~220 lines from SEBE, GoBus, Hansabuss and others) from the national timetable: searchable,
  on stop boards (merged with live city departures at shared platforms), line pages and My lines; timetable only.
  `BUS_COUNTY=false` turns them off.
- Single sign-on with any OpenID Connect provider (tested with Authentik): `BUS_OIDC_*`. Code flow with PKCE,
  accounts created on first sign-in (optional), admin role from a group claim, "Sign in with …" on the login page;
  SSO users can add a local password.

## 1.0.0 — 2026-10-22

First release, published as **Bus** (developed as "Tram"). MIT licensed. Multi-arch image
`ghcr.io/unmaykr-a/bus` (linux/amd64, linux/arm64); the Home Assistant integration (`bus.zip`, domain `bus`) and
`bus-card.js` are attached to the GitHub release.

- Renamed everything user-facing: environment variables are `BUS_*`, the binary is `/bus`, the database
  `bus.db`, API tokens start with `bus_`, metrics with `bus_`, Home Assistant entities with `sensor.bus_…`.

### M7 — Polish
- Prometheus `/metrics` (vehicles, feed health, HTTP by route, live clients, users, memory), restricted to private
  networks by default (`BUS_METRICS`, `BUS_METRICS_TOKEN`); Grafana dashboard in `deploy/`.
- Accessibility: axe WCAG 2.1 AA checks on every page in light and dark mode, keyboard-only e2e flow; bus and train
  colours darkened to meet 4.5:1 contrast with white text; readable map attribution and delay text in dark mode.
- Locale consistency test (en/et keys, interpolation variables).
- README with screenshots, monitoring and Home Assistant sections.

## M6 — Home Assistant (#7)
- Personal API tokens (read-only, never admin), created in Settings and listed/revoked by admins.
- `GET /api/v1/ha/lines` — every saved line with the next catchable departure, walk time and verdict in one request.
- `custom_components/bus`: config flow, reauth, options (lines, interval, walking origin), next / leave-in /
  catchable entities per line, diagnostics.
- `bus-card` Lovelace card with a visual editor.

## M5 — Vehicle matching and ETAs (#6)
- Vehicles matched to route patterns; next stops with ETAs in the vehicle sheet; markers glide along the route shape.
- `bus eta-check` with record/replay. Accuracy within ±1 min of SIRI: 92 % for arrivals up to 5 min away, 59 %
  over 2–20 min (target 80 % not met; see `docs/feeds.md`).

## M4 — My lines (#5)
- Saved lines with live countdowns, walking time (OTP → Transitous → straight-line estimate) and the
  can-I-make-it verdict; saved places; walking preferences; installable PWA.

## M3 — Accounts and admin (#4)
- Setup wizard, login, invites and open sign-up modes, sessions, rate limiting, CSRF protection.
- Admin: users, invites, settings (feed URLs and intervals apply live), feed health with history, audit log,
  backups.

## M2 — Static data (#3)
- Nightly import of stops, routes and patterns (OTP, Tallinn stops.txt, Elron GTFS) with platform letters.
- Search, stop pages with live departure boards, line pages with route maps.

## M1 — Live map (#2)
- Live GPS (city) and Elron trains on a MapLibre map with animated markers, mode and line filters, WebSocket with
  SSE fallback, stale-feed banner.

## M0 — Skeleton (#1)
- Go server with the embedded React app, multi-arch Docker image (amd64 + arm64), CI.
