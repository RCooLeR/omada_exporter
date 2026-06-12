# Home Assistant

OmadaBridge integrates with Home Assistant through MQTT Discovery. The bridge publishes retained discovery configs and JSON state payloads for Omada metrics. Home Assistant creates MQTT sensors, binary sensors, and device trackers from those configs.

Prometheus remains enabled when MQTT is enabled.

## Setup

1. Install or choose an MQTT broker.
2. Add the MQTT integration in Home Assistant under `Settings -> Devices & services`.
3. Confirm MQTT Discovery is enabled. The default discovery prefix is `homeassistant`.
4. Configure OmadaBridge to connect to the same broker.
5. Restart OmadaBridge and watch logs for `connected to mqtt broker`.

Example bridge environment:

```yaml
environment:
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
  OMADA_MQTT_TRACKED_CLIENT_MACS: "aa:bb:cc:dd:ee:ff,11:22:33:44:55:66"
```

If the broker does not require authentication, omit `OMADA_MQTT_USER` and `OMADA_MQTT_PASS`.

If the MQTT broker is unavailable when OmadaBridge starts, the exporter still starts and Prometheus endpoints remain available. MQTT publishing retries in the background until the broker is reachable.

## MQTT Topics

Default prefixes:

| Setting | Default |
| --- | --- |
| State topic prefix | `omada_exporter` |
| Discovery prefix | `homeassistant` |
| Discovery node | `omada_exporter` |

Availability:

```text
omada_exporter/status = online | offline
```

Metric discovery:

```text
homeassistant/<component>/omada_exporter/<object_id>/config
```

Metric state:

```text
omada_exporter/entities/<object_id>/state
```

Client tracker discovery:

```text
homeassistant/device_tracker/omada_exporter/<client_mac_slug>/config
```

Client tracker state:

```text
omada_exporter/device_trackers/<client_mac_slug>/state
omada_exporter/device_trackers/<client_mac_slug>/attributes
```

## Entity Identity

For metric entities:

```text
object_id = slug(metric name + stable identity labels) + "_" + 10 character sha1 hash
unique_id = "omada_exporter_" + object_id
```

Home Assistant uses the discovery `object_id` as the starting point for the final entity id, but users can rename entities and Home Assistant can add suffixes to avoid collisions. Treat `unique_id`, MQTT object id, and JSON attributes as the stable identifiers. Do not rely on the final `sensor.*` or `binary_sensor.*` entity id staying unchanged.

The object id builder uses these labels first when present:

```text
site_id, site, device_mac, mac, gateway_mac,
storage_name, upgrade_channel, port, lag_id
```

VPN metrics also include:

```text
vpn_id, tunnel_id, peer_id
```

When there is no hardware identifier such as `device_mac`, `mac`, or `gateway_mac`, the builder may add:

```text
interface_name, local_ip, remote_ip, connection_mode,
wifi_mode, ssid, name, peer_name
```

Examples:

```text
omada_device_cpu_percentage_device_mac_aa_bb_cc_dd_ee_ff_2f9d4c2a10
omada_port_link_status_site_default_device_mac_aa_bb_cc_dd_ee_ff_port_1_abc123def0
omada_client_signal_pct_mac_11_22_33_44_55_66_6c7d8e9f10
```

## Created Home Assistant Devices

### Omada Site Device

Used when a metric is site-level and does not identify a specific device, client, gateway, or VPN.

| Field | Value |
| --- | --- |
| Identifier | `omada_site_<site_id_or_site_slug>` |
| Name | `Omada <site name>` |
| Manufacturer | `TP-Link Omada` |
| Model | `Site` |

### Omada Infrastructure Device

Used when labels include `device_mac`. This covers controllers, gateways, switches, APs, and OLTs that are represented as Omada devices.

| Field | Value |
| --- | --- |
| Identifier | `omada_device_<device_mac_slug>` |
| Name | `device_name`, fallback `device_mac` |
| Manufacturer | `TP-Link` |
| Model | `device_show_model`, fallback `device_model` |
| Software version | `device_version` |
| Hardware version | `device_hw_version` |
| Configuration URL | `OMADA_HOST` |

### Gateway Device

Some ISP metrics identify a gateway through `gateway_mac` instead of `device_mac`.

| Field | Value |
| --- | --- |
| Identifier | `omada_device_<gateway_mac_slug>` |
| Name | `gateway_name`, fallback `gateway_mac` |
| Manufacturer | `TP-Link` |
| Configuration URL | `OMADA_HOST` |

### Client Device

Used for per-client metrics and MQTT device trackers.

| Field | Value |
| --- | --- |
| Identifier | `omada_client_<client_mac_slug>` |
| Name | `name`, `host_name`, `system_name`, `ip`, `mac`, fallback `Omada Client` |
| Manufacturer | `vendor`, fallback `Unknown` |
| Model | `device_type`, fallback `device_category` |

Client MAC slugs are lowercase MAC addresses with separators converted to underscores, for example `aa_bb_cc_dd_ee_ff`.

### VPN Device

Used when metrics include `vpn_id`.

| Field | Value |
| --- | --- |
| Identifier | `omada_vpn_<vpn_id_slug>` |
| Name | `name`, fallback `vpn_id` |
| Manufacturer | `TP-Link Omada` |
| Model | `vpn_type`, fallback `VPN` |

For VPN stats that do not include `vpn_id`, the publisher tries to link them to the status metric by unique `(name, vpn_mode, vpn_type)` or by unique VPN name.

## Created Entity Types

### Binary Sensors

These metrics are published as `binary_sensor` entities:

| Metric | Device class |
| --- | --- |
| `omada_controller_upgrade_available` | `problem` |
| `omada_device_need_upgrade` | `problem` |
| `omada_port_link_status` | `connectivity` |
| `omada_lag_link_status` | `connectivity` |
| `omada_isp_status` | `connectivity` |
| `omada_vpn_status` | `connectivity` |

Binary sensor config uses:

```yaml
value_template: "{{ value_json.value | int }}"
payload_on: "1"
payload_off: "0"
```

### Sensors

All other metrics are published as `sensor` entities. Sensor config uses:

```yaml
value_template: "{{ value_json.value }}"
json_attributes_topic: "<same as state_topic>"
```

Unit and device class hints are inferred from metric names:

| Metric pattern | Unit | Device class |
| --- | --- | --- |
| suffix `_bytes` | `B` | `data_size` |
| suffix `_seconds` or `_uptime` | `s` | `duration` |
| contains `latency` | `ms` | `duration` |
| contains `percentage`, suffix `_pct`, or suffix `_util` | `%` | none |
| suffix `_watts` | `W` | `power` |
| contains `_temp` | `C` | `temperature` |
| suffix `_mbps` | `Mbit/s` | none |
| `omada_client_rx_rate`, `omada_client_tx_rate` | `Kbit/s` | none |
| contains `_rate` or `_speed` | `bit/s` | none |
| contains `_download` or `_upload` | `B` | none |

Counters get `state_class: total_increasing`. Other sensors get `state_class: measurement`.

When `OMADA_MQTT_EXPIRE_AFTER` is greater than `0`, it is applied to sensor entities. Binary sensors and device trackers do not use `expire_after`.

### Device Trackers

Every active client with a MAC address gets a `device_tracker` discovery config. MAC addresses listed in `OMADA_MQTT_TRACKED_CLIENT_MACS` also get a tracker even when they are not currently active.

| Field | Value |
| --- | --- |
| Unique id | `omada_client_<client_mac_slug>` |
| Object id | `omada_client_<client_mac_slug>` |
| Source type | `router` |
| State payloads | `home`, `not_home` |

When a client appears in collected metrics, OmadaBridge publishes:

```text
omada_exporter/device_trackers/<client_mac_slug>/state = home
```

If a previously seen client disappears, OmadaBridge publishes:

```text
omada_exporter/device_trackers/<client_mac_slug>/state = not_home
```

If a configured tracked MAC is not present in collected metrics, OmadaBridge publishes discovery for that client and sends the same `not_home` state. The list accepts comma, semicolon, or whitespace separators and normalizes common MAC formats such as `aa:bb:cc:dd:ee:ff`, `aa-bb-cc-dd-ee-ff`, `aabb.ccdd.eeff`, and `aabbccddeeff`.

Configured trackers still rely on per-client metrics to switch to `home` from Omada data, so keep `OMADA_TRACK_CLIENT_METRICS=true` when online/offline presence should update automatically.

Attributes include Omada client labels such as IP, hostname, vendor, SSID, AP, switch, gateway, VLAN, RSSI, traffic, and attachment details when available.

## Entity Names

Metric entity names are generated from the metric name plus selected labels:

1. The `omada_` prefix is removed.
2. Remaining words are title-cased.
3. Qualifiers are appended when labels exist.

Qualifier label order:

```text
storage_name, upgrade_channel, port, lag_id, name,
connection_mode, wifi_mode, ssid
```

Examples:

| Metric and labels | Name |
| --- | --- |
| `omada_device_cpu_percentage` | `Device Cpu Percentage` |
| `omada_port_link_status`, `port=1` | `Port Link Status Port 1` |
| `omada_lag_link_speed_mbps`, `lag_id=2` | `Lag Link Speed Mbps LAG 2` |
| `omada_client_connected_total`, `connection_mode=wireless`, `wifi_mode=11ax` | `Client Connected Total wireless 11ax` |
| `omada_isp_status`, `name=WAN1` | `Isp Status WAN1` |

Home Assistant can still apply its own display naming rules depending on device/entity registry state.

## State Payloads

Metric state payloads are JSON:

```json
{
  "value": 1,
  "metric": "omada_port_link_status",
  "help": "A boolean representing the link status of the port.",
  "last_updated": "2026-05-06T12:00:00Z",
  "device_mac": "aa:bb:cc:dd:ee:ff",
  "device_name": "Core Switch",
  "port": "1",
  "name": "Port 1",
  "site": "Default",
  "site_id": "site-id"
}
```

The `metric`, `help`, `last_updated`, and every Prometheus label become entity attributes in Home Assistant.

## Data Expected By The Lovelace Cards

The bundled cards read Home Assistant entities, not OmadaBridge HTTP endpoints. They look for:

- entity attribute `metric` to classify metric entities
- `device_mac`, `device_name`, `device_type`, model/version/status labels for infrastructure devices
- `mac`, `ip`, `host_name`, `vendor`, `ssid`, `wifi_mode`, AP/switch/gateway labels for clients
- `port`, `lag_id`, and LAG attributes for wired path details
- `omada_isp_*`, `omada_wan_*`, and `omada_vpn_*` metrics for link tables
- `device_tracker` entities with a `mac` attribute for active client presence

See [card docs](../../ha-cards/docs/index.md).

## Verify MQTT Manually

State topics:

```bash
mosquitto_sub -h HOME_ASSISTANT_IP -u mqtt-user -P mqtt-password -t "omada_exporter/#" -v
```

Discovery topics:

```bash
mosquitto_sub -h HOME_ASSISTANT_IP -u mqtt-user -P mqtt-password -t "homeassistant/#" -v
```

With MQTT Explorer, inspect:

```text
omada_exporter/status
omada_exporter/entities/
omada_exporter/device_trackers/
homeassistant/sensor/omada_exporter/
homeassistant/binary_sensor/omada_exporter/
homeassistant/device_tracker/omada_exporter/
```

## Troubleshooting

No Home Assistant entities appear:

- Confirm `OMADA_MQTT_ENABLED=true`.
- Confirm broker URL, username, and password.
- Confirm Home Assistant MQTT Discovery is enabled and uses the same discovery prefix.
- Subscribe to `homeassistant/#` and confirm retained discovery configs exist.
- Subscribe to `omada_exporter/#` and confirm state topics exist.

Entities are stale after renaming metrics or changing topic prefixes:

- MQTT retained discovery topics can keep old entities alive.
- Clear old retained `homeassistant/+/omada_exporter/+/config` topics.
- Reload the Home Assistant MQTT integration or restart Home Assistant.

Client trackers stay `home`:

- Confirm per-client collection is enabled.
- Confirm the client has a MAC address in Omada data.
- The bridge marks dynamic clients `not_home` only after they were previously seen by the same running publisher. Add long-lived clients to `OMADA_MQTT_TRACKED_CLIENT_MACS` when they should be published as `not_home` even if already offline.

Cards show no data:

- Confirm Home Assistant has entities with a `metric` attribute beginning with `omada_`.
- Confirm the card `site` option matches the `site` attribute, or remove the `site` option.
- Confirm the card resource points at the built `omada-network-card.js`.
