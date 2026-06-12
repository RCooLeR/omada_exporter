# OmadaBridge / omada_exporter

Unofficial TP-Link Omada Controller bridge for Prometheus metrics and Home Assistant MQTT Discovery.

![Omada](https://raw.githubusercontent.com/RCooLeR/omada_exporter/main/bridge/docs/images/omada.png)

The container exposes Prometheus metrics on `/metrics` and can optionally publish the same Omada data to Home Assistant through MQTT Discovery. The image name remains `rcooler/omada_exporter` for compatibility.

## Compose Example

```yaml
services:
  omada_exporter:
    image: rcooler/omada_exporter:latest
    container_name: omada_exporter
    ports:
      - "9202:9202"
    environment:
      OMADA_HOST: "https://192.168.1.20:443"
      OMADA_USER: "exporter"
      OMADA_PASS: "change-me"
      OMADA_CLIENT_ID: "openapi-client-id"
      OMADA_SECRET_ID: "openapi-secret"
      OMADA_SITE: "Default"
      OMADA_INSECURE: "true"
      OMADA_TRACK_INSIGHT_METRICS: "false"
      OMADA_INSIGHT_WINDOW_SECONDS: "86400"
      OMADA_INSIGHT_APPLICATION_LIMIT: "50"

      OMADA_MQTT_ENABLED: "true"
      OMADA_MQTT_BROKER: "tcp://homeassistant.local:1883"
      OMADA_MQTT_USER: "omada_exporter"
      OMADA_MQTT_PASS: "mqtt-password"
      OMADA_MQTT_TOPIC_PREFIX: "omada_exporter"
      OMADA_MQTT_DISCOVERY_PREFIX: "homeassistant"
      OMADA_MQTT_TRACKED_CLIENT_MACS: "aa:bb:cc:dd:ee:ff"
    restart: unless-stopped
```

Health:

```text
http://localhost:9202/healthz
http://localhost:9202/readyz
http://localhost:9202/metrics
```

Documentation: https://github.com/RCooLeR/omada_exporter

This is an unofficial project and is not affiliated with, endorsed by, or sponsored by TP-Link, Omada, Home Assistant, or Prometheus.
