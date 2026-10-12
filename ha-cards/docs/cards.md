# Cards

## `custom:omada-network-card`

Full-screen panel card for an Omada site.

It renders:

- site summary chips
- ISP and VPN tables
- device list with controller, gateway, switch, and AP grouping
- client list with wired/wireless filtering
- selected device details, port previews, PoE budget, update state, and charts
- contextual AP interference/uplink counters and gateway WAN quality in the selected device details, when exported
- selected client details, path information, LAG-aware wired details, and activity/signal charts
- a DPI summary chip when valid insight traffic entities are available; clicking it opens classified traffic totals, query windows, categories, and applications in the main detail panel

Recommended Lovelace view:

```yaml
views:
  - title: Omada
    path: omada
    panel: true
    cards:
      - type: custom:omada-network-card
        site: Default
        logo_mode: auto
        device_limit: 100
        client_limit: 200
```

Options:

| Option | Default | Purpose |
| --- | --- | --- |
| `bridge_id` | empty | Select one bridge's exact `bridge_id` attribute. Empty/omitted selects **only legacy, unscoped entities**, not all bridges. |
| `site` | empty | Optional exact site-name filter within the selected bridge. Legacy cards also retain support for older entities without a site name. |
| `site_id` | empty | Optional exact site-ID filter; entities without that ID are excluded. Use when site names are repeated. |
| `logo_mode` | `auto` | `auto`, `light`, or `dark`. Controls which bundled Omada logo variant is rendered. |
| `device_limit` | `100` | Maximum devices rendered in the device list. |
| `client_limit` | `150` | Maximum clients rendered in the client list. |
| `show_vpn_peers` | `true` | Show per-peer VPN rows below VPN tunnel rows when peer metrics are available. |

### DPI insights

Enable `OMADA_TRACK_INSIGHT_METRICS=true` on the bridge with MQTT publishing enabled. No additional card configuration is required. After Home Assistant receives DPI sensors, a seventh **DPI** chip appears; select it to inspect traffic, or select a device/client to return to its details.

The panel shows every available exported category and application, sorted by traffic, with separate totals and windows for each site. The bridge defaults to a rolling 24-hour window (`OMADA_INSIGHT_WINDOW_SECONDS=86400`) and up to 50 applications (`OMADA_INSIGHT_APPLICATION_LIMIT=50`; `0` disables application metrics). These are windowed byte totals, not live throughput. Category/application totals may not add up to the site's classified total.

Select a category to show only its exported applications; **All categories** clears that site's filter. Selection persists across traffic refreshes and resets if the category disappears. Category dots and traffic bars use Omada's light-theme palette in descending traffic order, with applications inheriting their category's color. Omada assigns colors by rank, not fixed category ID, so colors can change as rankings change or differ if the controller displays categories not exported by the bridge. Filtering does not change color assignments or the site-total denominator used for shares.

Unavailable or invalid sensors are excluded, while explicit zero traffic displays as `0 B`. Rows from older MQTT snapshots are omitted when source timestamps are present. If all DPI traffic entities become unavailable, the chip disappears and the main panel returns to a device/client. This does not enable DPI inspection on the controller; it displays data the controller and bridge already provide.

### AP and gateway network quality

Select an AP or gateway to see its optional quality section below the device summary. No extra chip or card configuration is needed.

- **APs:** interference by supported radio band, plus wired uplink packet, drop and error counts in both directions. Counts are cumulative controller totals that can reset, not rates. Wireless-backhaul APs show that wired counters do not apply.
- **Gateways:** average latency, jitter, packet loss and MOS across the gateway, with the actual rolling query window (24 hours by default). These are not live readings or per-WAN-port measurements. The existing ISP/WAN table is unchanged.

Radio interference is collected automatically. Enable `OMADA_TRACK_NETWORK_QUALITY_METRICS=true` in the bridge container environment for uplink counters and WAN quality, with MQTT publishing enabled. Optionally set `OMADA_NETWORK_QUALITY_WINDOW_SECONDS`. Rebuild/recreate the bridge container after deploying the updated source/configuration, then allow a MQTT publishing cycle.

Missing fields display as `—`; valid zeroes remain zero. Failed/unavailable/offline sources hide previous readings rather than presenting retained values as current. Devices without any quality entities keep their existing layout. Update the JavaScript resource and refresh the browser when upgrading the cards. See [network quality](../../bridge/docs/network-quality.md) for controller support and collection details.

## `custom:omada-links-card`

Compact card for ISP and VPN status tables.

```yaml
type: custom:omada-links-card
site: Default
```

Options:

| Option | Default | Purpose |
| --- | --- | --- |
| `bridge_id` | empty | Exact bridge filter; empty/omitted selects only legacy, unscoped entities. |
| `site` | empty | Optional site-name filter within the selected bridge, with legacy site-less compatibility. |
| `site_id` | empty | Optional exact `site_id` filter, excluding entities without that ID. |
| `show_vpn_peers` | `true` | Show per-peer VPN rows below VPN tunnel rows when peer metrics are available. |

## Multiple bridges in one Home Assistant

Keep the existing bridge's `OMADA_BRIDGE_ID` unset and its card YAML unchanged. Its MQTT/HA identifiers and historical data remain unchanged. Configure the apartment bridge with `OMADA_BRIDGE_ID=slobidska44`, and add separate cards:

```yaml
type: custom:omada-network-card
title: Apartment
bridge_id: slobidska44
# Optional, after confirming the controller's actual site name/ID:
# site: Default
# site_id: <controller-site-id>
```

```yaml
type: custom:omada-links-card
bridge_id: slobidska44
```

Both cards require the updated JavaScript bundle before adding another bridge. The bridge filter applies to trackers, metrics, DPI, quality status and availability data before grouping, including when the controllers have identical MACs, site IDs or names. A missing/unknown bridge ID produces an empty card; it never falls back to the other bridge. There is intentionally no wildcard/all-bridges selection.

Within one bridge, device/client attachments are also scoped by site, and WAN lookups by site and gateway. Use `site_id` for a single-site view when two sites share the same name. A card for the existing legacy bridge does not need a `bridge_id` option. Home Assistant may assign its own entity-ID suffixes during discovery; the cards use attributes, not entity-ID naming patterns.

## Resource Registration

Both cards are registered by the same bundle:

```yaml
resources:
  - url: /local/omada-network-card.js?v=1
    type: module
```

The card types are:

```text
custom:omada-network-card
custom:omada-links-card
```
