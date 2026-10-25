import type { TemplateResult } from "lit";
import { describe, expect, it, vi } from "vitest";
import type { DashboardModel, HomeAssistant } from "./ha-types";
import type { ControlTarget } from "./control-model";
import { controlTarget, poePortTargets, poePoweredTarget } from "./control-model";

vi.hoisted(() => {
  vi.stubGlobal("window", {});
  vi.stubGlobal("customElements", { get: vi.fn(), define: vi.fn() });
});
vi.mock("./register-card", () => ({ registerCustomCard: vi.fn() }));
import { OmadaNetworkCard } from "./omada-network-card";

interface Harness {
  _model: DashboardModel;
  _controlState: { phase: string; message?: string } | undefined;
  _controlPending: boolean;
  willUpdate(changes: Map<string, unknown>): void;
  beginControl(target: ControlTarget): void;
  confirmControl(): Promise<void>;
  selectDevice(key: string): void;
  selectClient(key: string): void;
  renderDetail(): TemplateResult;
}

function templates(value: unknown): TemplateResult[] {
  if (Array.isArray(value)) return value.flatMap(templates);
  if (value && typeof value === "object" && "_$litType$" in value && "strings" in value && "values" in value) {
    const template = value as TemplateResult;
    return [template, ...template.values.flatMap(templates)];
  }
  if (value && typeof value === "object" && "_$litDirective$" in value && "values" in value && Array.isArray(value.values)) {
    const [items, , render] = value.values as unknown[];
    if (Array.isArray(items) && typeof render === "function") return items.flatMap((item, index) => templates(render(item, index)));
  }
  return [];
}

function markup(value: unknown): string {
  return templates(value).flatMap((template) => [...template.strings]).join("");
}

function fixture(service = vi.fn<NonNullable<HomeAssistant["callService"]>>(async () => {})) {
  const scope = { bridge_id: "slobidska44", site_id: "site-1", site: "Apartment" };
  const hass: HomeAssistant = { states: {}, callService: service };
  for (const [mac, name] of [["aa:bb:cc:dd:ee:ff", "Gateway"], ["aa:bb:cc:dd:ee:11", "Hall AP"]]) {
    const entityId = `sensor.${name}`;
    hass.states[entityId] = { entity_id: entityId, state: "10", attributes: {
      ...scope, metric: "omada_device_cpu_percentage", device_mac: mac, device_name: name,
      device_type: name === "Gateway" ? "gateway" : "ap", device_status: "Connected"
    } };
    const buttonId = `button.${name}`;
    hass.states[buttonId] = { entity_id: buttonId, state: "unknown", attributes: { ...scope, omada_control: "reboot_device", target_mac: mac } };
  }
  hass.states["device_tracker.phone"] = { entity_id: "device_tracker.phone", state: "home", attributes: {
    ...scope, mac: "11:22:33:44:55:66", name: "Phone", wireless: "true"
  } };
  hass.states["button.phone"] = { entity_id: "button.phone", state: "unknown", attributes: {
    ...scope, omada_control: "reconnect_client", target_mac: "11:22:33:44:55:66"
  } };
  const card = new OmadaNetworkCard();
  const harness = card as unknown as Harness;
  card.setConfig({ type: "custom:omada-network-card", ...scope });
  card.hass = hass;
  harness.willUpdate(new Map([["hass", undefined]]));
  const device = harness._model.devices.find((entry) => entry.name === "Gateway")!;
  harness.selectDevice(device.key);
  harness.willUpdate(new Map([["_selection", undefined]]));
  const target = controlTarget("device", device)!;
  return { card, harness, hass, target, service, scope };
}

describe("Omada card controls", () => {
  it("requires confirmation with target/scope and only reports request submission", async () => {
    const f = fixture();
    expect(markup(f.harness.renderDetail())).toContain("control-box");
    f.harness.beginControl(f.target);
    expect(f.service).not.toHaveBeenCalled();
    const content = templates(f.harness.renderDetail()).flatMap((template) => template.values);
    expect(content).toContain("Gateway");
    expect(content).toContain("slobidska44 · Apartment (site-1)");
    expect(content.filter((value) => typeof value === "string").join(" ")).toContain("may interrupt the network");
    expect(markup(f.harness.renderDetail())).toContain("Cancel");
    expect(markup(f.harness.renderDetail())).toContain("aria-label=");
    await f.harness.confirmControl();
    expect(f.service).toHaveBeenCalledExactlyOnceWith("button", "press", { entity_id: "button.Gateway" });
    expect(f.harness._controlState?.message).toContain("Request sent to bridge");
    expect(f.harness._controlState?.message).toContain("does not confirm");
  });

  it("cancel sends nothing", async () => {
    const f = fixture();
    f.harness.beginControl(f.target);
    const cancel = templates(f.harness.renderDetail()).find((template) => template.strings.join("").includes(">Cancel</button>"));
    const callbacks = cancel?.values.filter((value): value is () => void => typeof value === "function");
    callbacks?.at(-1)?.();
    await f.harness.confirmControl();
    expect(f.service).not.toHaveBeenCalled();
  });

  it("protects against double confirmation while pending", async () => {
    let complete!: () => void;
    const service = vi.fn(async () => new Promise<void>((resolve) => { complete = resolve; }));
    const f = fixture(service);
    f.harness.beginControl(f.target);
    const pending = f.harness.confirmControl();
    expect(f.harness._controlPending).toBe(true);
    await f.harness.confirmControl();
    f.harness.beginControl(f.target);
    expect(f.service).toHaveBeenCalledOnce();
    expect(markup(f.harness.renderDetail())).toContain("?disabled=");
    complete();
    await pending;
    expect(f.harness._controlPending).toBe(false);
  });

  it.each(["unavailable", "removed", "duplicated", "foreign", "rewired"])("revalidates %s immediately before sending", async (change) => {
    const f = fixture();
    f.harness.beginControl(f.target);
    const button = f.hass.states["button.Gateway"]!;
    const states = { ...f.hass.states };
    if (change === "unavailable") states[button.entity_id] = { ...button, state: "unavailable" };
    if (change === "removed") delete states[button.entity_id];
    if (change === "duplicated") states["button.copy"] = { ...button, entity_id: "button.copy" };
    if (change === "foreign") states[button.entity_id] = { ...button, attributes: { ...button.attributes, bridge_id: "other" } };
    if (change === "rewired") states[button.entity_id] = { ...button, attributes: { ...button.attributes, target_mac: "11:22:33:44:55:66" } };
    f.card.hass = { ...f.hass, states }; // Deliberately do not run Lit lifecycle.
    await f.harness.confirmControl();
    expect(f.service).not.toHaveBeenCalled();
    expect(f.harness._controlState?.phase).toBe("error");
  });

  it("clears a confirmation when the selection changes, including switching back", async () => {
    const f = fixture();
    f.harness.beginControl(f.target);
    f.harness.selectClient(f.harness._model.clients[0]!.key);
    f.harness.selectDevice(f.harness._model.devices.find((device) => device.name === "Gateway")!.key);
    await f.harness.confirmControl();
    expect(f.service).not.toHaveBeenCalled();
  });

  it("clears a confirmation when the bridge scope changes", async () => {
    const f = fixture();
    f.harness.beginControl(f.target);
    f.card.setConfig({ type: "custom:omada-network-card", bridge_id: "other" });
    await f.harness.confirmControl();
    expect(f.service).not.toHaveBeenCalled();
  });

  it("does not show completion on a different selection or allow another request while pending", async () => {
    let complete!: () => void;
    const f = fixture(vi.fn(async () => new Promise<void>((resolve) => { complete = resolve; })));
    f.harness.beginControl(f.target);
    const pending = f.harness.confirmControl();
    const client = f.harness._model.clients[0]!;
    f.harness.selectClient(client.key);
    f.harness.willUpdate(new Map([["_selection", undefined]]));
    f.harness.beginControl(controlTarget("client", client)!);
    expect(f.harness._controlState).toBeUndefined();
    complete();
    await pending;
    expect(f.harness._controlState).toBeUndefined();
    expect(f.service).toHaveBeenCalledOnce();
  });

  it("shows a service error without echoing sensitive server details", async () => {
    const f = fixture(vi.fn(async () => { throw new Error("sensitive server response"); }));
    f.harness.beginControl(f.target);
    await f.harness.confirmControl();
    expect(f.harness._controlState?.phase).toBe("error");
    expect(f.harness._controlState?.message).toContain("Unable to send");
    expect(f.harness._controlState?.message).not.toContain("sensitive");
    expect(f.harness._controlPending).toBe(false);
  });

  it("hides actions without service access and excludes unsupported targets", () => {
    const f = fixture();
    f.card.hass = { states: f.hass.states };
    expect(markup(f.harness.renderDetail())).not.toContain("control-box");
    f.harness.beginControl(f.target);
    expect(f.harness._controlState).toBeUndefined();
  });

  it("offers a Wi-Fi confirmation for wireless client details", async () => {
    const f = fixture();
    const client = f.harness._model.clients[0]!;
    f.harness.selectClient(client.key);
    f.harness.willUpdate(new Map([["_selection", undefined]]));
    f.harness.beginControl(controlTarget("client", client)!);
    const content = templates(f.harness.renderDetail()).flatMap((template) => template.values).filter((value) => typeof value === "string").join(" ");
    expect(content).toContain("Reconnect Wi-Fi client");
    expect(content).toContain("momentarily disconnects");
    await f.harness.confirmControl();
    expect(f.service).toHaveBeenCalledExactlyOnceWith("button", "press", { entity_id: "button.phone" });
  });
});

function poeFixture(service?: NonNullable<HomeAssistant["callService"]>) {
  const f = service ? fixture(vi.fn(service)) : fixture();
  const ownerMac = "aa:bb:cc:dd:ee:ff";
  for (let port = 1; port <= 24; port++) {
    const entityId = `sensor.gateway_port_${port}`;
    f.hass.states[entityId] = { entity_id: entityId, state: "5", attributes: {
      ...f.scope, metric: "omada_port_power_watts", device_mac: ownerMac, device_type: "gateway", device_name: "Gateway",
      port: String(port), name: `Port ${port}`, poe: "true", link_status: "Connected"
    } };
  }
  f.hass.states["device_tracker.camera"] = { entity_id: "device_tracker.camera", state: "home", attributes: {
    ...f.scope, mac: "11:22:33:44:55:77", name: "Camera", wireless: "false", switch_mac: ownerMac, port: "1"
  } };
  for (const [port, mac, name] of [[1, "11:22:33:44:55:77", "Camera"], [24, "aa:bb:cc:dd:ee:11", "Hall AP"]] as const) {
    const entityId = `button.poe_${port}`;
    f.hass.states[entityId] = { entity_id: entityId, state: "unknown", attributes: {
      ...f.scope, omada_control: "power_cycle_poe", target_mac: ownerMac, target_port: port,
      switch_name: "Gateway", port_name: `Power ${port}`, powered_device_mac: mac, powered_device_name: name
    } };
  }
  f.card.hass = { ...f.hass, states: { ...f.hass.states } };
  f.harness.willUpdate(new Map([["hass", f.hass]]));
  const owner = f.harness._model.devices.find((device) => device.mac === ownerMac)!;
  const target = poePortTargets(f.card.hass, owner).find((entry) => entry.port === 1)!;
  return { ...f, target, owner };
}

describe("Omada PoE card controls", () => {
  it("allows confirmation warnings to wrap inside otherwise nonwrapping table cells", () => {
    const styles = OmadaNetworkCard.styles.map((style) => style.cssText).join("\n");
    expect(styles).toMatch(/\.control-box\s*\{[^}]*white-space:\s*normal/);
  });

  it("renders all port actions, including port 24, without sending commands", async () => {
    const f = poeFixture();
    const content = templates(f.harness.renderDetail()).flatMap((template) => template.values);
    expect(content).toContain("Port 24");
    expect(content.filter((value) => value === "Power-cycle PoE port")).toHaveLength(2);
    expect(f.service).not.toHaveBeenCalled();
    const target = poePortTargets(f.card.hass!, f.owner).find((entry) => entry.port === 24)!;
    f.harness.beginControl(target);
    await f.harness.confirmControl();
    expect(f.service).toHaveBeenCalledExactlyOnceWith("button", "press", { entity_id: "button.poe_24" });
  });

  it("confirms the named owner, physical port, endpoint and hard power loss", async () => {
    const f = poeFixture();
    f.harness.beginControl(f.target);
    const content = templates(f.harness.renderDetail()).flatMap((template) => template.values).filter((value) => typeof value === "string").join(" ");
    expect(content).toContain("Gateway · port 1 (Power 1)");
    expect(content).toContain("Camera");
    expect(content).toContain("slobidska44 · Apartment (site-1)");
    expect(content).toContain("cuts power to all equipment powered through this port");
    expect(content).toContain("hard power loss");
    expect(f.service).not.toHaveBeenCalled();
    await f.harness.confirmControl();
    expect(f.service).toHaveBeenCalledExactlyOnceWith("button", "press", { entity_id: "button.poe_1" });
    expect(f.harness._controlState?.message).toContain("does not confirm");
  });

  it.each(["client", "device"] as const)("offers a backend-bound powered %s shortcut", async (kind) => {
    const f = poeFixture();
    const record = kind === "client" ? f.harness._model.clients.find((entry) => entry.name === "Camera")! :
      f.harness._model.devices.find((entry) => entry.name === "Hall AP")!;
    if (kind === "client") f.harness.selectClient(record.key);
    else f.harness.selectDevice(record.key);
    f.harness.willUpdate(new Map([["_selection", undefined]]));
    const target = poePoweredTarget(f.card.hass!, kind, record)!;
    expect(target).toBeDefined();
    expect(templates(f.harness.renderDetail()).flatMap((template) => template.values)).toContain("Power-cycle PoE port");
    f.harness.beginControl(target);
    await f.harness.confirmControl();
    expect(f.service).toHaveBeenCalledExactlyOnceWith("button", "press", { entity_id: kind === "client" ? "button.poe_1" : "button.poe_24" });
  });

  it.each(["binding", "removed binding", "port", "owner", "name", "replaced entity", "duplicate", "unavailable", "bridge", "site"])(
    "rejects changed PoE %s immediately before confirmation", async (change) => {
      const f = poeFixture();
      f.harness.beginControl(f.target);
      const button = f.hass.states["button.poe_1"]!;
      const attributes = { ...button.attributes };
      const states = { ...f.hass.states };
      if (change === "binding") attributes.powered_device_mac = "22:22:33:44:55:66";
      if (change === "removed binding") delete attributes.powered_device_mac;
      if (change === "port") attributes.target_port = 2;
      if (change === "owner") attributes.target_mac = "aa:bb:cc:dd:ee:11";
      if (change === "name") attributes.port_name = "Different equipment";
      if (change === "bridge") attributes.bridge_id = "other";
      if (change === "site") attributes.site_id = "other";
      states[button.entity_id] = { ...button, attributes, state: change === "unavailable" ? "unavailable" : button.state };
      if (change === "replaced entity" || change === "duplicate") {
        states["button.replacement"] = { ...button, entity_id: "button.replacement" };
        if (change === "replaced entity") delete states[button.entity_id];
      }
      f.card.hass = { ...f.hass, states }; // Confirm against the fresh snapshot before Lit runs.
      await f.harness.confirmControl();
      expect(f.service).not.toHaveBeenCalled();
      expect(f.harness._controlState?.phase).toBe("error");
    }
  );

  it("cancels a powered endpoint confirmation when its binding changes during update", () => {
    const f = poeFixture();
    const client = f.harness._model.clients.find((entry) => entry.name === "Camera")!;
    f.harness.selectClient(client.key);
    f.harness.willUpdate(new Map([["_selection", undefined]]));
    f.harness.beginControl(poePoweredTarget(f.card.hass!, "client", client)!);
    const previous = f.card.hass;
    const button = f.hass.states["button.poe_1"]!;
    f.card.hass = { ...f.hass, states: { ...f.hass.states, [button.entity_id]: { ...button,
      attributes: { ...button.attributes, powered_device_mac: "22:22:33:44:55:66" }
    } } };
    f.harness.willUpdate(new Map([["hass", previous]]));
    expect(f.harness._controlState).toBeUndefined();
    expect(f.service).not.toHaveBeenCalled();
  });

  it("does not infer a power action from a Wi-Fi AP or wired attachment", () => {
    const f = poeFixture();
    const client = f.harness._model.clients.find((entry) => entry.name === "Phone")!;
    f.harness.selectClient(client.key);
    f.harness.willUpdate(new Map([["_selection", undefined]]));
    expect(templates(f.harness.renderDetail()).flatMap((template) => template.values)).not.toContain("Power-cycle PoE port");
    const button = f.hass.states["button.poe_1"]!;
    f.card.hass = { ...f.hass, states: { ...f.hass.states, [button.entity_id]: { ...button,
      attributes: { ...button.attributes, powered_device_mac: undefined }
    } } };
    f.harness.selectClient(f.harness._model.clients.find((entry) => entry.name === "Camera")!.key);
    f.harness.willUpdate(new Map([["hass", f.hass], ["_selection", undefined]]));
    expect(templates(f.harness.renderDetail()).flatMap((template) => template.values)).not.toContain("Power-cycle PoE port");
    expect(f.service).not.toHaveBeenCalled();
  });

  it("cancels port controls on selection changes and never starts a hidden foreign target", async () => {
    const f = poeFixture();
    f.harness.beginControl(f.target);
    const ap = f.harness._model.devices.find((device) => device.name === "Hall AP")!;
    f.harness.selectDevice(ap.key);
    await f.harness.confirmControl();
    f.harness.beginControl(f.target);
    expect(f.harness._controlState).toBeUndefined();
    expect(f.service).not.toHaveBeenCalled();
  });

  it("keeps one pending command across reboot and multiple PoE ports", async () => {
    let complete!: () => void;
    const f = poeFixture(async () => new Promise<void>((resolve) => { complete = resolve; }));
    f.harness.beginControl(f.target);
    const pending = f.harness.confirmControl();
    f.harness.beginControl(controlTarget("device", f.owner)!);
    f.harness.beginControl(poePortTargets(f.card.hass!, f.owner).find((entry) => entry.port === 24)!);
    await f.harness.confirmControl();
    expect(f.service).toHaveBeenCalledOnce();
    complete();
    await pending;
    expect(f.harness._controlPending).toBe(false);
  });
});
