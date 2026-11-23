# DRITE unified project guide

DRITE (**D**istributed **R**eal-Time **I**nfrastructure **T**elemetry **E**ngine) is an event-driven telemetry platform built in Go with an interactive HTML5/Canvas dashboard.

This guide provides an overarching architectural map, unified execution instructions, and direct links to component documentation.

---

## 📚 Documentation index

| Document | Scope & Purpose |
| :--- | :--- |
| **[README.md](../README.md)** | Quickstart commands, environment variables, route surface, and verify steps. |
| **[BACKEND_GUIDE.md](BACKEND_GUIDE.md)** | Go backend architecture, use cases (UC1–UC7), concurrency, REST/WebSocket APIs, and profiling. |
| **[FRONTEND_GUIDE.md](FRONTEND_GUIDE.md)** | Web dashboard, single-use ticket handshake, Canvas charts, interactive controls, and UI testing. |
| **[VALIDATION.md](VALIDATION.md)** | Local verification record, 87.4% test coverage, race detection, and load test benchmarks. |

---

## 🏛️ End-to-end architecture

```text
[ Synthetic / Remote Agents ]
        │  (ws://host/ws/agent?agent_id=node-01)
        ▼
┌────────────────────────────────────────────────────────┐
│           DRITE Go Backend (internal/drite)            │
│                                                        │
│  ├── UC1: Bearer & Ticket Authentication               │
│  ├── UC2: Concurrent Metric Ingestion                  │
│  ├── UC6: CPU Threshold Rule Evaluation & Alerting     │
│  ├── UC4: Rolling 1m / 5m Aggregation (sync.RWMutex)   │
│  ├── UC3: Central Broadcast Hub (Fan-Out & Eviction)   │
│  ├── UC5: Heartbeat Ping/Pong Deadlines                │
│  ├── UC7: REST Reporting (/api/settings, /api/v1/..)  │
│  └── Static File Server (serves ./web at GET /)        │
└────────────────────────────────────────────────────────┘
        │
        ├── Full-Duplex WebSocket Push (ws://host/ws/dashboard?ticket=...)
        ▼
[ Web Dashboard Client (web/) ]
  ├── Real-Time CPU & Threshold Line Chart (Canvas)
  ├── Memory & Network Throughput Chart (Canvas)
  ├── Live Incident & Breach Feed (Web Audio chime)
  ├── Dynamic Telemetry Controls (Threshold & Alert Cooldown via POST /api/settings)
  ├── Active Node Fleet Matrix with Staleness Tracking
  └── Snapshot JSON Data Exporter
```

---

## 🚀 Unified execution walkthrough

### 1. Requirements
- Go 1.22 or later.
- Modern web browser (Chrome, Firefox, Safari, Edge).

### 2. Start the Telemetry Platform
In **Terminal 1**:
```bash
go run ./cmd/drite
```
*The server binds to port `8080` by default and hosts the static dashboard on `/`.*

### 3. Open the Dashboard UI
Visit **[http://localhost:8080](http://localhost:8080)** in your browser.
The dashboard will authenticate automatically via a single-use ticket and establish a live WebSocket connection (`🟢 Live Telemetry Active`).

### 4. Stream Metrics from Agents
In **Terminal 2** (Normal node):
```bash
go run ./cmd/agent -id node-01 -cpu 50
```
In **Terminal 3** (High-CPU node):
```bash
go run ./cmd/agent -id node-02 -cpu 95
```
*Observe the dashboard plotting multi-node lines, generating instant alert cards for `node-02`, and updating the fleet matrix.*

---

## 🧪 Comprehensive verification

```bash
# 1. Run full integration suite with race detector (16 tests, 87.4% statement coverage)
go test -race -v ./...

# 2. Run static analysis
go vet ./...

# 3. Execute microbenchmarks
go test -run '^$' -bench . -benchmem

# 4. Run concurrent load stress test (requires server running in another terminal)
go run ./cmd/loadtest -agents 10 -dashboards 2 -duration 10s
```

For detailed benchmark logs and profiling commands, see [VALIDATION.md](VALIDATION.md) and [BACKEND_GUIDE.md](BACKEND_GUIDE.md). For UI features and interactive controls, see [FRONTEND_GUIDE.md](FRONTEND_GUIDE.md).
