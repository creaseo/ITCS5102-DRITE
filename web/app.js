/**
 * DRITE Real-Time Dashboard Client
 * Phase 4 & 5: Enhanced Multi-Node Visualizations, Interactivity, Sound & Snapshot Export
 */

// Application State
const state = {
  config: {
    apiUrl: window.location.origin.startsWith('http') ? window.location.origin : 'http://localhost:8080',
    token: 'drite-demo-token',
    cpuThreshold: 90,
    alertCooldown: 5
  },
  connection: {
    status: 'connecting',
    socket: null,
    reconnectTimer: null
  },
  streaming: {
    isPaused: false,
    soundEnabled: true
  },
  selectedAgent: 'all', // 'all' or specific agent_id
  fleet: new Map(),     // agent_id -> { latestMetric, averages, lastSeen, color, history: [] }
  history: {
    maxPoints: 60,
    timestamps: [],
    cpuGlobal: [],
    memGlobal: [],
    netGlobal: []
  },
  incidents: [],
  totalIncidents: 0
};

// Node Color Palette Generator
const nodePalette = ['#06b6d4', '#ec4899', '#10b981', '#f59e0b', '#3b82f6', '#8b5cf6'];
let nextColorIdx = 0;
function getNodeColor(agentId) {
  const existing = state.fleet.get(agentId);
  if (existing && existing.color) return existing.color;
  const color = nodePalette[nextColorIdx % nodePalette.length];
  nextColorIdx++;
  return color;
}

// Canvas Chart State
let cpuCanvas, cpuCtx;
let memNetCanvas, memNetCtx;
let audioCtx = null;

// DOM Elements
const el = {
  statusBadge: document.getElementById('ws-status'),
  statusLabel: document.getElementById('status-label'),
  thresholdBadge: document.getElementById('current-threshold-badge'),
  cooldownBadge: document.getElementById('current-cooldown-badge'),
  thresholdSlider: document.getElementById('threshold-slider'),
  sliderValDisplay: document.getElementById('slider-val-display'),
  cooldownSlider: document.getElementById('cooldown-slider'),
  cooldownValDisplay: document.getElementById('cooldown-val-display'),
  btnUpdateThreshold: document.getElementById('btn-update-threshold'),
  settingsFeedback: document.getElementById('settings-feedback'),
  displayToken: document.getElementById('display-token'),

  // Header Buttons
  btnPauseStream: document.getElementById('btn-pause-stream'),
  pauseIcon: document.getElementById('pause-icon'),
  btnToggleSound: document.getElementById('btn-toggle-sound'),
  soundIcon: document.getElementById('sound-icon'),
  agentFilterSelect: document.getElementById('agent-filter-select'),
  btnExportSnapshot: document.getElementById('btn-export-snapshot'),

  // KPIs
  valCpuNow: document.getElementById('val-cpu-now'),
  valCpuAvg: document.getElementById('val-cpu-avg'),
  barCpu: document.getElementById('bar-cpu'),
  valMemNow: document.getElementById('val-mem-now'),
  valMemAvg: document.getElementById('val-mem-avg'),
  barMem: document.getElementById('bar-mem'),
  valNetNow: document.getElementById('val-net-now'),
  valNetAvg: document.getElementById('val-net-avg'),
  barNet: document.getElementById('bar-net'),
  valIncidentsCount: document.getElementById('val-incidents-count'),
  valActiveNodes: document.getElementById('val-active-nodes'),
  barIncidents: document.getElementById('bar-incidents'),

  // Feeds & Tables
  alertFeed: document.getElementById('alert-feed'),
  alertEmptyState: document.getElementById('alert-empty-state'),
  incidentCounterBadge: document.getElementById('incident-counter-badge'),
  btnClearAlerts: document.getElementById('btn-clear-alerts'),
  fleetTableBody: document.getElementById('fleet-table-body'),
  btnRefreshSnapshot: document.getElementById('btn-refresh-snapshot'),

  // Modal
  btnQuickConfig: document.getElementById('btn-quick-config'),
  configModal: document.getElementById('config-modal'),
  btnCloseModal: document.getElementById('btn-close-modal'),
  btnModalCancel: document.getElementById('btn-modal-cancel'),
  btnModalSave: document.getElementById('btn-modal-save'),
  cfgApiUrl: document.getElementById('cfg-api-url'),
  cfgAuthToken: document.getElementById('cfg-auth-token')
};

// Canvas Hover & Tooltip State
let hoverCpuX = null;
let hoverMemNetX = null;

// Initialize Application
document.addEventListener('DOMContentLoaded', () => {
  setupCanvases();
  setupEventListeners();
  loadInitialState();
  initConnection();
  window.addEventListener('resize', handleResize);
  setInterval(renderFleetTable, 2000); // Live node staleness check every 2s
});

function setupCanvases() {
  cpuCanvas = document.getElementById('cpu-chart-canvas');
  cpuCtx = cpuCanvas ? cpuCanvas.getContext('2d') : null;
  memNetCanvas = document.getElementById('mem-net-chart-canvas');
  memNetCtx = memNetCanvas ? memNetCanvas.getContext('2d') : null;

  if (cpuCanvas) {
    cpuCanvas.addEventListener('mousemove', (e) => {
      const rect = cpuCanvas.getBoundingClientRect();
      hoverCpuX = e.clientX - rect.left;
      renderCpuChart();
    });
    cpuCanvas.addEventListener('mouseleave', () => {
      hoverCpuX = null;
      renderCpuChart();
    });
  }

  if (memNetCanvas) {
    memNetCanvas.addEventListener('mousemove', (e) => {
      const rect = memNetCanvas.getBoundingClientRect();
      hoverMemNetX = e.clientX - rect.left;
      renderMemNetChart();
    });
    memNetCanvas.addEventListener('mouseleave', () => {
      hoverMemNetX = null;
      renderMemNetChart();
    });
  }

  handleResize();
}

function handleResize() {
  if (cpuCanvas && cpuCtx) {
    const rect = cpuCanvas.parentElement.getBoundingClientRect();
    cpuCanvas.width = rect.width * window.devicePixelRatio;
    cpuCanvas.height = rect.height * window.devicePixelRatio;
    cpuCtx.scale(window.devicePixelRatio, window.devicePixelRatio);
    renderCpuChart();
  }
  if (memNetCanvas && memNetCtx) {
    const rect = memNetCanvas.parentElement.getBoundingClientRect();
    memNetCanvas.width = rect.width * window.devicePixelRatio;
    memNetCanvas.height = rect.height * window.devicePixelRatio;
    memNetCtx.scale(window.devicePixelRatio, window.devicePixelRatio);
    renderMemNetChart();
  }
}

// Event Listeners
function setupEventListeners() {
  el.thresholdSlider.addEventListener('input', (e) => {
    el.sliderValDisplay.textContent = `${e.target.value}%`;
  });
  if (el.cooldownSlider) {
    el.cooldownSlider.addEventListener('input', (e) => {
      el.cooldownValDisplay.textContent = `${e.target.value}s`;
    });
  }

  el.btnUpdateThreshold.addEventListener('click', updateThresholdSettings);
  el.btnRefreshSnapshot.addEventListener('click', loadInitialState);

  // Agent Filter
  if (el.agentFilterSelect) {
    el.agentFilterSelect.addEventListener('change', (e) => {
      state.selectedAgent = e.target.value;
      renderCpuChart();
      renderMemNetChart();
    });
  }

  // Stream Pause / Resume
  if (el.btnPauseStream) {
    el.btnPauseStream.addEventListener('click', () => {
      state.streaming.isPaused = !state.streaming.isPaused;
      el.pauseIcon.textContent = state.streaming.isPaused ? '▶️' : '⏸️';
      el.btnPauseStream.title = state.streaming.isPaused ? 'Resume Stream' : 'Pause Stream';
    });
  }

  // Sound Toggle
  if (el.btnToggleSound) {
    el.btnToggleSound.addEventListener('click', () => {
      state.streaming.soundEnabled = !state.streaming.soundEnabled;
      el.soundIcon.textContent = state.streaming.soundEnabled ? '🔔' : '🔕';
      el.btnToggleSound.title = state.streaming.soundEnabled ? 'Mute Alert Sound' : 'Enable Alert Sound';
    });
  }

  // Clear Alerts Feed
  if (el.btnClearAlerts) {
    el.btnClearAlerts.addEventListener('click', () => {
      state.incidents = [];
      state.totalIncidents = 0;
      renderAlertFeed();
    });
  }

  // Export Snapshot JSON
  if (el.btnExportSnapshot) {
    el.btnExportSnapshot.addEventListener('click', exportSnapshotAsJSON);
  }

  // Config Modal
  el.btnQuickConfig.addEventListener('click', () => {
    el.cfgApiUrl.value = state.config.apiUrl;
    el.cfgAuthToken.value = state.config.token;
    el.configModal.style.display = 'flex';
  });

  const closeModal = () => { el.configModal.style.display = 'none'; };
  el.btnCloseModal.addEventListener('click', closeModal);
  el.btnModalCancel.addEventListener('click', closeModal);

  el.btnModalSave.addEventListener('click', () => {
    state.config.apiUrl = el.cfgApiUrl.value.trim().replace(/\/+$/, '');
    state.config.token = el.cfgAuthToken.value.trim();
    el.displayToken.textContent = state.config.token;
    closeModal();
    if (state.connection.socket) {
      state.connection.socket.close();
    }
    loadInitialState();
    initConnection();
  });
}

// Synthesize Soft Alert Beep via Web Audio API
function playAlertChime() {
  if (!state.streaming.soundEnabled) return;
  try {
    if (!audioCtx) {
      audioCtx = new (window.AudioContext || window.webkitAudioContext)();
    }
    if (audioCtx.state === 'suspended') {
      audioCtx.resume();
    }
    const osc = audioCtx.createOscillator();
    const gain = audioCtx.createGain();
    osc.type = 'sine';
    osc.frequency.setValueAtTime(880, audioCtx.currentTime); // A5 note
    osc.frequency.exponentialRampToValueAtTime(440, audioCtx.currentTime + 0.25);
    gain.gain.setValueAtTime(0.12, audioCtx.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.001, audioCtx.currentTime + 0.25);
    osc.connect(gain);
    gain.connect(audioCtx.destination);
    osc.start();
    osc.stop(audioCtx.currentTime + 0.25);
  } catch (e) {
    // AudioContext blocked or unsupported
  }
}

// 1. Initial State & Snapshot Hydration
async function loadInitialState() {
  try {
    const headers = { 'Authorization': `Bearer ${state.config.token}` };

    // Fetch Settings
    const settingsRes = await fetch(`${state.config.apiUrl}/api/settings`, { headers });
    if (settingsRes.ok) {
      const settings = await settingsRes.json();
      state.config.cpuThreshold = settings.cpu_threshold;
      el.thresholdBadge.textContent = settings.cpu_threshold;
      el.thresholdSlider.value = settings.cpu_threshold;
      el.sliderValDisplay.textContent = `${settings.cpu_threshold}%`;
      if (settings.alert_cooldown_sec !== undefined) {
        state.config.alertCooldown = settings.alert_cooldown_sec;
        if (el.cooldownBadge) el.cooldownBadge.textContent = settings.alert_cooldown_sec;
        if (el.cooldownSlider) el.cooldownSlider.value = settings.alert_cooldown_sec;
        if (el.cooldownValDisplay) el.cooldownValDisplay.textContent = `${settings.alert_cooldown_sec}s`;
      }
    }

    // Fetch Snapshot
    const snapRes = await fetch(`${state.config.apiUrl}/api/v1/snapshot`, { headers });
    if (snapRes.ok) {
      const snap = await snapRes.json();
      hydrateFromSnapshot(snap);
    }
  } catch (err) {
    console.warn('Initial snapshot fetch failed:', err);
  }
}

function hydrateFromSnapshot(snap) {
  // Global averages
  if (snap.averages) {
    updateKpiCards(snap.averages.one_minute, snap.averages.five_minute);
  }

  // Incidents
  if (snap.incidents && snap.incidents.length > 0) {
    state.incidents = snap.incidents.slice(-50).reverse();
    state.totalIncidents = snap.incidents.length;
    renderAlertFeed();
  }

  // Agents
  if (snap.agents) {
    for (const [id, avgs] of Object.entries(snap.agents)) {
      state.fleet.set(id, {
        averages: avgs,
        lastSeen: new Date(),
        color: getNodeColor(id),
        history: []
      });
    }
    updateAgentFilterOptions();
    renderFleetTable();
  }

  // Raw metrics into chart history
  if (snap.metrics && snap.metrics.length > 0) {
    state.history.timestamps = [];
    state.history.cpuGlobal = [];
    state.history.memGlobal = [];
    state.history.netGlobal = [];

    const recent = snap.metrics.slice(-state.history.maxPoints);
    for (const m of recent) {
      state.history.timestamps.push(new Date(m.timestamp));
      state.history.cpuGlobal.push(m.cpu_usage);
      state.history.memGlobal.push(m.memory_usage);
      state.history.netGlobal.push(m.network_speed / 1024);
    }
    renderCpuChart();
    renderMemNetChart();
  }
}

function updateAgentFilterOptions() {
  if (!el.agentFilterSelect) return;
  const currentVal = el.agentFilterSelect.value;
  let html = '<option value="all">Display: All Fleet Nodes</option>';
  for (const id of state.fleet.keys()) {
    html += `<option value="${escapeHtml(id)}">Node: ${escapeHtml(id)}</option>`;
  }
  el.agentFilterSelect.innerHTML = html;
  el.agentFilterSelect.value = state.fleet.has(currentVal) ? currentVal : 'all';
}

// 2. WebSocket Connection via Single-Use Ticket
async function initConnection() {
  updateStatus('connecting', 'Acquiring Ticket...');

  try {
    const authRes = await fetch(`${state.config.apiUrl}/api/v1/auth`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${state.config.token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({ role: 'dashboard' })
    });

    if (!authRes.ok) {
      updateStatus('disconnected', `Auth Failed (HTTP ${authRes.status})`);
      scheduleReconnect();
      return;
    }

    const { ticket } = await authRes.json();
    const wsProto = state.config.apiUrl.startsWith('https') ? 'wss' : 'ws';
    const wsHost = state.config.apiUrl.replace(/^https?:\/\//, '');
    const wsUrl = `${wsProto}://${wsHost}/ws/dashboard?ticket=${ticket}`;

    const ws = new WebSocket(wsUrl);
    state.connection.socket = ws;

    ws.onopen = () => {
      updateStatus('connected', 'Live Telemetry Active');
      clearTimeout(state.connection.reconnectTimer);
    };

    ws.onmessage = (event) => {
      if (state.streaming.isPaused) return; // Stream is paused
      try {
        const envelope = JSON.parse(event.data);
        handleEnvelope(envelope);
      } catch (e) {
        console.error('Failed to parse envelope:', e);
      }
    };

    ws.onclose = () => {
      updateStatus('disconnected', 'Disconnected. Retrying...');
      scheduleReconnect();
    };

    ws.onerror = () => {
      ws.close();
    };
  } catch (err) {
    updateStatus('disconnected', 'Connection Failed');
    scheduleReconnect();
  }
}

function scheduleReconnect() {
  clearTimeout(state.connection.reconnectTimer);
  state.connection.reconnectTimer = setTimeout(initConnection, 3000);
}

function updateStatus(status, text) {
  state.connection.status = status;
  el.statusLabel.textContent = text;
  if (status === 'connected') {
    el.statusBadge.classList.remove('disconnected');
  } else {
    el.statusBadge.classList.add('disconnected');
  }
}

// 3. Inbound Envelope Handler
function handleEnvelope(env) {
  if (env.type === 'metric' && env.metric) {
    const m = env.metric;
    const now = new Date(env.server_time || m.timestamp);

    // Append to global history
    state.history.timestamps.push(now);
    state.history.cpuGlobal.push(env.averages ? env.averages.one_minute.cpu_usage : m.cpu_usage);
    state.history.memGlobal.push(env.averages ? env.averages.one_minute.memory_usage : m.memory_usage);
    state.history.netGlobal.push(env.averages ? env.averages.one_minute.network_speed / 1024 : m.network_speed / 1024);

    if (state.history.timestamps.length > state.history.maxPoints) {
      state.history.timestamps.shift();
      state.history.cpuGlobal.shift();
      state.history.memGlobal.shift();
      state.history.netGlobal.shift();
    }

    // Update fleet record
    const isNewNode = !state.fleet.has(m.agent_id);
    const agent = state.fleet.get(m.agent_id) || {
      averages: null,
      color: getNodeColor(m.agent_id),
      history: []
    };
    agent.latestMetric = m;
    agent.lastSeen = now;
    agent.history.push({ time: now, cpu: m.cpu_usage, mem: m.memory_usage, net: m.network_speed / 1024 });
    if (agent.history.length > state.history.maxPoints) agent.history.shift();

    state.fleet.set(m.agent_id, agent);

    if (isNewNode) {
      updateAgentFilterOptions();
    }

    // Update KPIs
    if (env.averages) {
      updateKpiCards(env.averages.one_minute, env.averages.five_minute);
    }

    renderCpuChart();
    renderMemNetChart();
    renderFleetTable();
  } else if (env.type === 'alert' && env.alert) {
    state.totalIncidents++;
    state.incidents.unshift(env.alert);
    if (state.incidents.length > 50) state.incidents.pop();

    renderAlertFeed();
    triggerAlertVisualPulse();
    playAlertChime();
  }
}

function triggerAlertVisualPulse() {
  el.incidentCounterBadge.style.transform = 'scale(1.25)';
  setTimeout(() => { el.incidentCounterBadge.style.transform = 'scale(1)'; }, 250);
}

// 4. KPI Updates
function updateKpiCards(oneMin, fiveMin) {
  if (!oneMin) return;

  // CPU
  el.valCpuNow.textContent = `${oneMin.cpu_usage.toFixed(1)}%`;
  el.valCpuAvg.textContent = fiveMin ? `5m avg: ${fiveMin.cpu_usage.toFixed(1)}%` : '';
  el.barCpu.style.width = `${Math.min(100, Math.max(0, oneMin.cpu_usage))}%`;

  // Memory
  el.valMemNow.textContent = `${oneMin.memory_usage.toFixed(1)}%`;
  el.valMemAvg.textContent = fiveMin ? `5m avg: ${fiveMin.memory_usage.toFixed(1)}%` : '';
  el.barMem.style.width = `${Math.min(100, Math.max(0, oneMin.memory_usage))}%`;

  // Network
  const netKB = (oneMin.network_speed / 1024).toFixed(1);
  const net5mKB = fiveMin ? (fiveMin.network_speed / 1024).toFixed(1) : '0';
  el.valNetNow.textContent = `${netKB} KB/s`;
  el.valNetAvg.textContent = `5m avg: ${net5mKB} KB/s`;
  el.barNet.style.width = `${Math.min(100, (oneMin.network_speed / 1024 / 500) * 100)}%`;

  // Active Nodes & Incidents
  el.valIncidentsCount.textContent = state.totalIncidents;
  el.valActiveNodes.textContent = `${state.fleet.size} node${state.fleet.size === 1 ? '' : 's'} active`;
  el.barIncidents.style.width = `${Math.min(100, state.totalIncidents * 10)}%`;
}

// 5. Alert Feed Render
function renderAlertFeed() {
  if (state.incidents.length === 0) {
    el.alertEmptyState.style.display = 'block';
    el.alertFeed.innerHTML = '';
    el.alertFeed.appendChild(el.alertEmptyState);
    el.incidentCounterBadge.textContent = '0 Alerts';
    return;
  }

  el.alertEmptyState.style.display = 'none';
  el.incidentCounterBadge.textContent = `${state.totalIncidents} Alert${state.totalIncidents === 1 ? '' : 's'}`;
  
  el.alertFeed.innerHTML = state.incidents.map((a) => {
    const timeStr = new Date(a.occurred_at).toLocaleTimeString();
    return `
      <div class="alert-item-card">
        <div class="alert-item-header">
          <span>⚠️ High CPU Breach: ${escapeHtml(a.agent_id)}</span>
          <span>${timeStr}</span>
        </div>
        <div class="alert-item-meta">
          <span>Observed: <strong>${a.value.toFixed(1)}%</strong></span>
          <span>Threshold: ${a.threshold.toFixed(1)}%</span>
        </div>
      </div>
    `;
  }).join('');
}

// 6. Fleet Table Render
function renderFleetTable() {
  if (state.fleet.size === 0) {
    el.fleetTableBody.innerHTML = `
      <tr class="table-empty-row">
        <td colspan="7">No active agents reporting. Run <code>go run ./cmd/agent -id node-01</code> to start streaming.</td>
      </tr>
    `;
    return;
  }

  const rows = [];
  const now = Date.now();

  for (const [id, node] of state.fleet.entries()) {
    const diffSec = Math.round((now - (node.lastSeen ? node.lastSeen.getTime() : now)) / 1000);
    const m = node.latestMetric;
    const cpu = m ? `${m.cpu_usage.toFixed(1)}%` : '--';
    const mem = m ? `${m.memory_usage.toFixed(1)}%` : '--';
    const net = m ? `${(m.network_speed / 1024).toFixed(1)} KB/s` : '--';
    const isAlert = m && m.cpu_usage > state.config.cpuThreshold;

    let statusText = '🟢 Reporting';
    let statusColor = 'var(--status-healthy)';
    if (diffSec > 30) {
      statusText = '🔴 Offline';
      statusColor = 'var(--status-critical)';
    } else if (diffSec > 10) {
      statusText = '🟡 Stale';
      statusColor = 'var(--status-warning)';
    } else if (isAlert) {
      statusText = '🔴 In Alert';
      statusColor = 'var(--status-critical)';
    }

    rows.push(`
      <tr style="${state.selectedAgent !== 'all' && state.selectedAgent !== id ? 'opacity: 0.35' : ''}">
        <td>
          <span style="display: inline-block; width: 8px; height: 8px; border-radius: 50%; background: ${node.color}; margin-right: 8px;"></span>
          <strong>${escapeHtml(id)}</strong>
        </td>
        <td>
          <span style="color: ${statusColor}; font-weight: 500;">
            ${statusText}
          </span>
        </td>
        <td style="color: ${isAlert ? 'var(--status-critical)' : 'inherit'}">${cpu}</td>
        <td>${mem}</td>
        <td>${net}</td>
        <td>${node.history ? node.history.length : 1}</td>
        <td style="color: var(--text-muted)">${diffSec}s ago</td>
      </tr>
    `);
  }
  el.fleetTableBody.innerHTML = rows.join('');
}

// 7. Real-Time Canvas Charts
function renderCpuChart() {
  if (!cpuCtx || !cpuCanvas) return;
  const ctx = cpuCtx;
  const w = cpuCanvas.width / window.devicePixelRatio;
  const h = cpuCanvas.height / window.devicePixelRatio;

  ctx.clearRect(0, 0, w, h);
  drawGrid(ctx, w, h);

  // Draw Threshold Line
  const threshY = h - (state.config.cpuThreshold / 100) * (h - 40) - 20;
  ctx.save();
  ctx.setLineDash([6, 4]);
  ctx.strokeStyle = '#ef4444';
  ctx.lineWidth = 1.5;
  ctx.beginPath();
  ctx.moveTo(40, threshY);
  ctx.lineTo(w - 20, threshY);
  ctx.stroke();
  ctx.fillStyle = '#ef4444';
  ctx.font = '10px JetBrains Mono';
  ctx.fillText(`Rule: ${state.config.cpuThreshold}%`, w - 85, threshY - 6);
  ctx.restore();

  const step = (w - 60) / (state.history.maxPoints - 1);

  // If "all" is selected, draw individual node lines with reduced opacity
  if (state.selectedAgent === 'all') {
    for (const node of state.fleet.values()) {
      if (node.history && node.history.length > 1) {
        ctx.beginPath();
        node.history.forEach((pt, i) => {
          const x = 40 + i * step;
          const y = h - (pt.cpu / 100) * (h - 40) - 20;
          if (i === 0) ctx.moveTo(x, y);
          else ctx.lineTo(x, y);
        });
        ctx.strokeStyle = node.color;
        ctx.lineWidth = 1.5;
        ctx.globalAlpha = 0.4;
        ctx.stroke();
        ctx.globalAlpha = 1.0;
      }
    }

    // Draw Fleet Average Line prominently
    const data = state.history.cpuGlobal;
    if (data.length > 1) {
      ctx.beginPath();
      data.forEach((val, i) => {
        const x = 40 + i * step;
        const y = h - (val / 100) * (h - 40) - 20;
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      });
      ctx.strokeStyle = '#f97316';
      ctx.lineWidth = 2.5;
      ctx.stroke();

      // Area fill
      ctx.lineTo(40 + (data.length - 1) * step, h - 20);
      ctx.lineTo(40, h - 20);
      ctx.fillStyle = 'rgba(249, 115, 22, 0.12)';
      ctx.fill();
    }
  } else {
    // Specific agent selected
    const agent = state.fleet.get(state.selectedAgent);
    if (agent && agent.history && agent.history.length > 1) {
      ctx.beginPath();
      agent.history.forEach((pt, i) => {
        const x = 40 + i * step;
        const y = h - (pt.cpu / 100) * (h - 40) - 20;
        if (i === 0) ctx.moveTo(x, y);
        else ctx.lineTo(x, y);
      });
      ctx.strokeStyle = agent.color || '#f97316';
      ctx.lineWidth = 2.5;
      ctx.stroke();

      ctx.lineTo(40 + (agent.history.length - 1) * step, h - 20);
      ctx.lineTo(40, h - 20);
      ctx.fillStyle = 'rgba(249, 115, 22, 0.15)';
      ctx.fill();
    }
  }

  // Draw Tooltip & Crosshair if hovering
  if (hoverCpuX !== null && state.history.cpuGlobal.length > 0) {
    const idx = Math.min(state.history.cpuGlobal.length - 1, Math.max(0, Math.round((hoverCpuX - 40) / step)));
    const targetX = 40 + idx * step;
    const cpuVal = state.history.cpuGlobal[idx];
    const timeVal = state.history.timestamps[idx] ? state.history.timestamps[idx].toLocaleTimeString() : '';

    ctx.save();
    ctx.strokeStyle = 'rgba(255, 255, 255, 0.3)';
    ctx.setLineDash([4, 4]);
    ctx.beginPath();
    ctx.moveTo(targetX, 10);
    ctx.lineTo(targetX, h - 20);
    ctx.stroke();

    // Tooltip Box
    const tipText = `${timeVal} | CPU: ${cpuVal.toFixed(1)}%`;
    ctx.font = '11px JetBrains Mono';
    const textWidth = ctx.measureText(tipText).width;
    const boxX = Math.min(w - textWidth - 24, Math.max(40, targetX - textWidth / 2 - 10));
    ctx.fillStyle = '#111726';
    ctx.strokeStyle = '#f97316';
    ctx.lineWidth = 1;
    ctx.setLineDash([]);
    ctx.fillRect(boxX, 10, textWidth + 20, 24);
    ctx.strokeRect(boxX, 10, textWidth + 20, 24);
    ctx.fillStyle = '#fff';
    ctx.fillText(tipText, boxX + 10, 26);
    ctx.restore();
  }
}

function renderMemNetChart() {
  if (!memNetCtx || !memNetCanvas) return;
  const ctx = memNetCtx;
  const w = memNetCanvas.width / window.devicePixelRatio;
  const h = memNetCanvas.height / window.devicePixelRatio;

  ctx.clearRect(0, 0, w, h);
  drawGrid(ctx, w, h);

  const memData = state.history.memGlobal;
  const netData = state.history.netGlobal;
  if (memData.length < 2) return;

  const step = (w - 60) / (state.history.maxPoints - 1);

  // Plot Memory (0-100%)
  ctx.beginPath();
  memData.forEach((val, i) => {
    const x = 40 + i * step;
    const y = h - (val / 100) * (h - 40) - 20;
    if (i === 0) ctx.moveTo(x, y);
    else ctx.lineTo(x, y);
  });
  ctx.strokeStyle = '#a855f7';
  ctx.lineWidth = 2;
  ctx.stroke();

  // Plot Network
  const maxNet = Math.max(50, ...netData);
  ctx.beginPath();
  netData.forEach((val, i) => {
    const x = 40 + i * step;
    const y = h - (val / maxNet) * (h - 40) - 20;
    if (i === 0) ctx.moveTo(x, y);
    else ctx.lineTo(x, y);
  });
  ctx.strokeStyle = '#06b6d4';
  ctx.lineWidth = 2;
  ctx.stroke();

  // Draw Tooltip & Crosshair for Mem/Net
  if (hoverMemNetX !== null && memData.length > 0) {
    const idx = Math.min(memData.length - 1, Math.max(0, Math.round((hoverMemNetX - 40) / step)));
    const targetX = 40 + idx * step;
    const memVal = memData[idx];
    const netVal = netData[idx];
    const timeVal = state.history.timestamps[idx] ? state.history.timestamps[idx].toLocaleTimeString() : '';

    ctx.save();
    ctx.strokeStyle = 'rgba(255, 255, 255, 0.3)';
    ctx.setLineDash([4, 4]);
    ctx.beginPath();
    ctx.moveTo(targetX, 10);
    ctx.lineTo(targetX, h - 20);
    ctx.stroke();

    // Tooltip Box
    const tipText = `${timeVal} | Mem: ${memVal.toFixed(1)}% | Net: ${netVal.toFixed(1)} KB/s`;
    ctx.font = '11px JetBrains Mono';
    const textWidth = ctx.measureText(tipText).width;
    const boxX = Math.min(w - textWidth - 24, Math.max(40, targetX - textWidth / 2 - 10));
    ctx.fillStyle = '#111726';
    ctx.strokeStyle = '#a855f7';
    ctx.lineWidth = 1;
    ctx.setLineDash([]);
    ctx.fillRect(boxX, 10, textWidth + 20, 24);
    ctx.strokeRect(boxX, 10, textWidth + 20, 24);
    ctx.fillStyle = '#fff';
    ctx.fillText(tipText, boxX + 10, 26);
    ctx.restore();
  }
}

function drawGrid(ctx, w, h) {
  ctx.save();
  ctx.strokeStyle = 'rgba(255, 255, 255, 0.05)';
  ctx.lineWidth = 1;
  ctx.font = '10px JetBrains Mono';
  ctx.fillStyle = '#64748b';

  for (let pct = 0; pct <= 100; pct += 25) {
    const y = h - (pct / 100) * (h - 40) - 20;
    ctx.beginPath();
    ctx.moveTo(40, y);
    ctx.lineTo(w - 20, y);
    ctx.stroke();
    ctx.fillText(`${pct}%`, 10, y + 4);
  }
  ctx.restore();
}

// 8. Runtime Telemetry Controls Update
async function updateThresholdSettings() {
  const newThreshold = parseFloat(el.thresholdSlider.value);
  const newCooldown = el.cooldownSlider ? parseFloat(el.cooldownSlider.value) : state.config.alertCooldown;
  el.btnUpdateThreshold.disabled = true;
  el.settingsFeedback.textContent = 'Updating...';

  try {
    const res = await fetch(`${state.config.apiUrl}/api/settings`, {
      method: 'POST',
      headers: {
        'Authorization': `Bearer ${state.config.token}`,
        'Content-Type': 'application/json'
      },
      body: JSON.stringify({
        cpu_threshold: newThreshold,
        alert_cooldown_sec: newCooldown
      })
    });

    if (res.ok) {
      state.config.cpuThreshold = newThreshold;
      state.config.alertCooldown = newCooldown;
      el.thresholdBadge.textContent = newThreshold;
      if (el.cooldownBadge) el.cooldownBadge.textContent = newCooldown;
      el.settingsFeedback.textContent = '✓ Updated successfully';
      renderCpuChart();
    } else {
      el.settingsFeedback.textContent = '❌ Failed to update';
    }
  } catch (err) {
    el.settingsFeedback.textContent = '❌ Network error';
  } finally {
    el.btnUpdateThreshold.disabled = false;
    setTimeout(() => { el.settingsFeedback.textContent = ''; }, 3000);
  }
}

// 9. Snapshot JSON Export
async function exportSnapshotAsJSON() {
  try {
    const res = await fetch(`${state.config.apiUrl}/api/v1/snapshot`, {
      headers: { 'Authorization': `Bearer ${state.config.token}` }
    });
    if (!res.ok) {
      alert(`Failed to export snapshot: HTTP ${res.status}`);
      return;
    }
    const data = await res.json();
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `drite-snapshot-${new Date().toISOString().replace(/[:.]/g, '-')}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  } catch (err) {
    alert('Failed to export snapshot: ' + err.message);
  }
}

function escapeHtml(str) {
  return String(str).replace(/[&<>"']/g, (m) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#039;'
  }[m]));
}
