import type { ClientRecord, DeviceRecord, HassEntity, HomeAssistant } from "./ha-types";

export type ControlAction = "reboot_device" | "reconnect_client";
export interface ControlTarget {
  action: ControlAction;
  key: string;
  mac: string;
  name: string;
  bridgeId: string;
  siteId: string;
  site: string;
}

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
    (entity.attributes.omada_control === "reboot_device" || entity.attributes.omada_control === "reconnect_client");
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

/** Never guess entity IDs or choose one of multiple matching button entities. */
export function findControlEntity(hass: HomeAssistant, target: ControlTarget): HassEntity | undefined {
  const matches = Object.values(hass.states).filter((entity) => {
    if (!isControlEntity(entity) || entity.attributes.omada_control !== target.action) return false;
    if (macKey(entity.attributes.target_mac) !== target.mac || text(entity.attributes.bridge_id) !== target.bridgeId) return false;
    const siteId = text(entity.attributes.site_id);
    if (target.siteId || siteId) return !!target.siteId && siteId === target.siteId;
    return !!target.site && text(entity.attributes.site) === target.site;
  });
  return matches.length === 1 ? matches[0] : undefined;
}

export function controlAvailable(entity: HassEntity): boolean {
  // HA buttons remain `unknown` until first pressed; it is not an availability
  // failure. MQTT availability is represented by HA's `unavailable` state.
  return entity.state.trim().toLowerCase() !== "unavailable";
}

export function controlScopeLabel(target: ControlTarget): string {
  return `${target.bridgeId || "Legacy bridge"} · ${target.site || target.siteId}${target.siteId && target.site ? ` (${target.siteId})` : ""}`;
}
