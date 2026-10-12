import { describe, expect, it } from "vitest";
import type { HassEntity, HomeAssistant } from "./ha-types";
import { buildDashboardModel } from "./model";

const controllerMac = "02:00:00:00:00:01";
const site = { site: "Default", site_id: "site-a" };

function entity(id: string, state: string, attributes: Record<string, unknown>): HassEntity {
  return { entity_id: id, state, attributes: { ...site, ...attributes } };
}

function controller(attributes: Record<string, unknown> = {}, id = "sensor.controller"): HassEntity {
  return entity(id, "3600", {
    metric: "omada_controller_uptime_seconds",
    device_mac: controllerMac,
    device_name: "Real controller",
    device_model: "OC220",
    device_ip: "192.0.2.1",
    device_version: "6.3.0",
    device_status: "Connected",
    ...attributes
  });
}

function clientMetric(id: string, mac: string, attributes: Record<string, unknown> = {}): HassEntity {
  return entity(`sensor.${id}`, "1234", {
    metric: "omada_client_traffic_down_bytes",
    mac,
    name: "Controller-classified client",
    device_type: "Omada Controller",
    wireless: "false",
    ...attributes
  });
}

function tracker(id: string, mac: string, state: string, attributes: Record<string, unknown> = {}): HassEntity {
  return entity(`device_tracker.${id}`, state, {
    mac,
    name: "Controller-classified client",
    wireless: "false",
    ...attributes
  });
}

function hass(...entities: HassEntity[]): HomeAssistant {
  return { states: Object.fromEntries(entities.map((item) => [item.entity_id, item])) };
}

describe("controller identity", () => {
  it("does not promote 20 historical controller-classified clients to connected controllers", () => {
    const historical = Array.from({ length: 20 }, (_, index) => {
      const mac = `02:00:00:00:01:${index.toString(16).padStart(2, "0")}`;
      return [clientMetric(`old_${index}`, mac), tracker(`old_${index}`, mac, "not_home")];
    }).flat();

    const model = buildDashboardModel(hass(controller(), ...historical));

    expect(model.devices).toHaveLength(1);
    expect(model.devices[0]?.name).toBe("Real controller");
    expect(model.devices[0]?.type).toBe("controller");
    expect(model.clients).toHaveLength(0);
    expect(model.siteSummary.controllers).toBe(1);
    expect(model.siteSummary.devicesOnline).toBe(1);
  });

  it.each([undefined, "not_home", "unavailable", "unknown"])(
    "requires a home tracker for a classified client (tracker: %s)",
    (state) => {
      const mac = "02:00:00:00:02:01";
      const entities = [clientMetric("classified", mac)];
      if (state !== undefined) entities.push(tracker("classified", mac, state));

      const model = buildDashboardModel(hass(...entities));

      expect(model.devices).toHaveLength(0);
      expect(model.clients).toHaveLength(0);
      expect(model.siteSummary.controllers).toBe(0);
      expect(model.siteSummary.devicesOnline).toBe(0);
    }
  );

  it.each(["device_type", "device_model", "device_show_model"])(
    "keeps an active client classified as a controller by %s in the client list",
    (label) => {
      const mac = "02:00:00:00:02:02";
      const metric = clientMetric("active", mac, { device_type: "", [label]: "Omada Controller" });
      const model = buildDashboardModel(hass(metric, tracker("active", mac, "home")));

      expect(model.devices).toHaveLength(0);
      expect(model.clients).toHaveLength(1);
      expect(model.clients[0]?.mac).toBe(mac);
      expect(model.clients[0]?.metrics.omada_client_traffic_down_bytes).toBe(1234);
      expect(model.siteSummary.controllers).toBe(0);
      expect(model.siteSummary.wiredClients).toBe(1);
    }
  );

  it.each([false, true])(
    "joins same-MAC client metrics to a real controller without replacing its attributes (client first: %s)",
    (clientFirst) => {
      const genuine = controller();
      genuine.last_updated = "2026-10-12T10:00:00Z";
      const client = clientMetric("same_mac", "02-00-00-00-00-01", {
        name: "Stale client alias",
        ip: "192.0.2.99",
        device_model: "Misclassified client model",
        switch_mac: "02:00:00:00:03:01",
        port: "8",
        lag_id: "2"
      });
      client.last_updated = "2026-10-12T11:00:00Z";
      const entities = clientFirst ? [client, genuine] : [genuine, client];
      const model = buildDashboardModel(hass(...entities, tracker("same_mac", controllerMac, "home")));

      expect(model.devices).toHaveLength(1);
      expect(model.clients).toHaveLength(0);
      const device = model.devices[0]!;
      expect(device.type).toBe("controller");
      expect(device.name).toBe("Real controller");
      expect(device.model).toBe("OC220");
      expect(device.ip).toBe("192.0.2.1");
      expect(device.version).toBe("6.3.0");
      expect(device.attrs).toEqual(genuine.attributes);
      expect(device.metrics.omada_controller_uptime_seconds).toBe(3600);
      expect(device.metrics.omada_client_traffic_down_bytes).toBe(1234);
    }
  );

  it("never borrows controller identity across bridges with the same site and MAC", () => {
    const scoped = { bridge_id: "apartment" };
    const state = hass(
      controller(),
      clientMetric("apartment_client", controllerMac, scoped),
      tracker("apartment_client", controllerMac, "home", scoped)
    );

    const legacy = buildDashboardModel(state);
    const apartment = buildDashboardModel(state, scoped);

    expect(legacy.devices).toHaveLength(1);
    expect(legacy.devices[0]?.metrics.omada_client_traffic_down_bytes).toBeUndefined();
    expect(legacy.clients).toHaveLength(0);
    expect(apartment.devices).toHaveLength(0);
    expect(apartment.clients).toHaveLength(1);
    expect(apartment.clients[0]?.metrics.omada_client_traffic_down_bytes).toBe(1234);
    expect(apartment.clients[0]?.attrs.bridge_id).toBe("apartment");
  });

  it("never borrows controller identity across identically named sites with the same MAC", () => {
    const otherSite = { site_id: "site-b" };
    const state = hass(
      controller(),
      clientMetric("other_site_client", controllerMac, otherSite),
      tracker("other_site_client", controllerMac, "home", otherSite)
    );
    const model = buildDashboardModel(state);

    expect(model.devices).toHaveLength(1);
    expect(model.devices[0]?.attrs.site_id).toBe("site-a");
    expect(model.devices[0]?.metrics.omada_client_traffic_down_bytes).toBeUndefined();
    expect(model.clients).toHaveLength(1);
    expect(model.clients[0]?.attrs.site_id).toBe("site-b");
    expect(model.clients[0]?.metrics.omada_client_traffic_down_bytes).toBe(1234);
    expect(buildDashboardModel(state, { site_id: "site-b" }).devices).toHaveLength(0);
    expect(buildDashboardModel(state, { site_id: "site-a" }).clients).toHaveLength(0);
  });
});
