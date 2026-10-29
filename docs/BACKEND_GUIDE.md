# DRITE backend implementation and execution guide

This guide explains the backend implementation, connects it to the supplied
ideation documents, and provides reproducible execution and verification steps.
The implementation uses Go's standard library and the existing
`github.com/gorilla/websocket v1.5.3` dependency. It keeps the original repository's
root `drite` package and `cmd/drite` entry point.

## 1. Requirements analysis and scope decisions

The supplied files are `GoLang Project Ideas Analysis.docx` and
`GoLang Use Case Ideation.docx`, not PDFs. The analysis describes seven use cases,
three standalone language demonstrations, and performance experiments. The
ideation document calls the project six scenarios but also includes UC7 in its
diagram. This implementation covers all seven.

The original README takes precedence for the established route names and the
three environment variables. Its four routes remain available. The longer
analysis also calls for token issuance and diagnostic snapshots, so
`/api/v1/auth` and `/api/v1/snapshot` are additive routes. The generic `/ws` in the
ideation snippets is represented by the README's separate agent and dashboard
routes; no ambiguous `/ws` alias is introduced.

The analysis mentions memory threshold alerts as an example, while the README
explicitly requires configurable CPU alerts of kind `cpu_threshold`. The backend
implements that CPU rule. Memory and network data participate in ingestion,
averages, snapshots and live events; additional alert rules are future extensions.

| Requirement | Implementation | Main verification |
| --- | --- | --- |
| UC1 authentication and WebSocket upgrade | Shared bearer authentication; 60-second single-use tickets; role/agent binding; same-host origin policy | Authentication, untrusted origins, expiry and replay tests |
| UC2 concurrent ingestion | Dedicated reader per connection; typed JSON; field, range and identity validation | Invalid input, oversize input, healthy-agent isolation, concurrent ingestion tests |
| UC3 channel fan-out | Central hub owns client membership; buffered subscriber queues; slow clients disconnected | Multiple-subscriber delivery and slow-consumer isolation tests |
| UC4 rolling averages | Mutex-protected history; one-minute/five-minute global and per-agent averages | Window boundaries, out-of-order input, capacity, expiry and snapshot isolation tests |
| UC5 heartbeat cleanup | Server Ping; client Pong; read/write deadlines; explicit shutdown | Missing-Pong eviction, responsive client and shutdown tests |
| UC6 anomaly dispatch | CPU strictly above the active threshold produces an alert before its metric event, with per-agent alert cooldown throttling | Alert ordering, strict threshold boundary, incident snapshot, alert cooldown throttling tests |
| UC7 REST reporting | Settings, averages and snapshot with bounded incident history | Authentication, settings validation and snapshot contents tests |
| Program 1 | Four primitive types and eight operations | Runnable `demo-primitives` command |
| Program 2 | Structs, maps, slices, loops, conditionals and switch | Runnable `demo-structures` command |
| Program 3 | Worker pool, channels, mutex, WaitGroup, defer/panic/recover | Runnable `demo-concurrency` command |
| Empirical evaluation support | Synthetic workload, microbenchmarks, race tests, optional local pprof | Reproducible commands below; measured results in `VALIDATION.md` |

The original course rubric cited by the analysis is not included. Requirements
here are derived from the two supplied documents and README, not independently
verified against the instructor's rubric. The course report, presentation,
frontend and team contribution records remain separate submission work.

### 2. Repository and file responsibilities

```text
ITCS5102-DRITE/
├── README.md
├── .env.example
├── .gitignore
├── go.mod
├── go.sum
├── cmd/
│   ├── drite/
│   │   ├── main.go
│   │   └── profiling.go
│   ├── agent/main.go
│   └── loadtest/main.go
├── internal/
│   └── drite/
│       ├── server.go
│       ├── config.go
│       ├── models.go
│       ├── uc1_authentication.go
│       ├── uc2_ingestion.go
│       ├── uc3_broadcast.go
│       ├── uc4_aggregation.go
│       ├── uc5_heartbeat.go
│       ├── uc6_alerts.go
│       ├── uc7_rest_api.go
│       └── server_test.go
├── web/
│   ├── index.html
│   ├── style.css
│   └── app.js
└── docs/
    ├── PROJECT_GUIDE.md
    ├── BACKEND_GUIDE.md
    ├── FRONTEND_GUIDE.md
    └── VALIDATION.md
```

| File | Responsibility |
| :--- | :--- |
| `internal/drite/server.go` | Constructs the service and background goroutines; registers routes; coordinates shutdown; provides shared JSON/HTTP helpers |
| `internal/drite/config.go` | Defines runtime configuration; validates the original environment variables; supplies heartbeat defaults |
| `internal/drite/models.go` | Defines the shared metric, average, alert and outbound event contracts |
| `internal/drite/uc1_authentication.go` | Validates bearer credentials; creates and consumes tickets; binds identities; upgrades and admits connections |
| `internal/drite/uc2_ingestion.go` | Parses strict JSON and validates required values; stamps server receipt time; feeds the ingestion channel |
| `internal/drite/uc3_broadcast.go` | Owns the client map and outgoing channel closure; delivers events to dashboards; removes slow consumers |
| `internal/drite/uc4_aggregation.go` | Stores bounded raw history and incidents; computes incremental means; expires old samples/tickets |
| `internal/drite/uc5_heartbeat.go` | Defines connection state and the sole application writer; sends Pings; handles Pong deadlines and cleanup |
| `internal/drite/uc6_alerts.go` | Processes ingestion in order; records samples; checks the current threshold; publishes alert and metric events |
| `internal/drite/uc7_rest_api.go` | Exposes authenticated runtime settings, averages, per-agent summaries and snapshot copies |
| `internal/drite/server_test.go` | Unit and real HTTP/WebSocket integration tests; parsing and aggregation benchmarks |
| `cmd/drite/main.go` | Loads configuration, starts HTTP, handles interrupt/SIGTERM, shuts down HTTP and upgraded sockets |
| `cmd/drite/profiling.go` | Starts optional pprof on a loopback-only listener, separate from public routes |
| `cmd/agent/main.go` | Generates synthetic telemetry, with optional fixed CPU, and answers server Pings |
| `cmd/loadtest/main.go` | Creates concurrent agents/subscribers, counts deliveries, reports latency percentiles and failures |
| `web/` | Standalone real-time web dashboard client (HTML5/Canvas) |
| `go.mod`, `go.sum` | Preserve module identity, Go version requirement and WebSocket dependency checksums |
| `.env.example` | Lists environment settings without automatically loading them |
| `.gitignore` | Excludes local credentials, compiled binaries and profiling/test artifacts |
| `README.md` | Short introduction and startup reference |
| `docs/PROJECT_GUIDE.md` | Unified overarching system map and execution instructions |
| `docs/BACKEND_GUIDE.md` | Full backend implementation and run guide |
| `docs/FRONTEND_GUIDE.md` | Full frontend implementation and dashboard user guide |
| `docs/VALIDATION.md` | Records actual verification and experiment limitations |

## 3. Data flow and concurrency ownership

```text
Authenticated agent WebSocket
    -> connection reader (UC2)
    -> bounded ingestion channel
    -> processing goroutine (UC6)
         -> mutex-protected sample/incident store (UC4)
         -> alert event, if breached
         -> metric event with current averages
    -> bounded broadcast channel
    -> hub goroutine (UC3)
    -> bounded queue per dashboard
    -> connection writer (UC5)

Authenticated REST handler (UC7) -> read-locked snapshot of the same store
```

Each connection has one reader and one application writer. Control-frame writes
use Gorilla's concurrency-safe `WriteControl`. Only the hub owns membership and
closes client output channels, preventing send-on-closed-channel races. A full
subscriber queue disconnects that subscriber immediately, isolating its delay.
The ingestion and broadcast queues apply bounded backpressure rather than
silently dropping individual incoming events.

The processing goroutine combines sequential aggregation and alert evaluation
so an alert precedes its triggering metric on each subscriber's stream. Alerts
do not jump ahead of unrelated events already queued. `sync.RWMutex` protects
history and settings. Shutdown cancels background loops, closes sockets and
waits for all owned goroutines. HTTP shutdown alone would leave upgraded sockets
open, so the entry point also calls `Server.Close`.

## 4. Setup and startup

From the project root, use Go 1.22 or newer:

```bash
go version
go mod download
export PORT=8080
export DRITE_AUTH_TOKEN=drite-demo-token
export DRITE_CPU_THRESHOLD=90
go run ./cmd/drite
```

These examples use macOS/Linux shell syntax. On PowerShell, set values with
`$env:PORT="8080"`, `$env:DRITE_AUTH_TOKEN="drite-demo-token"`, and
`$env:DRITE_CPU_THRESHOLD="90"` before the same Go commands.

The initial threshold must be greater than 0 and at most 100; the runtime API
also allows 0 for demonstrating alerts on any positive CPU value. Invalid
startup configuration fails with an explanatory error. Settings are in memory;
a restart restores the environment configuration.

To build instead of running source directly:

```bash
go build -o bin/drite ./cmd/drite
./bin/drite
```

Stop with Ctrl+C. On Unix, SIGTERM also requests orderly shutdown. The default
HTTP listener is `:8080`, so it binds all available interfaces.

## 5. REST API examples

Set a shell variable for the example credential:

```bash
TOKEN=drite-demo-token
curl -sS -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/settings
curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"cpu_threshold":80,"alert_cooldown_sec":5}' http://localhost:8080/api/settings
curl -sS -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/metrics/averages
curl -sS -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8080/api/metrics/averages?agent_id=node-01'
curl -sS -H "Authorization: Bearer $TOKEN" http://localhost:8080/api/v1/snapshot
```

Settings return `{"auth_required":true,"cpu_threshold":80,"alert_cooldown_sec":5}`. Averages return:

```json
{
  "one_minute": {"cpu_usage":50,"memory_usage":40,"network_speed":10000,"count":10},
  "five_minute": {"cpu_usage":50,"memory_usage":40,"network_speed":10000,"count":10}
}
```

Averages are arithmetic means of retained samples, not time-weighted means and
not equally weighted averages of machines. Agents sampling more frequently
contribute more observations. Empty windows have zero values and `count: 0`;
clients should use the count to distinguish no data from measured zero usage.
The optional `agent_id` filter returns zero-count windows if no samples match.

Snapshots contain:

- `server_time`: snapshot time in UTC.
- `averages`: global one-minute and five-minute averages.
- `agents`: map from agent ID to its rolling averages.
- `metrics`: retained samples within the five-minute window.
- `incidents`: most recent 1,000 CPU threshold alerts, oldest retained first.
- `capacity_dropped`: cumulative samples removed by the 10,000-sample capacity
  limit, excluding normal time expiry.

Malformed/missing/out-of-range data produces HTTP 400. Missing or invalid REST
authentication produces 401. Unsupported methods produce 405 with an `Allow`
header. JSON responses disable caching. POST bodies are limited to 4 KiB and
reject unknown fields and multiple concatenated JSON documents.

## 6. WebSocket contract and future UI integration

Agents connect to `/ws/agent?agent_id=node-01`. IDs must be 1–64 characters,
begin with a letter or digit, and otherwise contain only letters, digits,
underscore, period or hyphen. Agent IDs are assigned by the authenticated caller;
this is a shared-token project, not a multi-tenant identity system.

Send a text frame containing:

```json
{
  "agent_id":"node-01",
  "cpu_usage":95,
  "memory_usage":45,
  "network_speed":12345
}
```

`cpu_usage` and `memory_usage` are percentages from 0 to 100. `network_speed` is
a finite nonnegative bytes/second rate. All three numeric fields are required;
zero is valid. `agent_id` can be omitted because the connection binds it; if
supplied, it must match. An optional RFC3339 `timestamp` is accepted for legacy
compatibility but replaced with server receipt time. This prevents agent clock
skew or forged future times from changing aggregation windows.

Unknown fields, binary data, invalid JSON, missing values and identity mismatches
close the agent with WebSocket code 1008. Frames over 4 KiB close it with 1009.
The invalid agent does not interrupt other connections. Two authenticated
connections using the same agent ID contribute to the same history; there is no
exclusive agent-ID ownership in this version.

Dashboards connect to `/ws/dashboard`. They receive event envelopes:

```json
{
  "type":"alert",
  "alert":{
    "level":"warning","kind":"cpu_threshold","agent_id":"node-01",
    "value":95,"threshold":90,"occurred_at":"2026-10-28T14:00:00Z",
    "message":"CPU usage exceeded the configured threshold"
  },
  "server_time":"2026-10-28T14:00:00Z"
}
```

```json
{
  "type":"metric",
  "metric":{
    "agent_id":"node-01","cpu_usage":95,"memory_usage":45,
    "network_speed":12345,"timestamp":"2026-10-28T14:00:00Z"
  },
  "averages":{
    "one_minute":{"cpu_usage":95,"memory_usage":45,"network_speed":12345,"count":1},
    "five_minute":{"cpu_usage":95,"memory_usage":45,"network_speed":12345,"count":1}
  },
  "server_time":"2026-10-28T14:00:00Z"
}
```

The dates above are illustrative. Each accepted sample strictly above the
threshold generates an alert; there is no cooldown or deduplication. Metrics
exactly equal to the threshold do not trigger alerts. Averages in live envelopes
are global. Dashboards should use snapshot `agents` or the REST filter for
per-agent aggregates. Subscriber streams are live only, with no replay guarantee;
a reconnecting UI should fetch a fresh snapshot.

### Browser-friendly one-use tickets

Native browser WebSocket constructors cannot set an Authorization header.
Exchange the credential for a ticket using REST:

```bash
curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"role":"dashboard"}' \
  http://localhost:8080/api/v1/auth
```

The 201 response contains `ticket` and `expires_at`. Open
`ws://localhost:8080/ws/dashboard?ticket=<returned-ticket>` within 60 seconds.
For agents, the issuance body is `{"role":"agent","agent_id":"node-01"}`
and the connection URL includes both `agent_id=node-01` and the returned ticket.
Tickets are single use, including an attempted upgrade after successful token
validation; obtain another ticket if the handshake fails. Role or identity
mismatches do not consume the ticket. Expired tickets are periodically removed;
at most 10,000 outstanding tickets are retained.

The default origin check permits requests with no Origin header or the same
host as the WebSocket endpoint. No permissive CORS or cross-origin allowlist is
configured. Serve the eventual UI from the same origin, or deliberately add a
trusted allowlist during the UI stage. The permanent bearer token is not
accepted in URLs. Avoid embedding it in committed frontend code.

Server Pings occur every 20 seconds; a Pong extends the read deadline by 60
seconds. Application writes have a 10-second timeout. Browser runtimes reply to
protocol Pings automatically. Go clients must keep reading to process Ping/Pong
frames, even when they only send telemetry.

## 7. Step-by-step backend demonstration

1. Start the server with the defaults in section 4.
2. Start `go run ./cmd/agent -id node-01 -cpu 50` in another terminal.
3. Request averages and snapshot; counts increase and CPU averages approach 50.
4. Set the threshold to 40 using the settings POST. New samples produce incidents.
5. Request the snapshot and inspect `incidents` to verify values and thresholds.
6. Start `go run ./cmd/agent -id node-02 -cpu 95` in a third terminal; inspect the
   `agents` map and global average to distinguish machine and fleet statistics.
7. Stop agents with Ctrl+C. Existing samples expire from averages after five
   minutes. The incident history remains until capacity eviction or restart.
8. For automated subscriber observation without a UI, restart with no other
   agents connected and run the load command below. It validates fan-out counts.
9. Run tests to demonstrate missing-Pong cleanup, invalid-agent isolation and
   multiple-dashboard alert delivery, which are awkward to inspect with curl.

Standalone milestone examples:

```bash
go run ./cmd/demo-primitives
go run ./cmd/demo-structures
go run ./cmd/demo-concurrency
```

Program 1 demonstrates `strconv.Itoa`, `bits.OnesCount`, `math.Round`, `math.Abs`,
`strings.ToUpper`, `strings.ReplaceAll`, `strconv.ParseBool` and `fmt.Sprintf`.
Program 2 uses maps of slices of structs, nested loops, if/else and switch.
Program 3 creates three workers, shares tasks and results through channels,
protects a completion count with a mutex, and waits before closing results.
A negative task intentionally panics and is recovered within that task. Backend
input errors use ordinary error handling, not panic/recover. Worker/map output
order may vary; that is expected.

## 8. Verification and empirical evaluation

```bash
go test -race ./...
go test -cover ./...
go vet ./...
go test -run '^$' -bench . -benchmem
```

Tests start local HTTP servers and real WebSocket clients, so the test environment
must allow local port binding. `-race` checks executed paths for data races; a
passing run is evidence for those paths, not a proof of all possible schedules.
`BenchmarkParseMetric` measures strict parsing. `BenchmarkRollingAverages`
measures concurrent reads over 1,000 retained samples. The sample/history tests
also verify capacity limits and the separation of snapshot copies from state.

Run a clean server without other agents or dashboards, then:

```bash
go run ./cmd/loadtest -agents 10 -dashboards 2 -duration 10s -interval 100ms
go run ./cmd/loadtest -agents 100 -dashboards 5 -duration 30s -interval 100ms
```

The tool uses the same `DRITE_AUTH_TOKEN` as the server. It reports successful
writes, expected and actual subscriber deliveries, connection errors, throughput
and p50/p95/p99 server-to-subscriber latency in milliseconds. It waits up to three
seconds for delivery after sending stops, and exits nonzero for losses/errors.
Other simultaneously active agents would affect its counts, so use an isolated
server for each experiment. Latency uses the event's server time; server and load
client clocks must be synchronized. It excludes the agent-to-server leg and
samples only the first 100,000 metric deliveries. Do not describe it as full
end-to-end latency or an uncapped distribution.

Optional profiling starts on a separate local-only port:

```bash
go run ./cmd/drite -pprof 127.0.0.1:6060
go tool pprof 'http://127.0.0.1:6060/debug/pprof/heap'
go tool pprof 'http://127.0.0.1:6060/debug/pprof/profile?seconds=30'
```

Run the workload in another terminal during profile capture. The profiler is off
by default and rejects non-loopback addresses. The profiling port exposes process
internals to local users and has no bearer authentication.

For the final report, record Go version, CPU/RAM, OS, client counts, send interval,
run duration, retained history size, race-mode status and failures. Repeat each
configuration at least three times and report variation. Increase concurrency
gradually (for example 10, 100, 500) before considering 1,000–10,000 connections;
check operating-system file descriptor limits. The source documents' 10,000
connections and 50,000 messages/second are proposed experiments, not established
capacity guarantees. A causal mutex-versus-channel comparison requires a
controlled alternative implementation; the supplied microbenchmarks alone do
not establish that comparison. Heartbeat CPU cost similarly requires matched
idle workloads and profile comparisons.

## 9. Limits and next submission steps

History is deliberately in memory: restart loses metrics, incidents, tickets
and runtime threshold changes. The global store retains at most 10,000 samples
for up to five minutes and at most 1,000 incidents independent of age. At high
sampling rates, count-based eviction shortens the effective retained window;
`capacity_dropped` exposes this condition. Averages reflect retained data, not
an implied complete history. Maintenance runs every second, while reads also
filter by time so expired samples are never reported as current.

Aggregation scans retained samples and snapshots copy them. This is a readable
baseline for the course and a bounded workload; it is not a claim of a tuned
high-throughput database. The server has no persistence, durable delivery,
cluster coordination, connection-count quotas or automatic reconnecting agent.
Shared credentials authorize all agent IDs and administrative routes. A deployed
version would need separate principals and roles, rate limiting and TLS (or a
trusted reverse proxy). These are explicit extensions rather than hidden claims
about the current implementation.

The next project stage is the dashboard UI, followed by a complete report and
presentation supported by actual experimental results. Review the instructor's
original rubric and the course's assistance/authorship rules. Each teammate
should understand, test and explain their contributions; this repository does
not fabricate commit histories, experimental results or team authorship.
