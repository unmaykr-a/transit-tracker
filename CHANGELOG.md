# Changelog

## Unreleased (1.0.0 candidate)

### M7 — Polish
- Prometheus `/metrics` (vehicles, feed health, HTTP by route, live clients, users, memory), restricted to private
  networks by default (`TRAM_METRICS`, `TRAM_METRICS_TOKEN`); Grafana dashboard in `deploy/`.
- Accessibility: axe WCAG 2.1 AA checks on every page in light and dark mode, keyboard-only e2e flow; bus and train
  colours darkened to meet 4.5:1 contrast with white text; readable map attribution and delay text in dark mode.
- Locale consistency test (en/et keys, interpolation variables).
- README with screenshots, monitoring and Home Assistant sections.

## M6 — Home Assistant (#7)
- Personal API tokens (read-only, never admin), created in Settings and listed/revoked by admins.
- `GET /api/v1/ha/lines` — every saved line with the next catchable departure, walk time and verdict in one request.
- `custom_components/tram`: config flow, reauth, options (lines, interval, walking origin), next / leave-in /
  catchable entities per line, diagnostics.
- `tram-card` Lovelace card with a visual editor.

## M5 — Vehicle matching and ETAs (#6)
- Vehicles matched to route patterns; next stops with ETAs in the vehicle sheet; markers glide along the route shape.
- `tram eta-check` with record/replay. Accuracy within ±1 min of SIRI: 92 % for arrivals up to 5 min away, 59 %
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
