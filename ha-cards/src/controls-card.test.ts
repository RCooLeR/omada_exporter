import type { TemplateResult } from "lit";
import { describe, expect, it, vi } from "vitest";
import type { DashboardModel, HomeAssistant } from "./ha-types";
import type { ControlTarget } from "./control-model";
import { controlTarget } from "./control-model";

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
