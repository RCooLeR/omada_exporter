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
