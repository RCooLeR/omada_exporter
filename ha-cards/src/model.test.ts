import { describe, expect, it } from "vitest";
import type { HomeAssistant } from "./ha-types";
import { buildDashboardModel, vpnModeLabel, vpnPeerLoginSeconds, vpnRemoteLabel, vpnTotalBytes } from "./model";

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

  it("groups site-to-site VPN metrics and exposes WireGuard IP details", () => {
    const hass: HomeAssistant = {
      states: {
        "binary_sensor.vpn_status": {
          entity_id: "binary_sensor.vpn_status",
          state: "on",
          attributes: {
            metric: "omada_vpn_status",
            vpn_id: "vpn-1",
            name: "Slobidska_Zahidna",
            purpose: "Site-to-Site",
            vpn_type: "WireGuard",
            remote_ip: "",
            allowed_ips: "10.42.0.2/32",
            endpoint_ip: "93.175.202.48",
            site: "Default"
          }
        },
        "sensor.vpn_down": {
          entity_id: "sensor.vpn_down",
          state: "1024",
          attributes: {
            metric: "omada_site_to_site_vpn_down_bytes",
            vpn_id: "vpn-1",
            name: "Slobidska_Zahidna",
            vpn_type: "WireGuard",
            site_vpn_type: "Site-to-Site",
            local_ip: "10.42.0.1",
            remote_networks: "10.42.0.0/24",
            site: "Default"
          }
        },
        "sensor.vpn_up": {
          entity_id: "sensor.vpn_up",
          state: "2048",
          attributes: {
            metric: "omada_site_to_site_vpn_up_bytes",
            vpn_id: "vpn-1",
            name: "Slobidska_Zahidna",
            vpn_type: "WireGuard",
            site_vpn_type: "Site-to-Site",
            site: "Default"
          }
        },
        "binary_sensor.vpn_peer_status": {
          entity_id: "binary_sensor.vpn_peer_status",
          state: "on",
          attributes: {
            metric: "omada_site_to_site_vpn_peer_status",
            vpn_id: "vpn-1",
            name: "Slobidska_Zahidna",
            peer_id: "peer-1",
            peer_name: "Home peer",
            vpn_type: "WireGuard",
            site_vpn_type: "Site-to-Site",
            remote_ip: "93.175.202.49",
            allowed_ips: "10.42.0.3/32",
            site: "Default"
          }
        },
        "sensor.vpn_peer_login": {
          entity_id: "sensor.vpn_peer_login",
          state: "1773156000",
          attributes: {
            metric: "omada_site_to_site_vpn_peer_login_timestamp",
            vpn_id: "vpn-1",
            name: "Slobidska_Zahidna",
            peer_id: "peer-1",
            peer_name: "Home peer",
            vpn_type: "WireGuard",
            site_vpn_type: "Site-to-Site",
            site: "Default"
          }
        },
        "sensor.vpn_peer_down": {
          entity_id: "sensor.vpn_peer_down",
          state: "512",
          attributes: {
            metric: "omada_site_to_site_vpn_peer_down_bytes",
            vpn_id: "vpn-1",
            name: "Slobidska_Zahidna",
            peer_id: "peer-1",
            peer_name: "Home peer",
            vpn_type: "WireGuard",
            site_vpn_type: "Site-to-Site",
            site: "Default"
          }
        }
      }
    };

    const model = buildDashboardModel(hass, "Default");
    const vpn = model.vpns[0];
    const peer = model.vpnPeers[0];

    expect(model.vpns).toHaveLength(1);
    expect(model.vpnPeers).toHaveLength(1);
    expect(vpn?.name).toBe("Slobidska_Zahidna");
    expect(vpn ? vpnRemoteLabel(vpn) : "").toBe("93.175.202.48");
    expect(vpn ? vpnModeLabel(vpn) : "").toBe("Site-to-Site");
    expect(vpn ? vpnTotalBytes(vpn) : 0).toBe(3072);
    expect(peer?.name).toBe("Home peer");
    expect(peer ? vpnRemoteLabel(peer) : "").toBe("93.175.202.49");
    expect(peer ? vpnTotalBytes(peer) : 0).toBe(512);
    expect(peer ? vpnPeerLoginSeconds(peer) : 0).toBe(1773156000);
  });
});
