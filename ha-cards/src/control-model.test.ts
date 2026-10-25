import { describe, expect, it } from "vitest";
import { controlAvailable, controlTarget, findControlEntity, poePortTargets, poePoweredTarget } from "./control-model";
import { buildDashboardModel, cardHassChanged } from "./model";
import type { HassEntity, HomeAssistant } from "./ha-types";

function fixture(bridgeId = "") {
  const attrs = { site: "Home", site_id: "site-1", bridge_id: bridgeId };
  const device: HassEntity = { entity_id: "sensor.gateway", state: "12", attributes: {
    ...attrs, metric: "omada_device_cpu_percentage", device_mac: "aa:bb:cc:dd:ee:ff", device_name: "Gateway", device_type: "gateway", device_status: "Connected"
  } };
  const button: HassEntity = { entity_id: "button.random_registry_name", state: "unknown", attributes: {
    ...attrs, omada_control: "reboot_device", target_mac: "AA-BB-CC-DD-EE-FF"
  } };
  const hass = { states: { [device.entity_id]: device, [button.entity_id]: button } };
  const record = buildDashboardModel(hass, { bridge_id: bridgeId }).devices[0]!;
  return { hass, device, button, record, target: controlTarget("device", record)! };
}

describe("Omada control matching", () => {
  it.each(["", "slobidska44"])("finds only an explicitly attributed button for bridge %s", (bridge) => {
    const { hass, button, target } = fixture(bridge);
    expect(findControlEntity(hass, target)).toBe(button);
    expect(controlAvailable(button)).toBe(true);
    expect(controlAvailable({ ...button, state: "unavailable" })).toBe(false);
  });

  it.each([
    { bridge_id: "another-bridge" }, { site_id: "another-site" }, { site_id: "" },
    { target_mac: "11:22:33:44:55:66" }, { target_mac: "garbageAA-BB-CC-DD-EE-FF" },
    { omada_control: "reconnect_client" }, { omada_control: undefined }
  ])("rejects foreign, incomplete or incorrect attributes %o", (attrs) => {
    const { hass, button, target } = fixture();
    hass.states[button.entity_id] = { ...button, attributes: { ...button.attributes, ...attrs } };
    expect(findControlEntity(hass, target)).toBeUndefined();
  });

  it("requires an exact nonempty site name when both IDs are absent", () => {
    const { hass, button, record } = fixture();
    delete record.attrs.site_id;
    delete button.attributes.site_id;
    expect(findControlEntity(hass, controlTarget("device", record)!)).toBe(button);
    button.attributes.site = "Other";
    expect(findControlEntity(hass, controlTarget("device", record)!)).toBeUndefined();
    record.site = "";
    expect(controlTarget("device", record)).toBeUndefined();
  });

  it("does not fall back to a site name when only the button has a site ID", () => {
    const { hass, record } = fixture();
    delete record.attrs.site_id;
    expect(findControlEntity(hass, controlTarget("device", record)!)).toBeUndefined();
  });

  it("rejects ambiguity even if one matching button is unavailable", () => {
    const { hass, button, target } = fixture();
    hass.states["button.duplicate"] = { ...button, entity_id: "button.duplicate", state: "unavailable" };
    expect(findControlEntity(hass, target)).toBeUndefined();
  });

  it("does not infer IDs or accept sensors masquerading as controls", () => {
    const { hass, button, target } = fixture();
    delete hass.states[button.entity_id];
    hass.states["sensor.reboot"] = { ...button, entity_id: "sensor.reboot" };
    expect(findControlEntity(hass, target)).toBeUndefined();
  });

  it("excludes controllers but allows a Fusion gateway", () => {
    const { record } = fixture();
    expect(controlTarget("device", record)?.action).toBe("reboot_device");
    expect(controlTarget("device", { ...record, type: "controller" })).toBeUndefined();
  });

  it("includes only wireless clients", () => {
    const { hass } = fixture();
    hass.states["device_tracker.phone"] = { entity_id: "device_tracker.phone", state: "home", attributes: {
      site: "Home", site_id: "site-1", mac: "11:22:33:44:55:66", wireless: "true"
    } };
    const client = buildDashboardModel(hass).clients[0]!;
    expect(controlTarget("client", client)?.action).toBe("reconnect_client");
    expect(controlTarget("client", { ...client, wireless: false })).toBeUndefined();
  });

  it("observes scoped control additions, availability changes and removal without adding metric records", () => {
    const { hass, button } = fixture("slobidska44");
    const previous: HomeAssistant = { states: { ...hass.states } };
    delete previous.states[button.entity_id];
    expect(cardHassChanged(hass, previous, { bridge_id: "slobidska44" })).toBe(true);
    expect(cardHassChanged(previous, hass, { bridge_id: "slobidska44" })).toBe(true);
    expect(cardHassChanged(hass, previous)).toBe(false);
    const next = { states: { ...hass.states, [button.entity_id]: { ...button, state: "unavailable" } } };
    expect(cardHassChanged(next, hass, { bridge_id: "slobidska44" })).toBe(true);
    button.attributes = { ...button.attributes, metric: "omada_device_cpu_percentage", device_mac: "11:22:33:44:55:66" };
    expect(buildDashboardModel(hass, { bridge_id: "slobidska44" }).devices).toHaveLength(1);
    expect(buildDashboardModel({ states: { [button.entity_id]: button } }, { bridge_id: "slobidska44" }).devices).toHaveLength(0);
  });

  it("updates when HA service capabilities become available", () => {
    const { hass } = fixture();
    expect(cardHassChanged({ ...hass, callService: async () => {} }, hass)).toBe(true);
  });
});

function poeFixture(bridgeId = "") {
  const f = fixture(bridgeId);
  const hass: HomeAssistant = f.hass;
  for (const port of [1, 2]) {
    const entityId = `sensor.port_${port}`;
    hass.states[entityId] = { entity_id: entityId, state: "0", attributes: {
      ...f.device.attributes, metric: "omada_port_power_watts", port: String(port), name: `Port ${port}`, poe: "false"
    } };
  }
  const button: HassEntity = { entity_id: "button.poe_1", state: "unknown", attributes: {
    ...f.button.attributes, omada_control: "power_cycle_poe", target_port: 1, switch_name: "Gateway", port_name: "Camera"
  } };
  hass.states[button.entity_id] = button;
  hass.states["device_tracker.camera"] = { entity_id: "device_tracker.camera", state: "home", attributes: {
    bridge_id: bridgeId, site: "Home", site_id: "site-1", mac: "11:22:33:44:55:66", name: "Camera", wireless: "false",
    switch_mac: "aa:bb:cc:dd:ee:ff", port: "1"
  } };
  const model = buildDashboardModel(hass, { bridge_id: bridgeId });
  return { ...f, hass, button, record: model.devices[0]!, client: model.clients[0]! };
}

describe("PoE control matching", () => {
  it.each(["", "slobidska44"])("uses discovered physical ports for bridge %s, never telemetry eligibility", (bridgeId) => {
    const f = poeFixture(bridgeId);
    const targets = poePortTargets(f.hass, f.record);
    expect(targets).toHaveLength(1);
    expect(targets[0]).toMatchObject({ action: "power_cycle_poe", port: 1, mac: "aabbccddeeff", name: "Gateway · port 1 (Camera)" });
    expect(findControlEntity(f.hass, targets[0]!)).toBe(f.button);
    delete f.hass.states[f.button.entity_id];
    f.record.ports.forEach((port) => { port.poe = true; port.metrics.omada_port_power_watts = 10; });
    expect(poePortTargets(f.hass, f.record)).toEqual([]);
  });

  it.each([undefined, null, 0, -1, 256, 1.5, "1", "01", true, NaN, Infinity])("rejects invalid discovery port %s", (port) => {
    const f = poeFixture();
    f.button.attributes.target_port = port;
    f.button.attributes.powered_device_mac = f.client.mac;
    expect(poePortTargets(f.hass, f.record)).toEqual([]);
    expect(poePoweredTarget(f.hass, "client", f.client)).toBeUndefined();
  });

  it("distinguishes ports on the same owner and rejects duplicate physical controls", () => {
    const f = poeFixture();
    const second = { ...f.button, entity_id: "button.poe_2", attributes: { ...f.button.attributes, target_port: 2 } };
    f.hass.states[second.entity_id] = second;
    const targets = poePortTargets(f.hass, f.record);
    expect(targets.map((target) => target.port)).toEqual([1, 2]);
    expect(targets[0]!.key).not.toBe(targets[1]!.key);
    expect(findControlEntity(f.hass, targets[1]!)).toBe(second);
    f.hass.states["button.duplicate"] = { ...f.button, entity_id: "button.duplicate", state: "unavailable",
      attributes: { ...f.button.attributes, powered_device_mac: f.client.mac } };
    expect(poePortTargets(f.hass, f.record).map((target) => target.port)).toEqual([2]);
    expect(findControlEntity(f.hass, targets[0]!)).toBeUndefined();
    expect(poePoweredTarget(f.hass, "client", f.client)).toBeUndefined();
  });

  it.each([{ bridge_id: "other" }, { site_id: "other" }, { site_id: "" }])("isolates port and endpoint controls from %o", (scope) => {
    const f = poeFixture();
    const target = poePortTargets(f.hass, f.record)[0]!;
    f.button.attributes = { ...f.button.attributes, ...scope, powered_device_mac: f.client.mac };
    expect(findControlEntity(f.hass, target)).toBeUndefined();
    expect(poePortTargets(f.hass, f.record)).toEqual([]);
    expect(poePoweredTarget(f.hass, "client", f.client)).toBeUndefined();
  });

  it("does not select the same owner MAC and port in another bridge or site", () => {
    const f = poeFixture();
    for (const [id, scope] of [["bridge", { bridge_id: "other" }], ["site", { site_id: "other" }]] as const) {
      f.hass.states[`button.${id}`] = { ...f.button, entity_id: `button.${id}`, attributes: { ...f.button.attributes, ...scope } };
    }
    const targets = poePortTargets(f.hass, f.record);
    expect(targets).toHaveLength(1);
    expect(findControlEntity(f.hass, targets[0]!)).toBe(f.button);
  });

  it("requires a physical port belonging to the same scoped switch or gateway", () => {
    const f = poeFixture();
    expect(poePortTargets(f.hass, { ...f.record, type: "switch" })).toHaveLength(1);
    expect(poePortTargets(f.hass, { ...f.record, type: "ap" })).toEqual([]);
    expect(poePortTargets(f.hass, { ...f.record, ports: [] })).toEqual([]);
    f.record.ports.forEach((port) => { port.attrs = { ...port.attrs, bridge_id: "other" }; });
    expect(poePortTargets(f.hass, f.record)).toEqual([]);
  });

  it("uses only an explicit unique powered endpoint binding for aliases", () => {
    const f = poeFixture();
    expect(poePoweredTarget(f.hass, "client", f.client)).toBeUndefined();
    expect(poePoweredTarget(f.hass, "client", { ...f.client, attachmentLagId: "1" })).toBeUndefined();
    f.button.attributes.powered_device_mac = f.client.mac;
    f.button.attributes.powered_device_name = "Camera";
    expect(poePoweredTarget(f.hass, "client", f.client)?.poweredDeviceMac).toBe("112233445566");
    expect(poePoweredTarget(f.hass, "device", { ...f.record, type: "ap", mac: f.client.mac })?.port).toBe(1);
    expect(poePoweredTarget(f.hass, "device", { ...f.record, type: "controller", mac: f.client.mac })).toBeUndefined();
    expect(poePoweredTarget(f.hass, "client", { ...f.client, wireless: true, apMac: f.record.mac })).toBeUndefined();
    f.hass.states["button.poe_2"] = { ...f.button, entity_id: "button.poe_2", attributes: { ...f.button.attributes, target_port: 2 } };
    expect(poePortTargets(f.hass, f.record)).toHaveLength(2);
    expect(poePoweredTarget(f.hass, "client", f.client)).toBeUndefined();
  });

  it.each([
    { powered_device_mac: "22:22:33:44:55:66" }, { powered_device_name: "Another camera" },
    { switch_name: "Renamed gateway" }, { port_name: "Another port" }
  ])("invalidates the original identity when binding or labels change: %o", (attrs) => {
    const f = poeFixture();
    f.button.attributes.powered_device_mac = f.client.mac;
    const target = poePortTargets(f.hass, f.record)[0]!;
    f.button.attributes = { ...f.button.attributes, ...attrs };
    expect(findControlEntity(f.hass, target)).toBeUndefined();
    expect(poePortTargets(f.hass, f.record)[0]!.key).not.toBe(target.key);
  });

  it.each(["not-a-mac", "aa:bb:cc:dd:ee:ff", false])("rejects malformed or self-powered bindings: %s", (poweredMac) => {
    const f = poeFixture();
    f.button.attributes.powered_device_mac = poweredMac;
    expect(poePortTargets(f.hass, f.record)).toEqual([]);
  });

  it("observes PoE discovery and binding changes without creating metric records", () => {
    const f = poeFixture("slobidska44");
    const previous: HomeAssistant = { states: { ...f.hass.states } };
    delete previous.states[f.button.entity_id];
    expect(cardHassChanged(f.hass, previous, { bridge_id: "slobidska44" })).toBe(true);
    expect(cardHassChanged(previous, f.hass, { bridge_id: "slobidska44" })).toBe(true);
    expect(cardHassChanged(f.hass, previous)).toBe(false);
    const next: HomeAssistant = { states: { ...f.hass.states, [f.button.entity_id]: {
      ...f.button, attributes: { ...f.button.attributes, powered_device_mac: f.client.mac }
    } } };
    expect(cardHassChanged(next, f.hass, { bridge_id: "slobidska44" })).toBe(true);
    expect(buildDashboardModel({ states: { [f.button.entity_id]: f.button } }, { bridge_id: "slobidska44" }).devices).toEqual([]);
  });
});
