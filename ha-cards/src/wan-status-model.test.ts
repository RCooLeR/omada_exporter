import { describe, expect, it } from "vitest";
import type { HassEntity, LinkRow } from "./ha-types";
import { buildDashboardModel, findWanForRow } from "./model";
import { linkMetricValue, wanDisplayStatus } from "./wan-status-model";

const attrs = { bridge_id: "apartment", site_id: "site-a", site: "Home", port: "1", name: "WAN 1" };

function row(metrics: Record<string, number>): LinkRow {
  return { key: "wan", name: "WAN 1", status: "", attrs, metrics };
}

function entity(id: string, metric: string, state: string, timestamp: string): HassEntity {
  return {
    entity_id: `sensor.${id}`, state, last_updated: timestamp,
    attributes: { ...attrs, metric, ...(metric.startsWith("omada_isp_")
      ? { gateway_mac: "aa:bb:cc:dd:ee:ff" } : { device_mac: "aa:bb:cc:dd:ee:ff" }) }
  };
}

describe("WAN display status", () => {
  it.each(["unknown", "unavailable", "", "  ", "NaN", "Infinity"])("preserves missing state %j", (state) => {
    expect(linkMetricValue(state)).toBeUndefined();
  });

  it.each([["on", 1], ["off", 0], ["0", 0], ["1", 1], ["2", 2], [" 39.5 ", 39.5]] as const)("parses %j", (state, value) => {
    expect(linkMetricValue(state)).toBe(value);
  });

  it.each([
    [{}, "Unknown", "status-unknown"],
    [{ omada_isp_status: 0 }, "Offline", "status-down"],
    [{ omada_isp_status: 1 }, "Online", "status-up"],
    [{ omada_isp_status: 2 }, "Unknown", "status-unknown"],
    [{ omada_isp_status: -1 }, "Unknown", "status-unknown"]
  ] as const)("does not guess unsupported ISP codes %j", (metrics, label, className) => {
    expect(wanDisplayStatus(row(metrics))).toEqual({ label, className });
  });

  it.each([
    [0, "Offline", "status-down"], [1, "Online", "status-up"],
    [2, "Connecting", "status-unknown"], [3, "Disconnecting", "status-unknown"]
  ] as const)("uses WAN internet state %i instead of the ISP enum", (internetState, label, className) => {
    for (const isp of [{}, { omada_isp_status: 0 }, { omada_isp_status: 1 }, { omada_isp_status: 2 }]) {
      expect(wanDisplayStatus(row(isp), row({ omada_wan_status: 1, omada_wan_internet_state: internetState })))
        .toEqual({ label, className });
    }
  });

  it("does not call an established physical link Internet Online", () => {
    expect(wanDisplayStatus(row({}), row({ omada_wan_status: 1 })).label).toBe("Unknown");
  });

  it("reports a disconnected physical link even if an older internet state is connected", () => {
    expect(wanDisplayStatus(row({ omada_isp_status: 1 }), row({ omada_wan_status: 0, omada_wan_internet_state: 1 })).label).toBe("Offline");
  });
});

describe("WAN and ISP model availability", () => {
  it.each(["omada_isp_status", "omada_wan_status", "omada_wan_internet_state"])("honours HA availability transitions with retained attributes for %s", (metric) => {
    for (const state of ["unknown", "unavailable"]) {
      const unavailable = entity("unavailable", metric, state, "2026-01-03T00:00:00Z");
      unavailable.attributes.last_updated = "2026-01-01T00:00:00Z";
      const stale = entity("stale", metric, "1", "2026-01-02T00:00:00Z");
      stale.attributes.last_updated = "2026-01-02T00:00:00Z";
      for (const entries of [[unavailable, stale], [stale, unavailable]]) {
        const model = buildDashboardModel({ states: Object.fromEntries(entries.map((item) => [item.entity_id, item])) }, { bridge_id: "apartment" });
        const rows = metric.startsWith("omada_isp_") ? model.isps : model.wans;
        expect(rows).toHaveLength(1);
        expect(rows[0]?.metrics[metric]).toBeUndefined();
      }
    }
  });

  it.each(["omada_isp_status", "omada_wan_status", "omada_wan_internet_state"])("keeps newer numeric data after an older HA availability transition for %s", (metric) => {
    for (const state of ["unknown", "unavailable"]) {
      const unavailable = entity("unavailable", metric, state, "2026-01-02T00:00:00Z");
      unavailable.attributes.last_updated = "2026-01-01T00:00:00Z";
      const fresh = entity("fresh", metric, "1", "2026-01-03T00:00:00Z");
      fresh.attributes.last_updated = "2026-01-03T00:00:00Z";
      for (const entries of [[unavailable, fresh], [fresh, unavailable]]) {
        const model = buildDashboardModel({ states: Object.fromEntries(entries.map((item) => [item.entity_id, item])) }, { bridge_id: "apartment" });
        const rows = metric.startsWith("omada_isp_") ? model.isps : model.wans;
        expect(rows).toHaveLength(1);
        expect(rows[0]?.metrics[metric]).toBe(1);
      }
    }
  });

  it.each(["omada_isp_status", "omada_wan_internet_state"])("retains the newest availability for %s", (metric) => {
    const fresh = entity("fresh", metric, "unavailable", "2026-01-02T00:00:00Z");
    const stale = entity("stale", metric, "1", "2026-01-01T00:00:00Z");
    for (const entries of [[fresh, stale], [stale, fresh]]) {
      const model = buildDashboardModel({ states: Object.fromEntries(entries.map((item) => [item.entity_id, item])) }, { bridge_id: "apartment" });
      const rows = metric.startsWith("omada_isp_") ? model.isps : model.wans;
      expect(rows).toHaveLength(1);
      expect(rows[0]?.metrics[metric]).toBeUndefined();
    }
  });

  it.each(["omada_isp_status", "omada_wan_internet_state"])("does not let stale unknown data replace live %s", (metric) => {
    const fresh = entity("fresh", metric, "1", "2026-01-02T00:00:00Z");
    const stale = entity("stale", metric, "unknown", "2026-01-01T00:00:00Z");
    const model = buildDashboardModel({ states: { fresh, stale } }, { bridge_id: "apartment" });
    expect((metric.startsWith("omada_isp_") ? model.isps : model.wans)[0]?.metrics[metric]).toBe(1);
  });

  it("does not use another bridge's online WAN to resolve an unknown ISP", () => {
    const isp = entity("isp", "omada_isp_status", "unknown", "2026-01-02T00:00:00Z");
    const wan = entity("wan", "omada_wan_internet_state", "1", "2026-01-02T00:00:00Z");
    wan.attributes.bridge_id = "house";
    const model = buildDashboardModel({ states: { isp, wan } }, { bridge_id: "apartment" });
    const match = findWanForRow(model, model.isps[0]!);
    expect(match).toBeUndefined();
    expect(wanDisplayStatus(model.isps[0]!, match).label).toBe("Unknown");
  });
});
