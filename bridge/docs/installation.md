# Installation

OmadaBridge is distributed as the `rcooler/omada_exporter` Docker image and as the `omada-exporter` binary in GitHub releases. The bridge always exposes Prometheus metrics and can optionally publish Home Assistant MQTT Discovery.

## Requirements

- TP-Link Omada Controller reachable from the bridge.
- Omada service user credentials for the controller Web API.
- For standard Omada controllers, an Omada OpenAPI client id and secret are recommended for OpenAPI-backed WAN, ISP, VPN, and client data.
- For Omada Fusion gateways, username/password credentials are enough; the bridge can use Fusion's web-session OpenAPI path.
- Docker or Docker Compose, or Go 1.27+ for a local source build.
- Optional: MQTT broker reachable by both OmadaBridge and Home Assistant.

Published container tags are multi-platform and support both `linux/amd64` and `linux/arm64`.

## Controller Compatibility

OmadaBridge targets the API paths used by current tested Omada Controller
releases; endpoint availability varies by controller model and firmware. Older
firmware can authenticate and expose part of the metric set while rejecting newer
endpoints. The OC200 Controller 5.13.30.20 / firmware 1.29.4 combination reported
in [issue #11](https://github.com/RCooLeR/omada_exporter/issues/11) is known not
to expose controller-status collection. An error such as
`controllerStatus returned errorCode -1600: Unsupported request path` means the
controller does not provide the status endpoint used by the current bridge; it is
not a Grafana or Prometheus configuration error. Upgrade the controller when this
occurs.

If an older controller must remain in service, expect its controller metrics and
dependent dashboard panels to be unavailable while other supported collectors may
continue to work. `/healthz` reports bridge process health, not support for every
controller endpoint.

## Omada Credentials

If you are setting this up for the first time, read [Omada Credentials](./omada-credentials.md). It explains how to create:

- a normal Omada user for `OMADA_USER` / `OMADA_PASS`,
- OpenAPI app credentials for `OMADA_CLIENT_ID` / `OMADA_SECRET_ID`,
- Fusion credentials when the UI only shows Webhooks.

Short version: create a service user under `Global View > Account > Account`. For standard Omada controllers, create an OpenAPI app under `Global View > Settings > Platform Integration > Open API`.

The CLI requires `OMADA_HOST`, `OMADA_USER`, and `OMADA_PASS`. `OMADA_CLIENT_ID` and `OMADA_SECRET_ID` are optional. When they are present, OmadaBridge uses standard OpenAPI client-credentials authentication. When they are absent and Fusion is detected, OmadaBridge uses Fusion's web-session OpenAPI mode. When OpenAPI is unavailable, Web API-backed controller, alert, device, port, and optional DPI insight metrics can still be collected.

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
      OMADA_SYSTEM_TYPE: "auto"
      OMADA_OPENAPI_AUTH: "auto"
      OMADA_SITE: "Default"
      OMADA_INSECURE: "true"
      LOG_LEVEL: "info"
      OMADA_TRACK_INSIGHT_METRICS: "false"
      OMADA_INSIGHT_WINDOW_SECONDS: "86400"
      OMADA_INSIGHT_APPLICATION_LIMIT: "50"

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
      OMADA_MQTT_TRACKED_CLIENT_MACS: "aa:bb:cc:dd:ee:ff,11:22:33:44:55:66"
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
  -e OMADA_SYSTEM_TYPE='auto' \
  -e OMADA_OPENAPI_AUTH='auto' \
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
  --system-type auto \
  --openapi-auth auto \
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

For Fusion gateways without OpenAPI app credentials:

```bash
./omada_exporter \
  --host https://192.168.188.1:443 \
  --username exporter \
  --password change-me \
  --system-type auto \
  --openapi-auth auto \
  --site Default \
  --insecure
```

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
| `OMADA_CLIENT_ID` | empty | no | Omada OpenAPI client id for standard client-credentials auth. |
| `OMADA_SECRET_ID` | empty | no | Omada OpenAPI secret for standard client-credentials auth. |
| `OMADA_SYSTEM_TYPE` | `auto` | no | Omada system type: `auto`, `standard`, or `fusion`. |
| `OMADA_OPENAPI_AUTH` | `auto` | no | OpenAPI auth mode: `auto`, `client_credentials`, `web_session`, or `disabled`. |
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
| `OMADA_TRACK_INSIGHT_METRICS` | `false` | Export optional DPI insight metrics from Omada Web API. |
| `OMADA_INSIGHT_WINDOW_SECONDS` | `86400` | Query window for DPI insight metrics. |
| `OMADA_INSIGHT_APPLICATION_LIMIT` | `50` | Maximum DPI application metric series to export. Set `0` to disable application metrics. |
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
| `OMADA_MQTT_TRACKED_CLIENT_MACS` | empty | Comma-separated client MAC addresses to publish `device_tracker` entities for even when currently offline. |
