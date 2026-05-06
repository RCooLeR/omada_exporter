# Installation

OmadaBridge is distributed as the `rcooler/omada_exporter` Docker image and as the `omada-exporter` binary in GitHub releases. The bridge always exposes Prometheus metrics and can optionally publish Home Assistant MQTT Discovery.

## Requirements

- TP-Link Omada Controller reachable from the bridge.
- Omada service user credentials for the controller Web API.
- Omada OpenAPI client id and secret for OpenAPI-backed WAN, ISP, VPN, and client data.
- Docker, Docker Compose, or a local Go toolchain.
- Optional: MQTT broker reachable by both OmadaBridge and Home Assistant.

## Omada Credentials

Create a service user in the Omada Controller account section and use a read-only or viewer-style role where your controller allows it. Create an OpenAPI client under `Settings -> Platform Integration`.

The current CLI marks `OMADA_HOST`, `OMADA_USER`, `OMADA_PASS`, `OMADA_CLIENT_ID`, and `OMADA_SECRET_ID` as required. If OpenAPI credentials are wrong, OpenAPI-backed collectors can be missing or incomplete.

## Docker Compose

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
      LOG_LEVEL: "info"

      OMADA_MQTT_ENABLED: "true"
      OMADA_MQTT_BROKER: "tcp://homeassistant.local:1883"
      OMADA_MQTT_USER: "omada_exporter"
      OMADA_MQTT_PASS: "mqtt-password"
      OMADA_MQTT_CLIENT_ID: "omada_exporter"
      OMADA_MQTT_TOPIC_PREFIX: "omada_exporter"
      OMADA_MQTT_DISCOVERY_PREFIX: "homeassistant"
      OMADA_MQTT_INTERVAL: "60"
      OMADA_MQTT_RETAIN: "true"
      OMADA_MQTT_EXPIRE_AFTER: "180"
    restart: unless-stopped
```

Start it:

```bash
docker compose up -d
docker compose logs -f omada_exporter
```

## Docker Run

```bash
docker run -d \
  --name omada_exporter \
  -p 9202:9202 \
  -e OMADA_HOST='https://192.168.1.20:443' \
  -e OMADA_USER='exporter' \
  -e OMADA_PASS='change-me' \
  -e OMADA_CLIENT_ID='openapi-client-id' \
  -e OMADA_SECRET_ID='openapi-secret' \
  -e OMADA_SITE='Default' \
  -e OMADA_INSECURE='true' \
  rcooler/omada_exporter:latest
```

Add the MQTT variables from the Compose example when Home Assistant discovery is needed.

## Binary

Download the latest release from GitHub and run:

```bash
./omada-exporter \
  --host https://192.168.1.20:443 \
  --username exporter \
  --password change-me \
  --client-id openapi-client-id \
  --secret-id openapi-secret \
  --site Default \
  --port 9202
```

## Source Build

From the repository root:

```bash
cd bridge
go test ./...
go build .
```

Run the local build:

```bash
./omada_exporter --host https://192.168.1.20:443 --username exporter --password change-me --client-id openapi-client-id --secret-id openapi-secret
```

On Windows, the local binary is usually `omada_exporter.exe`.

## Health Checks

Default HTTP port: `9202`.

```bash
curl http://localhost:9202/healthz
curl http://localhost:9202/readyz
curl http://localhost:9202/metrics
```

The Docker image health check calls `/healthz`.

## Core Configuration

| Variable | Default | Required | Purpose |
| --- | --- | --- | --- |
| `OMADA_HOST` | empty | yes | Controller URL including protocol. |
| `OMADA_USER` | empty | yes | Omada service user. |
| `OMADA_PASS` | empty | yes | Omada service user password. |
| `OMADA_CLIENT_ID` | empty | yes | Omada OpenAPI client id. |
| `OMADA_SECRET_ID` | empty | yes | Omada OpenAPI secret. |
| `OMADA_SITE` | `Default` | no | Site name to collect. |
| `OMADA_PORT` | `9202` | no | HTTP listen port. |
| `OMADA_INSECURE` | `false` | no | Skip TLS certificate verification for the controller. |
| `OMADA_REQUEST_TIMEOUT` | `15` | no | API request timeout in seconds. |
| `OMADA_CACHE_TTL` | `5` | no | Cache Omada API fetch results for this many seconds. Set `0` to disable. |
| `LOG_LEVEL` | `error` | no | Zerolog level such as `debug`, `info`, `warn`, or `error`. |

## Collector Options

| Variable | Default | Purpose |
| --- | --- | --- |
| `OMADA_INCLUDE_PORT_ACTIVITY_LABEL` | `true` | Include the `port_activity_label` label on port metrics. |
| `OMADA_TRACK_PORT_METRICS` | `true` | Export per-port metrics. |
| `OMADA_TRACK_CLIENT_METRICS` | `true` | Export per-client metrics. |
| `OMADA_DISABLE_GO_COLLECTOR` | `true` | Disable default Go runtime metrics. |
| `OMADA_DISABLE_PROCESS_COLLECTOR` | `true` | Disable default process metrics. |

## MQTT Options

| Variable | Default | Purpose |
| --- | --- | --- |
| `OMADA_MQTT_ENABLED` | `false` | Enable Home Assistant MQTT publishing. |
| `OMADA_MQTT_BROKER` | `tcp://localhost:1883` | MQTT broker URL. Plain host values are normalized to `tcp://host`. |
| `OMADA_MQTT_USER` | empty | MQTT username. |
| `OMADA_MQTT_PASS` | empty | MQTT password. |
| `OMADA_MQTT_CLIENT_ID` | `omada_exporter` | MQTT client id. |
| `OMADA_MQTT_TOPIC_PREFIX` | `omada_exporter` | MQTT state topic prefix. |
| `OMADA_MQTT_DISCOVERY_PREFIX` | `homeassistant` | Home Assistant discovery prefix. |
| `OMADA_MQTT_INTERVAL` | `60` | Publish interval in seconds. |
| `OMADA_MQTT_RETAIN` | `true` | Publish discovery and state messages as retained. |
| `OMADA_MQTT_EXPIRE_AFTER` | `180` | Home Assistant `expire_after` for sensor entities. Set `0` to disable. |
