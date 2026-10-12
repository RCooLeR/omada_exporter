# Card Installation

Download the tested `omada-network-card.js` bundle from the [latest release](https://github.com/RCooLeR/omada_exporter/releases/latest), or build it locally with Vite and Lit. Copy the bundle to Home Assistant `www`, then register the JavaScript module as a Lovelace resource.

## Release Download

Release assets include `omada-network-card.js` and the three sample Grafana dashboards (`dashboard.json`, `omada-device-details.json`, and `simple-omada-dashboard.json`). Download the bundle and `checksums.txt` from the same release. Its SHA-256 hash is listed in `checksums.txt`, and the release workflow generates GitHub artifact attestations for these assets.

On Linux or macOS, verify the downloaded bundle without requiring the other release assets:

```bash
grep '  omada-network-card.js$' checksums.txt | shasum -a 256 --check
```

For Home Assistant, install the JavaScript file below; the JSON files are imported separately in Grafana. No local Node.js installation is needed when using the release bundle.

## Build

Use Node.js 22.22.2+, 24.15.0+, or 26+ (Node.js 24 LTS is recommended) and npm 12.2.0. From `ha-cards/`:

```bash
npm ci
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
