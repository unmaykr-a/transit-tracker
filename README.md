# Tram — Tallinn transit tracker

Self-hosted, multi-user app showing live Tallinn buses, trams, trolleybuses and Elron trains,
telling you whether you can make your next departure, with a Home Assistant integration.
Runs as one small container on a Raspberry Pi 5 (arm64) or amd64.

> Status: **M0 — skeleton**. See [docs/PLAN.md](docs/PLAN.md) for the roadmap and
> [docs/feeds.md](docs/feeds.md) for the upstream data formats.

## Run

```sh
docker compose up --build    # http://localhost:8095
```

Production: copy `deploy/docker-compose.yml` and `deploy/.env.example` (as `.env`) to `/opt/tram/`.

## Develop

Requires Go 1.27+ and Node 22+.

```sh
make web build && ./bin/tram   # http://localhost:8080
make test lint
```

See [CLAUDE.md](CLAUDE.md) for layout and conventions.
