# Changelog

## 1.8.0 — 2026-10-23

Upgrading: pull the new image; the first start adds one column (`saved_lines.transfers`). Update the Home Assistant
integration through HACS for the new attributes and card rows.

- **Trips with changes.** When no line rides directly between two stops, or a change is quicker, the trip form
  suggests ways with one change, or two when one is not enough. Changes can be at one stop or between stops up to
  400 m apart. Ways are ranked by estimated time, and you can also choose the change stop yourself. Saved trips with
  changes can be edited like any other trip.
- **Each departure follows you to the destination.** The card shows the connection you can catch at each change (its
  line, time, the walk between platforms and the wait, with "tight change" when there is no time to spare) and an
  estimated arrival. Direct trips and lines saved with a stop to get off at show the arrival too. Ride times come
  from the live tracker's learned street paces where it has them.
- **Home Assistant**: next-departure sensors get `via`, `changes` and `arrive_at`; the card shows the changes and
  the arrival under the countdown.

## 1.7.0 — 2026-10-23

Upgrading: pull the new image; the first start adds one column (`user_prefs.show_rail`).

- **Railway lines are now opt-in**: off by default, switched on under Settings → Display (saved in your account,
  or in the browser for visitors).
- **Settings are grouped**: Account, Display, Walking, Integrations and About, with a bar to jump between them.
  Admin settings are grouped too (general, visitor defaults, map, live vehicles, timetables, walking and
  addresses, upstream contact), and the walking route providers and Transitous URL can now be edited there.
- **Built for many users at once**, tested with up to 3,500 simultaneous live-map clients plus browsing and
  signed-in users (`tools/loadtest`, results in `docs/load-testing.md`):
  - Live updates are encoded once per update and shared by every client with the same view. At 1,800 clients,
    worst-case response times fell from 1.4 s to 60 ms and memory from 719 MB to 112 MB.
  - The database keeps a pool of connections instead of reopening them under load (dashboard p95 400 ms → 2 ms).
  - Sign-ups no longer hold the database while hashing the password, which made other changes fail with
    "database is locked" during bursts of sign-ups.
- **Times on clock-change days**: live departures follow the feed's own clock, so they no longer shift by an hour
  on the day the clocks change (next: 25 October).
- **Keyboard**: choice groups (theme, language, walking-from, directions, stops) have one Tab stop and work with
  the arrow keys.
- **Layout fixes**:
  - Home has a tidy header with the walking-from choices in one row.
  - Removing a line asks on its own row.
  - Line names line up in lists.
  - Two vehicles approaching the same stop show "×2" instead of two badges.
  - Feed and role names are capitalised, and long session lists collapse.
- Open live streams re-check access on their own timer (tested). CI actions are pinned to commit SHAs, and
  Dependabot is set up to propose updates.

## 1.6.0 — 2026-10-22

A review and polish pass over the whole project; no new features. Upgrading: pull the new image, and update the Home
Assistant integration through HACS. Nothing to migrate.

- **Web**:
  - The admin settings for map styles, map start view, default language and default theme now take effect (visitors
    who never picked their own follow them).
  - After a deploy, open tabs pick up the new build by themselves, and the offline copy can no longer be replaced by
    an API or `/metrics` page.
  - Signing out (or in as someone else) clears the previous user's lines and places from the page.
  - The live connection closes in background tabs and reconnects at once when you come back or go back online.
  - Links to a platform open that platform's departures. New pages open scrolled to the top.
  - My lines: loading and error states, and reordering keeps up with quick taps.
  - Removing a line or place, and revoking an API token, now ask first.
  - Escape closes map sheets, and screen readers are no longer interrupted every few seconds.
  - A message shows when the map can't load. "On time" is used for delays under 30 s.
- **Home Assistant**:
  - Change the server URL or token under Reconfigure.
  - Picking every line in options now also follows lines you add later.
  - "Leave in" is a duration and is unknown when nothing is catchable, instead of a negative number.
  - New `error` and `destination` attributes.
  - The card shows unavailable lines, and its chips are readable in dark themes.
- **Security**:
  - The login rate limit can no longer be raced by parallel requests, and IPv6 clients are limited per /64.
  - Stricter post-login redirects.
  - Only `bus_…` bearer tokens are treated as API tokens.
  - An admin password reset also revokes the user's API tokens.
  - Sessions renew their cookie as they slide.
  - Password hashing is bounded, so a burst of logins can't exhaust memory.
- **Data and ETAs**:
  - Vehicles parked for minutes no longer teach the model that a street is slow.
  - An empty or collapsed GPS snapshot is refused instead of clearing the map.
  - If Elron or county data fails at the nightly import, the previous data is kept.
  - Cached departures are refreshed in the background (stale-while-revalidate), and upstream errors are cached briefly.
  - A city route listed twice upstream no longer fails the import.
- **Server**:
  - Graceful shutdown closes live streams promptly.
  - Live streams end when the session that opened them does.
  - Admin static-data refresh runs once even when clicked repeatedly.
  - Limits on saved places (20) and label length.
  - `/stops` and `/patterns` responses are encoded once per import.
  - Walks over 5 km use the straight-line estimate instead of routing.
  - `BUS_CONTACT` may be an email address.
- **Releases**: prereleases no longer move the `latest` image tag.

## 1.5.0 — 2026-10-22

- **Big screens**: line and stop pages show the live map (animated vehicles, tap for details, scroll to zoom) beside
  the details; My lines uses two columns.
- **Saved places on the map**, and **address search** when adding one (Estonian addresses from the Land Board's In-ADS
  gazetteer, through the server; admin setting `geocoder_url`).
- **Railway lines** stay visible when the map is zoomed out (follows the Train filter).
- **Security**: Content Security Policy and other hardening headers (HSTS behind HTTPS); the Go toolchain is pinned to
  1.27.3, which fixes 11 standard-library vulnerabilities `govulncheck` reported for 1.27.0.
- Fixed: the selected route's outline was missing (an invalid style expression, now caught by an e2e check).

## 1.4.0 — 2026-10-22

Upgrading: pull the new image; the first start adds one column (`saved_lines.origin_place_id`). In Home Assistant,
update the integration through HACS and restart; if you added `bus-card.js` as a dashboard resource by hand, remove it.

- **Map**: a selected vehicle's route is drawn in its line colour with an outline that stands out on light and dark
  maps, with its stops marked at every zoom; vehicles of other lines fade while it is selected. On phones the vehicle
  and stop sheets open compact (name and next stop) so the map stays visible, and expand on tap.
- **Trips are editable** (✎ in My lines → Edit): change the stops, label and a new **usual starting point** — a saved
  place walking time starts from whenever you are not sharing your location (also used by Home Assistant).
- **Saved places can be picked on a map** (Settings → Walking → Pick on map).
- **Home Assistant**:
  - The integration has its own icon.
  - The card is bundled and loads by itself: no download, no resource. With no lines picked it shows all of your
    saved lines, and it now looks like the web app (countdown with clock time, verdict chip, "then …", trips with
    their lines).
  - Walking time works out of the box: new setups walk from `zone.home`, and a person without GPS coordinates walks
    from the zone they are in. New attributes: `kind`, `lines`, `destination`, `walk_from`, `order`.
  - Ready-made template for Android/iOS home-screen widgets in `ha/README.md`.
- Fixed: concurrent sign-ups and other writes could fail with "database is locked" (transactions now begin
  `IMMEDIATE`).

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
