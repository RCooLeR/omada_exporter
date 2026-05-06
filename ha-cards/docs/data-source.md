# Data Source

The cards read Home Assistant state. They do not call OmadaBridge HTTP endpoints and they do not connect to MQTT directly.

## Entity Selection

The card model scans `hass.states` and uses entities whose attributes match OmadaBridge MQTT payloads.

Metric entities must include:

```text
attributes.metric = omada_...
```

The optional card `site` setting filters by:

```text
attributes.site
```

If an entity has no `site` attribute, it is still allowed through the filter so site-level or incomplete entities can remain visible.

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
device_mac + port
```

Radio rows are attached by:

```text
device_mac + band/radio labels
```

## Client Records

Client rows are built from:

- metric entities starting with `omada_client_`
- `device_tracker` entities with a `mac` attribute and state not equal to `not_home`

Important attributes:

```text
mac, ip, vendor, host_name, system_name, device_category,
device_type, wireless, ap_mac, ap_name, switch_mac, switch_name,
gateway_mac, gateway_name, port, lag_id, ssid, vlan_id, wifi_mode, site
```

Wired client path data is enriched from port and LAG metrics when `switch_mac` or `gateway_mac` plus `port` or `lag_id` match an existing port/LAG record.

## ISP, WAN, And VPN Tables

ISP rows use metrics starting with:

```text
omada_isp_
```

WAN rows use:

```text
omada_wan_
```

The card attempts to match WAN rows to ISP rows by WAN name or port, then uses WAN details such as latency, link speed, and RX/TX rate in the ISP table.

VPN rows use:

```text
omada_vpn_
```

The row key prefers:

```text
name + vpn_mode + vpn_type
vpn_id
entity_id
```

## Required Bridge Behavior

The default OmadaBridge MQTT publisher provides the attributes the cards expect. Avoid stripping JSON attributes from MQTT discovery configs. If Home Assistant entities exist but the card stays empty, inspect one entity in Developer Tools and confirm:

- it has `attributes.metric`
- it has a matching `attributes.site` or the card `site` option is removed
- client entities have `attributes.mac`
- device entities have `attributes.device_mac`

See [Home Assistant integration](../../bridge/docs/home-assistant.md) for MQTT naming and payload details.
