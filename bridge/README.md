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

### Code organization

- `cmd`: CLI commands, flags, environment bindings and validation. The release linker still sets `cmd.version`.
- `internal/app`: startup, HTTP endpoints, collector registration, health instrumentation and MQTT lifecycle.
- `internal/omada`: controller operations grouped by feature (devices, ports, clients, VPN, DPI, quality and guarded controls). Collectors call this client without selecting a protocol.
- `internal/omada/session`: shared authentication, HTTP transport, response validation and request cache. Telemetry retries and pinned, non-replayed control sessions stay here.
- `internal/model`: controller payloads and value mappings; `internal/collectors`: Prometheus descriptors and samples.
- `internal/hamqtt`: MQTT Discovery, presence and guarded commands; `internal/debugdump`: explicit read-only raw-response diagnostics.

Startup creates one session and one `omada.Client`. Reuse that client across consumers so authentication, cache invalidation and live controller/site identities remain shared. Raw protocol access is limited to the Omada implementation, startup and diagnostics; package-boundary tests enforce this separation.

`go run . mdocs` builds descriptors without contacting a controller and writes a deterministic metric table to the command output writer.

## License

MIT License. See [../LICENSE](../LICENSE).
