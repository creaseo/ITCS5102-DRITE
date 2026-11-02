# DRITE frontend implementation and user guide

This guide details the web dashboard frontend for DRITE. It explains the user interface architecture, single-use ticket authentication over WebSockets, the real-time Canvas visualization engine, interactive controls, and how to verify and test the UI.

For the underlying Go server engine and API contracts, see [BACKEND_GUIDE.md](BACKEND_GUIDE.md). For end-to-end architecture and navigation across the full system, see [PROJECT_GUIDE.md](PROJECT_GUIDE.md).

---

## 1. Overview & architecture

The DRITE dashboard is a high-performance, single-page web application located in the `web/` directory. It requires zero build tooling, bundling steps, or external package managers, relying on standard web APIs:

- **HTML5 & Modern CSS**: Responsive glassmorphism interface styled with custom CSS properties, ambient gradients, and curated typography (`Outfit` and `JetBrains Mono`).
- **Vanilla ES6+ JavaScript**: Client state store, resilient WebSocket connection lifecycle, and event envelope routing.
- **HTML5 Canvas Engine**: Optimized 60 FPS rolling time-series graph rendering that eliminates DOM overhead.
- **Static Hosting**: Served directly by the Go backend on root `GET /`, ensuring same-origin WebSocket connectivity without cross-origin configuration.

```text
Browser Client (web/app.js)
  │
  ├── 1. POST /api/v1/auth ─────────> Obtains 60s single-use ticket
  │
  ├── 2. GET /api/v1/snapshot ──────> Hydrates initial metrics, averages, incidents
  │
  ├── 3. GET /ws/dashboard?ticket=..> Establishes full-duplex WebSocket push stream
  │        ├── envelope: "metric" ──> Updates rolling charts, fleet table, KPIs
  │        └── envelope: "alert" ───> Triggers alert card, counter badge, audio chime
  │
  └── 4. POST /api/settings ────────> Dynamically updates CPU threshold and alert cooldown
```

---

## 2. File responsibilities

```text
web/
├── index.html   # Semantic layout: header, KPI badges, chart canvases, alert feed, fleet table, modal
├── style.css    # Dark telemetry design system, glassmorphism cards, responsive CSS grid, animations
└── app.js       # Client state, ticket handshake, Canvas charts, tooltips, staleness loop, settings
```

| File | Primary Role |
| :--- | :--- |
| `web/index.html` | Defines the layout hierarchy with unique IDs for metric binding, chart containers, control inputs, and connection status badges. |
| `web/style.css` | Implements the dark telemetry theme (`#07090e`), custom typography fallbacks, translucent card surfaces, glowing indicators, and mobile-friendly responsive breakpoints. |
| `web/app.js` | Manages state, coordinates ticket authentication via REST, handles WebSocket push events, renders multi-node Canvas charts with hover crosshairs, detects stale nodes, and triggers Web Audio chimes. |

---

## 3. Authentication & connection lifecycle

### Single-use ticket exchange
Native browser WebSocket implementations cannot attach custom headers such as `Authorization: Bearer <token>`. DRITE resolves this securely without exposing static long-lived credentials in URLs:

1. `app.js` issues a `POST /api/v1/auth` request with header `Authorization: Bearer <token>` and body `{"role":"dashboard"}`.
2. The server returns a 201 response containing an ephemeral single-use UUID ticket and expiration timestamp (60 seconds).
3. The dashboard opens `ws://localhost:8080/ws/dashboard?ticket=<ticket>`.
4. The backend burns the ticket during the handshake and admits the full-duplex connection.

### Reconnection and error handling
If the server restarts or the connection drops:
- The status pill immediately reflects `🔴 Disconnected. Retrying...`.
- An automatic exponential reconnection loop is scheduled every 3 seconds to request a fresh ticket and reconnect.
- Upon reconnection, the status pill shifts to `🟢 Live Telemetry Active` and data streams resume seamlessly.

---

## 4. Visualizations & Canvas engine

### Real-time CPU & threshold chart
- **60-Second Rolling Window**: Renders historical CPU percentages across 60 discrete time slices.
- **Multi-Agent Color Tracking**: Assigns distinct vibrant palette colors to individual nodes (`node-01` cyan, `node-02` pink, `node-03` emerald, etc.).
- **Fleet Average & Area Fill**: Highlights the fleet-wide arithmetic mean curve in bright orange with subtle gradient fill.
- **Threshold Rule Line**: Displays a red dashed horizontal line representing the active CPU alert threshold (e.g., 90%).
- **Interactive Hover Tooltip**: Hovering over the chart displays a vertical crosshair and a floating tooltip showing the exact timestamp and percentage.

### Memory & network throughput chart
- **Dual Stream Plotting**: Simultaneously tracks fleet memory utilization (0–100%, purple) and network throughput (scaled dynamically in KB/s, cyan).
- **Interactive Tooltip**: Hovering reveals synchronized memory and network throughput values for any historical point.

---

## 5. Interactive controls & features

1. **Runtime Telemetry Controls (Threshold & Alert Cooldown)**:
   - Operators can adjust the CPU alert threshold (1% to 100%) and the alert cooldown / throttling period (0s to 60s).
   - Clicking **Apply Controls** sends `POST /api/settings` with `{"cpu_threshold": <val>, "alert_cooldown_sec": <val>}`.
   - The server acknowledges the changes immediately without requiring a restart; the chart threshold line and header badge update in real time.
2. **Node Fleet Filter Dropdown**:
   - Located in the CPU chart header.
   - Allows operators to switch between viewing all fleet nodes simultaneously or isolating a specific agent (e.g., `Node: node-01`).
3. **Live Stream Pause / Resume (⏸️ / ▶️)**:
   - Clicking the pause button freezes graph rendering and incoming buffer pushes, enabling operators to inspect transient spikes without data sliding off-screen.
4. **Synthesized Alert Chime (🔔 / 🔕)**:
   - Uses the browser's Web Audio API (`AudioContext`) to synthesize a soft tone whenever a new threshold violation arrives. Toggleable via the bell button.
5. **Snapshot JSON Export**:
   - Clicking **Export JSON** in the fleet header triggers a download of the complete backend state (`drite-snapshot-<timestamp>.json`) containing global averages, per-agent breakdowns, recent incidents, and retained samples.
6. **Live Node Staleness Tracking**:
   - A background loop inspects each agent's last reported timestamp every 2 seconds:
     - `🟢 Reporting`: Received telemetry within 10 seconds.
     - `🔴 In Alert`: Received telemetry within 10 seconds and CPU exceeds threshold.
     - `🟡 Stale`: No telemetry received for 10–30 seconds.
     - `🔴 Offline`: No telemetry received for >30 seconds.
7. **Clear Alert Feed**:
   - Clicking **Clear** in the *Threshold Violations* card resets the visual alert cards and counter badge.

---

## 6. Step-by-step UI demonstration

1. **Start Backend**:
   ```bash
   go run ./cmd/drite
   ```
2. **Open Dashboard**:
   Navigate to [http://localhost:8080](http://localhost:8080) in your web browser.
   - Status badge shows `🟢 Live Telemetry Active`.
   - Fleet table shows waiting state.
3. **Start Agent 1 (Normal Load)**:
   ```bash
   go run ./cmd/agent -id node-01 -cpu 50
   ```
   - `node-01` appears in the fleet table with green status.
   - CPU chart plots a steady line around 50%.
4. **Start Agent 2 (Overheating Node)**:
   ```bash
   go run ./cmd/agent -id node-02 -cpu 95
   ```
   - Alert feed immediately chimes and adds a red breach card for `node-02`.
   - Incident counter increments.
   - Fleet table marks `node-02` as `🔴 In Alert`.
5. **Dynamic Threshold Tuning**:
   - Move the threshold slider to 40% and click **Apply Threshold**.
   - `node-01` (at 50% CPU) now triggers alert events as well.
   - Move the slider to 98% and click **Apply Threshold**; alerts cease immediately.
6. **Inspect Tooltips & Export**:
   - Hover over the CPU chart to inspect point values.
   - Click **Export JSON** to verify the snapshot payload.

---

## 7. Configuration & deployment

| Parameter | Default | How to Modify |
| :--- | :--- | :--- |
| Server Base URL | `http://localhost:8080` (or current host) | Top-right gear icon modal |
| Bearer Token | `drite-demo-token` | Top-right gear icon modal |
| Static Serving Path | `/` | Configured in `internal/drite/server.go` (`http.FileServer`) |

When deploying behind a reverse proxy (e.g., Nginx, Caddy) or production HTTPS/WSS:
- Ensure the proxy passes `Upgrade: websocket` and `Connection: Upgrade` headers for `/ws/*`.
- Use matching `wss://` protocol endpoints.
- Update `DRITE_AUTH_TOKEN` in the server environment and enter the corresponding token in the dashboard settings modal.
