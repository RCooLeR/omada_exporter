import type { ClientRecord, DeviceRecord, HassEntity, HomeAssistant } from "./ha-types";

export type ControlAction = "reboot_device" | "reconnect_client" | "power_cycle_poe";
interface ControlIdentity {
  key: string;
  mac: string;
  name: string;
  bridgeId: string;
  siteId: string;
  site: string;
}

export type PoeControlTarget = ControlIdentity & {
  action: "power_cycle_poe";
  port: number;
  switchName: string;
  portName: string;
  poweredDeviceMac: string;
  poweredDeviceName: string;
};

export type ControlTarget = (ControlIdentity & { action: "reboot_device" | "reconnect_client" }) | PoeControlTarget;

function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function macKey(value: unknown): string {
  const mac = text(value).toLowerCase();
  // Accept compact, colon, hyphen and Cisco dotted MAC notation, not arbitrary
  // strings from which removing characters might accidentally produce a MAC.
  if (!/^(?:[\da-f]{12}|(?:[\da-f]{2}[:-]){5}[\da-f]{2}|(?:[\da-f]{4}\.){2}[\da-f]{4})$/.test(mac)) return "";
  return mac.replace(/[:.-]/g, "");
}

export function isControlEntity(entity: HassEntity): boolean {
  return entity.entity_id.startsWith("button.") &&
    (entity.attributes.omada_control === "reboot_device" || entity.attributes.omada_control === "reconnect_client" ||
      entity.attributes.omada_control === "power_cycle_poe");
}

function matchesScope(attrs: Record<string, unknown>, target: Pick<ControlIdentity, "bridgeId" | "siteId" | "site">): boolean {
  if (text(attrs.bridge_id) !== target.bridgeId) return false;
  const siteId = text(attrs.site_id);
  if (target.siteId || siteId) return !!target.siteId && siteId === target.siteId;
  return !!target.site && text(attrs.site) === target.site;
}

function recordScope(record: DeviceRecord | ClientRecord) {
  return { bridgeId: text(record.attrs.bridge_id), siteId: text(record.attrs.site_id), site: text(record.site) };
}

export function controlTarget(kind: "device" | "client", record: DeviceRecord | ClientRecord): ControlTarget | undefined {
  if (kind === "device" && !["ap", "switch", "gateway"].includes((record as DeviceRecord).type)) return undefined;
  if (kind === "client" && !(record as ClientRecord).wireless) return undefined;
  const mac = macKey(record.mac);
  const siteId = text(record.attrs.site_id);
  const site = text(record.site);
  if (!mac || (!siteId && !site)) return undefined;
  const bridgeId = text(record.attrs.bridge_id);
  const action = kind === "device" ? "reboot_device" : "reconnect_client";
  return {
    action, mac, name: record.name, bridgeId, siteId, site,
    key: JSON.stringify([action, mac, bridgeId, siteId, site, record.name])
  };
}

function poeTarget(entity: HassEntity): PoeControlTarget | undefined {
  if (!isControlEntity(entity) || entity.attributes.omada_control !== "power_cycle_poe") return undefined;
  const attrs = entity.attributes;
  const mac = macKey(attrs.target_mac);
  const port = attrs.target_port;
  const siteId = text(attrs.site_id);
  const site = text(attrs.site);
  if (!mac || typeof port !== "number" || !Number.isInteger(port) || port < 1 || port > 255 || (!siteId && !site)) return undefined;
  const poweredDeviceMac = macKey(attrs.powered_device_mac);
  if ((attrs.powered_device_mac !== undefined && attrs.powered_device_mac !== "" && !poweredDeviceMac) || poweredDeviceMac === mac) return undefined;
  const bridgeId = text(attrs.bridge_id);
  const switchName = text(attrs.switch_name) || mac;
  const portName = text(attrs.port_name);
  const poweredDeviceName = text(attrs.powered_device_name);
  const action = "power_cycle_poe";
  return {
    action, mac, port, bridgeId, siteId, site, switchName, portName, poweredDeviceMac, poweredDeviceName,
    name: `${switchName} · port ${port}${portName ? ` (${portName})` : ""}`,
    key: JSON.stringify([action, mac, port, bridgeId, siteId, site, switchName, portName, poweredDeviceMac, poweredDeviceName])
  };
}

/** Port eligibility comes from discovery, never from power readings or PoE capability labels. */
export function poePortTargets(hass: HomeAssistant, device: DeviceRecord): PoeControlTarget[] {
  if (!["switch", "gateway"].includes(device.type)) return [];
  const mac = macKey(device.mac);
  const scope = recordScope(device);
  const ports = new Set(device.ports
    .filter((port) => macKey(port.deviceMac) === mac && matchesScope(port.attrs, scope) && /^[1-9]\d{0,2}$/.test(port.port))
    .map((port) => Number(port.port)));
  return Object.values(hass.states).flatMap((entity) => {
    const target = poeTarget(entity);
    return target && target.mac === mac && matchesScope(entity.attributes, scope) && ports.has(target.port) && findControlEntity(hass, target)
      ? [target] : [];
  });
}

/** A convenience action needs a unique backend-proven powered endpoint, not an attachment path. */
export function poePoweredTarget(hass: HomeAssistant, kind: "device" | "client", record: DeviceRecord | ClientRecord): PoeControlTarget | undefined {
  if (kind === "client" && (record as ClientRecord).wireless) return undefined;
  if (kind === "device" && !["ap", "switch", "gateway"].includes((record as DeviceRecord).type)) return undefined;
  const mac = macKey(record.mac);
  if (!mac) return undefined;
  const scope = recordScope(record);
  const matches = Object.values(hass.states).filter((entity) =>
    isControlEntity(entity) && entity.attributes.omada_control === "power_cycle_poe" &&
    macKey(entity.attributes.powered_device_mac) === mac && matchesScope(entity.attributes, scope));
  if (matches.length !== 1) return undefined;
  const target = poeTarget(matches[0]!);
  return target && findControlEntity(hass, target) ? target : undefined;
}

/** Never guess entity IDs or choose one of multiple matching button entities. */
export function findControlEntity(hass: HomeAssistant, target: ControlTarget): HassEntity | undefined {
  const matches = Object.values(hass.states).filter((entity) => {
    if (!isControlEntity(entity) || entity.attributes.omada_control !== target.action) return false;
    if (macKey(entity.attributes.target_mac) !== target.mac || !matchesScope(entity.attributes, target)) return false;
    return target.action !== "power_cycle_poe" || entity.attributes.target_port === target.port;
  });
  if (matches.length !== 1) return undefined;
  const entity = matches[0]!;
  // Include the discovered endpoint binding and display identity, not just the
  // port address, so a pending confirmation cannot silently change its target.
  if (target.action === "power_cycle_poe" && poeTarget(entity)?.key !== target.key) return undefined;
  return entity;
}

export function controlAvailable(entity: HassEntity): boolean {
  // HA buttons remain `unknown` until first pressed; it is not an availability
  // failure. MQTT availability is represented by HA's `unavailable` state.
  return entity.state.trim().toLowerCase() !== "unavailable";
}

export function controlScopeLabel(target: ControlTarget): string {
  return `${target.bridgeId || "Legacy bridge"} · ${target.site || target.siteId}${target.siteId && target.site ? ` (${target.siteId})` : ""}`;
}
