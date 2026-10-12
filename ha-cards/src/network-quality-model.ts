import type { DeviceQuality, DeviceRecord, HassEntity, UplinkPacketCounts } from "./ha-types";

const interferenceMetric = "omada_ap_interference_percentage";
const availableMetric = "omada_network_quality_available";
const successMetric = "omada_network_quality_scrape_success";
const uplinkMetrics: Record<string, keyof UplinkPacketCounts> = {
  omada_ap_uplink_packets_total: "packets",
  omada_ap_uplink_dropped_packets_total: "dropped",
  omada_ap_uplink_error_packets_total: "errors"
};
const wanMetrics = {
  omada_gateway_wan_quality_latency_average_seconds: "latencySeconds",
  omada_gateway_wan_quality_jitter_average_seconds: "jitterSeconds",
  omada_gateway_wan_quality_packet_loss_average_percentage: "packetLossPercentage",
  omada_gateway_wan_quality_mos_average: "mos",
  omada_gateway_wan_quality_window_seconds: "windowSeconds"
} as const;

type Sample = { entity: HassEntity; observedAt: number; snapshotAt?: number; value?: number };

function attribute(entity: HassEntity, key: string): string {
  return String(entity.attributes[key] ?? "").trim();
}

function macKey(value: string): string {
  const normalized = value.trim().toLowerCase();
  const compact = normalized.replace(/[^0-9a-f]/g, "");
  return compact.length === 12 ? compact : normalized;
}

function timestamp(value: unknown): number | undefined {
  const parsed = Date.parse(String(value ?? ""));
  return Number.isFinite(parsed) ? parsed : undefined;
}

function sample(entity: HassEntity): Sample {
  const state = entity.state.trim();
  const value = state === "" ? NaN : Number(state);
  const snapshotAt = timestamp(entity.attributes.last_updated);
  const stateChangedAt = timestamp(entity.last_updated) ?? 0;
  const result: Sample = {
    entity,
    observedAt: snapshotAt ?? stateChangedAt
  };
  if (snapshotAt !== undefined) result.snapshotAt = snapshotAt;
  if (Number.isFinite(value) && value >= 0) {
    result.value = value;
  } else {
    // HA can expire a sensor without changing its retained MQTT attributes.
    result.observedAt = Math.max(result.observedAt, stateChangedAt);
  }
  return result;
}

function latestSnapshot(samples: Iterable<Sample>, floor?: number): number | undefined {
  let newest = floor;
  for (const entry of samples) {
    if (entry.snapshotAt !== undefined && (newest === undefined || entry.snapshotAt > newest)) newest = entry.snapshotAt;
  }
  return newest;
}

function isCurrentSnapshot(entry: Sample, snapshotAt: number | undefined): boolean {
  // Only publisher timestamps identify a shared Gather. HA arrival timestamps
  // differ per entity, so they must never prune another metric or radio band.
  return snapshotAt === undefined || entry.snapshotAt === undefined || entry.snapshotAt >= snapshotAt;
}

function sameSite(device: DeviceRecord, entity: HassEntity): boolean {
  if (String(device.attrs.bridge_id ?? "").trim() !== attribute(entity, "bridge_id")) return false;
  const deviceId = String(device.attrs.site_id ?? "").trim();
  const entityId = attribute(entity, "site_id");
  if (deviceId || entityId) return deviceId === entityId;
  const deviceName = device.site.trim();
  const entityName = attribute(entity, "site");
  // Do not attach an unscoped retained sample to a device from another site.
  return Boolean(deviceName && entityName && deviceName === entityName);
}

function latestSamples(entities: HassEntity[], key: (entity: HassEntity) => string): Map<string, Sample> {
  const latest = new Map<string, Sample>();
  for (const entity of entities) {
    const identity = key(entity);
    const next = sample(entity);
    if (next.observedAt >= (latest.get(identity)?.observedAt ?? -1)) latest.set(identity, next);
  }
  return latest;
}

function isOffline(device: DeviceRecord): boolean {
  return Boolean(device.status.trim()) && !/^connected(?:\(|$)/i.test(device.status.trim());
}

function isWireless(device: DeviceRecord): boolean {
  const linked = String(device.attrs.device_wireless_linked ?? "").trim().toLowerCase();
  return linked === "true" || linked === "1" || /^connected\(wireless/i.test(device.status.trim());
}

function sourceSnapshot(entities: HassEntity[], inventory: HassEntity[], source: "ap_uplink" | "wan") {
  const values = latestSamples(entities, (entity) => `${attribute(entity, "metric")}:${attribute(entity, "direction")}`);
  const controls = latestSamples([
    ...entities.filter((entity) => [availableMetric, successMetric].includes(attribute(entity, "metric")) &&
      attribute(entity, "source") === source),
    ...inventory
  ], (entity) => `${attribute(entity, "metric")}:${attribute(entity, "source")}`);
  const snapshotAt = latestSnapshot([...values.values(), ...controls.values()]);
  let status: "available" | "unavailable" | "error" = "available";
  for (const control of controls.values()) {
    // A stale published failure can be superseded by a newer successful
    // snapshot. Without publisher timestamps, preserve the last known status
    // until that status entity recovers; a later metric arrival is not proof.
    if (!isCurrentSnapshot(control, snapshotAt) &&
      !(control.value === undefined && control.observedAt >= (snapshotAt ?? 0))) continue;
    if (control.value === 0 && attribute(control.entity, "metric") === successMetric) {
      status = "error";
    } else if (control.value !== 1 && status !== "error") {
      status = "unavailable";
    }
  }
  return { values, snapshotAt, status };
}

export function isNetworkQualityMetric(metric: string): boolean {
  return metric === interferenceMetric || metric in uplinkMetrics || metric in wanMetrics ||
    metric === availableMetric || metric === successMetric;
}

/** Attach sparse device-scoped quality data without altering inventory, ports, or WAN interface rows. */
export function attachNetworkQuality(devices: Iterable<DeviceRecord>, entities: HassEntity[]): void {
  const byMac = new Map<string, HassEntity[]>();
  const inventory: HassEntity[] = [];
  for (const entity of entities) {
    if (attribute(entity, "metric") === successMetric && attribute(entity, "source") === "inventory") {
      inventory.push(entity);
      continue;
    }
    const mac = macKey(attribute(entity, "device_mac"));
    if (!mac) continue;
    const samples = byMac.get(mac) ?? [];
    samples.push(entity);
    byMac.set(mac, samples);
  }

  for (const device of devices) {
    const type = device.type.toLowerCase();
    if (type !== "ap" && type !== "gateway") continue;
    const scoped = (byMac.get(macKey(device.mac)) ?? []).filter((entity) => sameSite(device, entity));
    const siteInventory = inventory.filter((entity) => sameSite(device, entity));
    const source = type === "ap" ? "ap_uplink" : "wan";
    const sourceEntities = scoped.filter((entity) => {
      const metric = attribute(entity, "metric");
      return type === "ap" ? metric in uplinkMetrics || attribute(entity, "source") === source
        : metric in wanMetrics || attribute(entity, "source") === source;
    });
    const radioEntities = type === "ap" ? scoped.filter((entity) => attribute(entity, "metric") === interferenceMetric) : [];
    if (sourceEntities.length === 0 && radioEntities.length === 0) continue;

    const quality: DeviceQuality = { interference: [] };
    const offline = isOffline(device);
    if (!offline) {
      const radios = latestSamples(radioEntities, (entity) => attribute(entity, "radio_band"));
      // Device inventory is always published in the same Gather as radio
      // interference, including when every radio field disappears entirely.
      const newestRadio = latestSnapshot(radios.values(), timestamp(device.attrs.last_updated));
      for (const [band, entry] of radios) {
        if (["2.4", "5", "5-2", "6"].includes(band) && isCurrentSnapshot(entry, newestRadio) &&
          entry.value !== undefined && entry.value <= 100) {
          quality.interference.push({ band, percentage: entry.value });
        }
      }
      const order = ["2.4", "5", "5-2", "6"];
      quality.interference.sort((left, right) => order.indexOf(left.band) - order.indexOf(right.band));
    }

    if (sourceEntities.length > 0) {
      const snapshot = sourceSnapshot(sourceEntities, siteInventory, source);
      if (type === "ap") {
        const uplink: NonNullable<DeviceQuality["uplink"]> = { status: snapshot.status, up: {}, down: {} };
        if (offline) uplink.status = "offline";
        else if (isWireless(device)) uplink.status = "wireless";
        if (uplink.status === "available") {
          for (const entry of snapshot.values.values()) {
            const field = uplinkMetrics[attribute(entry.entity, "metric")];
            const direction = attribute(entry.entity, "direction");
            if (field && (direction === "up" || direction === "down") && isCurrentSnapshot(entry, snapshot.snapshotAt) &&
              entry.value !== undefined && Number.isInteger(entry.value)) uplink[direction][field] = entry.value;
          }
          if (Object.keys(uplink.up).length + Object.keys(uplink.down).length === 0) uplink.status = "unavailable";
        }
        quality.uplink = uplink;
      } else {
        const wan: NonNullable<DeviceQuality["wan"]> = { status: offline ? "offline" : snapshot.status };
        if (wan.status === "available") {
          for (const entry of snapshot.values.values()) {
            const field = wanMetrics[attribute(entry.entity, "metric") as keyof typeof wanMetrics];
            if (!field || entry.value === undefined || !isCurrentSnapshot(entry, snapshot.snapshotAt)) continue;
            if (field === "packetLossPercentage" && entry.value > 100 || field === "mos" && entry.value > 5 ||
              field === "windowSeconds" && entry.value <= 0) continue;
            wan[field] = entry.value;
          }
          if ([wan.latencySeconds, wan.jitterSeconds, wan.packetLossPercentage, wan.mos].every((value) => value === undefined)) {
            wan.status = "unavailable";
            delete wan.windowSeconds;
          }
        }
        quality.wan = wan;
      }
    }
    device.quality = quality;
  }
}
