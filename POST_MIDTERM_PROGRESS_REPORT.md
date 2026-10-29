# DRITE — Post Mid-Term Progress Report

**Project title:** DRITE Telemetry Monitoring and Alerting System  
**Reporting date:** October 29, 2026  
**Version control:** [github.com/creaseo/ITCS5102-DRITE](https://github.com/creaseo/ITCS5102-DRITE)

## Current status

The project is **on track for the backend milestone** and has completed the first
major implementation phase. The repository currently contains a working Go
backend for infrastructure telemetry ingestion, rolling aggregation, alerting,
REST reporting, and WebSocket fan-out. The seven planned backend use cases are
implemented: authentication, concurrent ingestion, broadcast delivery,
time-window aggregation, heartbeat cleanup, CPU-threshold alerts, and REST
reporting. The implementation also includes command-line demonstrations and a
load-test client.

The current verification baseline is strong: `go test ./...` passes, and the
repository records passing race-enabled tests, `go vet`, and 87.4% backend
statement coverage. The documented local smoke workload used 10 agents and 2
dashboard subscribers for 2 seconds, sending 1,000 metrics with 2,000 expected
and actual dashboard deliveries, no connection errors, and server-to-dashboard
latency of 0.183 ms p50, 0.313 ms p95, and 0.552 ms p99. These results establish
correctness for a small local workload, not production capacity.

## Pending use cases and deliverables

The main pending use case is the **dashboard UI**: live metric visualization,
per-agent summaries, rolling averages, and CPU-alert presentation using the
existing REST and WebSocket interfaces. Additional alert types for memory and
network conditions remain future extensions; CPU threshold alerts are the
currently required rule.

The remaining project work is to complete the UI integration, run a systematic
performance study, and prepare the final report and presentation. The backend
still uses bounded in-memory state, so persistence, durable delivery, clustered
coordination, connection quotas, rate limiting, separate production identities,
and automatic agent reconnection are outside the current milestone.

## Challenges and adjustments

Several challenges were more substantial than initially anticipated. The
WebSocket design required explicit handling for authentication tickets, role
and agent binding, origin checks, heartbeat deadlines, shutdown, malformed or
oversized messages, and slow subscribers. Concurrent ingestion and snapshots
also required careful synchronization and bounded history so that the service
remains responsive without allowing unbounded memory growth.

The project scope was clarified during implementation because the source
materials describe six scenarios in one place but include a seventh use case in
the diagram. The backend now covers all seven, while treating the dashboard as
the next stage rather than claiming it is complete. Performance claims will
also be kept deliberately conservative: the current measurements are a local
smoke test, and larger-scale capacity will not be inferred without controlled
experiments.

## Six-week plan and experiments

1. **Weeks 1–2 — UI integration:** Build the dashboard against
   `/api/v1/snapshot`, `/api/metrics/averages`, and `/ws/dashboard`. Validate
   reconnect behavior, alert ordering, per-agent views, empty states, and
   threshold updates.
2. **Weeks 2–3 — Repeatable baseline experiments:** Run isolated server
   instances at 10 agents/2 dashboards and 100 agents/5 dashboards, using
   20 ms and 100 ms send intervals and 10–30 second runs. Repeat each
   configuration at least three times. Record successful messages, delivery
   loss, connection errors, throughput, p50/p95/p99 server-to-dashboard
   latency, CPU, memory, and race-test status.
3. **Weeks 4–5 — Scale and bottleneck study:** Increase clients gradually to
   500 and then higher only if file-descriptor and machine limits permit.
   Compare idle heartbeat overhead with active workloads, capture CPU and heap
   profiles using local pprof, and evaluate how the 10,000-sample/five-minute
   history bound affects memory and latency. The target is an evidence-based
   characterization, not an unsupported claim of 10,000 connections or
   50,000 messages per second.
4. **Week 6 — Finalization:** Consolidate experiment results, document
   limitations and deployment assumptions, complete the final report and
   presentation, and perform a clean end-to-end demonstration of agent,
   backend, dashboard, alert, and snapshot behavior.

Overall, the project is on track if the dashboard and controlled experiments
remain the next priorities. If scale testing exposes limits, the schedule will
prioritize a reproducible baseline and a clear bottleneck analysis over adding
unvalidated production features.
