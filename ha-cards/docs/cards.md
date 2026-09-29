# Cards

## `custom:omada-network-card`

Full-screen panel card for an Omada site.

It renders:

- site summary chips
- ISP and VPN tables
- device list with controller, gateway, switch, and AP grouping
- client list with wired/wireless filtering
- selected device details, port previews, PoE budget, update state, and charts
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
| `site` | empty | Optional filter. When set, only entities whose `site` attribute matches this value are used. |
| `logo_mode` | `auto` | `auto`, `light`, or `dark`. Controls which bundled Omada logo variant is rendered. |
| `device_limit` | `100` | Maximum devices rendered in the device list. |
| `client_limit` | `150` | Maximum clients rendered in the client list. |
| `show_vpn_peers` | `true` | Show per-peer VPN rows below VPN tunnel rows when peer metrics are available. |

### DPI insights

Enable `OMADA_TRACK_INSIGHT_METRICS=true` on the bridge with MQTT publishing enabled. No additional card configuration is required. After Home Assistant receives DPI sensors, a seventh **DPI** chip appears; select it to inspect traffic, or select a device/client to return to its details.

The panel shows every available exported category and application, sorted by traffic, with separate totals and windows for each site. The bridge defaults to a rolling 24-hour window (`OMADA_INSIGHT_WINDOW_SECONDS=86400`) and up to 50 applications (`OMADA_INSIGHT_APPLICATION_LIMIT=50`; `0` disables application metrics). These are windowed byte totals, not live throughput. Category/application totals may not add up to the site's classified total.

Select a category to show only its exported applications; **All categories** clears that site's filter. Selection persists across traffic refreshes and resets if the category disappears. Category dots and traffic bars use Omada's light-theme palette in descending traffic order, with applications inheriting their category's color. Omada assigns colors by rank, not fixed category ID, so colors can change as rankings change or differ if the controller displays categories not exported by the bridge. Filtering does not change color assignments or the site-total denominator used for shares.

Unavailable or invalid sensors are excluded, while explicit zero traffic displays as `0 B`. Rows from older MQTT snapshots are omitted when source timestamps are present. If all DPI traffic entities become unavailable, the chip disappears and the main panel returns to a device/client. This does not enable DPI inspection on the controller; it displays data the controller and bridge already provide.

## `custom:omada-links-card`

Compact card for ISP and VPN status tables.

```yaml
type: custom:omada-links-card
site: Default
```

Options:

| Option | Default | Purpose |
| --- | --- | --- |
| `site` | empty | Optional filter by Home Assistant entity `site` attribute. |
| `show_vpn_peers` | `true` | Show per-peer VPN rows below VPN tunnel rows when peer metrics are available. |

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
