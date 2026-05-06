# OmadaBridge Documentation

OmadaBridge reads Omada Controller data, exposes Prometheus metrics on HTTP, and can publish the same data to Home Assistant through MQTT Discovery.

## Bridge

- [Installation](./installation.md): Docker, binary, source build, Omada credentials, environment variables, and health checks.
- [Collected data](./collected-data.md): what data is read from Omada and how it maps to monitoring outputs.
- [Prometheus metrics](./prometheus.md): endpoints, scrape config, metric conventions, and metric reference.
- [Home Assistant integration](./home-assistant.md): MQTT setup, topics, created devices, entity ids, sensor naming, payloads, and troubleshooting.
- [Disclaimer](./disclaimer.md): unofficial project, trademarks, and operational responsibility.

## Assets

- [Grafana dashboards](./dashboards/)
- [Images](./images/)

## Card Package

- [Home Assistant card documentation](../../ha-cards/docs/index.md)
- [Card package README](../../ha-cards/README.md)

## Repository

- [Root README](../../README.md)
- [Notice](../../NOTICE)
- [License](../../LICENSE)
