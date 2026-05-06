# Omada Home Assistant Card Documentation

The Omada card package provides Lovelace UI components for Home Assistant entities created by OmadaBridge MQTT Discovery.

## Docs

- [Installation](./installation.md): build output, Home Assistant resource setup, and example views.
- [Cards](./cards.md): card types, options, and YAML examples.
- [Data source](./data-source.md): entities and attributes the cards read from Home Assistant.
- [Bridge Home Assistant guide](../../bridge/docs/home-assistant.md): MQTT Discovery, created devices, naming, and topics.

## Cards

| Card | Purpose |
| --- | --- |
| `custom:omada-network-card` | Full panel dashboard for site summary, ISP/VPN tables, devices, ports, clients, and detail charts. |
| `custom:omada-links-card` | Compact ISP and VPN table card. |

The cards do not call OmadaBridge directly. They read Home Assistant entity states and attributes that were created from MQTT Discovery.
