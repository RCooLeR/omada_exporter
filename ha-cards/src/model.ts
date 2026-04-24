import type {
  ClientRecord,
  DashboardModel,
  DeviceRecord,
  HassEntity,
  HomeAssistant,
  LinkRow,
  PortRecord,
  RadioRecord,
  SiteSummary
} from "./ha-types";
import { toNumber } from "./format";

const dashboardModelCache = new WeakMap<HomeAssistant["states"], Map<string, DashboardModel>>();

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

function isControllerEntity(entity: HassEntity): boolean {
  const deviceType = firstString(entity, "device_type", "device_model", "device_show_model").toLowerCase();
  return deviceType.includes("controller") || getMetric(entity).startsWith("omada_controller_");
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

function matchSite(entity: HassEntity, siteFilter?: string): boolean {
  if (!siteFilter) {
    return true;
  }

  const entitySite = attrString(entity, "site");
  if (!entitySite) {
    return true;
  }

  return entitySite === siteFilter;
}

function ensureDevice(map: Map<string, DeviceRecord>, entity: HassEntity): DeviceRecord {
  const deviceMac = attrString(entity, "device_mac");
  const key = deviceMac || attrString(entity, "device_name") || entity.entity_id;
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

  existing.name ||= attrString(entity, "device_name") || "Unnamed device";
  existing.type ||= attrString(entity, "device_type") || "device";
  existing.model ||= attrString(entity, "device_model") || attrString(entity, "device_show_model");
  existing.status ||= attrString(entity, "device_status");
  existing.ip ||= attrString(entity, "device_ip");
  existing.version ||= attrString(entity, "device_version");
  existing.site ||= attrString(entity, "site");
  existing.attrs = { ...existing.attrs, ...entity.attributes };

  return existing;
}

function ensureControllerDevice(map: Map<string, DeviceRecord>, entity: HassEntity): DeviceRecord {
  const mac = firstString(entity, "device_mac", "mac");
  const key = mac || firstString(entity, "device_name", "name") || entity.entity_id;
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
      attrs: { ...entity.attributes },
      metrics: {},
      ports: [],
      radios: [],
      clients: []
    };
    map.set(key, existing);
  }

  existing.name ||= firstString(entity, "device_name", "name") || "Omada Controller";
  existing.type = "controller";
  existing.model ||= firstString(entity, "device_model", "device_show_model", "device_type") || "Controller";
  existing.status ||= firstString(entity, "device_status");
  existing.ip ||= firstString(entity, "device_ip", "ip");
  existing.mac ||= mac;
  existing.version ||= firstString(entity, "device_version");
  existing.site ||= attrString(entity, "site");
  existing.attrs = { ...existing.attrs, ...entity.attributes };

  if (!existing.status) {
    existing.status = entity.state === "not_home" ? "Disconnected" : "Connected";
  }

  return existing;
}

function ensurePort(map: Map<string, PortRecord>, entity: HassEntity): PortRecord {
  const deviceMac = attrString(entity, "device_mac");
  const port = attrString(entity, "port");
  const key = `${deviceMac}:${port}:${attrString(entity, "name")}`;
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

  existing.name ||= attrString(entity, "name") || `Port ${port}`;
  existing.kind ||= attrString(entity, "type");
  existing.operation ||= attrString(entity, "operation");
  existing.status ||= attrString(entity, "link_status");
  existing.poe ||= attrString(entity, "poe") === "true";
  existing.attrs = { ...existing.attrs, ...entity.attributes };

  return existing;
}

function ensureClient(map: Map<string, ClientRecord>, entity: HassEntity, source: "metric" | "tracker" = "metric"): ClientRecord {
  const mac = attrString(entity, "mac");
  const key = mac || attrString(entity, "name") || entity.entity_id;
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
      port: attrString(entity, "port"),
      ssid: attrString(entity, "ssid"),
      vlanId: attrString(entity, "vlan_id"),
      wifiMode: attrString(entity, "wifi_mode"),
      site: attrString(entity, "site"),
      attrs: { ...entity.attributes },
      metrics: {}
    };
    map.set(key, existing);
  }

  if (source === "tracker") {
    const trackerName = preferredClientName(entity);
    if (trackerName) {
      existing.name = trackerName;
    }
  } else {
    existing.name ||= preferredClientName(entity);
  }
  existing.ip ||= attrString(entity, "ip");
  existing.vendor ||= attrString(entity, "vendor");
  existing.hostName ||= attrString(entity, "host_name") || attrString(entity, "system_name");
  existing.category ||= attrString(entity, "device_category");
  existing.clientType ||= attrString(entity, "device_type");
  existing.apMac ||= attrString(entity, "ap_mac");
  existing.apName ||= attrString(entity, "ap_name");
  existing.switchMac ||= attrString(entity, "switch_mac");
  existing.switchName ||= attrString(entity, "switch_name");
  existing.gatewayMac ||= attrString(entity, "gateway_mac");
  existing.gatewayName ||= attrString(entity, "gateway_name");
  existing.port ||= attrString(entity, "port");
  existing.ssid ||= attrString(entity, "ssid");
  existing.vlanId ||= attrString(entity, "vlan_id");
  existing.wifiMode ||= attrString(entity, "wifi_mode");
  existing.site ||= attrString(entity, "site");
  existing.wireless ||= attrString(entity, "wireless") === "true";
  existing.attrs = {
    ...existing.attrs,
    ...entity.attributes,
    entity_id: entity.entity_id,
    tracker_state: isClientTrackerEntity(entity) ? entity.state : existing.attrs.tracker_state
  };

  return existing;
}

function ensureLinkRow(map: Map<string, LinkRow>, entity: HassEntity, fallbackKeys: string[]): LinkRow {
  const key = fallbackKeys.find(Boolean) ?? entity.entity_id;
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
  const devicesOnline = devices.filter((device) => device.status.toLowerCase() === "connected").length;
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

export function buildDashboardModel(hass: HomeAssistant, siteFilter?: string): DashboardModel {
  const devices = new Map<string, DeviceRecord>();
  const ports = new Map<string, PortRecord>();
  const radios = new Map<string, RadioRecord>();
  const clients = new Map<string, ClientRecord>();
  const lags = new Map<string, { attrs: Record<string, unknown>; metrics: Record<string, number> }>();
  const isps = new Map<string, LinkRow>();
  const vpns = new Map<string, LinkRow>();
  const wans = new Map<string, LinkRow>();
  const deviceByMac = new Map<string, DeviceRecord>();
  const portByDeviceMacAndPort = new Map<string, PortRecord>();

  for (const entity of Object.values(hass.states)) {
    if (!matchSite(entity, siteFilter)) {
      continue;
    }

    if (isClientTrackerEntity(entity)) {
      if (entity.state === "not_home") {
        continue;
      }

      ensureClient(clients, entity, "tracker");
      continue;
    }

    const metric = getMetric(entity);
    if (!metric) {
      if (looksLikeClientEntity(entity) && entity.state !== "not_home") {
        ensureClient(clients, entity, "tracker");
      }
      continue;
    }

    const value = toNumber(entity.state);

    if (metric.startsWith("omada_device_")) {
      const device = ensureDevice(devices, entity);
      if (device.mac) {
        deviceByMac.set(device.mac, device);
      }
      device.metrics[metric] = value;
      continue;
    }

    if (metric.startsWith("omada_controller_")) {
      const device = ensureControllerDevice(devices, entity);
      if (device.mac) {
        deviceByMac.set(device.mac, device);
      }
      device.metrics[metric] = value;
      continue;
    }

    if (metric.startsWith("omada_port_")) {
      const port = ensurePort(ports, entity);
      portByDeviceMacAndPort.set(`${port.deviceMac}:${port.port}`, port);
      port.metrics[metric] = value;
      continue;
    }

    if (metric.startsWith("omada_radio_") || metric.startsWith("omada_ap_radio_")) {
      const deviceMac = attrString(entity, "device_mac");
      const band = attrString(entity, "band") || attrString(entity, "radio_name") || metric;
      const key = `${deviceMac}:${band}`;
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
      const key = `${deviceMac}:${lagId}`;
      const existing = lags.get(key) ?? { attrs: {}, metrics: {} };
      existing.attrs = { ...existing.attrs, ...entity.attributes };
      existing.metrics[metric] = value;
      lags.set(key, existing);
      continue;
    }

    if (metric.startsWith("omada_client_")) {
      if (!attrString(entity, "mac") || metric === "omada_client_connected_total") {
        continue;
      }
      if (isControllerEntity(entity)) {
        const controller = ensureControllerDevice(devices, entity);
        if (controller.mac) {
          deviceByMac.set(controller.mac, controller);
        }
        controller.metrics[metric] = value;
        continue;
      }
      const client = ensureClient(clients, entity, "metric");
      client.metrics[metric] = value;
      continue;
    }

    if (metric.startsWith("omada_isp_")) {
      const row = ensureLinkRow(isps, entity, [
        `${attrString(entity, "name")}:${attrString(entity, "port")}`,
        attrString(entity, "ip")
      ]);
      row.metrics[metric] = value;
      continue;
    }

    if (metric.startsWith("omada_vpn_")) {
      const row = ensureLinkRow(vpns, entity, [
        vpnRowKey(entity),
        attrString(entity, "vpn_id"),
        `${attrString(entity, "name")}:${attrString(entity, "vpn_mode")}:${attrString(entity, "vpn_type")}`,
        `${attrString(entity, "name")}:${attrString(entity, "vpn_mode")}`
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
        `${attrString(entity, "name")}:${attrString(entity, "port")}`,
        attrString(entity, "ip")
      ]);
      row.metrics[metric] = value;
      continue;
    }
  }

  for (const port of ports.values()) {
    const device = deviceByMac.get(port.deviceMac);
    if (device) {
      device.ports.push(port);
    }
  }

  for (const device of devices.values()) {
    device.ports.sort(comparePorts);
  }

  for (const radio of radios.values()) {
    const device = deviceByMac.get(radio.deviceMac);
    if (device) {
      device.radios.push(radio);
    }
  }

  for (const client of clients.values()) {
    const lagId = String(client.attrs.lag_id ?? client.attrs.lagId ?? "").trim();
    const lagDeviceMac = client.switchMac || client.gatewayMac;
    if (lagId && lagId !== "0" && lagDeviceMac) {
      const lag = lags.get(`${lagDeviceMac}:${lagId}`);
      if (lag) {
        client.attrs = { ...client.attrs, ...lag.attrs };
        client.metrics = { ...client.metrics, ...lag.metrics };
      }
    }

    if (client.switchMac && client.port) {
      const port = portByDeviceMacAndPort.get(`${client.switchMac}:${client.port}`);
      if (port) {
        port.clients.push(client);
      }
    }

    const attachmentMac = client.apMac || client.switchMac || client.gatewayMac;
    if (attachmentMac) {
      const device = deviceByMac.get(attachmentMac);
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

    const leftOnline = left.status === "Connected" ? 0 : 1;
    const rightOnline = right.status === "Connected" ? 0 : 1;
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
    siteFilter ||
    deviceList[0]?.site ||
    clientList[0]?.site ||
    Array.from(isps.values())[0]?.attrs.site?.toString() ||
    "Omada";

  const ispList = Array.from(isps.values()).sort((left, right) => left.name.localeCompare(right.name));
  const vpnList = Array.from(vpns.values()).sort((left, right) => left.name.localeCompare(right.name));
  const wanList = Array.from(wans.values()).sort((left, right) => left.name.localeCompare(right.name));

  return {
    siteSummary: summaryFrom(deviceList, clientList, siteName),
    devices: deviceList,
    clients: clientList,
    isps: ispList,
    vpns: vpnList,
    wans: wanList,
    deviceByKey: new Map(deviceList.map((device) => [device.key, device])),
    deviceByMac,
    portByDeviceMacAndPort,
    clientByKey: new Map(clientList.map((client) => [client.key, client])),
    wanByName: new Map(wanList.map((wan) => [wan.name, wan])),
    wanByPort: new Map(wanList.map((wan) => [String(wan.attrs.port), wan]))
  };
}

export function getDashboardModel(hass: HomeAssistant, siteFilter?: string): DashboardModel {
  const cacheKey = siteFilter ?? "";
  let cacheBySite = dashboardModelCache.get(hass.states);
  if (!cacheBySite) {
    cacheBySite = new Map<string, DashboardModel>();
    dashboardModelCache.set(hass.states, cacheBySite);
  }

  const cached = cacheBySite.get(cacheKey);
  if (cached) {
    return cached;
  }

  const model = buildDashboardModel(hass, siteFilter);
  cacheBySite.set(cacheKey, model);
  return model;
}
