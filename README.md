# Tram — Tallinn transit tracker

Self-hosted, multi-user app showing live Tallinn buses, trams, trolleybuses and Elron trains,
telling you whether you can make your next departure, with a Home Assistant integration.
Runs as one small container on a Raspberry Pi 5 (arm64) or amd64.

> Status: **M2 — search, stops and lines**: live map of every bus, tram, trolleybus and Elron train;
> search by line or stop; stop pages with live departures per platform; line pages with live positions. See [docs/PLAN.md](docs/PLAN.md) for the roadmap and
> [docs/feeds.md](docs/feeds.md) for the upstream data formats.

## Run

```sh
docker compose up --build    # http://localhost:8095
```

Production: copy `deploy/docker-compose.yml` and `deploy/.env.example` (as `.env`) to `/opt/tram/`.
Images are multi-arch (`linux/amd64` and `linux/arm64`), so the same tag runs on a Raspberry Pi 5 and an x86 server.
Behind Nginx Proxy Manager, enable **WebSockets support** for the proxy host (needed for `/ws`; the app falls
back to Server-Sent Events if WebSockets are blocked).

## Configuration

| Variable | Default | Purpose |
| --- | --- | --- |
| `TRAM_ADDR` | `:8080` | Listen address |
| `TRAM_DATA_DIR` | `./data` | Database and caches |
| `TRAM_BASE_URL` | — | Public URL, also used as the contact in the upstream User-Agent |
| `TRAM_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `TRAM_FAKE_FEEDS` | `false` | Replay recorded feeds instead of polling upstream |
| `TRAM_CONTACT` | base URL or repo URL | Contact URL sent to upstream feeds |
| `TRAM_GPS_URL`, `TRAM_GPS_INTERVAL` | Tallinn `gps.txt`, `5s` | City vehicle feed (minimum 2 s) |
| `TRAM_ELRON_URL`, `TRAM_ELRON_INTERVAL` | Elron `map_data.json`, `10s` | Train feed (minimum 5 s) |
| `TRAM_SIRI_URL`, `TRAM_OTP_URL` | transport.tallinn.ee, api.peatus.ee | Live departures and timetables |
| `TRAM_STOPS_URL`, `TRAM_ELRON_GTFS_URL` | transport.tallinn.ee, eu-gtfs.remix.com | Static data (imported nightly at 03:30) |

The SQLite database lives in `TRAM_DATA_DIR` (`/data` in the container). With a bind mount, make the host
directory writable by the container user.

## Develop

Requires Go 1.27+ and Node 22+.

```sh
make web build && ./bin/tram   # http://localhost:8080
TRAM_FAKE_FEEDS=1 ./bin/tram   # replay recorded feeds (offline, or at night)
make test lint
cd web && npm run e2e          # Playwright end-to-end tests
```

See [CLAUDE.md](CLAUDE.md) for layout and conventions.
