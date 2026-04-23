# Home Assistant MQTT

`omada_exporter` supports Home Assistant through MQTT Discovery while keeping the Prometheus exporter enabled.

Use this when you want Omada data in Home Assistant dashboards, automations, and Lovelace cards without losing Prometheus and Grafana support.

The MQTT integration is read-only from the Omada side. The exporter only reads Omada API data and publishes MQTT discovery/state messages. It does not subscribe to Home Assistant commands and does not change Omada configuration.

## Related Docs

- Main project overview and Prometheus setup: [README.md](README.md)
- Home Assistant Lovelace cards for these entities: [ha-cards/README.md](ha-cards/README.md)

## What Gets Published

The MQTT publisher gathers the same collectors used by `/metrics`, then creates Home Assistant entities for every metric instance.

Coverage includes:

- Controller uptime, storage, firmware update availability, and capacity details.
- Site alert count and connected client totals.
- Omada gateways, switches, access points, and OLT base device metrics.
- Device CPU, memory, uptime, temperature, traffic, firmware/version metadata, and upgrade status.
- Gateway WAN status, internet state, link speed, RX/TX rate, latency, IP, protocol, and port details.
- Switch, gateway, and AP port link status, speed, PoE power, RX/TX counters, operation mode, and type.
- Switch LAG status, speed, member ports, RX/TX counters, and LAG mode.
- AP radio utilization for 2.4 GHz, 5 GHz, 5 GHz-2, and 6 GHz where supported.
- Client traffic, activity, RSSI, signal, SNR, RX/TX rate, SSID, AP, switch, gateway, VLAN, vendor, hostname, and connection details.
- MQTT `device_tracker` entities for active clients.
- VPN status, mode, type, remote IP, tunnel uptime, traffic, and derived download/upload speeds.
- ISP status, gateway, port, load balancing, max bandwidth, download speed, and upload speed.

Prometheus labels are attached to each Home Assistant entity as JSON attributes, so Home Assistant keeps the detailed Omada metadata without creating separate entities for every label.

## Home Assistant Setup

1. Enable the MQTT integration in Home Assistant.
2. Use the official Mosquitto broker add-on or another broker supported by Home Assistant.
3. Make sure MQTT Discovery is enabled. The default discovery prefix is `homeassistant`.
4. Create MQTT credentials for `omada_exporter`.
5. Start `omada_exporter` with MQTT enabled.
6. Optionally keep Prometheus scraping enabled as well; both outputs can run together.

Home Assistant MQTT docs:

- MQTT integration: https://www.home-assistant.io/integrations/mqtt/
- MQTT device tracker: https://www.home-assistant.io/integrations/device_tracker.mqtt/

## Omada Permissions

Use read-only Omada credentials where possible:

- Service user: viewer/read-only role for Web API metrics.
- OpenAPI client: read-only/admin-compatible access as required by your controller for OpenAPI endpoints.

Some metrics come from Omada OpenAPI. If `OMADA_CLIENT_ID` and `OMADA_SECRET_ID` are not valid, client, WAN, VPN, VPN stats, and ISP MQTT entities may be missing or unavailable.

## Docker Compose Example

This example exposes Prometheus metrics on port `9202` and publishes Home Assistant entities to MQTT from the same container.

```yaml
services:
  omada_exporter:
    image: rcooler/omada_exporter:latest
    container_name: omada_exporter
    ports:
      - "9202:9202"
    environment:
      OMADA_HOST: "https://192.168.1.20:443"
      OMADA_USER: "exporter"
      OMADA_PASS: "mypassword"
      OMADA_SITE: "Default"
      OMADA_CLIENT_ID: "your-openapi-client-id"
      OMADA_SECRET_ID: "your-openapi-secret-id"
      OMADA_INSECURE: "true"

      OMADA_MQTT_ENABLED: "true"
      OMADA_MQTT_BROKER: "tcp://homeassistant.local:1883"
      OMADA_MQTT_USER: "omada_exporter"
      OMADA_MQTT_PASS: "mqtt-password"
      OMADA_MQTT_CLIENT_ID: "omada_exporter"
      OMADA_MQTT_TOPIC_PREFIX: "omada_exporter"
      OMADA_MQTT_DISCOVERY_PREFIX: "homeassistant"
      OMADA_MQTT_INTERVAL: "60"
      OMADA_MQTT_RETAIN: "true"
      OMADA_MQTT_EXPIRE_AFTER: "180"
    restart: unless-stopped
```

## Environment Variables

These variables only affect the MQTT and Home Assistant side. Core Omada and Prometheus options are documented in [README.md](README.md).

| Variable | Default | Purpose |
|---|---:|---|
| `OMADA_MQTT_ENABLED` | `false` | Enables Home Assistant MQTT publishing. |
| `OMADA_MQTT_BROKER` | `tcp://localhost:1883` | MQTT broker URL. Use `tcp://host:1883` or `ssl://host:8883`. |
| `OMADA_MQTT_USER` | empty | MQTT username. |
| `OMADA_MQTT_PASS` | empty | MQTT password. |
| `OMADA_MQTT_CLIENT_ID` | `omada_exporter` | MQTT client id. |
| `OMADA_MQTT_TOPIC_PREFIX` | `omada_exporter` | State topic prefix. |
| `OMADA_MQTT_DISCOVERY_PREFIX` | `homeassistant` | Home Assistant discovery prefix. |
| `OMADA_MQTT_INTERVAL` | `60` | Publish interval in seconds. |
| `OMADA_MQTT_RETAIN` | `true` | Retain discovery and state messages. |
| `OMADA_MQTT_EXPIRE_AFTER` | `180` | Mark sensor entities unavailable after this many seconds without a state update. Set `0` to disable. |

## MQTT Topics

Availability:

```text
omada_exporter/status
```

Discovery examples:

```text
homeassistant/sensor/omada_exporter/omada_device_cpu_percentage_device_mac_aa_bb_cc_dd_ee_ff_abc123/config
homeassistant/binary_sensor/omada_exporter/omada_port_link_status_device_mac_aa_bb_cc_dd_ee_ff_port_1_abc123/config
homeassistant/device_tracker/omada_exporter/aa_bb_cc_dd_ee_ff/config
```

State examples:

```text
omada_exporter/entities/omada_device_cpu_percentage_device_mac_aa_bb_cc_dd_ee_ff_abc123/state
omada_exporter/device_trackers/aa_bb_cc_dd_ee_ff/state
omada_exporter/device_trackers/aa_bb_cc_dd_ee_ff/attributes
```

Sensor state payloads are JSON:

```json
{
  "value": 12.5,
  "metric": "omada_device_cpu_percentage",
  "help": "Percentage of device CPU used.",
  "last_updated": "2026-04-20T10:15:30Z",
  "device_mac": "aa:bb:cc:dd:ee:ff",
  "device_name": "Core Switch",
  "device_model": "SG3428X-M2",
  "site": "Default",
  "site_id": "..."
}
```

Client tracker state payloads are plain text:

```text
home
```

or:

```text
not_home
```

Client attributes are JSON and include Omada details such as IP, hostname, vendor, SSID, AP, switch, gateway, VLAN, RSSI, and traffic labels when available.

## Notes

- Prometheus `/metrics` stays available when MQTT publishing is enabled.
- Discovery and state messages are retained by default so entities survive Home Assistant and broker restarts.
- Retained MQTT messages can leave old Home Assistant entities after hardware is removed or topic prefixes are changed. Clear the old retained discovery topics if that happens.
- The exporter publishes binary sensors for known boolean metrics such as port link status, LAG link status, ISP online status, VPN status, and upgrade availability.
- VPN MQTT entities are grouped onto the same Home Assistant device when status and tunnel stats can be matched to the same Omada VPN.
- Everything else is published as a sensor with units where the metric name clearly identifies the unit.
