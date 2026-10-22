# Tram — notes for Claude Code

Self-hosted Tallinn transit tracker. **`docs/PLAN.md` is the spec and source of truth**; `docs/feeds.md`
records the upstream formats as actually observed (it wins over PLAN.md where they disagree).
Work milestone by milestone (M0 → M7); each ends with tests green, `docker compose up` working, its
"Done when" check verified, and a commit.

## Layout

- `cmd/tram` — entrypoint: `tram` (serve), `tram healthcheck`, `tram version`.
- `internal/api` — chi router, `/healthz`, `/api/v1/*`, SPA serving of the embedded web build (`dist/`).
- `internal/config` — `TRAM_*` env config. `internal/buildinfo` — version vars set via ldflags.
- `internal/feeds` — pure parsers for upstream feeds; fixtures in `internal/feeds/testdata/`.
- `web/` — Vite + React 19 + TS + Tailwind v4 + shadcn/ui. Builds to `web/dist`, copied into
  `internal/api/dist` (gitignored except `.gitkeep`) and embedded via `go:embed`.
- `deploy/` — production compose + `.env.example`. Root `compose.yaml` is the local dev build.

## Commands

```sh
make web          # npm ci + vite build, copy into internal/api/dist
make build        # static binary → bin/tram (CGO_ENABLED=0)
make test         # go test -race ./... and vitest
make lint         # gofmt, go vet, eslint, tsc
make run          # go run ./cmd/tram  (UI: cd web && npm run dev → proxies /api, /ws to :8080)
docker compose up --build        # local image on http://localhost:8095
make docker       # buildx linux/amd64 + linux/arm64
```

Go toolchain is pinned by `go.mod` (`go 1.27`); older local Go auto-downloads it (GOTOOLCHAIN=auto).

## Conventions

- No CGO, ever: the image is `distroless/static` and must build for amd64 + arm64.
- Parsers in `internal/feeds` take an `io.Reader`, never do I/O, skip+count malformed rows instead of failing
  the snapshot, and get table-driven tests plus a test against a real fixture.
- Join keys between sources (see docs/feeds.md): Tallinn `ID` = OTP `code` = Elron GTFS `stop_code`;
  Tallinn `SiriID` = OTP gtfs id number. Trams are `T1`..`T6` everywhere. Vehicle ids are `<mode>-<number>`.
- `time/tzdata` is linked into the binary; always use `Europe/Tallinn` explicitly for feed times.
- Polite upstream use: one shared poller per feed, descriptive User-Agent, never poll faster than the feed updates.
- Ask before adding dependencies not named in PLAN.md. npm installs use `--save-exact`; prefer releases ≥ 2 weeks old.
- Never commit secrets; config examples go in `.env.example`.
