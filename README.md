# Bus — Tallinn transit tracker

Self-hosted, multi-user app showing live Tallinn buses, trams, trolleybuses and Elron trains,
telling you whether you can make your next departure, with a Home Assistant integration.
Runs as one small container on a Raspberry Pi 5 (arm64) or amd64.

- **My lines** — save your everyday lines and see live countdowns with a *can I make it?* verdict based on real
  walking time from where you are (or a saved place).
- **Live map** — every bus, tram, trolleybus and Elron train, gliding along its route, with the next stops and ETAs.
- **Search, stops and lines** — departure boards per platform (live SIRI times, timetable fallback), route maps.
- **Harju county buses** — the ~220 county lines (SEBE, GoBus, Hansabuss…) from the national timetable: search,
  stop boards, line pages and My lines. Timetable only: county buses publish no live positions.
- **Accounts** — invites, admin area with feed health, audit log, settings, backups and API tokens.
- **Home Assistant** — sensors per saved line and a Lovelace card ([ha/README.md](ha/README.md)).
- **Installable** as a PWA; English and Estonian; light and dark; Prometheus metrics.

| My lines | Live map | Vehicle | Stop |
| --- | --- | --- | --- |
| ![My lines](docs/screenshots/my-lines.png) | ![Map](docs/screenshots/map.png) | ![Vehicle sheet](docs/screenshots/vehicle.png) | ![Stop page](docs/screenshots/stop.png) |

![Dark map on desktop](docs/screenshots/map-dark-desktop.png)

See [docs/PLAN.md](docs/PLAN.md) for the original plan, [docs/feeds.md](docs/feeds.md) for the upstream data formats and
[CHANGELOG.md](CHANGELOG.md) for what changed.

## Run

Bus ships as a multi-arch image (`linux/amd64` and `linux/arm64`), so the same tag runs on a Raspberry Pi 5 and an
x86 server. Nothing needs to be built.

```sh
mkdir bus && cd bus
curl -fsSLO https://raw.githubusercontent.com/unmaykr-a/transit-tracker/main/compose.yaml
docker compose up -d          # http://localhost:8095
```

Optional settings go in a `.env` next to it ([.env.example](.env.example)). To upgrade:

```sh
docker compose pull && docker compose up -d
```

`latest` follows releases; pin a version (`ghcr.io/unmaykr-a/bus:1.1`) to upgrade on your own schedule. The first start
after an upgrade migrates the database and re-imports the timetable data by itself.

**Homelab deployment** (bind-mounted data under `/opt/bus`, behind a reverse proxy): copy
[deploy/docker-compose.yml](deploy/docker-compose.yml) and [deploy/.env.example](deploy/.env.example) (as `.env`) to
`/opt/bus/` and follow the comments at the top of the compose file. Behind Nginx Proxy Manager, enable **WebSockets
support** for the proxy host (needed for `/ws`; the app falls back to Server-Sent Events if WebSockets are blocked)
and set `BUS_TRUSTED_PROXIES` to the proxy's network.

**Backups:** everything lives in one SQLite file, `bus.db`, in the data volume. Download a consistent copy any time
from **Administration → About → Download backup**.

## First start

Open the app and create the administrator account, or set `BUS_ADMIN_USER` and `BUS_ADMIN_PASSWORD` to create it
on first start. Invite others from **Settings → Administration → Invites**. Sign-up is invite-only by default; the
map, search and stop pages are public unless you turn **Public map** off.

## Single sign-on

Bus can sign users in with any OpenID Connect provider; it is tested with Authentik. Create an OAuth2/OpenID
provider and application in Authentik (confidential client, scopes `openid profile email` plus a `groups` claim),
set the redirect URI to `https://bus.example.com/api/v1/auth/oidc/callback`, then set `BUS_BASE_URL`,
`BUS_OIDC_ISSUER` (the application's issuer URL), `BUS_OIDC_CLIENT_ID` and `BUS_OIDC_CLIENT_SECRET`. The login page
then shows *Sign in with …* next to the password form. Accounts are created on first sign-in (turn that off with
`BUS_OIDC_AUTO_CREATE=false`); with `BUS_OIDC_ADMIN_GROUP` the admin role follows group membership on every sign-in.
Single sign-on users can add a local password under **Settings → Account**.

## Home Assistant

Create an API token under **Settings → API tokens**, install the integration and card from the release
(`bus.zip`, `bus-card.js`; or add this repository to HACS as a custom *Integration* repository) and follow
[ha/README.md](ha/README.md). Each saved line becomes a device with `sensor.bus_<label>_next`,
`sensor.bus_<label>_leave_in` and `binary_sensor.bus_<label>_catchable`.

<img src="docs/screenshots/home-assistant.png" alt="bus-card in Home Assistant" width="320">

## Monitoring

`GET /metrics` serves Prometheus metrics: vehicles by mode (and how many are matched to routes), feed polls, errors,
latency and staleness, HTTP requests and latency by route, live map clients, users, Go memory. By default only
clients on loopback or private networks may scrape it (resolved through `BUS_TRUSTED_PROXIES`, so visitors coming
through your reverse proxy are refused). Import [deploy/grafana-dashboard.json](deploy/grafana-dashboard.json) into
Grafana for a ready-made dashboard.

```yaml
# prometheus.yml
scrape_configs:
  - job_name: bus
    static_configs:
      - targets: ["pi.lan:8095"]
```

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `BUS_ADDR` | `:8080` | Listen address |
| `BUS_DATA_DIR` | `./data` | Database and caches |
| `BUS_BASE_URL` | — | Public URL, also used as the contact in the upstream User-Agent |
| `BUS_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `BUS_ADMIN_USER`, `BUS_ADMIN_PASSWORD` | — | Create the first admin on an empty database |
| `BUS_TRUSTED_PROXIES` | — | CIDRs whose `X-Forwarded-For`/`-Proto` are trusted (your reverse proxy) |
| `BUS_METRICS` | `private` | `/metrics` access: `private` (loopback/private networks), `public` or `off` |
| `BUS_METRICS_TOKEN` | — | If set, `/metrics` requires `Authorization: Bearer <token>` instead |
| `BUS_OIDC_ISSUER`, `BUS_OIDC_CLIENT_ID`, `BUS_OIDC_CLIENT_SECRET` | — | Single sign-on (see above) |
| `BUS_OIDC_ADMIN_GROUP` | — | Members of this group are admins (others are demoted) |
| `BUS_OIDC_NAME`, `BUS_OIDC_AUTO_CREATE` | `single sign-on`, `true` | Button label; create accounts on first sign-in |
| `BUS_COUNTY` | `true` | Import Harju county bus timetables too |
| `BUS_FAKE_FEEDS` | `false` | Replay recorded feeds instead of polling upstream |
| `BUS_CONTACT` | base URL or repo URL | Contact URL sent to upstream feeds |
| `BUS_GPS_URL`, `BUS_GPS_INTERVAL` | Tallinn `gps.txt`, `5s` | City vehicle feed (minimum 2 s) |
| `BUS_ELRON_URL`, `BUS_ELRON_INTERVAL` | Elron `map_data.json`, `10s` | Train feed (minimum 5 s) |
| `BUS_SIRI_URL`, `BUS_OTP_URL` | transport.tallinn.ee, api.peatus.ee | Live departures and timetables |
| `BUS_STOPS_URL`, `BUS_ELRON_GTFS_URL` | transport.tallinn.ee, eu-gtfs.remix.com | Static data (imported nightly at 03:30) |

Feed URLs, poll intervals and the User-Agent contact can also be changed at runtime in the admin settings; those
override the environment. The SQLite database lives in `BUS_DATA_DIR` (`/data` in the container). With a bind mount,
make the host directory writable by the container user.

## Develop

Requires Go 1.27+ and Node 22+ (or just Docker: `docker compose -f compose.dev.yaml up --build` builds the image from
the checkout).

```sh
make web build && ./bin/bus   # http://localhost:8080
BUS_FAKE_FEEDS=1 ./bin/bus    # replay recorded feeds (offline, or at night)
make test lint
cd web && npm run e2e          # Playwright end-to-end tests (incl. axe accessibility checks)
cd ha && python -m pytest      # Home Assistant integration
cd ha/card && npm test         # Lovelace card
```

See [CLAUDE.md](CLAUDE.md) for layout and conventions.

## Data and license

Live and timetable data come from Tallinn Transport (`transport.tallinn.ee`), Elron (`elron.ee`, Elron GTFS) and
the national journey planner (`peatus.ee`); walking routes from peatus.ee and Transitous; the map from
OpenFreeMap and © OpenStreetMap contributors. Bus is not affiliated with any of them.

Bus is released under the [MIT License](LICENSE).
