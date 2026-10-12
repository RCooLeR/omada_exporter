import { describe, expect, it } from "vitest";
import type { HassEntity, HomeAssistant } from "./ha-types";
import { buildDashboardModel, cardHassChanged, findWanForRow, getDashboardModel, normalizeMacKey } from "./model";

const gatewayMac = "aa:bb:cc:dd:ee:ff";
const apMac = "aa:bb:cc:dd:ee:01";
const clientMac = "11:22:33:44:55:66";
const apartment = { bridge_id: "slobidska44" };

function fixture(bridge = "", siteId = "same-site", prefix = bridge || "legacy", clientState = "home"): HassEntity[] {
  const base = { bridge_id: bridge, site: "Default", site_id: siteId };
  const gateway = { device_mac: gatewayMac, device_name: "Gateway", device_type: "gateway", device_status: "Connected" };
  const ap = { device_mac: apMac, device_name: "AP", device_type: "ap", device_status: "Connected" };
  const client = { mac: clientMac, name: "Phone", wireless: "false", switch_mac: gatewayMac, port: "1" };
  const time = bridge ? "2026-10-08T11:00:00Z" : "2026-10-08T10:00:00Z";
  const make = (id: string, state: string, attrs: Record<string, unknown>): HassEntity => ({
    entity_id: id.startsWith("tracker") ? `device_tracker.${prefix}_${id}` : `sensor.${prefix}_${id}`,
    state, attributes: { ...base, ...attrs }, last_updated: time
  });
  return [
    make("cpu", bridge ? "20" : "10", { ...gateway, metric: "omada_device_cpu_percentage" }),
    make("ap_cpu", "12", { ...ap, metric: "omada_device_cpu_percentage" }),
    make("port", bridge ? "2500" : "1000", { ...gateway, metric: "omada_port_link_speed_mbps", port: "1", name: "Port 1" }),
    make("radio", "17", { ...ap, metric: "omada_radio_channel", band: "5" }),
    make("tracker", clientState, client),
    make("traffic", bridge ? "2048" : "1024", { ...client, metric: "omada_client_traffic_down_bytes" }),
    make("lag", bridge ? "5000" : "2000", { ...gateway, metric: "omada_lag_link_speed_mbps", lag_id: "1", lag_ports: "1,2" }),
    make("wan", bridge ? "45" : "12", { ...gateway, metric: "omada_wan_latency", name: "WAN 1", port: "1" }),
    make("isp", "1", { gateway_mac: gatewayMac, metric: "omada_isp_status", name: "WAN 1", port: "1" }),
    make("vpn", "1", { metric: "omada_vpn_status", name: "VPN", vpn_id: "vpn1", vpn_type: "WireGuard" }),
    make("peer", "1", { metric: "omada_site_to_site_vpn_peer_status", name: "VPN", vpn_id: "vpn1", vpn_type: "WireGuard", peer_id: "peer1" }),
    make("dpi", bridge ? "200" : "100", { metric: "omada_dpi_total_traffic_bytes" }),
    make("category", "100", { metric: "omada_dpi_category_traffic_bytes", family_id: "1", family_name: "Streaming" }),
    make("application", "100", { metric: "omada_dpi_application_traffic_bytes", family_id: "1", family_name: "Streaming", application_id: "1", application_name: "Example" }),
    make("interference", bridge ? "25" : "5", { ...ap, metric: "omada_ap_interference_percentage", radio_band: "5" }),
    make("quality", bridge ? "0.045" : "0.012", { ...gateway, metric: "omada_gateway_wan_quality_latency_average_seconds" }),
    make("quality_available", "1", { ...gateway, metric: "omada_network_quality_available", source: "wan" })
  ];
}

function hass(...entities: HassEntity[]): HomeAssistant {
  return { states: Object.fromEntries(entities.map((entity) => [entity.entity_id, entity])) };
}

describe("bridge scope", () => {
  it("defaults to legacy only and isolates identical controller/site/device/client/link/DPI identities", () => {
    const legacy = fixture();
    delete legacy[0]!.attributes.bridge_id; // Absent and empty both mean legacy.
    const state = hass(...legacy, ...fixture("slobidska44"));
    const house = buildDashboardModel(state);
    const flat = buildDashboardModel(state, apartment);
    for (const model of [house, flat]) {
      expect(model.devices).toHaveLength(2);
      expect(model.clients).toHaveLength(1);
      expect(model.wans).toHaveLength(1);
      expect(model.isps).toHaveLength(1);
      expect(model.vpns).toHaveLength(1);
      expect(model.vpnPeers).toHaveLength(1);
      expect(model.dpi.categories).toHaveLength(1);
      expect(model.dpi.applications).toHaveLength(1);
      expect(model.devices.find((row) => row.type === "gateway")?.clients).toHaveLength(1);
      expect(model.devices.find((row) => row.type === "ap")?.radios).toHaveLength(1);
    }
    expect(house.devices[0]?.metrics.omada_device_cpu_percentage).toBe(10);
    expect(flat.devices[0]?.metrics.omada_device_cpu_percentage).toBe(20);
    expect(house.clients[0]?.metrics.omada_client_traffic_down_bytes).toBe(1024);
    expect(flat.clients[0]?.metrics.omada_client_traffic_down_bytes).toBe(2048);
    expect(house.dpi.totalBytes).toBe(100);
    expect(flat.dpi.totalBytes).toBe(200);
    expect(house.devices[0]?.quality?.wan?.latencySeconds).toBe(0.012);
    expect(flat.devices[0]?.quality?.wan?.latencySeconds).toBe(0.045);
    expect(house.devices[1]?.quality?.interference[0]?.percentage).toBe(5);
    expect(flat.devices[1]?.quality?.interference[0]?.percentage).toBe(25);
    expect(findWanForRow(house, house.isps[0]!)?.metrics.omada_wan_latency).toBe(12);
    expect(findWanForRow(flat, flat.isps[0]!)?.metrics.omada_wan_latency).toBe(45);
    expect(buildDashboardModel(state, { bridge_id: "SLOBIDSKA44" }).devices).toHaveLength(0);
  });

  it("a newer away tracker in the apartment never suppresses the legacy home tracker", () => {
    const state = hass(...fixture(), ...fixture("slobidska44", "same-site", "flat", "not_home"), {
      entity_id: "device_tracker.third_party", state: "home", last_updated: "2026-10-08T12:00:00Z",
      attributes: { mac: clientMac, site: "Default", site_id: "same-site", name: "Other integration" }
    });
    expect(buildDashboardModel(state, apartment).clients).toHaveLength(0);
    const withoutThirdParty = { states: { ...state.states } };
    delete withoutThirdParty.states["device_tracker.third_party"];
    expect(buildDashboardModel(withoutThirdParty).clients[0]?.name).toBe("Phone");
    // Scoped sensors cannot borrow an unscoped Home Assistant tracker.
    delete state.states["device_tracker.flat_tracker"];
    expect(buildDashboardModel(state, apartment).clients).toHaveLength(0);
  });

  it("an apartment quality or inventory failure leaves legacy device readings intact", () => {
    const state = hass(...fixture(), ...fixture("slobidska44"));
    state.states["sensor.slobidska44_quality_available"]!.state = "0";
    expect(buildDashboardModel(state, apartment).devices[0]?.quality?.wan?.status).toBe("unavailable");
    expect(buildDashboardModel(state).devices[0]?.quality?.wan?.latencySeconds).toBe(0.012);
    state.states["sensor.apartment_inventory_failed"] = {
      entity_id: "sensor.apartment_inventory_failed", state: "0",
      attributes: { bridge_id: "slobidska44", site: "Default", site_id: "same-site",
        metric: "omada_network_quality_scrape_success", source: "inventory" }
    };
    expect(buildDashboardModel(state, apartment).devices[0]?.quality?.wan?.status).toBe("error");
    expect(buildDashboardModel(state).devices[0]?.quality?.wan?.latencySeconds).toBe(0.012);
  });

  it("keeps caches and update detection specific to bridge, site name, and site ID", () => {
    const previous = hass(...fixture(), ...fixture("slobidska44"));
    expect(getDashboardModel(previous)).not.toBe(getDashboardModel(previous, apartment));
    expect(getDashboardModel(previous, apartment)).toBe(getDashboardModel(previous, { ...apartment }));
    expect(getDashboardModel(previous, apartment)).not.toBe(getDashboardModel(previous, { ...apartment, site_id: "same-site" }));
    expect(getDashboardModel(previous, "Default")).toBe(getDashboardModel(previous, { site: "Default" }));
    const changed = { states: { ...previous.states,
      "sensor.slobidska44_cpu": { ...previous.states["sensor.slobidska44_cpu"]!, state: "50" }
    } };
    expect(cardHassChanged(changed, previous)).toBe(false);
    expect(cardHassChanged(changed, previous, apartment)).toBe(true);
    expect(cardHassChanged(changed, previous, { ...apartment, site_id: "other" })).toBe(false);
    const removed = { states: { ...previous.states } };
    delete removed.states["sensor.slobidska44_cpu"];
    expect(cardHassChanged(removed, previous, apartment)).toBe(true);
    expect(cardHassChanged(removed, previous)).toBe(false);
  });

  it("filters exact site IDs and keeps identically named sites separate inside one bridge", () => {
    const first = fixture("slobidska44", "site-a", "a");
    const second = fixture("slobidska44", "site-b", "b");
    second.find((entity) => entity.entity_id === "sensor.b_cpu")!.state = "80";
    second.find((entity) => entity.entity_id === "sensor.b_wan")!.state = "90";
    const state = hass(...first, ...second);
    const combined = buildDashboardModel(state, apartment);
    expect(combined.devices).toHaveLength(4);
    expect(combined.clients).toHaveLength(2);
    expect(combined.wans).toHaveLength(2);
    expect(combined.isps).toHaveLength(2);
    expect(combined.vpns).toHaveLength(2);
    expect(combined.vpnPeers).toHaveLength(2);
    expect(combined.dpi.sites).toHaveLength(2);
    for (const gateway of combined.devices.filter((row) => row.type === "gateway")) {
      expect(gateway.ports).toHaveLength(1);
      expect(gateway.clients).toHaveLength(1);
      expect(gateway.clients[0]?.attrs.site_id).toBe(gateway.attrs.site_id);
      expect(gateway.ports[0]?.clients).toHaveLength(1);
    }
    expect(combined.deviceByMac.has(normalizeMacKey(gatewayMac))).toBe(false);
    expect(combined.wanByName.has("WAN 1")).toBe(false);
    expect(combined.wanByPort.has("1")).toBe(false);
    for (const row of combined.isps) {
      expect(findWanForRow(combined, row)?.metrics.omada_wan_latency).toBe(row.attrs.site_id === "site-a" ? 45 : 90);
    }
    expect(buildDashboardModel(state, { ...apartment, site_id: "site-a" }).devices).toHaveLength(2);
    expect(buildDashboardModel(state, { ...apartment, site: "Default", site_id: "missing" }).devices).toHaveLength(0);
    expect(buildDashboardModel(state, { ...apartment, site: "Other" }).devices).toHaveLength(0);
  });

  it("does not join WAN 1 or port 1 across two gateways in the same site", () => {
    const entities = fixture("slobidska44");
    const otherGateway = "00:11:22:33:44:55";
    entities.push({
      entity_id: "sensor.other_wan", state: "99",
      attributes: { bridge_id: "slobidska44", site_id: "same-site", site: "Default", device_mac: otherGateway,
        metric: "omada_wan_latency", name: "WAN 1", port: "1" }
    }, {
      entity_id: "sensor.other_isp", state: "1",
      attributes: { bridge_id: "slobidska44", site_id: "same-site", site: "Default", gateway_mac: otherGateway,
        metric: "omada_isp_status", name: "WAN 1", port: "1" }
    });
    const model = buildDashboardModel(hass(...entities), apartment);
    expect(model.wans).toHaveLength(2);
    expect(model.isps).toHaveLength(2);
    for (const row of model.isps) {
      expect(findWanForRow(model, row)?.metrics.omada_wan_latency).toBe(row.attrs.gateway_mac === otherGateway ? 99 : 45);
    }
    expect(findWanForRow(model, { ...model.isps[0]!, attrs: { bridge_id: "slobidska44", site_id: "same-site", port: "1" } })).toBeUndefined();
  });

  it("allows legacy site-less entities but excludes site-less entities from named-bridge site filters", () => {
    const state = hass(...fixture(), ...fixture("slobidska44"));
    delete state.states["sensor.legacy_cpu"]!.attributes.site;
    delete state.states["sensor.slobidska44_cpu"]!.attributes.site;
    expect(buildDashboardModel(state, "Default").devices.some((row) => row.type === "gateway")).toBe(true);
    expect(buildDashboardModel(state, { ...apartment, site: "Default" }).devices.some((row) => row.type === "gateway")).toBe(false);
  });
});
