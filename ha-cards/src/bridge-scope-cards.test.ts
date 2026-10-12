import { describe, expect, it, vi } from "vitest";
import type { DashboardModel, HomeAssistant, LinkRow } from "./ha-types";

vi.hoisted(() => {
  vi.stubGlobal("window", {});
  vi.stubGlobal("customElements", { get: vi.fn(), define: vi.fn() });
});
vi.mock("./register-card", () => ({ registerCustomCard: vi.fn() }));
import { OmadaNetworkCard } from "./omada-network-card";
import { OmadaLinksCard } from "./omada-links-card";

interface Harness {
  _model: DashboardModel;
  willUpdate(changed: Map<string, unknown>): void;
  findWanFor(row: LinkRow): LinkRow | undefined;
}

function state(): HomeAssistant {
  const states: HomeAssistant["states"] = {};
  for (const bridge of ["", "slobidska44"]) {
    const attrs = { bridge_id: bridge, site: "Default", site_id: "site-1" };
    const id = bridge || "legacy";
    for (const gateway of ["aa:bb:cc:dd:ee:ff", "11:22:33:44:55:66"]) {
      const suffix = `${id}_${gateway.replaceAll(":", "")}`;
      for (const [metric, labels, value] of [
        ["omada_device_cpu_percentage", { device_mac: gateway, device_type: "gateway", device_name: suffix, device_status: "Connected" }, "10"],
        ["omada_isp_status", { gateway_mac: gateway, name: "WAN 1", port: "1" }, "1"],
        ["omada_wan_latency", { device_mac: gateway, name: "WAN 1", port: "1" }, gateway.startsWith("aa") ? "12" : "50"]
      ] as const) {
        const entityId = `sensor.${suffix}_${metric}`;
        states[entityId] = { entity_id: entityId, state: value, attributes: { ...attrs, ...labels, metric } };
      }
    }
  }
  return { states };
}

describe.each([
  ["network", OmadaNetworkCard],
  ["links", OmadaLinksCard]
] as const)("%s card bridge isolation", (name, Card) => {
  it("uses the full scope in each card instance and safely switches scopes", () => {
    const current = state();
    const legacy = new Card();
    const scoped = new Card();
    const legacyModel = legacy as unknown as Harness;
    const scopedModel = scoped as unknown as Harness;
    legacy.setConfig({ type: `custom:omada-${name}-card`, site: "Default" });
    scoped.setConfig({ type: `custom:omada-${name}-card`, site: "Default", bridge_id: "slobidska44" });
    for (const card of [legacy, scoped]) {
      card.hass = current;
      (card as unknown as Harness).willUpdate(new Map([["hass", undefined]]));
    }
    expect(legacyModel._model.devices).toHaveLength(2);
    expect(scopedModel._model.devices).toHaveLength(2);
    expect(legacyModel._model.devices.every((device) => !device.attrs.bridge_id)).toBe(true);
    expect(scopedModel._model.devices.every((device) => device.attrs.bridge_id === "slobidska44")).toBe(true);
    for (const harness of [legacyModel, scopedModel]) {
      for (const isp of harness._model.isps) {
        expect(harness.findWanFor(isp)?.metrics.omada_wan_latency).toBe(String(isp.attrs.gateway_mac).startsWith("aa") ? 12 : 50);
      }
    }
    scoped.setConfig({ type: `custom:omada-${name}-card`, bridge_id: "slobidska44", site_id: "missing" });
    scopedModel.willUpdate(new Map([["_config", undefined]]));
    expect(scopedModel._model.devices).toHaveLength(0);
    expect(legacyModel._model.devices).toHaveLength(2);
  });

  it("does not schedule a render for another bridge's updates but retains the latest snapshot", () => {
    const previous = state();
    const card = new Card();
    card.setConfig({ type: `custom:omada-${name}-card`, bridge_id: "slobidska44" });
    card.hass = previous;
    const requestUpdate = vi.spyOn(card, "requestUpdate");
    requestUpdate.mockClear();
    const legacyId = Object.keys(previous.states).find((key) => key.includes("legacy") && key.endsWith("cpu_percentage"))!;
    const current = { states: { ...previous.states, [legacyId]: { ...previous.states[legacyId]!, state: "99" } } };
    card.hass = current;
    expect(requestUpdate).not.toHaveBeenCalled();
    expect(card.hass).toBe(current);
    const scopedId = Object.keys(previous.states).find((key) => key.includes("slobidska44") && key.endsWith("cpu_percentage"))!;
    card.hass = { states: { ...current.states, [scopedId]: { ...current.states[scopedId]!, state: "99" } } };
    expect(requestUpdate).toHaveBeenCalledWith("hass", current);
  });
});
