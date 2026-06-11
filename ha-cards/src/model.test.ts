import { describe, expect, it } from "vitest";
import type { HomeAssistant } from "./ha-types";
import { buildDashboardModel } from "./model";

describe("buildDashboardModel", () => {
  it("groups Omada device, client metric, and online tracker entities", () => {
    const hass: HomeAssistant = {
      states: {
        "sensor.gateway_cpu": {
          entity_id: "sensor.gateway_cpu",
          state: "42",
          attributes: {
            metric: "omada_device_cpu_percentage",
            device_mac: "aa:bb:cc:dd:ee:ff",
            device_name: "Gateway",
            device_type: "gateway",
            device_status: "Connected",
            site: "Default"
          }
        },
        "device_tracker.phone": {
          entity_id: "device_tracker.phone",
          state: "home",
          attributes: {
            mac: "11:22:33:44:55:66",
            name: "Phone",
            wireless: "true",
            site: "Default"
          }
        },
        "sensor.phone_signal": {
          entity_id: "sensor.phone_signal",
          state: "85",
          attributes: {
            metric: "omada_client_signal_pct",
            mac: "11:22:33:44:55:66",
            name: "Phone",
            wireless: "true",
            ap_mac: "aa:bb:cc:dd:ee:ff",
            site: "Default"
          }
        },
        "device_tracker.offline": {
          entity_id: "device_tracker.offline",
          state: "not_home",
          attributes: {
            mac: "22:33:44:55:66:77",
            name: "Offline client",
            site: "Default"
          }
        }
      }
    };

    const model = buildDashboardModel(hass, "Default");

    expect(model.devices).toHaveLength(1);
    expect(model.devices[0]?.name).toBe("Gateway");
    expect(model.clients).toHaveLength(1);
    expect(model.clients[0]?.name).toBe("Phone");
    expect(model.clients[0]?.metrics.omada_client_signal_pct).toBe(85);
    expect(model.siteSummary.devicesOnline).toBe(1);
    expect(model.siteSummary.wirelessClients).toBe(1);
  });
});
