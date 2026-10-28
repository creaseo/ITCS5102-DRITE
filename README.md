# DRITE telemetry backend

DRITE is a Go backend for receiving infrastructure telemetry, computing rolling
averages, and streaming metrics and CPU alerts to dashboard subscribers. This
stage contains backend code and command-line demonstrations. A UI can be added
later without changing the API routes below.

## Run

Requires Go 1.22 or later.

```bash
go mod download
export PORT=8080
export DRITE_AUTH_TOKEN=drite-demo-token
export DRITE_CPU_THRESHOLD=90
go run ./cmd/drite
```

In a second terminal, generate synthetic metrics:

```bash
go run ./cmd/agent -id node-01 -cpu 95
```

If you changed the authentication token, export that same value in the agent's
terminal. The example agent generates data; it does not measure actual computer resource.

## Environment variables

| Variable | Default | Meaning |
| --- | --- | --- |
| `PORT` | `8080` | HTTP server port, 1–65535 |
| `DRITE_AUTH_TOKEN` | `drite-demo-token` | Shared bearer credential |
| `DRITE_CPU_THRESHOLD` | `90` | Initial CPU alert threshold, greater than 0 and at most 100 |

`.env.example` documents these values. `.env` files are not loaded automatically.
Use a private token and HTTPS/WSS when running beyond a local demonstration.

## REST and WebSocket surface

The original paths are preserved:

| Method | Path | Purpose |
| --- | --- | --- |
| GET, POST | `/api/settings` | Read or update the CPU alert threshold |
| GET | `/api/metrics/averages` | Read one-minute and five-minute averages |
| GET (WebSocket) | `/ws/agent?agent_id=node-01` | Submit metrics for one agent |
| GET (WebSocket) | `/ws/dashboard` | Subscribe to live metric and alert events |

Two additive routes implement the fuller ideation document:

| Method | Path | Purpose |
| --- | --- | --- |
| POST | `/api/v1/auth` | Exchange a bearer credential for a one-use WebSocket ticket |
| GET | `/api/v1/snapshot` | Retrieve averages, recent metrics, per-agent summaries, and incidents |

REST requests require `Authorization: Bearer <DRITE_AUTH_TOKEN>`. WebSockets accept
that header or a short-lived `ticket` query parameter obtained from `/api/v1/auth`.
A ticket is bound to its role and, for agents, its agent ID.

## Implementation by use case

1. `uc1_authentication.go`: bearer authentication, one-use tickets, safe upgrade.
2. `uc2_ingestion.go`: concurrent readers, JSON parsing and metric validation.
3. `uc3_broadcast.go`: channel-based fan-out and slow-consumer eviction.
4. `uc4_aggregation.go`: bounded history and thread-safe rolling averages.
5. `uc5_heartbeat.go`: ping/pong deadlines and connection cleanup.
6. `uc6_alerts.go`: CPU rule evaluation and live alert dispatch.
7. `uc7_rest_api.go`: settings, averages and diagnostic snapshot APIs.

## Verify

```bash
go test -race ./...
go vet ./...
go test -run '^$' -bench . -benchmem
go run ./cmd/loadtest -agents 10 -dashboards 2 -duration 10s
```

The load tool requires the server to be running. It exits unsuccessfully if
connections fail or the expected deliveries are incomplete.

See [BACKEND_GUIDE.md](BACKEND_GUIDE.md) for every file's role, request and response
formats, a step-by-step demonstration, design decisions, testing, profiling,
requirements coverage, and remaining submission work.
