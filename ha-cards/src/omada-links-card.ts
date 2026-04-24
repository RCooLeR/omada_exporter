import { css, html, LitElement } from "lit";
import { repeat } from "lit/directives/repeat.js";
import type { DashboardModel, HomeAssistant, LinkRow, LovelaceCardConfig } from "./ha-types";
import { formatBytes, formatLatency, formatRateBytes, formatSpeedMbps, formatUptimeMinutes } from "./format";
import { getDashboardModel } from "./model";

declare global {
  interface Window {
    customCards?: Array<Record<string, unknown>>;
  }
}

export class OmadaLinksCard extends LitElement {
  static properties = {
    hass: { attribute: false },
    _model: { state: true }
  };

  static styles = css`
    :host {
      display: block;
      --bg: linear-gradient(135deg, #08131d, #0b1d2f 42%, #10253a);
      --surface: rgba(9, 20, 34, 0.76);
      --border: rgba(146, 196, 255, 0.16);
      --text: #edf4ff;
      --muted: #97aac0;
      --accent: #54d1ff;
      --good: #1eb980;
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
      display: grid;
      gap: 0.75rem;
      padding: 0.75rem;
      background:
        radial-gradient(circle at top left, rgba(84, 209, 255, 0.18), transparent 28%),
        radial-gradient(circle at 85% 10%, rgba(30, 185, 128, 0.16), transparent 24%);
    }
    .table-card {
      display: grid;
      min-width: 0;
      min-height: 0;
      padding: 0.75rem 0.75rem 0;
      border: 1px solid var(--border);
      border-radius: 24px;
      background: var(--surface);
      backdrop-filter: blur(18px);
      box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
    }
    .table {
      overflow: auto;
      min-width: 0;
      min-height: 0;
      border-radius: 18px;
      border: 1px solid rgba(255, 255, 255, 0.05);
    }
    table {
      width: 100%;
      border-collapse: collapse;
      font-size: 0.82rem;
    }
    th, td {
      padding: 0.7rem 0.75rem;
      text-align: left;
      white-space: nowrap;
    }
    th {
      position: sticky;
      top: 0;
      z-index: 1;
      color: var(--accent);
      background: rgba(9, 20, 34, 0.94);
      letter-spacing: 0.1em;
      text-transform: uppercase;
      font-size: 0.7rem;
      font-weight: 600;
    }
    tbody tr:nth-child(odd) {
      background: rgba(255, 255, 255, 0.015);
    }
    .status-dot {
      width: 0.7rem;
      height: 0.7rem;
      border-radius: 999px;
      display: inline-block;
      margin-right: 0.45rem;
      background: var(--muted);
      box-shadow: 0 0 18px currentColor;
      vertical-align: middle;
    }
    .status-up {
      color: var(--good);
      background: var(--good);
    }
    .status-down {
      color: var(--bad);
      background: var(--bad);
    }
    .empty {
      display: grid;
      place-items: center;
      min-height: 10rem;
      color: var(--muted);
      padding: 1.5rem;
      text-align: center;
    }
  `;

  public hass?: HomeAssistant;
  private _config?: LovelaceCardConfig;
  private _model?: DashboardModel;

  public setConfig(config: LovelaceCardConfig): void {
    if (!config?.type) {
      throw new Error("Card type is required");
    }
    this._config = config;
  }

  public getCardSize(): number {
    return 5;
  }

  protected willUpdate(changed: Map<string, unknown>): void {
    if (changed.has("hass") && this.hass) {
      this._model = getDashboardModel(this.hass, this._config?.site);
    }
  }

  protected render() {
    if (!this._config) {
      return html`<ha-card><div class="empty">Card is not configured.</div></ha-card>`;
    }

    if (!this._model) {
      return html`<ha-card><div class="empty">Waiting for Omada MQTT entities.</div></ha-card>`;
    }

    return html`
      <ha-card>
        <div class="frame">
          <div class="table-card">${this.renderIspTable()}</div>
          <div class="table-card">${this.renderVpnTable()}</div>
        </div>
      </ha-card>
    `;
  }

  private renderIspTable() {
    return html`
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

  private renderVpnTable() {
    return html`
      <div class="table">
        <table>
          <thead>
            <tr>
              <th>VPN</th>
              <th>Remote IP</th>
              <th>Mode</th>
              <th>Status</th>
              <th>Uptime</th>
              <th>Total</th>
            </tr>
          </thead>
          <tbody>
            ${repeat(this._model!.vpns, (row) => row.key, (row) => {
              const isUp = (row.metrics.omada_vpn_status ?? 0) > 0;
              const uptime = row.metrics.omada_vpn_uptime ?? 0;
              const total = (row.metrics.omada_vpn_up_bytes ?? 0) + (row.metrics.omada_vpn_down_bytes ?? 0);
              const remoteIp = String(row.attrs.remote_ip_preferred ?? row.attrs.remote_ip ?? row.attrs.remote_ip_runtime ?? "-") || "-";
              return html`
                <tr>
                  <td>${row.name}</td>
                  <td>${remoteIp}</td>
                  <td>${String(row.attrs.vpn_mode ?? "-")}</td>
                  <td><span class="status-dot ${isUp ? "status-up" : "status-down"}"></span>${isUp ? "Online" : "Offline"}</td>
                  <td>${formatUptimeMinutes(uptime)}</td>
                  <td>${formatBytes(total)}</td>
                </tr>
              `;
            })}
          </tbody>
        </table>
      </div>
    `;
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
}

customElements.define("omada-links-card", OmadaLinksCard);
window.customCards = window.customCards || [];
window.customCards.push({
  type: "omada-links-card",
  name: "Omada Links Card",
  description: "Compact ISP and VPN summary card for Home Assistant."
});
