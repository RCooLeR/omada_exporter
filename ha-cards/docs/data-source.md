# Data Source

The cards read Home Assistant state. They do not call OmadaBridge HTTP endpoints and they do not connect to MQTT directly.

## Entity Selection

The card model scans `hass.states` and uses entities whose attributes match OmadaBridge MQTT payloads.

Metric entities must include:

```text
attributes.metric = omada_...
```

Before grouping, both cards select exactly one source using the optional
`bridge_id` card setting and `attributes.bridge_id`. Omitted/empty `bridge_id`
means **legacy entities only** (missing or empty attribute), not all bridges.
For example, `bridge_id: slobidska44` selects only apartment entities and never
uses legacy trackers, availability, DPI, or quality data as a fallback.

The optional card `site` setting filters the selected bridge by:

```text
attributes.site
```

Only legacy cards allow an entity without `site` through this name filter,
preserving compatibility with older payloads. An optional `site_id` setting
matches `attributes.site_id` exactly and excludes missing IDs. Caches and update
detection use all three scope fields, so updates to another bridge do not
rebuild this card's model.

## Device Records

Device rows are built from entities whose metric starts with:

```text
omada_device_
omada_controller_
```

Important attributes:

```text
device_mac, device_name, device_type, device_model, device_show_model,
device_status, device_ip, device_version, site
```

Ports are attached to devices by:

```text
bridge_id + site_id (or site) + device_mac + port
```

Radio rows are attached by:

```text
bridge_id + site_id (or site) + device_mac + band/radio labels
```

## Client Records

Client rows require a `device_tracker` entity with a `mac` attribute whose
current state is exactly `home`. Metric entities starting with
`omada_client_` enrich those active rows with traffic, signal, and attachment
data. Retained metric entities never create a client by themselves, so clients
that are `not_home`, unavailable, or missing a tracker are not shown. Tracker
presence is matched within the selected bridge and site: an apartment's newer
`not_home` state cannot suppress a phone currently `home` on the legacy bridge.

Important attributes:

```text
mac, ip, vendor, host_name, system_name, device_category,
device_type, wireless, ap_mac, ap_name, switch_mac, switch_name,
gateway_mac, gateway_name, port, lag_id,
ssid, vlan_id, wifi_mode, site
```

Wired client path data references port and LAG metrics when `switch_mac` or
`gateway_mac` plus `port` or `lag_id` match an existing
parent port/LAG record in the same bridge and site. The card copies only attachment display values such as
link speed and LAG member ports; switch-owned LAG attributes and metrics remain
on the switch.

## ISP, WAN, And VPN Tables

ISP rows use metrics starting with:

```text
omada_isp_
```

WAN rows use:

```text
omada_wan_
```

The card matches WAN rows to ISP rows by WAN name or port **within the same
bridge, site, and owning gateway** (`device_mac`/`gateway_mac`). Ambiguous or
unscoped matches are not borrowed from another gateway. WAN details such as
latency, link speed, and RX/TX rate then enrich the ISP table.

VPN rows use:

```text
omada_vpn_
```

Within the bridge, site and owning device, the row key prefers:

```text
name + vpn_mode + vpn_type
vpn_id
entity_id
```

## DPI Insights

The network card reads these optional MQTT metric sensors:

```text
omada_dpi_total_traffic_bytes
omada_dpi_scrape_window_seconds
omada_dpi_category_traffic_bytes
omada_dpi_application_traffic_bytes
```

Rows use `site_id` (falling back to `site`), `family_id` / `family_name`, and `application_id` / `application_name`. The publisher's shared `attributes.last_updated` timestamp identifies a snapshot, so retained rows dropped from the latest export do not linger in the card. Home Assistant's own entity timestamp is not treated as a shared snapshot timestamp.

The chip appears only when there is a valid non-negative traffic value, including zero. A query-window sensor alone is not evidence of traffic availability. No controller credentials or direct HTTP requests are required by the card.

## Device Network Quality

The network card attaches optional quality data to existing devices by normalized MAC and site, without creating extra devices, radios or WAN rows:

```text
omada_ap_interference_percentage                          (radio_band)
omada_ap_uplink_packets_total                            (direction)
omada_ap_uplink_dropped_packets_total                    (direction)
omada_ap_uplink_error_packets_total                      (direction)
omada_gateway_wan_quality_latency_average_seconds
omada_gateway_wan_quality_jitter_average_seconds
omada_gateway_wan_quality_packet_loss_average_percentage
omada_gateway_wan_quality_mos_average
omada_gateway_wan_quality_window_seconds
omada_network_quality_available                         (source)
omada_network_quality_scrape_success                    (source)
```

`radio_band` distinguishes `2.4`, `5`, `5-2` and `6`; `direction` distinguishes `up` and `down`. Source availability and the publisher's snapshot timestamps prevent old retained counters from appearing as current after a failed or unsupported query. Offline devices suppress previous quality values. Wireless-backhaul APs can show radio interference, but not wired-uplink counters. Quality sensors never replace the device's richer metadata.

## Required Bridge Behavior

The default OmadaBridge MQTT publisher provides the attributes the cards expect. Avoid stripping JSON attributes from MQTT discovery configs. If Home Assistant entities exist but the card stays empty, inspect one entity in Developer Tools and confirm:

- metric sensors have `attributes.metric` (trackers instead have `attributes.mac`)
- its `attributes.bridge_id` exactly matches the card setting; omit the setting for legacy entities
- it has a matching `attributes.site` or the card `site` option is removed
- its `attributes.site_id` matches when the card specifies `site_id`
- client entities have `attributes.mac`
- device entities have `attributes.device_mac`

See [Home Assistant integration](../../bridge/docs/home-assistant.md) for MQTT naming and payload details.
