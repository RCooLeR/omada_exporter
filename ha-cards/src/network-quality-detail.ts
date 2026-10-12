import { css, html, nothing } from "lit";
import type { DeviceRecord } from "./ha-types";

function number(value: number | undefined, unit = "", scale = 1): string {
  return value === undefined ? "—" : `${(value * scale).toLocaleString(undefined, { maximumFractionDigits: 2 })}${unit}`;
}

export function qualityWindowLabel(seconds: number | undefined): string {
  if (seconds === undefined) return "Rolling window not reported";
  if (seconds % 3600 === 0) return `Rolling window: ${number(seconds / 3600)} h`;
  if (seconds % 60 === 0) return `Rolling window: ${number(seconds / 60)} min`;
  return `Rolling window: ${number(seconds)} s`;
}

function unavailableMessage(status: string): string {
  switch (status) {
    case "error": return "Collection failed. Previous readings are hidden.";
    case "offline": return "Device offline. Previous readings are hidden.";
    case "wireless": return "Wireless backhaul — wired uplink counters do not apply.";
    default: return "No supported data reported by the controller.";
  }
}

function stat(label: string, value: string) {
  return html`<div class="detail-stat"><div class="detail-stat-label">${label}</div><div class="detail-stat-value">${value}</div></div>`;
}

export function renderNetworkQualityDetail(device: DeviceRecord) {
  const quality = device.quality;
  if (!quality) return nothing;

  if (device.type === "gateway" && quality.wan) {
    const wan = quality.wan;
    return html`<section class="quality-panel detail-card" aria-label="Gateway WAN quality">
      <div class="quality-heading"><div class="section-title">WAN Quality</div><span class="detail-sub">${qualityWindowLabel(wan.windowSeconds)}</span></div>
      <div class="detail-sub quality-note">Gateway-wide rolling averages, not individual WAN ports or live readings.</div>
      ${wan.status === "available" ? html`
        <div class="quality-stats">
          ${stat("Avg latency", number(wan.latencySeconds, " ms", 1000))}
          ${stat("Avg jitter", number(wan.jitterSeconds, " ms", 1000))}
          ${stat("Avg packet loss", number(wan.packetLossPercentage, "%"))}
          ${stat("Avg MOS", number(wan.mos, " / 5"))}
        </div>
        <div class="detail-sub quality-note">Missing fields are shown as —, not zero.</div>
      ` : html`<div class="quality-empty">${unavailableMessage(wan.status)}</div>`}
    </section>`;
  }

  if (device.type !== "ap") return nothing;
  const uplink = quality.uplink;
  return html`<section class="quality-columns" aria-label="Access point network quality">
    <section class="quality-panel detail-card" aria-label="Radio interference">
      <div class="section-title">Radio Interference</div>
      <div class="detail-sub quality-note">Controller-reported interference by band.</div>
      ${quality.interference.length ? html`<div class="quality-radios">
        ${quality.interference.map((radio) => {
          const band = radio.band === "5-2" ? "5 GHz · radio 2" : `${radio.band} GHz`;
          return html`<div class="quality-radio">
            <span>${band}</span>
            <meter min="0" max="100" value=${radio.percentage} aria-label=${`${band} interference`}></meter>
            <span class="quality-number">${number(radio.percentage, "%")}</span>
          </div>`;
        })}
      </div>` : html`<div class="quality-empty">No current radio interference data.</div>`}
    </section>
    ${uplink ? html`<section class="quality-panel detail-card" aria-label="Wired uplink packet counters">
      <div class="section-title">Wired Uplink</div>
      ${uplink.status === "available" ? html`
        <div class="table quality-table" tabindex="0" aria-label="Cumulative uplink packet counters">
          <table>
            <caption>Cumulative packet counters · not rates</caption>
            <thead><tr><th scope="col">Direction</th><th scope="col">Packets</th><th scope="col">Dropped</th><th scope="col">Errors</th></tr></thead>
            <tbody>${(["up", "down"] as const).map((direction) => html`<tr>
              <th scope="row">${direction === "up" ? "Up" : "Down"}</th>
              <td>${number(uplink[direction].packets)}</td><td>${number(uplink[direction].dropped)}</td><td>${number(uplink[direction].errors)}</td>
            </tr>`)}</tbody>
          </table>
        </div>
        <div class="detail-sub quality-note">Totals can reset. — means not reported.</div>
      ` : html`<div class="quality-empty">${unavailableMessage(uplink.status)}</div>`}
    </section>` : nothing}
  </section>`;
}

export const networkQualityStyles = css`
  .detail-shell.has-quality {
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: auto auto auto minmax(14rem, 1fr);
    overflow-y: auto;
    align-content: start;
    scrollbar-gutter: stable;
  }
  .quality-columns { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 18rem), 1fr)); gap: 0.75rem; min-width: 0; }
  .quality-panel { padding: 0.75rem; min-width: 0; display: grid; gap: 0.6rem; align-content: start; }
  .quality-heading { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: 0.4rem 0.75rem; }
  .quality-heading > .detail-sub, .quality-note, .quality-empty { font-size: 0.78rem; line-height: 1.45; }
  .quality-empty { color: var(--muted); padding: 0.4rem 0; }
  .quality-stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 7rem), 1fr)); gap: 0.5rem; }
  .quality-stats .detail-stat-value { font-variant-numeric: tabular-nums; overflow-wrap: anywhere; }
  .quality-radios { display: grid; gap: 0.65rem; padding: 0.25rem 0; }
  .quality-radio { display: grid; grid-template-columns: 7rem minmax(2rem, 1fr) 3.5rem; align-items: center; gap: 0.5rem; font-size: 0.8rem; }
  .quality-number { text-align: right; font-variant-numeric: tabular-nums; }
  .quality-radio meter { appearance: none; width: 100%; height: 0.45rem; border: 0; border-radius: 999px; background: var(--border); }
  .quality-radio meter::-webkit-meter-bar { background: var(--border); border: 0; border-radius: 999px; }
  .quality-radio meter::-webkit-meter-optimum-value { background: var(--accent); border-radius: 999px; }
  .quality-radio meter::-moz-meter-bar { background: var(--accent); border-radius: 999px; }
  .quality-table { border-radius: 10px; }
  .quality-table caption { color: var(--muted); text-align: left; padding: 0.4rem 0.5rem; font-size: 0.78rem; }
  .quality-table th, .quality-table td { padding: 0.45rem 0.5rem; font-variant-numeric: tabular-nums; }
  .quality-table td, .quality-table thead th:not(:first-child) { text-align: right; }
  .quality-table tbody th { position: static; background: none; text-transform: none; letter-spacing: normal; }
`;
