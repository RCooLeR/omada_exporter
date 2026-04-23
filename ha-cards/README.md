# Omada Home Assistant Cards

Custom Lovelace cards for `omada_exporter` Home Assistant entities published over MQTT.

These cards are an optional Home Assistant UI layer on top of the exporter:

- Project overview and Prometheus setup: [README.md](../README.md)
- MQTT and Home Assistant setup: [ha.md](../ha.md)

## Cards

- `custom:omada-network-card`
  - Full-screen Omada dashboard card for a panel view
  - Header with summary chips, ISP table, VPN table
  - Device list, client list, shared detail workspace
  - Device details: ports, attached clients, health charts, PoE budget, updates
  - Client details: wired/wireless path, live activity, LAG-aware wired details

- `custom:omada-links-card`
  - Compact dashboard card
  - ISP and VPN tables only
  - Same columns and visual style as the main card

## Build

```powershell
cd ha-cards
npm install
npm run build
```

Output:

- `dist/omada-network-card.js`

## Home Assistant

Prerequisite: configure `omada_exporter` MQTT publishing first so Home Assistant has the entities these cards expect.

Resource:

```yaml
resources:
  - url: /local/omada-network-card.js?v=1
    type: module
```

Examples:

- [examples/omada-network-card.yaml](/D:/Work/Projects/Go/src/RCooLeR/omada_exporter/ha-cards/examples/omada-network-card.yaml)
- [examples/omada-links-card.yaml](/D:/Work/Projects/Go/src/RCooLeR/omada_exporter/ha-cards/examples/omada-links-card.yaml)

## Notes

- Data is built from Home Assistant MQTT entities published by this exporter.
- The main card is tuned for a 16:9 full-tab dashboard.
- The compact links card is intended for a normal dashboard view.
