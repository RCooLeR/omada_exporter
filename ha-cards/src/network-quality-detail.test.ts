import { nothing, type TemplateResult } from "lit";
import { describe, expect, it } from "vitest";
import type { DeviceRecord } from "./ha-types";
import { qualityWindowLabel, renderNetworkQualityDetail } from "./network-quality-detail";

function text(value: unknown): string {
  if (value === nothing || value == null) return "";
  if (Array.isArray(value)) return value.map(text).join("");
  if (typeof value === "object" && "strings" in value && "values" in value) {
    const template = value as TemplateResult;
    return template.strings.map((part, index) => part + text(template.values[index])).join("");
  }
  return String(value);
}

function device(type: string, quality?: DeviceRecord["quality"]): DeviceRecord {
  return { key: "device", name: "Device", type, model: "", status: "Connected", ip: "", mac: "", version: "", site: "Home",
    attrs: {}, metrics: {}, ports: [], radios: [], clients: [], ...(quality ? { quality } : {}) };
}

describe("network quality detail", () => {
  it("does not change cards with no quality data or unsupported device types", () => {
    expect(renderNetworkQualityDetail(device("ap"))).toBe(nothing);
    expect(renderNetworkQualityDetail(device("switch", { interference: [] }))).toBe(nothing);
  });

  it("shows every available radio, genuine zeroes, cumulative packet counters and missing fields", () => {
    const view = text(renderNetworkQualityDetail(device("ap", {
      interference: [{ band: "2.4", percentage: 0 }, { band: "5-2", percentage: 12.5 }, { band: "6", percentage: 100 }],
      uplink: { status: "available", up: { packets: 1200, dropped: 0 }, down: { errors: 4 } }
    })));
    expect(view).toContain("5 GHz · radio 2");
    expect(view).toContain(`${(12.5).toLocaleString()}%`);
    expect(view).toContain("0%");
    expect(view).toContain("100%");
    expect(view).toContain("Cumulative packet counters · not rates");
    expect(view).toContain("Totals can reset");
    expect(view).toContain("<td>0</td><td>—</td>");
    expect(view).not.toContain("packets/s");
  });

  it.each([
    ["wireless", "Wireless backhaul"], ["offline", "Device offline"],
    ["error", "Collection failed"], ["unavailable", "No supported data"]
  ] as const)("shows %s uplinks neutrally and hides any old counts", (status, message) => {
    const view = text(renderNetworkQualityDetail(device("ap", {
      interference: [], uplink: { status, up: { packets: 999 }, down: {} }
    })));
    expect(view).toContain(message);
    expect(view).not.toContain("999");
    expect(view).not.toContain("<table>");
  });

  it("displays rolling WAN averages with ms conversion and no false zero for missing values", () => {
    const view = text(renderNetworkQualityDetail(device("gateway", { interference: [], wan: {
      status: "available", latencySeconds: 0.0125, jitterSeconds: 0, packetLossPercentage: 0, windowSeconds: 86400
    } })));
    expect(view).toContain("Gateway-wide rolling averages");
    expect(view).toContain("Rolling window: 24 h");
    expect(view).toContain(`${(12.5).toLocaleString()} ms`);
    expect(view).toContain("0 ms");
    expect(view).toContain("0%");
    expect(view).toContain('Avg MOS</div><div class="detail-stat-value">—');
  });

  it.each(["offline", "error", "unavailable"] as const)("hides old WAN values when %s", (status) => {
    const view = text(renderNetworkQualityDetail(device("gateway", { interference: [], wan: {
      status, mos: 4.8, latencySeconds: 0.987
    } })));
    expect(view).not.toContain("987 ms");
    expect(view).not.toContain("4.8 / 5");
    expect(view).toContain('class="quality-empty"');
  });

  it("labels the configured window instead of assuming one day", () => {
    expect(qualityWindowLabel(7200)).toBe("Rolling window: 2 h");
    expect(qualityWindowLabel(900)).toBe("Rolling window: 15 min");
    expect(qualityWindowLabel(45)).toBe("Rolling window: 45 s");
    expect(qualityWindowLabel(undefined)).toBe("Rolling window not reported");
  });
});
