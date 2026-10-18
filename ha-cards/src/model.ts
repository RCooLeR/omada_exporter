import type {
  ClientRecord,
  DashboardModel,
  DashboardScope,
  DeviceRecord,
  DpiApplicationRecord,
  DpiCategoryRecord,
  DpiSiteRecord,
  DpiSummary,
  HassEntity,
  HomeAssistant,
  LinkRow,
  PortRecord,
  RadioRecord,
  SiteSummary
} from "./ha-types";
import { toNumber } from "./format";
import { attachNetworkQuality, isNetworkQualityMetric } from "./network-quality-model";
import { isControlEntity } from "./control-model";
import { linkMetricValue } from "./wan-status-model";

const dashboardModelCache = new WeakMap<HomeAssistant["states"], Map<string, DashboardModel>>();
const recordUpdatedAt = new WeakMap<object, number>();
const metricUpdatedAt = new WeakMap<object, Map<string, number>>();

function entityObservedAt(entity: HassEntity): number {
  const value = String(entity.attributes.last_updated ?? entity.last_updated ?? "").trim();
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function isLatestRecord(record: object, entity: HassEntity): boolean {
  const observedAt = entityObservedAt(entity);
  const previous = recordUpdatedAt.get(record) ?? -1;
  if (observedAt < previous) {
    return false;
  }
  recordUpdatedAt.set(record, observedAt);
  return true;
}

function setLatestMetric(
  record: { metrics: Record<string, number> },
  metric: string,
  value: number | undefined,
  entity: HassEntity
): void {
  let timestamps = metricUpdatedAt.get(record);
  if (!timestamps) {
    timestamps = new Map<string, number>();
    metricUpdatedAt.set(record, timestamps);
  }

  let observedAt = entityObservedAt(entity);
  if (value === undefined) {
    // HA can mark a sensor unavailable without updating its retained MQTT
    // attributes. That newer availability state must invalidate older values.
    const stateChangedAt = Date.parse(entity.last_updated ?? "");
    if (Number.isFinite(stateChangedAt)) observedAt = Math.max(observedAt, stateChangedAt);
  }
  if (observedAt < (timestamps.get(metric) ?? -1)) {
    return;
  }
  timestamps.set(metric, observedAt);
  if (value === undefined) delete record.metrics[metric];
  else record.metrics[metric] = value;
}

function getMetric(entity: HassEntity): string {
  return String(entity.attributes.metric ?? "");
}

function attrString(entity: HassEntity, key: string): string {
  const value = entity.attributes[key];
  return value == null ? "" : String(value);
}

function entityFriendlyName(entity: HassEntity): string {
  return attrString(entity, "friendly_name");
}

function firstString(entity: HassEntity, ...keys: string[]): string {
  for (const key of keys) {
    const value = attrString(entity, key);
    if (value) {
      return value;
    }
  }

  return "";
}

function isClientTrackerEntity(entity: HassEntity): boolean {
  return entity.entity_id.startsWith("device_tracker.") && attrString(entity, "mac") !== "";
}

export function normalizeMacKey(mac: string): string {
  const normalized = mac.trim().toLowerCase();
  const compact = normalized.replace(/[^0-9a-f]/g, "");
  return compact.length === 12 ? compact : normalized;
}

export function isDeviceConnected(status: string): boolean {
  return /^connected(?:\(|$)/i.test(status.trim());
}

function isDashboardEntity(entity: HassEntity): boolean {
  return isControlEntity(entity) || getMetric(entity).startsWith("omada_") || isClientTrackerEntity(entity) || looksLikeClientEntity(entity);
}

function dashboardStatesChanged(current: HomeAssistant["states"], previous: HomeAssistant["states"], scope: DashboardScope): boolean {
  if (Object.is(current, previous)) {
    return false;
  }

  for (const [entityId, entity] of Object.entries(current)) {
    if (matchesScope(entity, scope) && isDashboardEntity(entity) && previous[entityId] !== entity) {
      return true;
    }
  }

  for (const [entityId, entity] of Object.entries(previous)) {
    if (matchesScope(entity, scope) && isDashboardEntity(entity) && current[entityId] !== entity) {
      return true;
    }
  }

  return false;
}

/**
 * Lit change detector for Home Assistant's frequently replaced `hass` object.
 * Entity state objects are immutable in Home Assistant, so identity changes are
 * sufficient to skip full dashboard rebuilds caused by unrelated integrations.
 */
export function cardHassChanged(current: HomeAssistant | undefined, previous: HomeAssistant | undefined, scope: DashboardScope = {}): boolean {
  if (Object.is(current, previous)) {
    return false;
  }
  if (!current || !previous) {
    return true;
  }
  if (current.themes?.darkMode !== previous.themes?.darkMode) {
    return true;
  }
  if (typeof current.callService !== typeof previous.callService) return true;

  return dashboardStatesChanged(current.states, previous.states, scope);
}

function isActiveClientTracker(entity: HassEntity): boolean {
  return isClientTrackerEntity(entity) && entity.state.trim().toLowerCase() === "home";
}

function looksLikeClientEntity(entity: HassEntity): boolean {
  if (attrString(entity, "mac") === "") {
    return false;
  }

  if (attrString(entity, "device_mac") !== "") {
    return false;
  }

  return [
    "ap_mac",
    "switch_mac",
    "gateway_mac",
    "ssid",
    "connect_dev_type",
    "wireless",
    "wifi_mode",
    "device_category"
  ].some((key) => attrString(entity, key) !== "");
}

function preferredClientName(entity: HassEntity): string {
  return (
    attrString(entity, "name") ||
    entityFriendlyName(entity) ||
    attrString(entity, "host_name") ||
    attrString(entity, "system_name") ||
    attrString(entity, "ip") ||
    attrString(entity, "mac") ||
    "Unnamed client"
  );
}

function vpnRowKey(entity: HassEntity): string {
  const name = attrString(entity, "name").trim();
  const mode = attrString(entity, "vpn_mode").trim();
  const vpnType = attrString(entity, "vpn_type").trim();
  const vpnId = attrString(entity, "vpn_id").trim();

  const modeTypeNameKey = [name, mode, vpnType].filter(Boolean).join(":");
  if (modeTypeNameKey) {
    return modeTypeNameKey;
  }

  if (vpnId) {
    return `vpn_id:${vpnId}`;
  }

  return entity.entity_id;
}

function isVpnMetric(metric: string): boolean {
  return metric.startsWith("omada_vpn_") || metric.startsWith("omada_site_to_site_vpn_");
}

function isVpnPeerMetric(metric: string): boolean {
  return metric.startsWith("omada_site_to_site_vpn_peer_");
}

function vpnPeerRowKey(entity: HassEntity): string {
  const vpnId = attrString(entity, "vpn_id").trim();
  const peerId = attrString(entity, "peer_id").trim();
  const peerName = attrString(entity, "peer_name").trim();
  const remoteIp = attrString(entity, "remote_ip").trim();

  return ["vpn_peer", vpnId, peerId || peerName || remoteIp].filter(Boolean).join(":") || entity.entity_id;
}

function rowAttrString(row: LinkRow, key: string): string {
  const value = row.attrs[key];
  return value == null ? "" : String(value).trim();
}

function firstRowAttr(row: LinkRow, ...keys: string[]): string {
  for (const key of keys) {
    const value = rowAttrString(row, key);
    if (value) {
      return value;
    }
  }

  return "";
}

export function vpnRemoteLabel(row: LinkRow): string {
  return (
    firstRowAttr(
      row,
      "remote_ip_preferred",
      "endpoint_ip",
      "endpoint",
      "remote_ip",
      "remote_ip_runtime",
      "remote_peer_ip",
      "allowed_ips",
      "remote_networks"
    ) || "-"
  );
}

export function vpnModeLabel(row: LinkRow): string {
  return firstRowAttr(row, "vpn_mode", "site_vpn_type", "purpose", "vpn_type") || "-";
}

export function vpnTotalBytes(row: LinkRow): number {
  const tunnelTotal = (row.metrics.omada_vpn_up_bytes ?? 0) + (row.metrics.omada_vpn_down_bytes ?? 0);
  const siteToSiteTotal =
    (row.metrics.omada_site_to_site_vpn_up_bytes ?? 0) + (row.metrics.omada_site_to_site_vpn_down_bytes ?? 0);
  const directPeerTotal =
    (row.metrics.omada_site_to_site_vpn_peer_up_bytes ?? 0) + (row.metrics.omada_site_to_site_vpn_peer_down_bytes ?? 0);
  const peerTotal =
    (row.metrics.omada_site_to_site_vpn_peer_up_bytes_total ?? 0) +
    (row.metrics.omada_site_to_site_vpn_peer_down_bytes_total ?? 0);

  return tunnelTotal || siteToSiteTotal || directPeerTotal || peerTotal;
}

export function vpnUptimeSeconds(row: LinkRow): number {
  return row.metrics.omada_vpn_uptime ?? 0;
}

export function vpnPeerLoginSeconds(row: LinkRow): number {
  return row.metrics.omada_site_to_site_vpn_peer_login_timestamp ?? 0;
}

function normalizeScope(scope: string | DashboardScope | undefined): DashboardScope {
  return typeof scope === "string" ? { site: scope } : scope ?? {};
}

function matchesScope(entity: HassEntity, scope: DashboardScope): boolean {
  const bridge = scope.bridge_id?.trim() ?? "";
  if (attrString(entity, "bridge_id").trim() !== bridge) return false;
  if (scope.site_id && attrString(entity, "site_id") !== scope.site_id) return false;
  if (!scope.site) return true;
  const site = attrString(entity, "site");
  // Preserve old site-less tracker support only for the legacy bridge. A
  // named bridge never borrows another integration's unscoped entities.
  return site === scope.site || (!bridge && !scope.site_id && !site);
}

export function scopedRecordKey(attrs: Record<string, unknown>, key: string): string {
  const bridge = String(attrs.bridge_id ?? "").trim();
  const siteId = String(attrs.site_id ?? "").trim();
  const site = String(attrs.site ?? "").trim();
  // Tag ID/name separately: a literal site name such as "id:123" must not
  // collide with a controller-issued site ID of "123".
  return bridge || siteId || site ? JSON.stringify([bridge, siteId ? ["id", siteId] : ["name", site], key]) : key;
}

function scopedEntityKey(entity: HassEntity, key: string): string {
  return scopedRecordKey(entity.attributes, key);
}

function linkOwner(attrs: Record<string, unknown>): string {
  return normalizeMacKey(String(attrs.device_mac || attrs.gateway_mac || ""));
}

function scopedLinkKey(entity: HassEntity, key: string): string {
  const owner = linkOwner(entity.attributes);
  return scopedEntityKey(entity, owner ? JSON.stringify([owner, key]) : key);
}

/** Unqualified aliases are safe only when exactly one row owns the key. */
function uniqueIndex<T>(rows: T[], key: (row: T) => string): Map<string, T> {
  const result = new Map<string, T>();
  const duplicates = new Set<string>();
  for (const row of rows) {
    const value = key(row);
    if (!value || duplicates.has(value)) continue;
    if (result.has(value)) {
      result.delete(value);
      duplicates.add(value);
    } else result.set(value, row);
  }
  return result;
}

/** Match ISP rows to their own gateway/site, never a global WAN 1 alias. */
export function findWanForRow(model: DashboardModel, row: LinkRow): LinkRow | undefined {
  const candidates = model.wans.filter((wan) =>
    scopedRecordKey(wan.attrs, "") === scopedRecordKey(row.attrs, "") &&
    linkOwner(wan.attrs) === linkOwner(row.attrs));
  const byName = candidates.filter((wan) => wan.name === row.name);
  const port = String(row.attrs.port ?? "");
  const byPort = port ? candidates.filter((wan) => String(wan.attrs.port ?? "") === port) : [];
  if (byName.length === 1 && (!port || String(byName[0]!.attrs.port ?? "") === port)) return byName[0];
  return byPort.length === 1 ? byPort[0] : undefined;
}

function ensureDevice(map: Map<string, DeviceRecord>, entity: HassEntity): DeviceRecord {
  const deviceMac = attrString(entity, "device_mac");
  const key = scopedEntityKey(entity, normalizeMacKey(deviceMac) || attrString(entity, "device_name") || entity.entity_id);
  let existing = map.get(key);

  if (!existing) {
    existing = {
      key,
      name: attrString(entity, "device_name") || "Unnamed device",
      type: attrString(entity, "device_type") || "device",
      model: attrString(entity, "device_model") || attrString(entity, "device_show_model"),
      status: attrString(entity, "device_status"),
      ip: attrString(entity, "device_ip"),
      mac: deviceMac,
      version: attrString(entity, "device_version"),
      site: attrString(entity, "site"),
      attrs: { ...entity.attributes },
      metrics: {},
      ports: [],
      radios: [],
      clients: []
    };
    map.set(key, existing);
  }

  if (isLatestRecord(existing, entity)) {
    existing.name = attrString(entity, "device_name") || existing.name || "Unnamed device";
    existing.type = attrString(entity, "device_type") || existing.type || "device";
    existing.model = attrString(entity, "device_model") || attrString(entity, "device_show_model") || existing.model;
    existing.status = attrString(entity, "device_status") || existing.status;
    existing.ip = attrString(entity, "device_ip") || existing.ip;
    existing.version = attrString(entity, "device_version") || existing.version;
    existing.site = attrString(entity, "site") || existing.site;
    existing.attrs = { ...entity.attributes };
  }

  return existing;
}

function ensureControllerDevice(
  map: Map<string, DeviceRecord>,
  entity: HassEntity,
  includeEntityAttributes = true
): DeviceRecord {
  const mac = firstString(entity, "device_mac", "mac");
  const key = scopedEntityKey(entity, normalizeMacKey(mac) || firstString(entity, "device_name", "name") || entity.entity_id);
  let existing = map.get(key);

  if (!existing) {
    existing = {
      key,
      name: firstString(entity, "device_name", "name") || "Omada Controller",
      type: "controller",
      model: firstString(entity, "device_model", "device_show_model", "device_type") || "Controller",
      status: firstString(entity, "device_status"),
      ip: firstString(entity, "device_ip", "ip"),
      mac,
      version: firstString(entity, "device_version"),
      site: attrString(entity, "site"),
      attrs: includeEntityAttributes ? { ...entity.attributes } : {},
      metrics: {},
      ports: [],
      radios: [],
      clients: []
    };
    map.set(key, existing);
  }

  if (isLatestRecord(existing, entity)) {
    existing.name = firstString(entity, "device_name", "name") || existing.name || "Omada Controller";
    existing.type = "controller";
    existing.model = firstString(entity, "device_model", "device_show_model", "device_type") || existing.model || "Controller";
    existing.status = firstString(entity, "device_status") || existing.status;
    existing.ip = firstString(entity, "device_ip", "ip") || existing.ip;
    existing.mac = mac || existing.mac;
    existing.version = firstString(entity, "device_version") || existing.version;
    existing.site = attrString(entity, "site") || existing.site;
    if (includeEntityAttributes) {
      existing.attrs = { ...entity.attributes };
    }
  }

  if (!existing.status) {
    existing.status = entity.state === "not_home" ? "Disconnected" : "Connected";
  }

  return existing;
}

function ensurePort(map: Map<string, PortRecord>, entity: HassEntity): PortRecord {
  const deviceMac = attrString(entity, "device_mac");
  const port = attrString(entity, "port");
  const key = scopedEntityKey(entity, `${normalizeMacKey(deviceMac)}:${port}`);
  let existing = map.get(key);

  if (!existing) {
    existing = {
      key,
      deviceMac,
      name: attrString(entity, "name") || `Port ${port}`,
      port,
      kind: attrString(entity, "type"),
      operation: attrString(entity, "operation"),
      status: attrString(entity, "link_status"),
      poe: attrString(entity, "poe") === "true",
      attrs: { ...entity.attributes },
      metrics: {},
      clients: []
    };
    map.set(key, existing);
  }

  if (isLatestRecord(existing, entity)) {
    existing.name = attrString(entity, "name") || `Port ${port}`;
    existing.kind = attrString(entity, "type");
    existing.operation = attrString(entity, "operation");
    existing.status = attrString(entity, "link_status");
    existing.poe = attrString(entity, "poe") === "true";
    existing.attrs = { ...entity.attributes };
  }

  return existing;
}

function ensureClient(map: Map<string, ClientRecord>, entity: HassEntity): ClientRecord {
  const mac = attrString(entity, "mac");
  const key = scopedEntityKey(entity, normalizeMacKey(mac) || attrString(entity, "name") || entity.entity_id);
  let existing = map.get(key);

  if (!existing) {
    existing = {
      key,
      name: preferredClientName(entity),
      mac,
      ip: attrString(entity, "ip"),
      vendor: attrString(entity, "vendor"),
      hostName: attrString(entity, "host_name") || attrString(entity, "system_name"),
      category: attrString(entity, "device_category"),
      clientType: attrString(entity, "device_type"),
      wireless: attrString(entity, "wireless") === "true",
      apMac: attrString(entity, "ap_mac"),
      apName: attrString(entity, "ap_name"),
      switchMac: attrString(entity, "switch_mac"),
      switchName: attrString(entity, "switch_name"),
      gatewayMac: attrString(entity, "gateway_mac"),
      gatewayName: attrString(entity, "gateway_name"),
      attachmentPort: attrString(entity, "port"),
      attachmentLagId: firstString(entity, "lag_id", "lagId"),
      attachmentLagPorts: "",
      ssid: attrString(entity, "ssid"),
      vlanId: attrString(entity, "vlan_id"),
      wifiMode: attrString(entity, "wifi_mode"),
      site: attrString(entity, "site"),
      attrs: { ...entity.attributes },
      metrics: {}
    };
    map.set(key, existing);
  }

  if (isLatestRecord(existing, entity)) {
    const trackerState = isClientTrackerEntity(entity) ? entity.state : existing.attrs.tracker_state;
    const incomingName = preferredClientName(entity);
    existing.name = incomingName || existing.name;
    existing.ip = attrString(entity, "ip");
    existing.vendor = attrString(entity, "vendor");
    existing.hostName = attrString(entity, "host_name") || attrString(entity, "system_name");
    existing.category = attrString(entity, "device_category");
    existing.clientType = attrString(entity, "device_type");
    existing.wireless = attrString(entity, "wireless") === "true";
    existing.apMac = attrString(entity, "ap_mac");
    existing.apName = attrString(entity, "ap_name");
    existing.switchMac = attrString(entity, "switch_mac");
    existing.switchName = attrString(entity, "switch_name");
    existing.gatewayMac = attrString(entity, "gateway_mac");
    existing.gatewayName = attrString(entity, "gateway_name");
    existing.attachmentPort = attrString(entity, "port");
    existing.attachmentLagId = firstString(entity, "lag_id", "lagId");
    existing.attachmentLagPorts = "";
    delete existing.attachmentLinkSpeedMbps;
    existing.ssid = existing.wireless ? attrString(entity, "ssid") : "";
    existing.vlanId = attrString(entity, "vlan_id");
    existing.wifiMode = existing.wireless ? attrString(entity, "wifi_mode") : "";
    existing.site = attrString(entity, "site");
    existing.attrs = {
      ...entity.attributes,
      entity_id: entity.entity_id,
      tracker_state: trackerState
    };
  }

  return existing;
}

function ensureLinkRow(map: Map<string, LinkRow>, entity: HassEntity, fallbackKeys: string[]): LinkRow {
  const key = scopedLinkKey(entity, fallbackKeys.find(Boolean) ?? entity.entity_id);
  let existing = map.get(key);

  if (!existing) {
    existing = {
      key,
      name: attrString(entity, "name") || key,
      status: attrString(entity, "status"),
      attrs: { ...entity.attributes },
      metrics: {}
    };
    map.set(key, existing);
  }

  existing.name ||= attrString(entity, "name") || key;
  existing.status ||= attrString(entity, "status");
  existing.attrs = { ...existing.attrs, ...entity.attributes };

  return existing;
}

function joinKeyParts(...parts: string[]): string {
  return parts.map((part) => part.trim()).filter(Boolean).join(":");
}

function ensureVpnParentRow(map: Map<string, LinkRow>, entity: HassEntity): LinkRow {
  const key = scopedLinkKey(entity, vpnRowKey(entity));
  let existing = map.get(key);

  if (!existing) {
    existing = {
      key,
      name: attrString(entity, "name") || key,
      status: "",
      attrs: {
        vpn_id: attrString(entity, "vpn_id"),
        name: attrString(entity, "name"),
        vpn_type: attrString(entity, "vpn_type"),
        site_vpn_type: attrString(entity, "site_vpn_type"),
        site: attrString(entity, "site"),
        site_id: attrString(entity, "site_id"),
        bridge_id: attrString(entity, "bridge_id"),
        device_mac: attrString(entity, "device_mac"),
        gateway_mac: attrString(entity, "gateway_mac")
      },
      metrics: {}
    };
    map.set(key, existing);
  }

  return existing;
}

function firstNumber(value: string): number | undefined {
  const match = value.match(/\d+/);
  if (!match) {
    return undefined;
  }

  const parsed = Number(match[0]);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function comparePorts(left: PortRecord, right: PortRecord): number {
  const leftPort = firstNumber(left.port) ?? firstNumber(left.name) ?? Number.MAX_SAFE_INTEGER;
  const rightPort = firstNumber(right.port) ?? firstNumber(right.name) ?? Number.MAX_SAFE_INTEGER;
  if (leftPort !== rightPort) {
    return leftPort - rightPort;
  }

  return left.name.localeCompare(right.name, undefined, { numeric: true, sensitivity: "base" });
}

function summaryFrom(devices: DeviceRecord[], clients: ClientRecord[], siteName: string): SiteSummary {
  const wiredClients = clients.filter((client) => !client.wireless).length;
  const wirelessClients = clients.filter((client) => client.wireless).length;
  const devicesOnline = devices.filter((device) => isDeviceConnected(device.status)).length;
  const devicesOffline = devices.length - devicesOnline;
  const maxCpuDevice = devices.reduce<DeviceRecord | undefined>((max, device) => {
    if (!max) {
      return device;
    }

    return (device.metrics.omada_device_cpu_percentage ?? 0) > (max.metrics.omada_device_cpu_percentage ?? 0) ? device : max;
  }, undefined);
  const maxMemDevice = devices.reduce<DeviceRecord | undefined>((max, device) => {
    if (!max) {
      return device;
    }

    return (device.metrics.omada_device_mem_percentage ?? 0) > (max.metrics.omada_device_mem_percentage ?? 0) ? device : max;
  }, undefined);
  const maxCpu = maxCpuDevice?.metrics.omada_device_cpu_percentage ?? 0;
  const maxMem = maxMemDevice?.metrics.omada_device_mem_percentage ?? 0;

  return {
    site: siteName,
    wiredClients,
    wirelessClients,
    devicesOnline,
    devicesOffline,
    gateways: devices.filter((device) => device.type === "gateway").length,
    switches: devices.filter((device) => device.type === "switch").length,
    aps: devices.filter((device) => device.type === "ap").length,
    controllers: devices.filter((device) => device.type === "controller").length,
    maxCpu,
    maxCpuDevice: maxCpuDevice?.name || "-",
    maxMem,
    maxMemDevice: maxMemDevice?.name || "-"
  };
}

const dpiMetrics = new Set([
  "omada_dpi_total_traffic_bytes",
  "omada_dpi_scrape_window_seconds",
  "omada_dpi_category_traffic_bytes",
  "omada_dpi_application_traffic_bytes"
]);

function dpiValue(entity: HassEntity): number | undefined {
  // Missing/unavailable data must not become an apparently healthy zero.
  const state = entity.state.trim();
  if (!state) {
    return undefined;
  }
  const value = Number(state);
  return Number.isFinite(value) && value >= 0 ? value : undefined;
}

function dpiObservedAt(entity: HassEntity): number {
  const observedAt = entityObservedAt(entity);
  if (dpiValue(entity) !== undefined) {
    return observedAt;
  }
  // HA can mark a retained sensor unavailable without replacing its MQTT
  // attributes, including the source timestamp. Honour that newer state.
  const stateChangedAt = Date.parse(entity.last_updated ?? "");
  return Number.isFinite(stateChangedAt) ? Math.max(observedAt, stateChangedAt) : observedAt;
}

function compareDpiRows(left: DpiCategoryRecord, right: DpiCategoryRecord): number {
  return right.bytes - left.bytes || left.name.localeCompare(right.name) || left.key.localeCompare(right.key);
}

function dpiSnapshotTimestamp(entity: HassEntity): number | undefined {
  // MQTT assigns every metric in one Gather the same source timestamp. HA's
  // individual state-change timestamps cannot identify a shared snapshot.
  const parsed = Date.parse(attrString(entity, "last_updated"));
  return Number.isFinite(parsed) ? parsed : undefined;
}

function buildDpiSummary(entities: HassEntity[]): DpiSummary {
  const siteGroups = new Map<string, { latest: HassEntity; entities: Map<string, HassEntity> }>();
  for (const entity of entities) {
    const metric = getMetric(entity);
    if (!dpiMetrics.has(metric)) {
      continue;
    }
    const siteId = attrString(entity, "site_id").trim();
    const site = attrString(entity, "site").trim();
    const siteKey = JSON.stringify(siteId ? ["id", siteId] : ["name", site]);
    const group = siteGroups.get(siteKey) ?? { latest: entity, entities: new Map<string, HassEntity>() };
    if (dpiObservedAt(entity) >= dpiObservedAt(group.latest)) {
      group.latest = entity;
    }
    const family = attrString(entity, "family_id").trim() || attrString(entity, "family_name").trim();
    const application = attrString(entity, "application_id").trim() || attrString(entity, "application_name").trim();
    const rowKey = JSON.stringify([
      metric,
      metric === "omada_dpi_category_traffic_bytes" || metric === "omada_dpi_application_traffic_bytes" ? family : "",
      metric === "omada_dpi_application_traffic_bytes" ? application : ""
    ]);
    const previous = group.entities.get(rowKey);
    if (!previous || dpiObservedAt(entity) >= dpiObservedAt(previous)) {
      // Select before validating so a newer unavailable duplicate cannot
      // resurrect an older retained metric with the same stable identity.
      group.entities.set(rowKey, entity);
    }
    siteGroups.set(siteKey, group);
  }

  const sites: DpiSiteRecord[] = [];
  for (const [key, group] of siteGroups) {
    const snapshotTimes = Array.from(group.entities.values())
      .filter((entity) => ["omada_dpi_total_traffic_bytes", "omada_dpi_scrape_window_seconds"].includes(getMetric(entity)))
      .map(dpiSnapshotTimestamp)
      .filter((timestamp): timestamp is number => timestamp !== undefined);
    const snapshotAt = snapshotTimes.length > 0 ? Math.max(...snapshotTimes) : undefined;
    const site: DpiSiteRecord = {
      key,
      site: attrString(group.latest, "site").trim() || "Unknown site",
      siteId: attrString(group.latest, "site_id").trim(),
      categories: [],
      applications: []
    };
    if (snapshotAt !== undefined) {
      site.observedAt = new Date(snapshotAt).toISOString();
    }
    for (const [rowKey, entity] of group.entities) {
      const entitySnapshotAt = dpiSnapshotTimestamp(entity);
      if (snapshotAt !== undefined && entitySnapshotAt !== undefined && entitySnapshotAt < snapshotAt) {
        // Disappearing/capped DPI rows can remain retained in HA. Do not show
        // a previous query window's applications as part of the current one.
        continue;
      }
      const value = dpiValue(entity);
      if (value === undefined) {
        continue;
      }
      const metric = getMetric(entity);
      if (metric === "omada_dpi_total_traffic_bytes") {
        site.totalBytes = value;
        continue;
      }
      if (metric === "omada_dpi_scrape_window_seconds") {
        site.windowSeconds = value;
        continue;
      }
      const category: DpiCategoryRecord = {
        key: JSON.stringify([key, rowKey]),
        siteKey: key,
        site: site.site,
        siteId: site.siteId,
        familyId: attrString(entity, "family_id").trim(),
        name: attrString(entity, "family_name").trim() || "Unknown category",
        bytes: value
      };
      if (metric === "omada_dpi_category_traffic_bytes") {
        site.categories.push(category);
      } else {
        const application: DpiApplicationRecord = {
          ...category,
          applicationId: attrString(entity, "application_id").trim(),
          categoryName: category.name,
          name: attrString(entity, "application_name").trim() || "Unknown application"
        };
        site.applications.push(application);
      }
    }
    site.categories.sort(compareDpiRows);
    site.applications.sort(compareDpiRows);
    // A scrape-window entity alone proves collection is configured, not that
    // usable traffic data exists. An explicit zero traffic value is valid.
    if (site.totalBytes !== undefined || site.categories.length > 0 || site.applications.length > 0) {
      sites.push(site);
    }
  }
  sites.sort((left, right) => left.site.localeCompare(right.site) || left.key.localeCompare(right.key));
  const summary: DpiSummary = {
    available: sites.length > 0,
    sites,
    categories: sites.flatMap((site) => site.categories).sort(compareDpiRows),
    applications: sites.flatMap((site) => site.applications).sort(compareDpiRows)
  };
  if (sites.length > 0 && sites.every((site) => site.totalBytes !== undefined)) {
    const totalBytes = sites.reduce((sum, site) => sum + (site.totalBytes ?? 0), 0);
    if (Number.isFinite(totalBytes)) {
      summary.totalBytes = totalBytes;
    }
  }
  return summary;
}

export function buildDashboardModel(hass: HomeAssistant, filter?: string | DashboardScope): DashboardModel {
  const scope = normalizeScope(filter);
  // Home Assistant exposes every MQTT entity as a flat dictionary. The card is
  // easier to render if we first group those entities back into domain objects:
  // devices, ports, clients, WAN rows, VPN rows, and ISP rows.
  const devices = new Map<string, DeviceRecord>();
  const ports = new Map<string, PortRecord>();
  const radios = new Map<string, RadioRecord>();
  const clients = new Map<string, ClientRecord>();
  const lags = new Map<
    string,
    { attrs: Record<string, unknown>; metrics: Record<string, number>; updatedAt: number; metricUpdatedAt: Map<string, number> }
  >();
  const isps = new Map<string, LinkRow>();
  const vpns = new Map<string, LinkRow>();
  const vpnPeers = new Map<string, LinkRow>();
  const wans = new Map<string, LinkRow>();
  const portByDeviceMacAndPort = new Map<string, PortRecord>();
  const infrastructureEntityByMac = new Map<string, HassEntity>();
  const clientTrackerByMac = new Map<string, HassEntity>();
  const dpiEntities: HassEntity[] = [];
  const qualityEntities: HassEntity[] = [];

  for (const entity of Object.values(hass.states)) {
    if (entity.entity_id.startsWith("button.") || !matchesScope(entity, scope)) {
      continue;
    }
    if (isClientTrackerEntity(entity)) {
      const mac = scopedEntityKey(entity, normalizeMacKey(attrString(entity, "mac")));
      const previous = clientTrackerByMac.get(mac);
      if (!previous || entityObservedAt(entity) >= entityObservedAt(previous)) {
        clientTrackerByMac.set(mac, entity);
      }
      continue;
    }
    const metric = getMetric(entity);
    const deviceMac = attrString(entity, "device_mac");
    if (deviceMac && (metric.startsWith("omada_device_") || metric.startsWith("omada_controller_"))) {
      const key = scopedEntityKey(entity, normalizeMacKey(deviceMac));
      const previous = infrastructureEntityByMac.get(key);
      if (!previous || entityObservedAt(entity) >= entityObservedAt(previous)) {
        infrastructureEntityByMac.set(key, entity);
      }
    }
  }

  for (const entity of Object.values(hass.states)) {
    if (entity.entity_id.startsWith("button.") || !matchesScope(entity, scope)) {
      continue;
    }

    if (isClientTrackerEntity(entity)) {
      const mac = scopedEntityKey(entity, normalizeMacKey(attrString(entity, "mac")));
      // The latest tracker state is the source of truth for dashboard
      // presence. Retained metric sensors can outlive the network session and
      // must not recreate clients that are away or unavailable.
      if (clientTrackerByMac.get(mac) !== entity || !isActiveClientTracker(entity) || infrastructureEntityByMac.has(mac)) {
        continue;
      }

      ensureClient(clients, entity);
      continue;
    }

    const metric = getMetric(entity);
    if (!metric) {
      const mac = scopedEntityKey(entity, normalizeMacKey(attrString(entity, "mac")));
      const activeTracker = clientTrackerByMac.get(mac);
      if (looksLikeClientEntity(entity) && activeTracker && isActiveClientTracker(activeTracker)) {
        ensureClient(clients, entity);
      }
      continue;
    }

    const value = toNumber(entity.state);

    if (isNetworkQualityMetric(metric)) {
      qualityEntities.push(entity);
      continue;
    }

    if (metric.startsWith("omada_dpi_")) {
      dpiEntities.push(entity);
      continue;
    }

    if (metric.startsWith("omada_device_")) {
      const device = ensureDevice(devices, entity);
      setLatestMetric(device, metric, value, entity);
      continue;
    }

    if (metric.startsWith("omada_controller_")) {
      const device = ensureControllerDevice(devices, entity);
      setLatestMetric(device, metric, value, entity);
      continue;
    }

    if (metric.startsWith("omada_port_")) {
      const port = ensurePort(ports, entity);
      portByDeviceMacAndPort.set(port.key, port);
      setLatestMetric(port, metric, value, entity);
      continue;
    }

    if (metric.startsWith("omada_radio_") || metric.startsWith("omada_ap_radio_")) {
      const deviceMac = attrString(entity, "device_mac");
      const band = attrString(entity, "band") || attrString(entity, "radio_name") || metric;
      const key = scopedEntityKey(entity, `${normalizeMacKey(deviceMac)}:${band}`);
      let radio = radios.get(key);

      if (!radio) {
        radio = {
          key,
          deviceMac,
          band,
          attrs: { ...entity.attributes },
          metrics: {}
        };
        radios.set(key, radio);
      }

      radio.metrics[metric] = value;
      radio.attrs = { ...radio.attrs, ...entity.attributes };
      continue;
    }

    if (metric.startsWith("omada_lag_")) {
      const deviceMac = attrString(entity, "device_mac");
      const lagId = attrString(entity, "lag_id");
      const key = scopedEntityKey(entity, `${normalizeMacKey(deviceMac)}:${lagId}`);
      const observedAt = entityObservedAt(entity);
      const existing =
        lags.get(key) ?? { attrs: {}, metrics: {}, updatedAt: -1, metricUpdatedAt: new Map<string, number>() };
      if (observedAt >= existing.updatedAt) {
        existing.attrs = { ...entity.attributes };
        existing.updatedAt = observedAt;
      }
      if (observedAt >= (existing.metricUpdatedAt.get(metric) ?? -1)) {
        existing.metrics[metric] = value;
        existing.metricUpdatedAt.set(metric, observedAt);
      }
      lags.set(key, existing);
      continue;
    }

    if (metric.startsWith("omada_client_")) {
      const rawMac = normalizeMacKey(attrString(entity, "mac"));
      if (!rawMac || metric === "omada_client_connected_total") {
        continue;
      }
      const mac = scopedEntityKey(entity, rawMac);
      const infrastructureEntity = infrastructureEntityByMac.get(mac);
      if (infrastructureEntity) {
        const infrastructureMetric = getMetric(infrastructureEntity);
        const device = infrastructureMetric.startsWith("omada_controller_")
          ? ensureControllerDevice(devices, infrastructureEntity)
          : ensureDevice(devices, infrastructureEntity);
        setLatestMetric(device, metric, value, entity);
        continue;
      }
      // Client classifications are descriptive, not infrastructure identity.
      // Only the scoped MAC lookup above can identify a real controller;
      // retained client metrics must still pass the active-tracker check.
      const activeTracker = clientTrackerByMac.get(mac);
      if (!activeTracker || !isActiveClientTracker(activeTracker)) {
        continue;
      }
      const client = ensureClient(clients, entity);
      setLatestMetric(client, metric, value, entity);
      continue;
    }

    if (metric.startsWith("omada_isp_")) {
      const row = ensureLinkRow(isps, entity, [
        joinKeyParts(attrString(entity, "name"), attrString(entity, "port")),
        attrString(entity, "ip")
      ]);
      setLatestMetric(row, metric, linkMetricValue(entity.state), entity);
      continue;
    }

    if (isVpnPeerMetric(metric)) {
      const peer = ensureLinkRow(vpnPeers, entity, [
        vpnPeerRowKey(entity),
        attrString(entity, "peer_id"),
        joinKeyParts(attrString(entity, "vpn_id"), attrString(entity, "peer_name")),
        joinKeyParts(attrString(entity, "name"), attrString(entity, "peer_name"))
      ]);
      peer.name = attrString(entity, "peer_name") || attrString(entity, "remote_ip") || attrString(entity, "peer_id") || peer.name;
      peer.metrics[metric] = value;

      const parent = ensureVpnParentRow(vpns, entity);
      if (metric === "omada_site_to_site_vpn_peer_down_bytes") {
        parent.metrics.omada_site_to_site_vpn_peer_down_bytes_total =
          (parent.metrics.omada_site_to_site_vpn_peer_down_bytes_total ?? 0) + value;
      }
      if (metric === "omada_site_to_site_vpn_peer_up_bytes") {
        parent.metrics.omada_site_to_site_vpn_peer_up_bytes_total =
          (parent.metrics.omada_site_to_site_vpn_peer_up_bytes_total ?? 0) + value;
      }
      continue;
    }

    if (isVpnMetric(metric)) {
      const row = ensureLinkRow(vpns, entity, [
        vpnRowKey(entity),
        attrString(entity, "vpn_id"),
        joinKeyParts(attrString(entity, "name"), attrString(entity, "vpn_mode"), attrString(entity, "vpn_type")),
        joinKeyParts(attrString(entity, "name"), attrString(entity, "vpn_mode"))
      ]);
      const remoteIp = attrString(entity, "remote_ip");
      if (remoteIp) {
        if (metric === "omada_vpn_status") {
          row.attrs.remote_ip_preferred = remoteIp;
        } else if (!row.attrs.remote_ip_runtime) {
          row.attrs.remote_ip_runtime = remoteIp;
        }
      }
      row.metrics[metric] = value;
      continue;
    }

    if (metric.startsWith("omada_wan_")) {
      const row = ensureLinkRow(wans, entity, [
        joinKeyParts(attrString(entity, "name"), attrString(entity, "port")),
        attrString(entity, "ip")
      ]);
      setLatestMetric(row, metric, linkMetricValue(entity.state), entity);
      continue;
    }
  }

  attachNetworkQuality(devices.values(), qualityEntities);

  for (const port of ports.values()) {
    const device = devices.get(scopedRecordKey(port.attrs, normalizeMacKey(port.deviceMac)));
    if (device) {
      device.ports.push(port);
    }
  }

  for (const device of devices.values()) {
    device.ports.sort(comparePorts);
  }

  for (const radio of radios.values()) {
    const device = devices.get(scopedRecordKey(radio.attrs, normalizeMacKey(radio.deviceMac)));
    if (device) {
      device.radios.push(radio);
    }
  }

  for (const client of clients.values()) {
    // LAG metrics remain owned by the parent switch/gateway. Copy only the
    // small set of attachment display values; never merge switch metrics or
    // attributes into the client itself.
    const lagId = client.attachmentLagId;
    const lagDeviceMac = client.switchMac || client.gatewayMac;
    if (lagId && lagId !== "0" && lagDeviceMac) {
      const lag = lags.get(scopedRecordKey(client.attrs, `${normalizeMacKey(lagDeviceMac)}:${lagId}`));
      if (lag) {
        client.attachmentLagPorts = String(lag.attrs.lag_ports ?? "").trim();
        const linkSpeed = lag.metrics.omada_lag_link_speed_mbps ?? Number(lag.attrs.link_speed ?? 0);
        if (Number.isFinite(linkSpeed)) {
          client.attachmentLinkSpeedMbps = linkSpeed;
        } else {
          delete client.attachmentLinkSpeedMbps;
        }
      }
    }

    if (client.switchMac && client.attachmentPort) {
      const port = portByDeviceMacAndPort.get(scopedRecordKey(client.attrs, `${normalizeMacKey(client.switchMac)}:${client.attachmentPort}`));
      if (port) {
        port.clients.push(client);
      }
    }

    const attachmentMac = client.apMac || client.switchMac || client.gatewayMac;
    if (attachmentMac) {
      const device = devices.get(scopedRecordKey(client.attrs, normalizeMacKey(attachmentMac)));
      if (device) {
        device.clients.push(client);
      }
    }
  }

  const deviceList = Array.from(devices.values()).sort((left, right) => {
    const typeOrder: Record<string, number> = {
      controller: 0,
      gateway: 1,
      switch: 2,
      ap: 3
    };
    const leftType = typeOrder[left.type] ?? 99;
    const rightType = typeOrder[right.type] ?? 99;
    if (leftType !== rightType) {
      return leftType - rightType;
    }

    const leftOnline = isDeviceConnected(left.status) ? 0 : 1;
    const rightOnline = isDeviceConnected(right.status) ? 0 : 1;
    if (leftOnline !== rightOnline) {
      return leftOnline - rightOnline;
    }

    return left.name.localeCompare(right.name);
  });

  const clientList = Array.from(clients.values()).sort((left, right) => {
    const leftSignal = left.metrics.omada_client_signal_pct ?? 0;
    const rightSignal = right.metrics.omada_client_signal_pct ?? 0;
    if (leftSignal !== rightSignal) {
      return rightSignal - leftSignal;
    }

    return left.name.localeCompare(right.name);
  });

  const siteName =
    scope.site ||
    deviceList[0]?.site ||
    clientList[0]?.site ||
    Array.from(isps.values())[0]?.attrs.site?.toString() ||
    "Omada";

  const ispList = Array.from(isps.values()).sort((left, right) => left.name.localeCompare(right.name));
  const vpnList = Array.from(vpns.values()).sort((left, right) => left.name.localeCompare(right.name));
  const vpnPeerList = Array.from(vpnPeers.values()).sort((left, right) => {
    const leftVpn = String(left.attrs.name ?? "");
    const rightVpn = String(right.attrs.name ?? "");
    if (leftVpn !== rightVpn) {
      return leftVpn.localeCompare(rightVpn);
    }
    return left.name.localeCompare(right.name);
  });
  const wanList = Array.from(wans.values()).sort((left, right) => left.name.localeCompare(right.name));

  // Retain legacy convenience lookups only where the MAC/port is unambiguous.
  // All internal joins and UI consumers use the scoped keys above.
  const deviceByMac = uniqueIndex(deviceList, (device) => normalizeMacKey(device.mac));
  for (const [key, port] of uniqueIndex(Array.from(ports.values()), (port) => `${normalizeMacKey(port.deviceMac)}:${port.port}`)) {
    portByDeviceMacAndPort.set(key, port);
  }

  return {
    siteSummary: summaryFrom(deviceList, clientList, siteName),
    dpi: buildDpiSummary(dpiEntities),
    devices: deviceList,
    clients: clientList,
    isps: ispList,
    vpns: vpnList,
    vpnPeers: vpnPeerList,
    wans: wanList,
    deviceByKey: new Map(deviceList.map((device) => [device.key, device])),
    deviceByMac,
    portByDeviceMacAndPort,
    clientByKey: new Map(clientList.map((client) => [client.key, client])),
    wanByName: uniqueIndex(wanList, (wan) => wan.name),
    wanByPort: uniqueIndex(wanList, (wan) => String(wan.attrs.port ?? ""))
  };
}

export function getDashboardModel(hass: HomeAssistant, filter?: string | DashboardScope): DashboardModel {
  const scope = normalizeScope(filter);
  const cacheKey = JSON.stringify([scope.bridge_id?.trim() ?? "", scope.site ?? "", scope.site_id ?? ""]);
  let cacheBySite = dashboardModelCache.get(hass.states);
  if (!cacheBySite) {
    cacheBySite = new Map<string, DashboardModel>();
    dashboardModelCache.set(hass.states, cacheBySite);
  }

  const cached = cacheBySite.get(cacheKey);
  if (cached) {
    return cached;
  }

  const model = buildDashboardModel(hass, scope);
  cacheBySite.set(cacheKey, model);
  return model;
}
