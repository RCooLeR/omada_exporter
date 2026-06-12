# Collected Data

OmadaBridge gathers data from the configured Omada Controller site and exposes the same collection through Prometheus and optional Home Assistant MQTT Discovery.

## Data Flow

1. OmadaBridge authenticates to the Omada Controller with the configured service user and OpenAPI credentials.
2. Collectors gather controller, site, device, client, WAN, ISP, and VPN data.
3. Prometheus scrapes `/metrics` or per-collector endpoints.
4. When MQTT is enabled, the Home Assistant publisher gathers the same collectors, creates MQTT Discovery configs, and publishes JSON state payloads for every metric instance.

No database is used. The bridge state is the latest collected Omada API response, the configured MQTT tracked-client MAC list, plus the current MQTT publisher memory used to mark disappeared client trackers as `not_home`.

## Collector Groups

| Collector | Endpoint | Data |
| --- | --- | --- |
| `controller` | `/metrics/controller` | Controller uptime, storage, capacity, version, and upgrade availability. |
| `alert` | `/metrics/alert` | Site alert count. |
| `device` | `/metrics/device` | Controller, gateway, switch, AP, OLT, port, LAG, WAN, radio, PoE, traffic, CPU, memory, temperature, and firmware data. |
| `client` | `/metrics/client` | Connected client totals and per-client traffic, signal, identity, attachment, SSID, VLAN, and vendor data. |
| `vpn` | `/metrics/vpn` | VPN status and configured VPN identity. |
| `vpn-stats` | `/metrics/vpn-stats` | VPN tunnel uptime, traffic, packets, peer stats, and derived MQTT speed sensors. |
| `isp` | `/metrics/isp` | ISP status, gateway, port, IP, load balancing, max bandwidth, and configured speed. |
| `insights` | `/metrics/insights` | Optional DPI traffic totals by category and application. Disabled by default. |

## Controller And Site

Collected controller data includes:

- controller uptime
- storage used and storage available by storage name
- total storage capacity by storage name
- firmware upgrade availability by upgrade channel
- controller device name, model, version, firmware, MAC, and IP
- Omada site name and site id
- site alert count
- site capacity fields reported by the controller

## Omada Devices

Device-level metrics cover gateways, switches, APs, controllers, and OLTs when the controller exposes them:

- device status
- CPU and memory percentage
- uptime
- current device upload and download counters
- device RX and TX rate
- temperature
- firmware and upgrade metadata
- device type, subtype, model, display model, hardware version, firmware version, IP, MAC, and name
- PoE remaining power and PoE support fields for devices that expose PoE data

## Ports And LAGs

Port collection includes:

- link status
- link speed in Mbit/s
- link RX and TX counters
- PoE status and current PoE watts
- operation mode, type, max speed, link status label, and optional `port_activity_label`

LAG collection includes:

- LAG link status
- LAG link speed in Mbit/s
- LAG RX and TX counters
- LAG id, type, member ports, and status labels

Set `OMADA_TRACK_PORT_METRICS=false` to suppress per-port metrics. Set `OMADA_INCLUDE_PORT_ACTIVITY_LABEL=false` to reduce label churn caused by the controller's port activity text.

## Wireless Radios

AP radio utilization is exposed as device metrics when supported:

- 2.4 GHz RX and TX utilization
- 5 GHz RX and TX utilization
- 5 GHz-2 RX and TX utilization
- 6 GHz RX and TX utilization
- WLAN group, radio mode, max rate, and channel width labels

## WAN And ISP

WAN metrics are device-attached and describe physical or logical WAN links:

- WAN status
- internet state
- link speed
- RX and TX rate
- RX and TX negotiation rate in Kbit/s
- latency
- port, name, description, IP, protocol, and type labels

ISP metrics describe Omada ISP configuration/status:

- ISP online/enabled status
- configured download and upload speed
- gateway name, gateway MAC, port, status, IP, load balancing, max bandwidth, and site labels

## Clients

Client metrics include:

- connected client totals by connection mode and Wi-Fi mode
- per-client current download and upload activity
- total traffic down and up
- RX and TX rate
- wireless signal percentage, SNR, and RSSI
- MAC, IP, VLAN, connection type, device category/type, vendor, host name, system name, and display name
- gateway, switch, AP, port, LAG id, SSID, Wi-Fi mode, and wired/wireless labels

Set `OMADA_TRACK_CLIENT_METRICS=false` to suppress per-client metrics while keeping aggregate connected-client totals.

## DPI Insights

Optional DPI insight metrics use Omada Web API endpoints and are disabled by default. Enable them with `OMADA_TRACK_INSIGHT_METRICS=true`.

Collected DPI insight data includes:

- total classified traffic for the configured query window
- classified traffic by DPI category
- classified traffic by DPI application, capped by `OMADA_INSIGHT_APPLICATION_LIMIT`
- the configured query window in seconds

These metrics are gauges for the requested time window, not monotonic counters. Omada may not attribute every byte in a category to an application, so application totals can be lower than category totals.

## VPN

VPN collection includes:

- VPN status
- VPN id, name, purpose, mode, type, remote IP, site, and site id
- tunnel uptime
- up/down packets
- up/down bytes
- site-to-site aggregate traffic
- site-to-site peer traffic
- site-to-site peer login timestamp

The MQTT publisher also creates derived speed sensors from VPN byte counters:

- `omada_vpn_down_speed`
- `omada_vpn_up_speed`
- `omada_site_to_site_vpn_down_speed`
- `omada_site_to_site_vpn_up_speed`
- `omada_site_to_site_vpn_peer_down_speed`
- `omada_site_to_site_vpn_peer_up_speed`

These derived speed sensors are MQTT/Home Assistant entities only. They are calculated from consecutive publisher samples and are not exposed as Prometheus metrics.

## Home Assistant Entity Attributes

For MQTT entities, every Prometheus label is copied into the JSON state payload and becomes Home Assistant entity attributes through `json_attributes_topic`. This keeps detailed Omada metadata available without creating a separate Home Assistant entity for every label.

Example state payload:

```json
{
  "value": 12.5,
  "metric": "omada_device_cpu_percentage",
  "help": "Percentage of device CPU used.",
  "last_updated": "2026-05-06T12:00:00Z",
  "device_mac": "aa:bb:cc:dd:ee:ff",
  "device_name": "Core Switch",
  "device_model": "SG3428X-M2",
  "site": "Default",
  "site_id": "site-id"
}
```
