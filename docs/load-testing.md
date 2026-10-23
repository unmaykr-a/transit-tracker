# Load testing

`tools/loadtest` puts a running server under the load of many simultaneous users and prints latency
percentiles per endpoint, errors, how late live updates arrive, and the server's heap, goroutines and CPU.
Always run it against fake feeds (`BUS_FAKE_FEEDS=1`): departures and walks would otherwise reach the
public upstream services.

```sh
BUS_FAKE_FEEDS=1 BUS_ADMIN_USER=admin BUS_ADMIN_PASSWORD=load-test-admin ./bin/bus &
go run ./tools/loadtest -url http://127.0.0.1:8080 -ws 1500 -sse 300 -browsers 200 \
  -users 500 -admin admin:load-test-admin -d 100s -pid $(pgrep -x bus)
```

- `-ws`, `-sse`: live-map clients (every fifth WebSocket client filters to trams, like the map's mode chips).
- `-browsers`: people opening stops, departures, lines, search, `/stops` and `/patterns` every 0.5–2 s.
- `-users`: accounts signed up through admin invites, each with three saved lines, a place and an API token;
  they load their dashboard every 15–25 s (the Home page) and the Home Assistant feed every 30 s.

## Results (4-core sandbox, load generator on the same machine), 1.7.0

| Load | p50 / p99 (API) | Live updates (p50 / max) | Server |
|---|---|---|---|
| 500 live, 50 browsing | 0.8 ms / 15 ms | 13 ms / 130 ms | 25 % of a core, 64 MB heap |
| 1,800 live, 200 browsing, 500 signed-in | 0.6–1.3 ms / 37–95 ms | 49 ms / 525 ms | 71 % of a core, 89 MB heap after |
| 3,500 live, 300 browsing | 0.6 ms / 180–320 ms | 111 ms / 990 ms | 43 % of a core, 228 MB peak |

No connection or request failed in any run; goroutines return to idle afterwards. A browser on the map during
the 1,800-client run saw the map ready in 2.5 s, markers moving on every sample and vehicle sheets opening.

What the first runs found, and what changed:

- **Live fan-out**: every client copied the vehicle store, diffed and JSON-encoded on each update (1,800
  clients: p99 1.4 s, 719 MB heap). Now one frame per update is shared, with one encoded diff per distinct
  filter (`internal/api/livehub.go`): p99 60 ms, 112 MB.
- **SQLite pool**: `database/sql` kept two idle connections, so busy moments reopened connections (pragmas and
  schema read under a lock): dashboard p95 400 ms. A fixed pool of 16 removed it.
- **Sign-up held the write lock while hashing the password**: concurrent sign-ups made other writes fail with
  `SQLITE_BUSY`. Passwords are now hashed before the transaction.
