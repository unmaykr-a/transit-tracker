# Bus — Tallinn transit tracker

Self-hosted, multi-user app showing live Tallinn buses, trams, trolleybuses and Elron trains,
telling you whether you can make your next departure, with a Home Assistant integration.
Runs as one small container on a Raspberry Pi 5 (arm64) or amd64.

- **My lines** — save your everyday lines and see live countdowns with a *can I make it?* verdict based on real
  walking time from where you are (or a saved place).
- **Live map** — every bus, tram, trolleybus and Elron train, gliding along its route, with the next stops and ETAs.
- **Search, stops and lines** — departure boards per platform (live SIRI times, timetable fallback), route maps.
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

```sh
docker compose up --build    # http://localhost:8095
```

Production: copy `deploy/docker-compose.yml` and `deploy/.env.example` (as `.env`) to `/opt/bus/`.
Images are multi-arch (`linux/amd64` and `linux/arm64`), so the same tag runs on a Raspberry Pi 5 and an x86 server.
Behind Nginx Proxy Manager, enable **WebSockets support** for the proxy host (needed for `/ws`; the app falls
back to Server-Sent Events if WebSockets are blocked).

## First start

Open the app and create the administrator account, or set `BUS_ADMIN_USER` and `BUS_ADMIN_PASSWORD` to create it
on first start. Invite others from **Settings → Administration → Invites**. Sign-up is invite-only by default; the
map, search and stop pages are public unless you turn **Public map** off.

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
| `BUS_FAKE_FEEDS` | `false` | Replay recorded feeds instead of polling upstream |
| `BUS_CONTACT` | base URL or repo URL | Contact URL sent to upstream feeds |
| `BUS_GPS_URL`, `BUS_GPS_INTERVAL` | Tallinn `gps.txt`, `5s` | City vehicle feed (minimum 2 s) |
| `BUS_ELRON_URL`, `BUS_ELRON_INTERVAL` | Elron `map_data.json`, `10s` | Train feed (minimum 5 s) |
| `BUS_SIRI_URL`, `BUS_OTP_URL` | transport.tallinn.ee, api.peatus.ee | Live departures and timetables |
| `BUS_STOPS_URL`, `BUS_ELRON_GTFS_URL` | transport.tallinn.ee, eu-gtfs.remix.com | Static data (imported nightly at 03:30) |

Feed URLs, poll intervals and the User-Agent contact can also be changed at runtime in the admin settings; those
override the environment. The SQLite database lives in `BUS_DATA_DIR` (`/data` in the container). With a bind mount, make the host
directory writable by the container user.

## Develop

Requires Go 1.27+ and Node 22+.

```sh
make web build && ./bin/bus   # http://localhost:8080
BUS_FAKE_FEEDS=1 ./bin/bus   # replay recorded feeds (offline, or at night)
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
