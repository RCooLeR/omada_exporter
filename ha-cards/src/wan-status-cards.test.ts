import type { TemplateResult } from "lit";
import { describe, expect, it, vi } from "vitest";
import type { HassEntity, HomeAssistant } from "./ha-types";

vi.hoisted(() => {
  vi.stubGlobal("window", {});
  vi.stubGlobal("customElements", { get: vi.fn(), define: vi.fn() });
});
vi.mock("./register-card", () => ({ registerCustomCard: vi.fn() }));
vi.mock("lit/directives/repeat.js", () => ({
  repeat: (items: unknown[], _key: unknown, render: (item: unknown, index: number) => unknown) => items.map(render)
}));
import { OmadaNetworkCard } from "./omada-network-card";
import { OmadaLinksCard } from "./omada-links-card";

interface Harness {
  willUpdate(changed: Map<string, unknown>): void;
  renderIspBlock(): TemplateResult;
  renderIspTable(): TemplateResult;
}

function boundValues(value: unknown): unknown[] {
  if (Array.isArray(value)) return value.flatMap(boundValues);
  if (value && typeof value === "object" && "_$litType$" in value && "values" in value) {
    return (value as TemplateResult).values.flatMap(boundValues);
  }
  return [value];
}

function hass(ispState: string, internetState?: string): HomeAssistant {
  const scope = { bridge_id: "apartment", site_id: "site-a", name: "WAN 1", port: "1" };
  const isp: HassEntity = {
    entity_id: "binary_sensor.isp", state: ispState,
    attributes: { ...scope, gateway_mac: "aa:bb:cc:dd:ee:ff", metric: "omada_isp_status" }
  };
  const states: HomeAssistant["states"] = { [isp.entity_id]: isp };
  if (internetState !== undefined) {
    const wan: HassEntity = {
      entity_id: "sensor.wan_internet", state: internetState,
      attributes: { ...scope, device_mac: "aa:bb:cc:dd:ee:ff", metric: "omada_wan_internet_state" }
    };
    states[wan.entity_id] = wan;
  }
  return { states, callService: vi.fn() };
}

describe.each([["network", OmadaNetworkCard], ["links", OmadaLinksCard]] as const)("%s card WAN display", (name, Card) => {
  it.each([
    ["unknown", undefined, "Unknown", "status-unknown"],
    ["unavailable", undefined, "Unknown", "status-unknown"],
    ["2", undefined, "Unknown", "status-unknown"],
    ["unknown", "1", "Online", "status-up"],
    ["off", "1", "Online", "status-up"],
    ["on", "0", "Offline", "status-down"],
    ["on", "2", "Connecting", "status-unknown"],
    ["on", "3", "Disconnecting", "status-unknown"],
    ["off", undefined, "Offline", "status-down"],
    ["on", undefined, "Online", "status-up"]
  ] as const)("renders ISP %s and WAN %s as %s", (ispState, internetState, label, className) => {
    const card = new Card();
    card.setConfig({ type: `custom:omada-${name}-card`, bridge_id: "apartment" });
    card.hass = hass(ispState, internetState);
    const harness = card as unknown as Harness;
    harness.willUpdate(new Map([["hass", undefined]]));
    const values = boundValues(name === "network" ? harness.renderIspBlock() : harness.renderIspTable());
    expect(values).toContain(label);
    expect(values).toContain(className);
    if (label !== "Offline") expect(values).not.toContain("Offline");
    expect(card.hass.callService).not.toHaveBeenCalled();
  });
});
