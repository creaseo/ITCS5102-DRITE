DRITE is an event-driven telemetry platform for gathering, viewing, and displaying live operational data from server agents.

## Backend capabilities implemented

- **UC1**: Token-authenticated HTTP handshake with safe WebSocket upgrade for `/ws/agent` and `/ws/dashboard`.
- **UC2**: Dedicated goroutine per agent connection for concurrent JSON metric ingestion.
- **UC3**: Central channel-based broadcast hub that fans out live events to all connected dashboards.
- **UC4**: Thread-safe rolling 1-minute and 5-minute metric aggregation protected with `sync.RWMutex`.
- **UC5**: Heartbeat ping/pong monitoring with read/write deadlines and automatic connection cleanup.
- **UC6**: CPU threshold alert dispatch (`cpu_threshold`) pushed immediately over dashboard WebSockets.

## REST + WebSocket surface

- `GET/POST /api/settings` for runtime threshold settings.
- `GET /api/metrics/averages` for rolling average telemetry snapshots.
- `GET /ws/agent` for agent metric streams.
- `GET /ws/dashboard` for real-time dashboard event streams.

## Run

```bash
go run ./cmd/drite
```

Environment variables:

- `PORT` (default: `8080`)
- `DRITE_AUTH_TOKEN` (default: `drite-demo-token`)
- `DRITE_CPU_THRESHOLD` (default: `90`)
