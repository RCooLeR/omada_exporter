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

## Optional Device Controls

Control buttons are **disabled by default**. To opt in, keep MQTT enabled and set:

```yaml
environment:
  OMADA_ALLOW_DEVICE_CONTROL: "true"
```

The bridge discovers Home Assistant MQTT buttons for:

- Rebooting a currently connected AP, standalone switch, or gateway, including a supported Fusion gateway. Controller appliances/software, OLTs, and switch stacks are excluded.
- Reconnecting a currently active, unblocked wireless client. This disconnects its Wi-Fi association; the client decides whether and when to reconnect. It does not reboot or block the client.

Reboots interrupt service; rebooting the gateway may also interrupt access to the controller. The Home Assistant cards ask for confirmation. Native Home Assistant button entities and MQTT publishers can bypass the card's confirmation, so restrict MQTT command-topic publishing and Home Assistant access to trusted administrators. This switch is not an authentication mechanism. Controller accounts must have the appropriate permissions, and no HTTP write endpoint is added to the exporter.

Eligibility comes from fresh controller inventory, independently of telemetry caches, and is checked again immediately before each controller command. An inventory or subscription failure makes controls unavailable. Availability requires the bridge, the controls subsystem, and the target to all be online. A bridge restart or MQTT reconnect starts unavailable until inventory is refreshed. Setting the option back to `false` publishes a retained controls-offline marker: previously discovered buttons cannot remain available, and the disabled bridge does not subscribe to commands.

Commands use QoS 0, `retain: false`, and the exact payload `PRESS`. Retained/replayed, duplicate, non-QoS-0, malformed, unknown-target, stale, or wrong-site requests are rejected. The bridge allows one pending/in-flight command globally and a 60-second cooldown per action and target. Queued requests expire after 15 seconds and cannot cross an MQTT reconnect. A timed-out or failed controller mutation is **never automatically retried**; verify the device's state before trying again.

Buttons use the existing device/client registry identity and expose these JSON attributes for card integrations:

```yaml
omada_control: reboot_device # or reconnect_client
target_mac: aa:bb:cc:dd:ee:ff
site: Default
site_id: controller-site-id
bridge_id: slobidska44 # omitted for the legacy bridge
```

Control attributes deliberately have no `metric` field. After a request, `last_status` is `accepted`, `rejected`, or `failed`, with `last_status_at` as a UTC timestamp; controller response bodies and credentials are not published. `accepted` means the controller accepted the request, **not** that the reboot/reconnection completed. Cards therefore report “Request sent” and continue showing normal live telemetry.

Command and target-availability topics are scoped below `<effective MQTT prefix>/controls/<site key>/<action>/<MAC key>/`. Named bridges use their existing `_<bridge_id>` prefix and discovery node. Retained button cleanup can remove only verified control discovery records in that bridge's own namespace; it does not adopt another bridge's devices or commands. Removed clients/devices have their buttons withdrawn after a successful inventory refresh.

## Multiple bridges on one MQTT broker and Home Assistant

Leave `OMADA_BRIDGE_ID` unset on an existing installation to retain its MQTT
topics, discovery identifiers, Home Assistant entities, history and automations.
Give every additional bridge a permanent, unique ID, for example:

```yaml
environment:
  OMADA_BRIDGE_ID: "slobidska44"
  OMADA_MQTT_ENABLED: "true"
  OMADA_MQTT_BROKER: "tcp://homeassistant.local:1883"
```

The ID is added automatically; do not also append it to the configured MQTT
prefix/client ID. With the default MQTT settings the effective values are:

| Resource | Existing bridge (no ID) | Apartment (`slobidska44`) |
| --- | --- | --- |
| MQTT client ID | `omada_exporter` | `omada_exporter_slobidska44` |
| State prefix | `omada_exporter` | `omada_exporter_slobidska44` |
| Availability | `omada_exporter/status` | `omada_exporter_slobidska44/status` |
| Discovery node | `omada_exporter` | `omada_exporter_slobidska44` |
| Discovery root | `homeassistant` | `homeassistant` |
| JSON `bridge_id` attribute | absent | `slobidska44` |

Custom state prefixes and MQTT client IDs are also suffixed with `_<bridge_id>`.
For example, `my/omada` becomes `my/omada_slobidska44`, not
`my/omada/slobidska44`. The discovery root stays unchanged so Home Assistant
can discover every bridge. Keep effective prefixes/client IDs unique; do not
manually configure one installation to use another's effective namespace.

Metric entities include the bridge ID in their identity. Named bridges also
scope Home Assistant device identifiers and client trackers by bridge and site,
so the same phone, MAC address, VPN name or controller-provided site ID can
exist independently at both locations. Presence means “connected at this
bridge's site”: a phone may correctly be `home` at one location and `not_home`
at the other. A disconnect/restart affects only that bridge's availability.

State/derived metric payloads and online, configured-offline and recovered
trackers all carry `bridge_id`. Retained startup recovery and duplicate cleanup
are limited to the bridge's own topic namespace and identity; a named bridge
does not adopt or delete the legacy bridge's retained entities.

Do not rename an ID once deployed unless you intend to create a separate set
of entities. Setting an ID on the existing bridge is not an automatic migration
and does not remove its previous retained records. The default card selection
without `bridge_id` is the legacy installation; set `bridge_id: slobidska44` in
each apartment card. See [Home Assistant cards](../../ha-cards/README.md) for
the card configuration.

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

The object id builder uses `site_id` when present, with `site` as its fallback,
then exactly one owning hardware identifier:

```text
device_mac | mac | gateway_mac
```

Metric-specific stable subresources are added only where they identify a real
series:

```text
controller storage: storage_name
controller upgrades: upgrade_channel
ports/WAN/ISP: port
switch LAGs: lag_id
VPNs/peers: vpn_id, peer_id
client totals: connection_mode, wifi_mode
DPI: family_id, application_id
```

Client attachment properties such as gateway, switch, port, LAG, AP, SSID, and
Wi-Fi mode are attributes, not entity identity. Moving a client updates the
same entity.

VPN state payloads also expose optional WireGuard/site-to-site details when the Omada API returns them:

```text
local_ip, local_networks, remote_networks,
allowed_ips, endpoint, endpoint_ip
```

Legacy VPN metrics without an API VPN ID use stable tunnel descriptors such as:

```text
name, interface_name, vpn_mode, vpn_type
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
| `omada_site_to_site_vpn_peer_status` | `connectivity` |

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

Attributes include Omada client labels such as IP, hostname, vendor, SSID, AP,
switch, gateway, VLAN, RSSI, traffic, and attachment details when available.
With `OMADA_MQTT_RETAIN=true`, they are retained and published only when the
effective attribute payload changes. With retention disabled, they are replayed
on each collection cycle so a restarted broker or Home Assistant MQTT consumer
can rebuild the tracker metadata. Active trackers do not receive a polling
timestamp. When a client changes from `home` to `not_home`, OmadaBridge
publishes `last_seen` once with the time of the final successful observation.
Configured clients additionally carry the stable `configured: true` attribute
while offline. Tracker attributes do not contain `last_updated`.

Presence continues to come exclusively from the `home`/`not_home` state topic.
With retention enabled, OmadaBridge restores retained tracker states and
attributes at startup, then marks a previously-home client `not_home` if it is
absent from the first fresh collection.

## Entity Names

Metric entity names are generated from the metric name plus selected labels:

1. The `omada_` prefix is removed.
2. Remaining words are title-cased.
3. Qualifiers are appended when labels exist.

Qualifiers are metric-specific:

```text
controller storage: storage_name
controller upgrades: upgrade_channel
ports: port
switch LAGs: lag_id
WAN/ISP: port, name
client totals: connection_mode, wifi_mode
VPNs: name, peer_name
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
- client `port` and `lag_id` attributes for its parent wired path
- `lag_id` and LAG metrics only on the owning switch/gateway
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

Superseded retained entities:

- MQTT retained discovery topics can keep old entities alive.
- At startup OmadaBridge inventories its retained metric topics. It removes an
  old discovery/state pair only when a current entity has the same metric,
  owning identity, and matching site ID or site name.
- When several retained entities represent that same current property, the
  newest retained counterpart is reused as the canonical entity. Its Home
  Assistant identity and history are preserved while older copies are removed.
- Records without a confirmed current counterpart are preserved. This prevents
  transient API failures or offline clients from causing broad deletion.
- Automatic reconciliation requires the MQTT account to subscribe to the
  publisher's discovery and `entities/+/state` topics. If broker ACLs deny
  those reads, publishing continues and cleanup is safely disabled with a log
  warning.
- Changing the MQTT discovery or state topic prefix moves the publisher outside
  the old inventory scope; clear the old prefix manually in that case.
- Reload the Home Assistant MQTT integration or restart Home Assistant.

Client trackers stay `home`:

- Confirm per-client collection is enabled.
- Confirm the client has a MAC address in Omada data.
- With retained MQTT enabled, the bridge restores previously-home dynamic
  tracker IDs at startup and marks absent clients `not_home` after the first
  fresh publish. The MQTT account must be able to subscribe to the configured
  `device_trackers/+/state` and `device_trackers/+/attributes` topics for this
  restart recovery and attribute-cache restoration.
- Add long-lived clients to `OMADA_MQTT_TRACKED_CLIENT_MACS` when their tracker
  should be created as `not_home` even if the current publisher has never seen
  them online.

Cards show no data:

- Confirm Home Assistant has entities with a `metric` attribute beginning with `omada_`.
- Confirm the card `site` option matches the `site` attribute, or remove the `site` option.
- Confirm the card resource points at the built `omada-network-card.js`.
