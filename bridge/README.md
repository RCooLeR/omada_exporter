# OmadaBridge Bridge

This directory contains the Go bridge that reads TP-Link Omada Controller data and exposes it through Prometheus and optional Home Assistant MQTT Discovery.

## Documentation

- [Documentation index](./docs/index.md)
- [Installation](./docs/installation.md)
- [Omada credentials](./docs/omada-credentials.md)
- [Collected data](./docs/collected-data.md)
- [Prometheus metrics](./docs/prometheus.md)
- [Home Assistant integration](./docs/home-assistant.md)
- [Disclaimer and trademark notice](./docs/disclaimer.md)

## Local Development

Go 1.27 or newer is required. Docker image builds use Docker Buildx.

```bash
go test ./...
go build .
```

CI also runs `go mod tidy` as a cleanliness check, so run it before committing dependency changes.

The released container image and binary keep the existing `omada_exporter` / `omada-exporter` names for compatibility.

## License

MIT License. See [../LICENSE](../LICENSE).
