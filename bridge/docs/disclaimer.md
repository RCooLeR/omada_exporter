# Disclaimer

OmadaBridge is an unofficial DIY open-source project for compatibility, monitoring, and home automation integration. It is not affiliated with, endorsed by, or sponsored by TP-Link, Omada, Home Assistant, Prometheus, or Grafana Labs.

The bridge reads Omada API data using the credentials you configure and publishes monitoring state to HTTP, Prometheus, MQTT, and Home Assistant. Treat those outputs as operational data. Do not expose the HTTP endpoint, MQTT broker, or Prometheus scrape target to untrusted networks.

OmadaBridge does not replace Omada Controller backups, vendor-supported management tools, network monitoring, or security alerting. Validate the collected data against your own controller and firmware before using it for automations or alerts.

See [../../NOTICE](../../NOTICE) for trademark and affiliation notice.
