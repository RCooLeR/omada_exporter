import { describe, expect, it } from "vitest";
import type { HassEntity, HomeAssistant } from "./ha-types";
import { buildDashboardModel } from "./model";

const oldTime = "2026-09-29T10:00:00Z";
const newTime = "2026-09-29T10:01:00Z";
const labels = { device_mac: "aa:bb:cc:dd:ee:ff", device_name: "AP", device_type: "ap", site: "Home", site_id: "home" };

function entity(id: string, metric: string, state: string, attrs: Record<string, unknown> = {}, time = oldTime): HassEntity {
  return { entity_id: `sensor.${id}`, state, last_updated: time, attributes: { ...labels, metric, last_updated: time, ...attrs } };
}

function haOnly(sample: HassEntity): HassEntity {
  delete sample.attributes.last_updated;
  return sample;
}

function device(attrs: Record<string, unknown> = {}): HassEntity {
  return entity("device", "omada_device_cpu_percentage", "10", { device_status: "Connected", ...attrs });
}

function model(entities: HassEntity[], site?: string) {
  const hass: HomeAssistant = { states: Object.fromEntries(entities.map((sample) => [sample.entity_id, sample])) };
  return buildDashboardModel(hass, site);
}

function available(state = "1", time = oldTime, source = "ap_uplink"): HassEntity {
  return entity("available", "omada_network_quality_available", state, { source }, time);
}

function success(state = "1", time = oldTime, source = "ap_uplink"): HassEntity {
  return entity("success", "omada_network_quality_scrape_success", state, { source }, time);
}

function packets(id = "packets", value = "12", time = oldTime, direction = "up"): HassEntity {
  return entity(id, "omada_ap_uplink_packets_total", value, { direction }, time);
}

function latency(value = "0.012", time = oldTime): HassEntity {
  return entity("latency", "omada_gateway_wan_quality_latency_average_seconds", value, {}, time);
}

describe("network-quality dashboard model", () => {
  it("does not add quality fields before new metrics are discovered", () => {
    expect(model([device()]).devices[0]).not.toHaveProperty("quality");
  });

  it("keeps true zero measurements and separates packet directions and all radio bands", () => {
    const result = model([
      device(), available(), success(), packets("up", "0"), packets("down", "42", oldTime, "down"),
      entity("drop", "omada_ap_uplink_dropped_packets_total", "0", { direction: "up" }),
      entity("errors", "omada_ap_uplink_error_packets_total", "2", { direction: "down" }),
      ...["6", "5-2", "2.4", "5"].map((band) => entity(`radio${band}`, "omada_ap_interference_percentage", "0", { radio_band: band }))
    ]);
    expect(result.devices[0]?.quality).toEqual({
      interference: ["2.4", "5", "5-2", "6"].map((band) => ({ band, percentage: 0 })),
      uplink: { status: "available", up: { packets: 0, dropped: 0 }, down: { packets: 42, errors: 2 } }
    });
    expect(result.devices[0]?.radios).toEqual([]);
    expect(result.wans).toEqual([]);
  });

  it("keeps WAN window dynamic and zero-valued averages without producing interface rows", () => {
    const result = model([
      device({ device_type: "gateway" }), available("1", oldTime, "wan"), success("1", oldTime, "wan"), latency("0"),
      entity("jitter", "omada_gateway_wan_quality_jitter_average_seconds", "0"),
      entity("loss", "omada_gateway_wan_quality_packet_loss_average_percentage", "0"),
      entity("mos", "omada_gateway_wan_quality_mos_average", "0"),
      entity("window", "omada_gateway_wan_quality_window_seconds", "3600")
    ]);
    expect(result.devices[0]?.quality?.wan).toEqual({ status: "available", latencySeconds: 0, jitterSeconds: 0,
      packetLossPercentage: 0, mos: 0, windowSeconds: 3600 });
    expect(result.wans).toHaveLength(0);
  });

  it.each(["unavailable", "unknown", "", " ", "NaN", "Infinity", "-1", "false"])(
    "does not turn invalid state %j into a quality zero",
    (state) => {
      expect(model([device(), available(), packets("invalid", state)]).devices[0]?.quality?.uplink).toEqual({ status: "unavailable", up: {}, down: {} });
      expect(model([device({ device_type: "gateway" }), latency(state)]).devices[0]?.quality?.wan).toEqual({ status: "unavailable" });
    }
  );

  it("rejects impossible values and missing or invalid label qualifiers", () => {
    const ap = model([
      device(), packets("fraction", "1.5"), packets("wrong-direction", "5", oldTime, "sideways"),
      entity("invalid-band", "omada_ap_interference_percentage", "12", { radio_band: "other" }),
      entity("over100", "omada_ap_interference_percentage", "100.1", { radio_band: "5" })
    ]).devices[0];
    expect(ap?.quality).toEqual({ interference: [], uplink: { status: "unavailable", up: {}, down: {} } });
    const gateway = model([
      device({ device_type: "gateway" }), latency(),
      entity("loss", "omada_gateway_wan_quality_packet_loss_average_percentage", "101"),
      entity("mos", "omada_gateway_wan_quality_mos_average", "5.1"),
      entity("window", "omada_gateway_wan_quality_window_seconds", "0")
    ]).devices[0];
    expect(gateway?.quality?.wan).toEqual({ status: "available", latencySeconds: 0.012 });
  });

  it("matches normalized MAC plus site ID, with site-name fallback and no phantom devices", () => {
    const good = packets();
    good.attributes.device_mac = "AA-BB-CC-DD-EE-FF";
    good.attributes.site = "Old Home name";
    const wrong = packets("wrong", "999", newTime);
    wrong.attributes.site_id = "elsewhere";
    const orphan = packets("orphan", "777", newTime);
    orphan.attributes.device_mac = "11:22:33:44:55:66";
    const unscoped = packets("unscoped", "888", newTime);
    unscoped.attributes.site_id = "";
    unscoped.attributes.site = "";
    const result = model([device(), good, wrong, orphan, unscoped]);
    expect(result.devices).toHaveLength(1);
    expect(result.devices[0]?.quality?.uplink?.up.packets).toBe(12);
    const byName = packets();
    byName.attributes.site_id = "";
    expect(model([device({ site_id: "" }), byName], "Home").devices[0]?.quality?.uplink?.up.packets).toBe(12);
    expect(model([device(), byName], "Other").devices).toHaveLength(0);
  });

  it("picks newer duplicates regardless of entity insertion order, including unavailable states", () => {
    const current = packets("current", "0", newTime);
    expect(model([device(), current, packets("old", "99")]).devices[0]?.quality?.uplink?.up.packets).toBe(0);
    const invalid = packets("current", "unavailable", newTime);
    expect(model([device(), invalid, packets("old", "99")]).devices[0]?.quality?.uplink).toEqual({ status: "unavailable", up: {}, down: {} });
  });

  it("uses MQTT observation timestamps rather than HA arrival order", () => {
    const newest = packets("newest", "0", oldTime);
    newest.attributes.last_updated = newTime;
    const stale = packets("stale", "99", newTime);
    stale.attributes.last_updated = oldTime;
    expect(model([device(), newest, stale]).devices[0]?.quality?.uplink?.up.packets).toBe(0);
  });

  it("retains all WAN fields when only staggered HA arrival timestamps are available", () => {
    const readings = [
      latency("0.01"),
      entity("jitter", "omada_gateway_wan_quality_jitter_average_seconds", "0.002"),
      entity("loss", "omada_gateway_wan_quality_packet_loss_average_percentage", "0"),
      entity("mos", "omada_gateway_wan_quality_mos_average", "4.5"),
      entity("window", "omada_gateway_wan_quality_window_seconds", "3600"),
      available("1", oldTime, "wan"), success("1", oldTime, "wan")
    ].map((entry, index) => {
      entry.last_updated = `2026-09-29T10:00:00.00${index}Z`;
      return haOnly(entry);
    });
    expect(model([haOnly(device({ device_type: "gateway" })), ...readings]).devices[0]?.quality?.wan)
      .toEqual({ status: "available", latencySeconds: 0.01, jitterSeconds: 0.002, packetLossPercentage: 0, mos: 4.5, windowSeconds: 3600 });
  });

  it("retains both uplink directions and every radio band with independent HA timestamps", () => {
    const readings = [
      packets("up", "10"), packets("down", "20", oldTime, "down"),
      entity("drop", "omada_ap_uplink_dropped_packets_total", "0", { direction: "up" }),
      entity("errors", "omada_ap_uplink_error_packets_total", "1", { direction: "down" }),
      ...["2.4", "5", "5-2", "6"].map((band) => entity(`radio${band}`, "omada_ap_interference_percentage", "0", { radio_band: band })),
      available(), success()
    ].map((entry, index) => {
      entry.last_updated = `2026-09-29T10:00:00.00${index}Z`;
      return haOnly(entry);
    });
    expect(model([haOnly(device()), ...readings]).devices[0]?.quality).toEqual({
      interference: ["2.4", "5", "5-2", "6"].map((band) => ({ band, percentage: 0 })),
      uplink: { status: "available", up: { packets: 10, dropped: 0 }, down: { packets: 20, errors: 1 } }
    });
  });

  it("keeps HA-only unavailable/failure controls authoritative until those controls recover", () => {
    const readings = [haOnly(device()), haOnly(packets("up", "10", newTime))];
    expect(model([...readings, haOnly(available("0")), haOnly(success())]).devices[0]?.quality?.uplink?.status).toBe("unavailable");
    expect(model([...readings, haOnly(available()), haOnly(success("0"))]).devices[0]?.quality?.uplink?.status).toBe("error");
    expect(model([...readings, haOnly(available("unknown")), haOnly(success())]).devices[0]?.quality?.uplink?.status).toBe("unavailable");
    expect(model([...readings, haOnly(available()), haOnly(success())]).devices[0]?.quality?.uplink?.up.packets).toBe(10);
  });

  it("honors a newer HA-unavailable state even when it retains an older MQTT timestamp", () => {
    const expired = packets("expired", "unavailable", oldTime);
    expired.last_updated = newTime;
    expect(model([device(), packets("retained", "99", oldTime), expired]).devices[0]?.quality?.uplink)
      .toEqual({ status: "unavailable", up: {}, down: {} });
    const unavailableControl = available("unavailable", oldTime);
    unavailableControl.last_updated = "2026-09-29T10:02:00Z";
    expect(model([device(), packets("current", "5", newTime), unavailableControl]).devices[0]?.quality?.uplink?.status).toBe("unavailable");
  });

  it.each(["0", "unknown", "unavailable"])("suppresses retained values when newer availability is %s", (state) => {
    expect(model([device(), packets(), available(state, newTime), success()]).devices[0]?.quality?.uplink)
      .toEqual({ status: "unavailable", up: {}, down: {} });
  });

  it("reports collection failures and site inventory failures, but recovers with newer data", () => {
    expect(model([device(), packets(), available("0", newTime), success("0", newTime)]).devices[0]?.quality?.uplink?.status).toBe("error");
    const inventory = success("0", newTime, "inventory");
    inventory.attributes.device_mac = "";
    expect(model([device(), packets(), inventory]).devices[0]?.quality?.uplink).toEqual({ status: "error", up: {}, down: {} });
    expect(model([device(), packets("fresh", "42", newTime), available("0"), success("0")]).devices[0]?.quality?.uplink?.up.packets).toBe(42);
    expect(model([device(), inventory]).devices[0]).not.toHaveProperty("quality");
  });

  it("ignores other-site inventory failures and does not present a window alone as usable WAN data", () => {
    const inventory = success("0", newTime, "inventory");
    inventory.attributes.site_id = "other";
    inventory.attributes.device_mac = "";
    expect(model([device(), packets(), inventory]).devices[0]?.quality?.uplink?.status).toBe("available");
    const windowOnly = entity("window", "omada_gateway_wan_quality_window_seconds", "86400");
    expect(model([device({ device_type: "gateway" }), windowOnly]).devices[0]?.quality?.wan).toEqual({ status: "unavailable" });
    expect(model([device({ device_type: "gateway" }), latency(), success("0", newTime, "wan")]).devices[0]?.quality?.wan)
      .toEqual({ status: "error" });
  });

  it("omits retained fields that were not included in a newer successful snapshot", () => {
    const uplink = model([device(), packets(), packets("newdown", "5", newTime, "down"), available("1", newTime)]).devices[0]?.quality?.uplink;
    expect(uplink).toEqual({ status: "available", up: {}, down: { packets: 5 } });
    const wan = model([device({ device_type: "gateway" }), latency(), available("1", newTime, "wan")]).devices[0]?.quality?.wan;
    expect(wan).toEqual({ status: "unavailable" });
  });

  it("hides old radio samples from later snapshots and suppresses offline quality", () => {
    const radio = entity("radio", "omada_ap_interference_percentage", "12", { radio_band: "5" });
    const fresh = entity("radio2", "omada_ap_interference_percentage", "0", { radio_band: "6" }, newTime);
    expect(model([device(), radio, fresh]).devices[0]?.quality?.interference).toEqual([{ band: "6", percentage: 0 }]);
    expect(model([device({ device_status: "Disconnected" }), radio, packets()]).devices[0]?.quality)
      .toEqual({ interference: [], uplink: { status: "offline", up: {}, down: {} } });
    expect(model([device({ device_type: "gateway", device_status: "Disconnected" }), latency()]).devices[0]?.quality?.wan)
      .toEqual({ status: "offline" });
  });

  it("removes entirely disappeared radio samples using the AP shared inventory snapshot", () => {
    const radio = entity("radio", "omada_ap_interference_percentage", "12", { radio_band: "5" });
    const currentDevice = device({ last_updated: newTime });
    expect(model([currentDevice, radio]).devices[0]?.quality?.interference).toEqual([]);
    // Inventory arrival time alone cannot identify the radio's collection.
    const haInventory = haOnly(device());
    haInventory.last_updated = newTime;
    expect(model([haInventory, haOnly(radio)]).devices[0]?.quality?.interference).toEqual([{ band: "5", percentage: 12 }]);
  });

  it.each([{ device_status: "Connected(Wireless)" }, { device_wireless_linked: "true" }, { device_wireless_linked: true }])(
    "shows radio quality but not a fictitious wired uplink for wireless backhaul %j",
    (attrs) => {
      const quality = model([device(attrs), packets(), entity("radio", "omada_ap_interference_percentage", "0", { radio_band: "5" })]).devices[0]?.quality;
      expect(quality).toEqual({ interference: [{ band: "5", percentage: 0 }], uplink: { status: "wireless", up: {}, down: {} } });
    }
  );
});
