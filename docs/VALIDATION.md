# Backend validation record

Validation date: October 28, 2026. Environment: macOS, Apple M5 (arm64),
Go 1.27.2. These are local observations from this implementation, not guarantees
for other hardware or larger deployments.

## Automated verification

- `go test -race -cover ./...`: passed all 16 backend tests; no detected data races.
- Backend package statement coverage: **87.4%**.
- Command packages report 0% automated coverage; the separate executable checks
  below exercise their main successful paths and do not count toward coverage.
- `go vet ./...`: passed.
- `git diff --check`: passed.
- Server, agent and load-test commands built successfully.
- All three standalone milestone demonstration programs executed successfully.
- The synthetic agent rejected `-cpu=-0.5` with a nonzero exit as expected.

The integration suite covers authentication, ticket role/identity restrictions,
ticket expiry and replay, origin rejection, invalid and oversized telemetry,
multiple subscribers, CPU alerts and ordering, runtime threshold changes,
concurrent ingestion/settings/snapshots, one-minute/five-minute boundaries,
per-agent aggregation, history bounds, large finite numeric values, heartbeat
expiry, responsive Pong handling, slow-consumer isolation and shutdown.

## Small live workload

A separately compiled server was started on local port 18080, with its optional
profiler on loopback port 16060. The compiled load tool ran:

```bash
/tmp/drite-loadtest -url http://127.0.0.1:18080 \
  -agents 10 -dashboards 2 -duration 2s -interval 20ms
```

The `/tmp` executables were validation artifacts; normal project commands are
listed in README and BACKEND_GUIDE.

| Measurement | Observed value |
| --- | --- |
| Agents | 10 |
| Dashboard subscribers | 2 |
| Send duration | 2.0013 seconds |
| Successfully sent metrics | 1,000 |
| Expected subscriber deliveries | 2,000 |
| Actual subscriber deliveries | 2,000 |
| Connection errors | 0 |
| Send rate | 499.66 metrics/second |
| Server-to-subscriber p50 | 0.183 ms |
| Server-to-subscriber p95 | 0.313 ms |
| Server-to-subscriber p99 | 0.552 ms |
| Latency observations | 2,000 |

All expected messages reached the two subscribers in this run. The profiler
returned HTTP 200. A separate synthetic agent with CPU 95 created both a
per-agent snapshot entry and a CPU incident. SIGTERM stopped the server cleanly
with exit code 0. The server and temporary clients were stopped after validation.

These latency values measure from event creation in the server to receipt by
the subscriber on the same machine. They do not measure the agent-to-server leg,
internet latency, or sustained production capacity. This is a short smoke test,
not the final empirical study.

## Microbenchmarks

Command: `go test -run '^$' -bench . -benchmem`.

| Benchmark | Time per operation | Allocated bytes per operation | Allocations per operation |
| --- | --- | --- | --- |
| ParseMetric | 455.0 ns | 704 | 11 |
| RollingAverages, parallel reads over 1,000 samples | 2,152 ns | 0 | 0 |

The parallel benchmark reports amortized benchmark time per operation; it is
not an individual request latency percentile. Results vary with runtime, load,
hardware and race instrumentation. No claim is made about 10,000 concurrent
connections, 50,000 messages/second, or a measured mutex-versus-channel advantage.
The guide describes how to expand the experiment and record reproducible data.
