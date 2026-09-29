import { css, html } from "lit";
import { formatBytes, formatUptimeSeconds } from "./format";
import { dpiCategoryColor } from "./dpi-colors";
import type { DpiSiteRecord, DpiSummary } from "./ha-types";

export function formatDpiBytes(bytes: number | undefined): string {
  return bytes === undefined ? "Unavailable" : bytes === 0 ? "0 B" : formatBytes(bytes);
}

export function dpiTrafficShare(bytes: number, total: number | undefined): number | undefined {
  if (total === undefined || (total === 0 && bytes > 0)) {
    return undefined;
  }
  return total === 0 ? 0 : (bytes / total) * 100;
}

function trafficCell(bytes: number) {
  return html`<td class="dpi-number" title=${`${bytes.toLocaleString()} bytes`}>${formatDpiBytes(bytes)}</td>`;
}

function shareCell(bytes: number, total: number | undefined) {
  const share = dpiTrafficShare(bytes, total);
  return html`<td class="dpi-number dpi-share">
    ${share === undefined ? "—" : html`
      <span class="dpi-share-track" aria-hidden="true"><span style=${`width: ${Math.min(100, share)}%`}></span></span>
      <span>${share.toFixed(1)}%</span>
    `}
  </td>`;
}

function stat(label: string, value: string) {
  return html`<div class="detail-stat"><div class="detail-stat-label">${label}</div><div class="detail-stat-value">${value}</div></div>`;
}

type SelectCategory = (siteKey: string, categoryKey: string | undefined) => void;

function renderSite(site: DpiSiteRecord, filters: ReadonlyMap<string, string>, onSelectCategory: SelectCategory) {
  const selected = site.categories.find((row) => row.key === filters.get(site.key));
  const applications = selected ? site.applications.filter((row) => selected.familyId
    ? row.familyId === selected.familyId
    : row.categoryName === selected.name) : site.applications;
  // Assign colors from the full traffic ranking, never from the filtered list.
  const applicationColor = (familyId: string, categoryName: string) => dpiCategoryColor(
    site.categories.findIndex((row) => row.familyId ? row.familyId === familyId : row.name === categoryName)
  );
  return html`
    <section class="dpi-site" aria-label=${`DPI traffic for ${site.site || "Omada site"}`}>
      <div class="dpi-site-heading">
        <div class="section-title">${site.site || "Omada site"}</div>
        ${site.observedAt ? html`<time class="detail-sub" datetime=${site.observedAt}>Updated ${new Date(site.observedAt).toLocaleString()}</time>` : null}
      </div>
      <div class="detail-stats">
        ${stat("Classified traffic", formatDpiBytes(site.totalBytes))}
        ${stat("Insight window", site.windowSeconds ? formatUptimeSeconds(site.windowSeconds) : "Unavailable")}
        ${stat("Categories", String(site.categories.length))}
        ${stat("Applications", String(site.applications.length))}
      </div>
      <div class="dpi-columns">
        <section class="dpi-table-panel" aria-label="DPI categories">
          <div class="dpi-table-heading">
            <div class="section-title">Categories <span>${site.categories.length}</span></div>
            <button type="button" class="dpi-reset" aria-pressed=${!selected}
              @click=${() => onSelectCategory(site.key, undefined)}>All categories</button>
          </div>
          <div class="detail-sub dpi-filter-hint">Select a category to filter applications.</div>
          <div class="table dpi-table" tabindex="0" aria-label="Category traffic table">
            <table>
              <thead><tr><th scope="col">Category</th><th scope="col" class="dpi-number">Traffic</th><th scope="col" class="dpi-number">Share</th></tr></thead>
              <tbody>
                ${site.categories.map((row, index) => html`<tr class=${selected?.key === row.key ? "dpi-selected" : ""} style=${`--dpi-color: ${dpiCategoryColor(index)}`}>
                  <td class="dpi-name" title=${`Category ID: ${row.familyId || "unavailable"}`}>
                    <button type="button" class="dpi-category-button" aria-pressed=${selected?.key === row.key}
                      @click=${() => onSelectCategory(site.key, row.key)}>
                      <span class="dpi-color-dot" aria-hidden="true"></span><span>${row.name}</span>
                    </button>
                  </td>
                  ${trafficCell(row.bytes)}${shareCell(row.bytes, site.totalBytes)}
                </tr>`)}
                ${site.categories.length === 0 ? html`<tr><td colspan="3" class="dpi-empty">No category data exported.</td></tr>` : null}
              </tbody>
            </table>
          </div>
        </section>
        <section class="dpi-table-panel" aria-label="DPI applications">
          <div class="dpi-table-heading" aria-live="polite" aria-atomic="true">
            <div class="section-title">Applications <span>${applications.length}</span></div>
            <div class="dpi-filter-label">${selected?.name ?? "All categories"}</div>
          </div>
          <div class="detail-sub dpi-filter-hint">Traffic and shares of the site's classified total.</div>
          <div class="table dpi-table" tabindex="0" aria-label="Application traffic table">
            <table>
              <thead><tr><th scope="col">Application / category</th><th scope="col" class="dpi-number">Traffic</th><th scope="col" class="dpi-number">Share</th></tr></thead>
              <tbody>
                ${applications.map((row) => html`<tr style=${`--dpi-color: ${applicationColor(row.familyId, row.categoryName)}`}>
                  <td class="dpi-name" title=${`Application ID: ${row.applicationId || "unavailable"} · Category ID: ${row.familyId || "unavailable"}`}>
                    <span>${row.name}</span><span class="dpi-category-name"><span class="dpi-color-dot" aria-hidden="true"></span>${row.categoryName}</span>
                  </td>
                  ${trafficCell(row.bytes)}${shareCell(row.bytes, site.totalBytes)}
                </tr>`)}
                ${applications.length === 0 ? html`<tr><td colspan="3" class="dpi-empty">${selected ? "No application data exported for this category." : "No application data exported."}</td></tr>` : null}
              </tbody>
            </table>
          </div>
        </section>
      </div>
    </section>
  `;
}

export function renderDpiDetail(dpi: DpiSummary, filters: ReadonlyMap<string, string> = new Map(), onSelectCategory: SelectCategory = () => {}) {
  return html`
    <div class="dpi-shell">
      <div class="dpi-heading">
        <div class="detail-name">DPI Insights</div>
        <div class="detail-sub">Classified traffic over the controller's rolling insight window, not live throughput.</div>
        <div class="detail-sub">Select a device or client to return to its details.</div>
      </div>
      <div class="dpi-scroll" tabindex="0" aria-label="DPI insight details">
        ${dpi.sites.map((site) => renderSite(site, filters, onSelectCategory))}
        <p class="dpi-note">Showing available exported rows, highest traffic first. Select All categories to show every exported application. Application coverage is limited by the bridge's export setting; category and application totals may not equal classified traffic. Shares use each site's classified total.</p>
      </div>
    </div>
  `;
}

export const dpiStyles = css`
  .dpi-shell { display: grid; grid-template-rows: auto minmax(0, 1fr); min-height: 0; padding: 0.9rem; gap: 1rem; }
  .dpi-heading { display: grid; gap: 0.5rem; }
  .dpi-scroll { overflow: auto; min-height: 0; container-type: inline-size; padding-right: 0.25rem; }
  .dpi-site { display: grid; gap: 0.9rem; padding-bottom: 1.2rem; }
  .dpi-site + .dpi-site { border-top: 1px solid var(--border); padding-top: 1.2rem; }
  .dpi-site-heading { display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: 0.5rem; }
  .dpi-site-heading time, .dpi-note { font-size: 0.78rem; color: var(--muted); }
  .dpi-columns { display: grid; grid-template-columns: minmax(0, 0.85fr) minmax(0, 1.15fr); gap: 0.75rem; align-items: start; }
  .dpi-table-panel { min-width: 0; display: grid; gap: 0.6rem; }
  .dpi-table-heading { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: 0.5rem; min-height: 2rem; }
  .dpi-filter-label { color: var(--text); font-size: 0.8rem; overflow-wrap: anywhere; }
  .dpi-filter-hint { font-size: 0.75rem; }
  .dpi-reset, .dpi-category-button { font: inherit; color: var(--text); cursor: pointer; }
  .dpi-reset { border: 1px solid var(--border); border-radius: 999px; background: var(--surface); padding: 0.35rem 0.65rem; font-size: 0.75rem; }
  .dpi-reset[aria-pressed="true"] { border-color: var(--accent); }
  .dpi-category-button { display: flex; align-items: center; gap: 0.5rem; text-align: left; width: 100%; min-height: 2.5rem; border: 0; background: transparent; padding: 0; }
  .dpi-color-dot { display: inline-block; width: 0.65rem; height: 0.65rem; flex: 0 0 auto; border-radius: 50%; background: var(--dpi-color); }
  .dpi-selected { background: color-mix(in srgb, var(--dpi-color) 14%, transparent); }
  .dpi-selected > td:first-child { box-shadow: inset 3px 0 var(--dpi-color); }
  .dpi-selected .dpi-category-button { font-weight: 700; }
  .dpi-category-button:hover, .dpi-reset:hover { text-decoration: underline; }
  .dpi-category-button:focus-visible, .dpi-reset:focus-visible { outline: 2px solid var(--accent); outline-offset: 3px; border-radius: 3px; }
  .dpi-table .dpi-name { white-space: normal; overflow-wrap: anywhere; min-width: 6rem; }
  .dpi-table .dpi-number { text-align: right; font-variant-numeric: tabular-nums; }
  .dpi-category-name { display: block; color: var(--muted); margin-top: 0.2rem; font-size: 0.75rem; }
  .dpi-category-name .dpi-color-dot { width: 0.5rem; height: 0.5rem; margin-right: 0.4rem; }
  .dpi-table th, .dpi-table td { padding: 0.65rem 0.5rem; }
  .dpi-share { min-width: 3rem; }
  .dpi-share-track { display: block; height: 3px; background: var(--border); margin-bottom: 0.3rem; border-radius: 3px; overflow: hidden; }
  .dpi-share-track > span { display: block; height: 100%; background: var(--dpi-color, var(--accent)); }
  .dpi-empty { white-space: normal; color: var(--muted); }
  .dpi-note { line-height: 1.5; margin: 0; }
  .dpi-scroll:focus-visible, .dpi-table:focus-visible { outline: 2px solid var(--accent); outline-offset: -2px; }
  @container (max-width: 720px) { .dpi-columns { grid-template-columns: minmax(0, 1fr); } }
  @media (max-width: 1100px) { .dpi-scroll { max-height: 70vh; } }
`;
