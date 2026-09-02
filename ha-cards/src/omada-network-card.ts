import { BarChart, GaugeChart, RadarChart } from "echarts/charts";
import { AriaComponent, GridComponent, RadarComponent } from "echarts/components";
import { use } from "echarts/core";
import { CanvasRenderer } from "echarts/renderers";
import type { EChartsOption } from "echarts";
import { css, html, LitElement, nothing } from "lit";
import { repeat } from "lit/directives/repeat.js";
import { unsafeSVG } from "lit/directives/unsafe-svg.js";
import logoDark from "./assets/logo-dark.svg?raw";
import logoLight from "./assets/logo-light.svg?raw";
import { inspectionAriaLabel } from "./accessibility";
import { ChartController, deviceResourceSummarySignature } from "./chart-controller";
import {
  formatBytes,
  formatLatency,
  formatPercent,
  formatRateBits,
  formatRateBytes,
  formatSpeedMbps,
  formatUptimeSeconds,
  qualityLabel
} from "./format";
import type {
  ClientRecord,
  DashboardModel,
  DeviceRecord,
  HomeAssistant,
  LinkRow,
  LovelaceCardConfig
} from "./ha-types";
import {
  cardHassChanged,
  getDashboardModel,
  isDeviceConnected,
  normalizeMacKey,
  vpnModeLabel,
  vpnPeerLoginSeconds,
  vpnRemoteLabel,
  vpnTotalBytes,
  vpnUptimeSeconds
} from "./model";
import { registerCustomCard } from "./register-card";

type Selection = { kind: "device"; key: string } | { kind: "client"; key: string };
type DeviceMeta = {
  pendingUpdate: boolean;
  updateTarget: string;
  poeBudget: { used: number; remaining: number; total: number } | undefined;
  poePortCount: number;
  connectedPorts: number;
  uplinkMbps: number;
  radioRows: Array<{ label: string; value: number }>;
  topPorts: DeviceRecord["ports"];
  portsPreview: DeviceRecord["ports"];
  clientPreview: ClientRecord[];
};
type ClientMeta = {
  liveRate: number;
  liveRateLabel: string;
  rateBreakdown: string;
  bandLabel: string;
  wiredPathLabel: string;
  wiredConnectionLabel: string;
  wiredLinkSpeed: string;
  lagPorts: string;
  attachmentLabel: string;
  quality: string;
};
type ChartOptionCacheEntry = {
  signature: string;
  option: EChartsOption;
};

use([BarChart, GaugeChart, RadarChart, AriaComponent, GridComponent, RadarComponent, CanvasRenderer]);

function chartAria(description: string) {
  return {
    enabled: true,
    label: { enabled: true, description },
    decal: { show: true }
  } as const;
}

export class OmadaNetworkCard extends LitElement {
  static override properties = {
    hass: { attribute: false, hasChanged: cardHassChanged },
    _config: { state: true },
    _model: { state: true },
    _selection: { state: true },
    _clientFilter: { state: true },
    _deviceFilter: { state: true }
  };

  static override styles = css`
    :host {
      display: block;
      --bg: linear-gradient(135deg, #08131d, #0b1d2f 42%, #10253a);
      --surface: rgba(9, 20, 34, 0.76);
      --surface-strong: rgba(13, 26, 42, 0.92);
      --border: rgba(146, 196, 255, 0.16);
      --text: #edf4ff;
      --muted: #97aac0;
      --accent: #54d1ff;
      --good: #1eb980;
      --warn: #ffb648;
      --bad: #ff6b7e;
      font-family: "Segoe UI", "Trebuchet MS", sans-serif;
      color: var(--text);
    }
    ha-card {
      overflow: hidden;
      color: var(--text);
      background: var(--bg);
      border: 1px solid rgba(255, 255, 255, 0.06);
      border-radius: 28px;
      box-shadow: 0 24px 56px rgba(0, 0, 0, 0.28);
    }
    .frame {
      aspect-ratio: 16 / 9;
      display: grid;
      grid-template-rows: auto 1fr;
      gap: 1rem;
      padding: 1rem;
      background:
        radial-gradient(circle at top left, rgba(84, 209, 255, 0.18), transparent 28%),
        radial-gradient(circle at 85% 10%, rgba(30, 185, 128, 0.16), transparent 24%);
    }
    .panel {
      border: 1px solid var(--border);
      border-radius: 24px;
      background: var(--surface);
      backdrop-filter: blur(18px);
      box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
    }
    .header, .content, .link-grid, .detail-hero, .chart-stack, .detail-bottom { display: grid; gap: 1rem; }
    .header { grid-template-columns: minmax(160px, 0.45fr) minmax(0, 2.55fr); }
    .content { grid-template-columns: minmax(260px, 0.92fr) minmax(0, 1.7fr) minmax(320px, 1.02fr); min-height: 0; height: 100%; }
    .link-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0.75rem; }
    .brand { padding: 0.9rem 1rem; display: grid; place-items: center; min-height: 7rem; }
    .brand-logo { display: flex; align-items: center; justify-content: center; }
    .brand-logo svg { width: clamp(88px, 9vw, 128px); height: auto; }
    .eyebrow, th { color: var(--accent); letter-spacing: 0.1em; text-transform: uppercase; font-size: 0.7rem; font-weight: 600; }
    .site-name { font-size: clamp(1.6rem, 2.6vw, 2.3rem); font-weight: 600; line-height: 1; }
    .site-meta, .row-subtitle, .chip-sub, .detail-sub { color: var(--muted); }
    .header-right, .list-shell, .detail-shell, .table-card { display: grid; gap: 0.75rem; padding: 0.9rem; min-height: 0; min-width: 0; }
    .header-right {
      grid-template-rows: auto 1fr;
      background: none;
      box-shadow: none;
      border: none;
    }
    .table-card { grid-template-rows: auto minmax(0, 1fr); align-content: start; align-items: start; height: 100%; align-self: stretch; padding: 0.75rem 0.75rem 0; }
    .chips { display: grid; grid-template-columns: repeat(6, minmax(0, 1fr)); gap: 0.65rem; }
    .chip, .detail-stat, .card-row, .detail-card, .chart-card {
      border-radius: 18px;
      border: 1px solid rgba(255, 255, 255, 0.05);
      background: linear-gradient(180deg, rgba(255, 255, 255, 0.04), rgba(255, 255, 255, 0.015));
    }
    .chip, .detail-stat { padding: 0.65rem 0.75rem; }
    .chip-label, .detail-stat-label { color: var(--muted); font-size: 0.7rem; text-transform: uppercase; letter-spacing: 0.08em; font-weight: 600; }
    .chip-value, .detail-stat-value { margin-top: 0.35rem; font-size: 1rem; font-weight: 600; }
    .chip {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 0.7rem;
      min-height: 3rem;
      border-color: rgba(84, 209, 255, 0.35);
      background: linear-gradient(180deg, rgba(84, 209, 255, 0.12), rgba(255, 255, 255, 0.02));
    }
    .chip-copy {
      min-width: 0;
      display: grid;
      gap: 0.16rem;
    }
    .chip-value {
      margin-top: 0;
      font-size: 1rem;
      flex: 0 0 auto;
    }
    .table { overflow: auto; min-width: 0; min-height: 0; align-self: stretch; border-radius: 18px; border: 1px solid rgba(255, 255, 255, 0.05); }
    table { width: 100%; border-collapse: collapse; font-size: 0.82rem; }
    th, td { padding: 0.7rem 0.75rem; text-align: left; white-space: nowrap; }
    th { position: sticky; top: 0; z-index: 1; background: rgba(9, 20, 34, 0.94); color: var(--muted); }
    tbody tr:nth-child(odd) { background: rgba(255, 255, 255, 0.015); }
    .table.tight table { table-layout: fixed; }
    .table.tight th, .table.tight td { overflow: hidden; text-overflow: ellipsis; }
    .table.clickable tbody tr { cursor: pointer; }
    .table.clickable tbody tr:hover { background: rgba(84, 209, 255, 0.08); }
    .table.clickable tbody tr:focus-within { background: rgba(84, 209, 255, 0.08); }
    .client-row-action {
      appearance: none;
      padding: 0;
      border: 0;
      color: inherit;
      background: none;
      font: inherit;
      text-align: left;
      cursor: pointer;
    }
    .col-name { width: 46%; }
    .col-ip { width: 24%; }
    .col-signal, .col-path { width: 15%; }
    .section-title, .row-top, .row-bottom, .list-toolbar, .detail-title { display: flex; align-items: center; justify-content: space-between; gap: 0.75rem; }
    .section-title, .row-title, .detail-name { font-weight: 600; }
    .row-title { font-size: 0.95rem; line-height: 1.2; }
    .card-row .row-title, .card-row .row-subtitle { display: block; }
    .detail-name { font-size: clamp(1.35rem, 2vw, 1.9rem); line-height: 1.05; }
    .pill-row, .metric-group { display: flex; gap: 0.45rem; flex-wrap: wrap; }
    .mini-pill, .metric-tag {
      border-radius: 999px;
      padding: 0.38rem 0.6rem;
      background: rgba(255, 255, 255, 0.04);
      border: 1px solid rgba(255, 255, 255, 0.08);
      color: var(--text);
      font-size: 0.74rem;
    }
    .mini-pill { cursor: pointer; color: var(--muted); }
    .card-row:focus-visible, .mini-pill:focus-visible, .client-row-action:focus-visible {
      outline: 2px solid var(--accent);
      outline-offset: 2px;
    }
    .mini-pill.active, .card-row.selected { border-color: rgba(84, 209, 255, 0.35); background: linear-gradient(180deg, rgba(84, 209, 255, 0.12), rgba(255, 255, 255, 0.02)); }
    .list-panel, .detail-panel { min-height: 0; height: 100%; display: grid; overflow: hidden; }
    .list-shell { grid-template-rows: auto auto minmax(0, 1fr); min-height: 0; height: 100%; }
    .list-scroll { overflow: auto; min-height: 0; height: calc(100% - 2rem); display: grid; gap: 0.7rem; align-content: start; padding-right: 0.2rem; }
    .card-row {
      box-sizing: border-box;
      width: 100%;
      padding: 0.9rem;
      display: grid;
      gap: 0.55rem;
      color: inherit;
      font: inherit;
      text-align: left;
      cursor: pointer;
      transition: transform 120ms ease, border-color 120ms ease;
    }
    .card-row:hover { transform: translateY(-1px); border-color: rgba(84, 209, 255, 0.28); }
    .signal-bar { display: block; height: 0.42rem; border-radius: 999px; background: rgba(255, 255, 255, 0.06); overflow: hidden; }
    .signal-bar > span { display: block; height: 100%; background: linear-gradient(90deg, var(--bad), var(--warn), var(--good)); }
    .status-dot { width: 0.7rem; height: 0.7rem; border-radius: 999px; display: inline-block; margin-right: 0.45rem; background: var(--muted); box-shadow: 0 0 18px currentColor; vertical-align: middle; }
    .status-up { color: var(--good); background: var(--good); }
    .status-down { color: var(--bad); background: var(--bad); }
    .detail-shell { grid-template-rows: auto auto minmax(0, 1fr); min-height: 0; height: calc(100% - 2rem); }
    .detail-panel .detail-stat { padding: 0.5rem; }
    .detail-hero { grid-template-columns: minmax(0, 1.15fr) minmax(0, 0.85fr); }
    .detail-stats { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 0.65rem; }
    .detail-stats-wired { grid-template-columns: repeat(3, minmax(0, 1fr)); }
    .chart-stack, .detail-bottom { grid-template-columns: repeat(2, minmax(0, 1fr)); min-height: 0; align-items: stretch; }
    .chart-card { padding: 0.75rem; min-height: 14rem; display: grid; grid-template-rows: auto 1fr; }
    .chart { min-height: 0; width: 100%; height: 100%; }
    .path-layout { display: grid; gap: 0.65rem; }
    .path-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 0.65rem; }
    .path-main .detail-stat-value { font-size: 0.96rem; line-height: 1.25; }
    .empty { display: grid; place-items: center; color: var(--muted); min-height: 18rem; text-align: center; padding: 2rem; }
    @media (max-width: 1400px) {
      .chips { grid-template-columns: repeat(3, minmax(0, 1fr)); }
      .detail-stats { grid-template-columns: repeat(2, minmax(0, 1fr)); }
      .detail-stats-wired { grid-template-columns: repeat(3, minmax(0, 1fr)); }
      .path-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
    }
    @media (max-width: 1100px) {
      .frame { height: auto; aspect-ratio: auto; min-height: auto; }
      .header, .content, .link-grid, .detail-hero, .chart-stack, .detail-bottom { grid-template-columns: 1fr; }
    }
  `;

  public hass?: HomeAssistant;
  private _config?: LovelaceCardConfig;
  private _model?: DashboardModel;
  private _selection: Selection | undefined;
  private _clientFilter: "all" | "wireless" | "wired" = "all";
  private _deviceFilter: "all" | "controller" | "gateway" | "switch" | "ap" = "all";
  private _filteredClients: ClientRecord[] = [];
  private _filteredDevices: DeviceRecord[] = [];
  private _visibleClients: ClientRecord[] = [];
  private _visibleDevices: DeviceRecord[] = [];
  private _pendingUpdateCount = 0;
  private _selectedDevice: DeviceRecord | undefined;
  private _selectedClient: ClientRecord | undefined;
  private readonly _chartController = new ChartController(this, () => this.syncCharts());
  private readonly _deviceMeta = new WeakMap<DeviceRecord, DeviceMeta>();
  private readonly _clientMeta = new WeakMap<ClientRecord, ClientMeta>();
  private readonly _devicePrimaryOptionCache = new WeakMap<DeviceRecord, ChartOptionCacheEntry>();
  private readonly _deviceSecondaryOptionCache = new WeakMap<DeviceRecord, ChartOptionCacheEntry>();
  private readonly _clientPrimaryOptionCache = new WeakMap<ClientRecord, ChartOptionCacheEntry>();
  private readonly _clientSecondaryOptionCache = new WeakMap<ClientRecord, ChartOptionCacheEntry>();
  public setConfig(config: LovelaceCardConfig): void {
    if (!config?.type) {
      throw new Error("Card type is required");
    }
    this._config = { logo_mode: "auto", device_limit: 100, client_limit: 150, show_vpn_peers: true, ...config };
  }

  public getCardSize(): number {
    return 16;
  }

  protected override willUpdate(changed: Map<string, unknown>): void {
    let modelChanged = false;
    const configChanged = changed.has("_config");
    if ((changed.has("hass") || configChanged) && this.hass) {
      this._model = getDashboardModel(this.hass, this._config?.site);
      modelChanged = true;
    }

    if (modelChanged || changed.has("_clientFilter")) {
      this._filteredClients = this.computeFilteredClients();
    }

    if (modelChanged || changed.has("_deviceFilter")) {
      this._filteredDevices = this.computeFilteredDevices();
    }

    if (modelChanged && this._model) {
      const model = this._model;
      this._pendingUpdateCount = model.devices.reduce(
        (count, device) => count + (this.getDeviceMeta(device).pendingUpdate ? 1 : 0),
        0
      );
      if (!this._selection || !this.selectionExists(this._selection)) {
        const device = model.devices[0];
        const client = this._filteredClients[0];
        this._selection = device ? { kind: "device", key: device.key } : client ? { kind: "client", key: client.key } : undefined;
      }
    }

    if (modelChanged || configChanged || changed.has("_clientFilter") || changed.has("_deviceFilter")) {
      this.refreshVisibleLists();
    }

    if (modelChanged || changed.has("_selection")) {
      this.refreshSelectedRecords();
    }
  }

  private refreshVisibleLists(): void {
    this._visibleDevices = this._filteredDevices.slice(0, this._config?.device_limit ?? this._filteredDevices.length);
    this._visibleClients = this._filteredClients.slice(0, this._config?.client_limit ?? this._filteredClients.length);
  }

  private refreshSelectedRecords(): void {
    if (!this._model || !this._selection) {
      this._selectedDevice = undefined;
      this._selectedClient = undefined;
      return;
    }

    this._selectedDevice = this._selection.kind === "device" ? this._model.deviceByKey.get(this._selection.key) : undefined;
    this._selectedClient = this._selection.kind === "client" ? this._model.clientByKey.get(this._selection.key) : undefined;
  }

  protected override render() {
    if (!this._config) {
      return html`<ha-card><div class="empty">Card is not configured.</div></ha-card>`;
    }
    if (!this._model) {
      return html`<ha-card><div class="empty">Waiting for Omada MQTT entities.</div></ha-card>`;
    }
    return html`
      <ha-card>
        <div class="frame">
          <section class="header">
            <div class="panel brand">
              <div class="brand-logo">${unsafeSVG(this.logoSvg)}</div>
            </div>
            <div class="panel header-right">
              <div class="chips">${this.renderSummaryChips()}</div>
              <div class="link-grid">
                <div class="panel table-card">${this.renderIspBlock()}</div>
                <div class="panel table-card">${this.renderVpnBlock()}</div>
              </div>
            </div>
          </section>
          <section class="content">
            <div class="panel list-panel">${this.renderDeviceList()}</div>
            <div class="panel detail-panel">${this.renderDetail()}</div>
            <div class="panel list-panel">${this.renderClientList()}</div>
          </section>
        </div>
      </ha-card>
    `;
  }

  private computeFilteredClients(filter = this._clientFilter): ClientRecord[] {
    const clients = this._model?.clients ?? [];
    const filtered =
      filter === "wireless"
        ? clients.filter((client) => client.wireless)
        : filter === "wired"
          ? clients.filter((client) => !client.wireless)
          : clients;

    return filtered
      .map((client) => ({
        client,
        liveRate: this.getClientMeta(client).liveRate,
        signal: client.metrics.omada_client_signal_pct ?? 0
      }))
      .sort((left, right) => {
        const rateDelta = right.liveRate - left.liveRate;
        if (rateDelta !== 0) {
          return rateDelta;
        }

        const signalDelta = right.signal - left.signal;
        if (signalDelta !== 0) {
          return signalDelta;
        }

        return left.client.name.localeCompare(right.client.name);
      })
      .map(({ client }) => client);
  }

  private computeFilteredDevices(filter = this._deviceFilter): DeviceRecord[] {
    const devices = this._model?.devices ?? [];
    if (filter === "all") {
      return devices;
    }
    return devices.filter((device) => device.type === filter);
  }

  private setDeviceFilter(filter: "all" | "controller" | "gateway" | "switch" | "ap"): void {
    if (this._deviceFilter === filter) {
      return;
    }

    const nextDevices = this.computeFilteredDevices(filter);
    this._deviceFilter = filter;

    if (this._selection?.kind === "device" && !nextDevices.some((device) => device.key === this._selection?.key)) {
      const fallback = nextDevices[0];
      if (fallback) {
        this.selectDevice(fallback.key);
      }
    }
  }

  private setClientFilter(filter: "all" | "wireless" | "wired"): void {
    if (this._clientFilter === filter) {
      return;
    }

    const nextClients = this.computeFilteredClients(filter);
    this._clientFilter = filter;

    if (this._selection?.kind === "client" && !nextClients.some((client) => client.key === this._selection?.key)) {
      const fallback = nextClients[0];
      if (fallback) {
        this.selectClient(fallback.key);
      }
    }
  }

  private get logoSvg(): string {
    const mode = this._config?.logo_mode ?? "auto";
    const darkMode = this.hass?.themes?.darkMode ?? true;
    if (mode === "light") {
      return logoLight;
    }
    if (mode === "dark") {
      return logoDark;
    }
    return darkMode ? logoDark : logoLight;
  }

  private selectionExists(selection: Selection): boolean {
    if (!this._model) {
      return false;
    }
    return selection.kind === "device"
      ? this._model.deviceByKey.has(selection.key)
      : this._model.clientByKey.has(selection.key);
  }

  private selectDevice(key: string): void {
    this._selection = { kind: "device", key };
  }

  private selectClient(key: string): void {
    this._selection = { kind: "client", key };
  }

  private renderSummaryChips() {
    const summary = this._model!.siteSummary;
    const totalClients = summary.wiredClients + summary.wirelessClients;
    return [
      { label: "Clients", value: String(totalClients), sub: `${summary.wirelessClients} wireless` },
      { label: "Devices", value: String(this._model!.devices.length), sub: `${summary.devicesOnline} online` },
      { label: "Updates", value: String(this._pendingUpdateCount), sub: "Devices pending" },
      { label: "Peak CPU", value: formatPercent(summary.maxCpu), sub: summary.maxCpuDevice || "-" },
      { label: "Peak RAM", value: formatPercent(summary.maxMem), sub: summary.maxMemDevice || "-" },
      { label: "VPN", value: String(this._model!.vpns.length), sub: this.vpnPeerCountLabel() }
    ].map(
      (chip) => html`
        <div class="chip">
          <div class="chip-copy">
            <div class="chip-label">${chip.label}</div>
            <div class="chip-sub">${chip.sub}</div>
          </div>
          <div class="chip-value">${chip.value}</div>
        </div>
      `
    );
  }

  private renderIspBlock() {
    return html`
      <div class="section-title"><span>ISP / WAN</span><span>${this._model!.isps.length} links</span></div>
      <div class="table">
        <table>
          <thead>
            <tr>
              <th>ISP</th>
              <th>IP</th>
              <th>Status</th>
              <th>Latency</th>
              <th>Link</th>
              <th>RX / TX</th>
            </tr>
          </thead>
          <tbody>
            ${repeat(this._model!.isps, (row) => row.key, (row) => {
              const wan = this.findWanFor(row);
              const ispName = this.ispDisplayName(row, wan);
              const isUp = (row.metrics.omada_isp_status ?? wan?.metrics.omada_wan_status ?? 0) > 0;
              const latency = wan?.metrics.omada_wan_latency ?? 0;
              const speed = wan?.metrics.omada_wan_link_speed_mbps ?? 0;
              const rx = (wan?.metrics.omada_wan_rx_rate ?? 0) * 1024;
              const tx = (wan?.metrics.omada_wan_tx_rate ?? 0) * 1024;
              return html`
                <tr>
                  <td>${ispName}</td>
                  <td>${String(row.attrs.ip ?? "-")}</td>
                  <td><span class="status-dot ${isUp ? "status-up" : "status-down"}"></span>${isUp ? "Online" : "Offline"}</td>
                  <td>${formatLatency(latency)}</td>
                  <td>${formatSpeedMbps(speed)}</td>
                  <td>${formatRateBytes(rx)} / ${formatRateBytes(tx)}</td>
                </tr>
              `;
            })}
          </tbody>
        </table>
      </div>
    `;
  }

  private renderVpnBlock() {
    const showPeers = this._config?.show_vpn_peers !== false;
    const vpnPeerCount = showPeers ? this._model!.vpnPeers.length : 0;
    return html`
      <div class="section-title"><span>VPN</span><span>${this._model!.vpns.length} tunnels${vpnPeerCount ? ` / ${vpnPeerCount} peers` : ""}</span></div>
      <div class="table">
        <table>
          <thead>
            <tr>
              <th>VPN</th>
              <th>Remote / Allowed</th>
              <th>Mode</th>
              <th>Status</th>
              <th>Uptime</th>
              <th>Total</th>
            </tr>
          </thead>
          <tbody>
            ${repeat(this._model!.vpns, (row) => row.key, (row) => {
              const isUp = (row.metrics.omada_vpn_status ?? 0) > 0;
              const uptime = vpnUptimeSeconds(row);
              const total = vpnTotalBytes(row);
              const remoteIp = vpnRemoteLabel(row);
              return html`
                <tr>
                  <td>${row.name}</td>
                  <td>${remoteIp}</td>
                  <td>${vpnModeLabel(row)}</td>
                  <td><span class="status-dot ${isUp ? "status-up" : "status-down"}"></span>${isUp ? "Online" : "Offline"}</td>
                  <td>${formatUptimeSeconds(uptime)}</td>
                  <td>${formatBytes(total)}</td>
                </tr>
              `;
            })}
            ${showPeers ? repeat(this._model!.vpnPeers, (row) => row.key, (row) => {
              const statusMetric = row.metrics.omada_site_to_site_vpn_peer_status;
              const login = vpnPeerLoginSeconds(row);
              const isUp = statusMetric == null ? login > 0 : statusMetric > 0;
              const total = vpnTotalBytes(row);
              const remoteIp = vpnRemoteLabel(row);
              return html`
                <tr>
                  <td>${String(row.attrs.name ?? "-")} / ${row.name}</td>
                  <td>${remoteIp}</td>
                  <td>${vpnModeLabel(row)}</td>
                  <td><span class="status-dot ${isUp ? "status-up" : "status-down"}"></span>${isUp ? "Online" : "Offline"}</td>
                  <td>${this.formatVpnPeerLogin(login)}</td>
                  <td>${formatBytes(total)}</td>
                </tr>
              `;
            }) : nothing}
          </tbody>
        </table>
      </div>
    `;
  }

  private vpnPeerCountLabel(): string {
    const peers = this._model?.vpnPeers.length ?? 0;
    return peers ? `${peers} peers` : "Discovered tunnels";
  }

  private formatVpnPeerLogin(value: number): string {
    if (!value) {
      return "-";
    }

    return new Date(value * 1000).toLocaleString();
  }

  private renderDeviceList() {
    const devices = this._visibleDevices;
    return html`
      <div class="list-shell">
        <div class="list-toolbar">
          <div class="section-title">Devices</div>
          <div class="row-subtitle">${this._filteredDevices.length} shown</div>
        </div>
        <div class="pill-row" role="group" aria-label="Device filters">
          ${this.renderDeviceFilterPill("all", `All (${this._model!.devices.length})`)}
          ${this.renderDeviceFilterPill("controller", `Controller (${this._model!.siteSummary.controllers})`)}
          ${this.renderDeviceFilterPill("gateway", `Gateway (${this._model!.siteSummary.gateways})`)}
          ${this.renderDeviceFilterPill("switch", `Switch (${this._model!.siteSummary.switches})`)}
          ${this.renderDeviceFilterPill("ap", `AP (${this._model!.siteSummary.aps})`)}
        </div>
        <div class="list-scroll">
          ${repeat(devices, (device) => device.key, (device) => {
            const selected = this._selection?.kind === "device" && this._selection.key === device.key;
            const cpu = device.metrics.omada_device_cpu_percentage ?? 0;
            const mem = device.metrics.omada_device_mem_percentage ?? 0;
            const isUp = isDeviceConnected(device.status);
            const meta = this.getDeviceMeta(device);
            return html`
              <button
                type="button"
                class="card-row ${selected ? "selected" : ""}"
                aria-label=${inspectionAriaLabel("device", device.name)}
                aria-pressed=${selected}
                @click=${() => this.selectDevice(device.key)}
              >
                <span class="row-top">
                  <span>
                    <span class="row-title">${device.name}</span>
                    <span class="row-subtitle">${device.model || device.type}</span>
                  </span>
                  <span class="metric-tag"><span class="status-dot ${isUp ? "status-up" : "status-down"}"></span>${isUp ? "Online" : "Offline"}</span>
                </span>
                <span class="metric-group">
                  <span class="metric-tag">${device.type}</span>
                  <span class="metric-tag">CPU ${formatPercent(cpu)}</span>
                  <span class="metric-tag">RAM ${formatPercent(mem)}</span>
                  ${meta.pendingUpdate ? html`<span class="metric-tag">Update ${meta.updateTarget || "pending"}</span>` : nothing}
                  <span class="metric-tag">${device.clients.length} clients</span>
                </span>
                <span class="row-bottom">
                  <span class="row-subtitle">${device.ip || "No IP"} · ${device.version || "n/a"}</span>
                  <span class="row-subtitle">${device.ports.length} ports</span>
                </span>
              </button>
            `;
          })}
        </div>
      </div>
    `;
  }

  private renderDeviceFilterPill(
    filter: "all" | "controller" | "gateway" | "switch" | "ap",
    label: string
  ) {
    return html`
      <button
        type="button"
        class="mini-pill ${this._deviceFilter === filter ? "active" : ""}"
        aria-pressed=${this._deviceFilter === filter}
        @click=${() => this.setDeviceFilter(filter)}
      >
        ${label}
      </button>
    `;
  }

  private renderClientList() {
    const clients = this._visibleClients;
    return html`
      <div class="list-shell">
        <div class="list-toolbar">
          <div class="section-title">Clients</div>
          <div class="row-subtitle">${this._filteredClients.length} shown</div>
        </div>
        <div class="pill-row" role="group" aria-label="Client filters">
          ${this.renderClientFilterPill("all", "All")}
          ${this.renderClientFilterPill("wireless", "Wireless")}
          ${this.renderClientFilterPill("wired", "Wired")}
        </div>
        <div class="list-scroll">
          ${repeat(clients, (client) => client.key, (client) => {
            const selected = this._selection?.kind === "client" && this._selection.key === client.key;
            const signal = client.metrics.omada_client_signal_pct ?? 0;
            const rssi = client.metrics.omada_client_rssi_dbm ?? 0;
            const meta = this.getClientMeta(client);
            const attachment = meta.attachmentLabel;
            return html`
              <button
                type="button"
                class="card-row ${selected ? "selected" : ""}"
                aria-label=${inspectionAriaLabel("client", client.name)}
                aria-pressed=${selected}
                @click=${() => this.selectClient(client.key)}
              >
                <span class="row-top">
                  <span>
                    <span class="row-title">${client.name}</span>
                    <span class="row-subtitle">${attachment}${client.attachmentPort && !client.wireless ? ` · port ${client.attachmentPort}` : ""}</span>
                  </span>
                  <span class="metric-tag">${client.wireless ? "Wireless" : "Wired"}</span>
                </span>
                <span class="metric-group">
                  ${client.ssid ? html`<span class="metric-tag">${client.ssid}</span>` : nothing}
                  ${client.vendor ? html`<span class="metric-tag">${client.vendor}</span>` : nothing}
                  <span class="metric-tag">${meta.liveRateLabel}</span>
                  <span class="metric-tag">${meta.quality}</span>
                  <span class="metric-tag">${rssi ? `${rssi} dBm` : "n/a"}</span>
                </span>
                <span class="signal-bar"><span style="width:${Math.max(0, Math.min(signal, 100))}%"></span></span>
                <span class="row-bottom">
                  <span class="row-subtitle">${client.ip || "No IP"} · VLAN ${client.vlanId || "-"}</span>
                  <span class="row-subtitle">${meta.rateBreakdown}</span>
                </span>
              </button>
            `;
          })}
        </div>
      </div>
    `;
  }

  private renderClientFilterPill(filter: "all" | "wireless" | "wired", label: string) {
    return html`
      <button
        type="button"
        class="mini-pill ${this._clientFilter === filter ? "active" : ""}"
        aria-pressed=${this._clientFilter === filter}
        @click=${() => this.setClientFilter(filter)}
      >
        ${label}
      </button>
    `;
  }

  private renderDetail() {
    if (!this._model || !this._selection) {
      return html`<div class="empty">Select a device or client to inspect it.</div>`;
    }
    if (this._selection.kind === "device") {
      return this._selectedDevice ? this.renderDeviceDetail(this._selectedDevice) : html`<div class="empty">Device not found.</div>`;
    }
    return this._selectedClient ? this.renderClientDetail(this._selectedClient) : html`<div class="empty">Client not found.</div>`;
  }

  private renderDeviceDetail(device: DeviceRecord) {
    const meta = this.getDeviceMeta(device);
    const cpu = device.metrics.omada_device_cpu_percentage ?? 0;
    const mem = device.metrics.omada_device_mem_percentage ?? 0;
    const uptime =
      device.metrics.omada_device_uptime_seconds ??
      device.metrics.omada_controller_uptime_seconds ??
      device.metrics.omada_device_uptime ??
      0;
    const rx = device.metrics.omada_device_rx_rate ?? 0;
    const download = device.metrics.omada_device_download ?? 0;
    const pendingUpdate = meta.pendingUpdate;
    const updateTarget = meta.updateTarget;
    const poeBudget = meta.poeBudget;
    return html`
      <div class="detail-shell">
        <div class="detail-hero">
          <div class="detail-card">
            <div class="detail-title">
              <div>
                <div class="detail-name">${device.name}</div>
                <div class="detail-sub">${device.model || device.type} · ${device.ip || "No IP"} · ${device.status || "Unknown"}</div>
              </div>
              <div class="metric-group">
                <span class="metric-tag">${device.type}</span>
                <span class="metric-tag">${device.version || "No version"}</span>
                ${pendingUpdate ? html`<span class="metric-tag">Update ${updateTarget || "pending"}</span>` : nothing}
              </div>
            </div>
            <div class="detail-stats">
              ${this.renderDetailStat("CPU", formatPercent(cpu))}
              ${this.renderDetailStat("RAM", formatPercent(mem))}
              ${this.renderDetailStat("Uptime", formatUptimeSeconds(uptime))}
              ${this.renderDetailStat("Clients", String(device.clients.length))}
            </div>
          </div>
          <div class="detail-card">
            <div class="section-title">Quick Read</div>
            <div class="detail-stats">
              ${this.renderDetailStat("Ports", String(device.ports.length))}
              ${this.renderDetailStat("RX Rate", formatRateBytes(rx))}
              ${poeBudget
                ? this.renderDetailStat("PoE Used", this.formatWatts(poeBudget.used))
                : this.renderDetailStat("Traffic", formatBytes(download))}
              ${poeBudget
                ? this.renderDetailStat("PoE Left", this.formatWatts(poeBudget.remaining))
                : this.renderDetailStat(pendingUpdate ? "Update" : "PoE", pendingUpdate ? (updateTarget || "Pending") : String(meta.poePortCount))}
            </div>
          </div>
        </div>
        <div class="chart-stack">
          <div class="chart-card"><div class="section-title">Health Profile</div><div class="chart" data-chart="detail-primary"></div></div>
          <div class="chart-card"><div class="section-title">${this.deviceSecondaryTitle(device)}</div><div class="chart" data-chart="detail-secondary"></div></div>
        </div>
        <div class="detail-bottom">
          <div class="panel table-card">
            <div class="section-title"><span>Ports</span><span>${device.ports.length}</span></div>
            <div class="table">
              <table>
                <thead><tr><th>Port</th><th>Status</th><th>Speed</th><th>PoE</th></tr></thead>
                <tbody>
                  ${repeat(meta.portsPreview, (port) => port.key, (port) => {
                    const speed = port.metrics.omada_port_link_speed_mbps ?? 0;
                    const isUp = port.status === "Connected";
                    return html`
                      <tr>
                        <td>${port.name}</td>
                        <td><span class="status-dot ${isUp ? "status-up" : "status-down"}"></span>${port.status || "-"}</td>
                        <td>${formatSpeedMbps(speed)}</td>
                        <td>${this.portPoeLabel(port)}</td>
                      </tr>
                    `;
                  })}
                </tbody>
              </table>
            </div>
          </div>
          <div class="panel table-card">
            <div class="section-title"><span>Attached Clients</span><span>${device.clients.length}</span></div>
            <div class="table tight clickable">
              <table>
                <thead><tr><th class="col-name">Name</th><th class="col-ip">IP</th><th class="col-signal">Signal</th><th class="col-path">Path</th></tr></thead>
                <tbody>
                  ${repeat(meta.clientPreview, (client) => client.key, (client) => {
                    const clientMeta = this.getClientMeta(client);
                    return html`<tr @click=${() => this.selectClient(client.key)}>
                      <td class="col-name" title=${client.name}>
                        <button
                          type="button"
                          class="client-row-action"
                          aria-label=${inspectionAriaLabel("client", client.name)}
                          @click=${(event: Event) => {
                            event.stopPropagation();
                            this.selectClient(client.key);
                          }}
                        >${client.name}</button>
                      </td>
                      <td class="col-ip" title=${client.ip || "-"}>${client.ip || "-"}</td>
                      <td class="col-signal">${client.wireless ? formatPercent(client.metrics.omada_client_signal_pct ?? 0) : "-"}</td>
                      <td class="col-path" title=${client.wireless ? clientMeta.bandLabel : clientMeta.wiredPathLabel}>${client.wireless ? clientMeta.bandLabel : clientMeta.wiredPathLabel}</td>
                    </tr>
                  `;})}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>
    `;
  }

  private renderClientDetail(client: ClientRecord) {
    const meta = this.getClientMeta(client);
    const signal = client.metrics.omada_client_signal_pct ?? 0;
    const rssi = client.metrics.omada_client_rssi_dbm ?? 0;
    const rx = client.metrics.omada_client_rx_rate ?? 0;
    const tx = client.metrics.omada_client_tx_rate ?? 0;
    const downActivity = client.metrics.omada_client_download_activity_bytes ?? 0;
    const upActivity = client.metrics.omada_client_upload_activity_bytes ?? 0;
    const totalTraffic = (client.metrics.omada_client_traffic_down_bytes ?? 0) + (client.metrics.omada_client_traffic_up_bytes ?? 0);
    const pathMetricLabel = client.wireless ? "Band" : "Path";
    const pathMetricValue = client.wireless ? meta.bandLabel : meta.wiredPathLabel;
    const wiredLinkSpeed = meta.wiredLinkSpeed;
    return html`
      <div class="detail-shell">
        <div class="detail-hero">
          <div class="detail-card">
            <div class="detail-title">
              <div>
                <div class="detail-name">${client.name}</div>
                <div class="detail-sub">${client.ip || "No IP"} · ${client.vendor || "Unknown vendor"} · ${client.wireless ? "Wireless" : "Wired"}</div>
              </div>
              <div class="metric-group">
                ${client.ssid ? html`<span class="metric-tag">${client.ssid}</span>` : nothing}
                ${client.wifiMode ? html`<span class="metric-tag">${client.wifiMode}</span>` : nothing}
              </div>
            </div>
            <div class="detail-stats ${client.wireless ? "" : "detail-stats-wired"}">
              ${client.wireless
                ? this.renderDetailStat("Signal", formatPercent(signal))
                : this.renderDetailStat("Link", wiredLinkSpeed)}
              ${client.wireless
                ? this.renderDetailStat("RSSI", rssi ? `${rssi} dBm` : "-")
                : this.renderDetailStat("Download", downActivity ? formatRateBytes(downActivity) : formatRateBits(rx))}
              ${client.wireless
                ? this.renderDetailStat("RX", formatRateBits(rx))
                : this.renderDetailStat("Upload", upActivity ? formatRateBytes(upActivity) : formatRateBits(tx))}
              ${client.wireless ? this.renderDetailStat("TX", formatRateBits(tx)) : nothing}
            </div>
          </div>
          <div class="detail-card">
            <div class="section-title">Path</div>
            <div class="path-layout">
              <div class="path-main">
                ${this.renderDetailStat("Attachment", client.wireless ? client.apName || "-" : meta.attachmentLabel)}
              </div>
              <div class="path-grid">
                ${this.renderDetailStat(pathMetricLabel, pathMetricValue)}
                ${this.renderDetailStat("VLAN", client.vlanId || "-")}
                ${this.renderDetailStat("Traffic", formatBytes(totalTraffic))}
              </div>
            </div>
          </div>
        </div>
        <div class="chart-stack">
          <div class="chart-card"><div class="section-title">${client.wireless ? "Link Quality" : "Connection"}</div><div class="chart" data-chart="detail-primary"></div></div>
          <div class="chart-card"><div class="section-title">Live Activity</div><div class="chart" data-chart="detail-secondary"></div></div>
        </div>
        <div class="detail-bottom">
          <div class="panel table-card">
            <div class="section-title"><span>Client Attributes</span><span>${client.wireless ? meta.quality : "Wired"}</span></div>
            <div class="table">
              <table><tbody>
                ${this.attributeRow("MAC", client.mac)}
                ${this.attributeRow("Host", client.hostName)}
                ${this.attributeRow("Vendor", client.vendor)}
                ${this.attributeRow("Category", client.category)}
                ${this.attributeRow("Type", client.clientType)}
                ${client.wireless ? this.attributeRow("SSID", client.ssid) : nothing}
                ${client.wireless ? this.attributeRow("AP", client.apName) : this.attributeRow("Switch", client.switchName)}
                ${client.wireless ? this.attributeRow("Band", meta.bandLabel) : this.attributeRow("Gateway", client.gatewayName)}
                ${!client.wireless ? this.attributeRow("Attachment port", client.attachmentPort) : nothing}
              </tbody></table>
            </div>
          </div>
          <div class="panel table-card">
            <div class="section-title"><span>Traffic + Link Metrics</span><span>${client.wireless ? "Wireless path" : "Wired path"}</span></div>
            <div class="table">
              <table><tbody>
                ${this.attributeRow("Download activity", formatRateBytes(downActivity))}
                ${this.attributeRow("Upload activity", formatRateBytes(upActivity))}
                ${this.attributeRow("RX rate", formatRateBits(rx))}
                ${this.attributeRow("TX rate", formatRateBits(tx))}
                ${this.attributeRow("Traffic down", formatBytes(client.metrics.omada_client_traffic_down_bytes ?? 0))}
                ${this.attributeRow("Traffic up", formatBytes(client.metrics.omada_client_traffic_up_bytes ?? 0))}
                ${client.wireless ? this.attributeRow("Signal", formatPercent(signal)) : this.attributeRow("Connection", meta.wiredConnectionLabel)}
                ${client.wireless ? this.attributeRow("RSSI", rssi ? `${rssi} dBm` : "-") : this.attributeRow("Link speed", wiredLinkSpeed)}
                ${!client.wireless && meta.lagPorts ? this.attributeRow("LAG ports", meta.lagPorts) : nothing}
                ${this.attributeRow("VLAN", client.vlanId)}
              </tbody></table>
            </div>
          </div>
        </div>
      </div>
    `;
  }

  private renderDetailStat(label: string, value: string) {
    return html`<div class="detail-stat"><div class="detail-stat-label">${label}</div><div class="detail-stat-value">${value}</div></div>`;
  }

  private attributeRow(label: string, value: string) {
    return html`<tr><th>${label}</th><td>${value || "-"}</td></tr>`;
  }

  private getDeviceMeta(device: DeviceRecord): DeviceMeta {
    const cached = this._deviceMeta.get(device);
    if (cached) {
      return cached;
    }

    const pendingUpdate = this.deviceHasPendingUpdate(device);
    const updateTarget = this.deviceUpdateTarget(device);
    const poeBudget = this.devicePoeBudget(device);
    const poePortCount = device.ports.reduce((count, port) => count + (port.poe ? 1 : 0), 0);
    const connectedPorts = device.ports.reduce((count, port) => count + (port.status === "Connected" ? 1 : 0), 0);
    const uplinkMbps = device.ports.reduce((max, port) => Math.max(max, port.metrics.omada_port_link_speed_mbps ?? 0), 0);
    const radioMetrics = [
      ["2.4 GHz RX", "omada_device_2g_rx_util"],
      ["2.4 GHz TX", "omada_device_2g_tx_util"],
      ["5 GHz RX", "omada_device_5g_rx_util"],
      ["5 GHz TX", "omada_device_5g_tx_util"],
      ["5 GHz-2 RX", "omada_device_5g2_rx_util"],
      ["5 GHz-2 TX", "omada_device_5g2_tx_util"],
      ["6 GHz RX", "omada_device_6g_rx_util"],
      ["6 GHz TX", "omada_device_6g_tx_util"]
    ] satisfies Array<[string, string]>;
    const radioRows = radioMetrics
      .map(([label, metric]) => ({ label, value: device.metrics[metric] ?? -1 }))
      .filter((row) => row.value >= 0);
    const topPorts = device.ports
      .slice()
      .sort((left, right) => (right.metrics.omada_port_link_speed_mbps ?? 0) - (left.metrics.omada_port_link_speed_mbps ?? 0))
      .slice(0, 12);
    const portsPreview = device.ports.slice(0, 18);
    const clientPreview = device.clients.slice(0, 18);

    const meta = {
      pendingUpdate,
      updateTarget,
      poeBudget,
      poePortCount,
      connectedPorts,
      uplinkMbps,
      radioRows,
      topPorts,
      portsPreview,
      clientPreview
    };
    this._deviceMeta.set(device, meta);
    return meta;
  }

  private getClientMeta(client: ClientRecord): ClientMeta {
    const cached = this._clientMeta.get(client);
    if (cached) {
      return cached;
    }

    const liveRate = this.clientLiveRate(client);
    const meta = {
      liveRate,
      liveRateLabel: this.clientLiveRateLabel(client),
      rateBreakdown: this.clientRateBreakdown(client),
      bandLabel: this.clientBandLabel(client),
      wiredPathLabel: this.wiredPathLabel(client),
      wiredConnectionLabel: this.wiredConnectionLabel(client),
      wiredLinkSpeed: this.wiredClientLinkSpeed(client),
      lagPorts: this.wiredLagPorts(client),
      attachmentLabel: client.wireless ? client.apName || "AP" : client.switchName || client.gatewayName || "Wired",
      quality: client.wireless
        ? qualityLabel(client.metrics.omada_client_signal_pct ?? 0, client.metrics.omada_client_rssi_dbm ?? 0)
        : "Wired"
    };
    this._clientMeta.set(client, meta);
    return meta;
  }

  private deviceHasPendingUpdate(device: DeviceRecord): boolean {
    if ((device.metrics.omada_device_need_upgrade ?? 0) > 0) {
      return true;
    }

    if ((device.metrics.omada_controller_upgrade_available ?? 0) > 0) {
      return true;
    }

    const versionUpgrade = String(device.attrs.device_version_upgrade ?? "").trim();
    const currentVersion = String(device.version ?? "").trim();
    return Boolean(versionUpgrade && currentVersion && versionUpgrade !== currentVersion);
  }

  private deviceUpdateTarget(device: DeviceRecord): string {
    const versionUpgrade = String(device.attrs.device_version_upgrade ?? "").trim();
    const currentVersion = String(device.version ?? "").trim();
    if (versionUpgrade && versionUpgrade !== currentVersion) {
      return versionUpgrade;
    }

    return "";
  }

  private devicePoeBudget(device: DeviceRecord): { used: number; remaining: number; total: number } | undefined {
    const remaining = device.metrics.omada_device_poe_remain_watts ?? 0;
    const used = device.ports.reduce((sum, port) => sum + (port.metrics.omada_port_power_watts ?? 0), 0);
    const hasBudget =
      String(device.attrs.device_poe_support ?? "").toLowerCase() === "yes" ||
      remaining > 0 ||
      used > 0;

    if (!hasBudget) {
      return undefined;
    }

    return {
      used,
      remaining,
      total: used + remaining
    };
  }

  private formatWatts(value: number): string {
    if (!Number.isFinite(value) || value <= 0) {
      return "0 W";
    }

    return `${value >= 10 ? value.toFixed(0) : value.toFixed(1)} W`;
  }

  private wiredPathLabel(client: ClientRecord): string {
    const lagId = client.attachmentLagId;
    if (lagId && lagId !== "0") {
      const parent = client.switchName || client.gatewayName || "parent device";
      return `${parent} / LAG ${lagId}`;
    }

    return client.attachmentPort ? `Port ${client.attachmentPort}` : "-";
  }

  private wiredLagPorts(client: ClientRecord): string {
    return client.attachmentLagPorts;
  }

  private wiredClientLinkSpeed(client: ClientRecord): string {
    const lagId = client.attachmentLagId;
    if (lagId && lagId !== "0") {
      const lagSpeed = client.attachmentLinkSpeedMbps ?? 0;
      return formatSpeedMbps(lagSpeed);
    }

    const port = this.clientAttachmentPort(client);
    if (port) {
      const portSpeed =
        port.metrics.omada_port_link_speed_mbps ??
        Number(port.attrs.link_speed ?? port.attrs.max_speed ?? 0);
      return formatSpeedMbps(portSpeed);
    }

    return "-";
  }

  private clientAttachmentPort(client: ClientRecord): DeviceRecord["ports"][number] | undefined {
    if (!this._model) {
      return undefined;
    }

    const deviceMac = client.switchMac || client.gatewayMac;
    if (!deviceMac || !client.attachmentPort) {
      return undefined;
    }

    return this._model.portByDeviceMacAndPort.get(`${normalizeMacKey(deviceMac)}:${client.attachmentPort}`);
  }

  private clientLiveRate(client: ClientRecord): number {
    const downloadActivity = client.metrics.omada_client_download_activity_bytes ?? 0;
    const uploadActivity = client.metrics.omada_client_upload_activity_bytes ?? 0;
    const activityRate = downloadActivity + uploadActivity;
    if (activityRate > 0) {
      return activityRate * 8;
    }

    return (client.metrics.omada_client_rx_rate ?? 0) + (client.metrics.omada_client_tx_rate ?? 0);
  }

  private clientLiveRateLabel(client: ClientRecord): string {
    const downloadActivity = client.metrics.omada_client_download_activity_bytes ?? 0;
    const uploadActivity = client.metrics.omada_client_upload_activity_bytes ?? 0;
    const activityRate = downloadActivity + uploadActivity;
    if (activityRate > 0) {
      return formatRateBytes(activityRate);
    }

    const negotiationRate = (client.metrics.omada_client_rx_rate ?? 0) + (client.metrics.omada_client_tx_rate ?? 0);
    return formatRateBits(negotiationRate);
  }

  private clientRateBreakdown(client: ClientRecord): string {
    const downloadActivity = client.metrics.omada_client_download_activity_bytes ?? 0;
    const uploadActivity = client.metrics.omada_client_upload_activity_bytes ?? 0;

    if (downloadActivity > 0 || uploadActivity > 0) {
      return `${formatRateBytes(downloadActivity)} down / ${formatRateBytes(uploadActivity)} up`;
    }

    const rx = client.metrics.omada_client_rx_rate ?? 0;
    const tx = client.metrics.omada_client_tx_rate ?? 0;
    return `${formatRateBits(rx)} down / ${formatRateBits(tx)} up`;
  }

  private findWanFor(row: LinkRow): LinkRow | undefined {
    if (!this._model) {
      return undefined;
    }

    return this._model.wanByName.get(row.name) ?? this._model.wanByPort.get(String(row.attrs.port));
  }

  private ispDisplayName(row: LinkRow, wan?: LinkRow): string {
    return String(wan?.attrs.desc ?? row.attrs.desc ?? row.name ?? "-") || "-";
  }

  private syncCharts(): void {
    if (!this._model) {
      this._chartController.clear();
      return;
    }
    if (this._selection?.kind === "device" && this._selectedDevice) {
      const primarySignature = this.devicePrimarySignature(this._selectedDevice);
      const secondarySignature = this.deviceSecondarySignature(this._selectedDevice);
      this.renderChart(
        "detail-primary",
        this.getCachedChartOption(this._devicePrimaryOptionCache, this._selectedDevice, primarySignature, () =>
          this.buildDevicePrimaryOption(this._selectedDevice!)
        ),
        primarySignature
      );
      this.renderChart(
        "detail-secondary",
        this.getCachedChartOption(this._deviceSecondaryOptionCache, this._selectedDevice, secondarySignature, () =>
          this.buildDeviceSecondaryOption(this._selectedDevice!)
        ),
        secondarySignature
      );
    } else if (this._selection?.kind === "client" && this._selectedClient) {
      const primarySignature = this.clientPrimarySignature(this._selectedClient);
      const secondarySignature = this.clientSecondarySignature(this._selectedClient);
      this.renderChart(
        "detail-primary",
        this.getCachedChartOption(this._clientPrimaryOptionCache, this._selectedClient, primarySignature, () =>
          this.buildClientPrimaryOption(this._selectedClient!)
        ),
        primarySignature
      );
      this.renderChart(
        "detail-secondary",
        this.getCachedChartOption(this._clientSecondaryOptionCache, this._selectedClient, secondarySignature, () =>
          this.buildClientSecondaryOption(this._selectedClient!)
        ),
        secondarySignature
      );
    } else {
      this._chartController.clear();
    }
  }

  private getCachedChartOption<T extends object>(
    cache: WeakMap<T, ChartOptionCacheEntry>,
    key: T,
    signature: string,
    build: () => EChartsOption
  ): EChartsOption {
    const cached = cache.get(key);
    if (cached && cached.signature === signature) {
      return cached.option;
    }

    const option = build();
    cache.set(key, { signature, option });
    return option;
  }

  private renderChart(key: string, option: EChartsOption, signature: string): void {
    const element = this.renderRoot.querySelector<HTMLElement>(`[data-chart="${key}"]`);
    if (!element) {
      return;
    }
    this._chartController.render(key, element, option, signature);
  }

  private buildDevicePrimaryOption(device: DeviceRecord): EChartsOption {
    const meta = this.getDeviceMeta(device);
    const portLoad = device.ports.length ? (meta.connectedPorts / device.ports.length) * 100 : 0;
    const clientDensity = Math.min(device.clients.length * 12, 100);
    const uplink = Math.min(meta.uplinkMbps / 100, 100);
    return {
      aria: chartAria(
        `${device.name} health: CPU ${formatPercent(device.metrics.omada_device_cpu_percentage ?? 0)}, ` +
        `memory ${formatPercent(device.metrics.omada_device_mem_percentage ?? 0)}, ` +
        `${meta.connectedPorts} connected ports and ${device.clients.length} clients.`
      ),
      radar: {
        radius: "63%",
        indicator: [
          { name: "CPU", max: 100 },
          { name: "RAM", max: 100 },
          { name: "Ports", max: 100 },
          { name: "Clients", max: 100 },
          { name: "Uplink", max: 100 }
        ],
        splitLine: { lineStyle: { color: "rgba(255,255,255,0.08)" } },
        splitArea: { areaStyle: { color: ["rgba(255,255,255,0.02)", "rgba(255,255,255,0.035)"] } },
        axisName: { color: "#97aac0" }
      },
      series: [{
        type: "radar",
        symbolSize: 6,
        lineStyle: { width: 2, color: "#54d1ff" },
        itemStyle: { color: "#54d1ff" },
        areaStyle: { color: "rgba(84, 209, 255, 0.22)" },
        data: [{ value: [device.metrics.omada_device_cpu_percentage ?? 0, device.metrics.omada_device_mem_percentage ?? 0, portLoad, clientDensity, uplink] }]
      }]
    };
  }

  private buildDeviceSecondaryOption(device: DeviceRecord): EChartsOption {
    const meta = this.getDeviceMeta(device);
    if (device.type === "ap") {
      const rows = meta.radioRows;
      if (rows.length) {
        return {
          aria: chartAria(`${device.name} radio utilization: ${rows.map((row) => `${row.label} ${formatPercent(row.value)}`).join(", ")}.`),
          grid: { top: 10, left: 14, right: 18, bottom: 12, containLabel: true },
          xAxis: {
            type: "value",
            max: 100,
            axisLabel: { color: "#97aac0", formatter: (value: number) => `${value}%` },
            splitLine: { lineStyle: { color: "rgba(255,255,255,0.06)" } }
          },
          yAxis: { type: "category", data: rows.map((row) => row.label), axisLabel: { color: "#edf4ff" } },
          series: [{
            type: "bar",
            data: rows.map((row) => ({ value: row.value, itemStyle: { color: "#54d1ff" } })),
            barWidth: 16,
            itemStyle: { borderRadius: 99 }
          }]
        };
      }
    }

    const rows = meta.topPorts;
    if (!rows.length) {
      return {
        aria: chartAria(
          `${device.name} resource summary: CPU ${formatPercent(device.metrics.omada_device_cpu_percentage ?? 0)}, ` +
          `memory ${formatPercent(device.metrics.omada_device_mem_percentage ?? 0)} and ${device.clients.length} clients.`
        ),
        grid: { top: 10, left: 14, right: 18, bottom: 12, containLabel: true },
        xAxis: { type: "value", axisLabel: { color: "#97aac0" } },
        yAxis: { type: "category", data: ["CPU", "RAM", "Clients"], axisLabel: { color: "#edf4ff" } },
        series: [{
          type: "bar",
          data: [
            { value: device.metrics.omada_device_cpu_percentage ?? 0, itemStyle: { color: "#54d1ff" } },
            { value: device.metrics.omada_device_mem_percentage ?? 0, itemStyle: { color: "#ffb648" } },
            { value: Math.min(device.clients.length * 10, 100), itemStyle: { color: "#1eb980" } }
          ],
          barWidth: 18,
          itemStyle: { borderRadius: 99 }
        }]
      };
    }
    return {
      aria: chartAria(
        `${device.name} port link speeds: ${rows
          .map((port) => `${port.name} ${formatSpeedMbps(port.metrics.omada_port_link_speed_mbps ?? 0)}`)
          .join(", ")}.`
      ),
      grid: { top: 10, left: 14, right: 18, bottom: 12, containLabel: true },
      xAxis: {
        type: "value",
        axisLabel: { color: "#97aac0", formatter: (value: number) => value / 1000 >= 1 ? `${(value / 1000).toFixed(1)}G` : `${value}` },
        splitLine: { lineStyle: { color: "rgba(255,255,255,0.06)" } }
      },
      yAxis: { type: "category", data: rows.map((port) => port.name), axisLabel: { color: "#edf4ff" } },
      series: [{
        type: "bar",
        data: rows.map((port) => ({ value: port.metrics.omada_port_link_speed_mbps ?? 0, itemStyle: { color: port.poe ? "#ffb648" : "#54d1ff" } })),
        barWidth: 16,
        itemStyle: { borderRadius: 99 }
      }]
    };
  }

  private buildClientPrimaryOption(client: ClientRecord): EChartsOption {
    if (!client.wireless) {
      return {
        aria: chartAria(
          `${client.name} traffic: receive ${formatRateBits(client.metrics.omada_client_rx_rate ?? 0)}, ` +
          `transmit ${formatRateBits(client.metrics.omada_client_tx_rate ?? 0)}.`
        ),
        grid: { top: 10, left: 18, right: 18, bottom: 20, containLabel: true },
        xAxis: {
          type: "value",
          axisLabel: {
            color: "#97aac0",
            formatter: (value: number) => value >= 1024 * 1024 ? `${(value / (1024 * 1024)).toFixed(1)}M` : value >= 1024 ? `${(value / 1024).toFixed(1)}K` : `${value}`
          },
          splitLine: { lineStyle: { color: "rgba(255,255,255,0.06)" } }
        },
        yAxis: {
          type: "category",
          data: ["RX", "TX", "Traffic Down", "Traffic Up"],
          axisLabel: { color: "#edf4ff" }
        },
        series: [{
          type: "bar",
          data: [
            { value: client.metrics.omada_client_rx_rate ?? 0, itemStyle: { color: "#54d1ff" } },
            { value: client.metrics.omada_client_tx_rate ?? 0, itemStyle: { color: "#1eb980" } },
            { value: client.metrics.omada_client_traffic_down_bytes ?? 0, itemStyle: { color: "#ffb648" } },
            { value: client.metrics.omada_client_traffic_up_bytes ?? 0, itemStyle: { color: "#ff6b7e" } }
          ],
          barWidth: 16,
          itemStyle: { borderRadius: 99 }
        }]
      };
    }

    const meta = this.getClientMeta(client);
    const signal = client.metrics.omada_client_signal_pct ?? 0;
    return {
      aria: chartAria(`${client.name} Wi-Fi signal is ${formatPercent(signal)}, rated ${meta.quality}.`),
      series: [{
        type: "gauge",
        center: ["50%", "58%"],
        radius: "84%",
        min: 0,
        max: 100,
        progress: { show: true, width: 14, itemStyle: { color: signal >= 70 ? "#1eb980" : signal >= 50 ? "#ffb648" : "#ff6b7e" } },
        axisLine: { lineStyle: { width: 14, color: [[1, "rgba(255,255,255,0.08)"]] } },
        axisTick: { show: false },
        splitLine: { show: false },
        axisLabel: { show: false },
        pointer: { show: false },
        anchor: { show: false },
        detail: { valueAnimation: true, offsetCenter: [0, "6%"], color: "#edf4ff", fontSize: 30, formatter: "{value}%" },
        title: { offsetCenter: [0, "42%"], color: "#97aac0", fontSize: 14 },
        data: [{ value: signal, name: meta.quality }]
      }]
    };
  }

  private buildClientSecondaryOption(client: ClientRecord): EChartsOption {
    return {
      aria: chartAria(`${client.name} receive, transmit, download, and upload activity.`),
      grid: { top: 10, left: 18, right: 18, bottom: 20, containLabel: true },
      xAxis: { type: "category", data: ["RX", "TX", "Down act.", "Up act."], axisLabel: { color: "#97aac0" } },
      yAxis: {
        type: "value",
        axisLabel: {
          color: "#97aac0",
          formatter: (value: number) => value >= 1024 * 1024 ? `${(value / (1024 * 1024)).toFixed(1)}M` : value >= 1024 ? `${(value / 1024).toFixed(1)}K` : `${value}`
        },
        splitLine: { lineStyle: { color: "rgba(255,255,255,0.06)" } }
      },
      series: [{
        type: "bar",
        barWidth: 22,
        data: [
          { value: client.metrics.omada_client_rx_rate ?? 0, itemStyle: { color: "#54d1ff" } },
          { value: client.metrics.omada_client_tx_rate ?? 0, itemStyle: { color: "#1eb980" } },
          { value: client.metrics.omada_client_download_activity_bytes ?? 0, itemStyle: { color: "#ffb648" } },
          { value: client.metrics.omada_client_upload_activity_bytes ?? 0, itemStyle: { color: "#ff6b7e" } }
        ],
        itemStyle: { borderRadius: 99 }
      }]
    };
  }

  private devicePrimarySignature(device: DeviceRecord): string {
    const meta = this.getDeviceMeta(device);
    return [
      device.key,
      device.name,
      device.metrics.omada_device_cpu_percentage ?? 0,
      device.metrics.omada_device_mem_percentage ?? 0,
      meta.connectedPorts,
      device.ports.length,
      device.clients.length,
      meta.uplinkMbps
    ].join("|");
  }

  private deviceSecondarySignature(device: DeviceRecord): string {
    const meta = this.getDeviceMeta(device);
    if (device.type === "ap" && meta.radioRows.length) {
      return `ap|${device.name}|${meta.radioRows.map((row) => `${row.label}:${row.value}`).join("|")}`;
    }

    if (!meta.topPorts.length) {
      return deviceResourceSummarySignature(
        device.name,
        device.metrics.omada_device_cpu_percentage ?? 0,
        device.metrics.omada_device_mem_percentage ?? 0,
        device.clients.length
      );
    }

    return `ports|${device.name}|${meta.topPorts.map((port) => `${port.key}:${port.name}:${port.metrics.omada_port_link_speed_mbps ?? 0}:${port.poe ? 1 : 0}`).join("|")}`;
  }

  private clientPrimarySignature(client: ClientRecord): string {
    if (!client.wireless) {
      return [
        client.key,
        client.name,
        client.metrics.omada_client_rx_rate ?? 0,
        client.metrics.omada_client_tx_rate ?? 0,
        client.metrics.omada_client_traffic_down_bytes ?? 0,
        client.metrics.omada_client_traffic_up_bytes ?? 0
      ].join("|");
    }

    return [
      client.key,
      client.name,
      client.metrics.omada_client_signal_pct ?? 0,
      client.metrics.omada_client_rssi_dbm ?? 0
    ].join("|");
  }

  private clientSecondarySignature(client: ClientRecord): string {
    return [
      client.key,
      client.name,
      client.metrics.omada_client_rx_rate ?? 0,
      client.metrics.omada_client_tx_rate ?? 0,
      client.metrics.omada_client_download_activity_bytes ?? 0,
      client.metrics.omada_client_upload_activity_bytes ?? 0
    ].join("|");
  }

  private deviceSecondaryTitle(device: DeviceRecord): string {
    if (device.type === "ap") {
      return this.getDeviceMeta(device).radioRows.length ? "Radio Utilization" : "Port Throughput";
    }
    return "Port Throughput";
  }

  private portPoeLabel(port: DeviceRecord["ports"][number]): string {
    if (!port.poe) {
      return "-";
    }

    const activityLabel = String(port.attrs.port_activity_label ?? "");
    const match = activityLabel.match(/⚡\s*([0-9]+(?:\.[0-9]+)?)\s*w/i);
    if (match) {
      return `⚡ ${match[1]} W`;
    }

    return "⚡";
  }

  private clientBandLabel(client: ClientRecord): string {
    const mode = (client.wifiMode || "").toLowerCase();
    if (mode.includes("bea") || mode.includes("6g")) {
      return "6 GHz";
    }
    if (mode.includes("axa") || mode.includes("ac") || mode.includes("na") || mode.endsWith("a")) {
      return "5 GHz";
    }
    if (mode.includes("axg") || mode.includes("ng") || mode.includes("11g") || mode.endsWith("g") || mode.endsWith("b")) {
      return "2.4 GHz";
    }
    return client.ssid ? "Wi-Fi" : "-";
  }

  private wiredConnectionLabel(client: ClientRecord): string {
    const lagId = client.attachmentLagId;
    if (lagId && lagId !== "0") {
      const parent = client.switchName || client.gatewayName || "parent device";
      return `Wired via ${parent} LAG ${lagId}`;
    }

    if (client.attachmentPort && client.attachmentPort !== "0") {
      return "Wired";
    }

    return "Wired";
  }
}

registerCustomCard(customElements, window, "omada-network-card", OmadaNetworkCard, {
  type: "omada-network-card",
  name: "Omada Network Card",
  description: "Full-screen Omada operations card for Home Assistant."
});
