import { describe, expect, it } from "vitest";
import { controlAvailable, controlTarget, findControlEntity } from "./control-model";
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
