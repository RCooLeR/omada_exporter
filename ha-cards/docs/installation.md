# Card Installation

The cards are built with Vite and Lit. Build them locally, copy the output to Home Assistant `www`, then register the JavaScript module as a Lovelace resource.

## Build

From `ha-cards/`:

```bash
npm install
npm run build
```

Output:

```text
dist/omada-network-card.js
```

The same bundle registers both custom cards.

## Install In Home Assistant

Copy the built file to Home Assistant:

```text
/config/www/omada-network-card.js
```

Register the resource:

```yaml
resources:
  - url: /local/omada-network-card.js?v=1
    type: module
```

Reload browser cache after replacing the file. Increment the `v=` query value when Home Assistant keeps an old bundle cached.

## Prerequisite Entities

Configure OmadaBridge MQTT publishing before adding the cards. The cards need Home Assistant entities with:

- `metric` attributes beginning with `omada_`
- device labels such as `device_mac`, `device_name`, `device_type`, and `site`
- client labels such as `mac`, `ip`, `ssid`, `ap_mac`, `switch_mac`, and `gateway_mac`
- ISP, WAN, and VPN metric entities for the link tables

See [bridge Home Assistant guide](../../bridge/docs/home-assistant.md).

## Examples

Ready-to-paste examples:

- [omada-network-card.yaml](../examples/omada-network-card.yaml)
- [omada-links-card.yaml](../examples/omada-links-card.yaml)
