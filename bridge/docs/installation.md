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

Controllers in MSP mode are supported with MSP-level credentials; see
[MSP Mode](#msp-mode).

## Omada Credentials

If you are setting this up for the first time, read [Omada Credentials](./omada-credentials.md). It explains how to create:

- a normal Omada user for `OMADA_USER` / `OMADA_PASS`,
- OpenAPI app credentials for `OMADA_CLIENT_ID` / `OMADA_SECRET_ID`,
- Fusion credentials when the UI only shows Webhooks.

Short version: create a service user under `Global View > Account > Account`. For standard Omada controllers, create an OpenAPI app under `Global View > Settings > Platform Integration > Open API`.

The CLI requires `OMADA_HOST`, `OMADA_USER`, and `OMADA_PASS`. `OMADA_CLIENT_ID` and `OMADA_SECRET_ID` are optional. When they are present, OmadaBridge uses standard OpenAPI client-credentials authentication. When they are absent and Fusion is detected, OmadaBridge uses Fusion's web-session OpenAPI mode. When OpenAPI is unavailable, Web API-backed controller, alert, device, port, and optional DPI insight metrics can still be collected.

On a standard controller without either optional OpenAPI credential, startup emits
one `openapi_disabled` configuration notice naming the missing variables and the
skipped data. It is visible even at the default `LOG_LEVEL=error`, without error
severity; `LOG_LEVEL=disabled` suppresses it. Client, ISP, VPN, WAN enrichment and
optional network-quality polling are skipped instead of logging the same failure
on every Prometheus scrape or MQTT poll. Their per-collector endpoints are omitted
from the index and return HTTP 404 when unregistered. Explicit
`OMADA_OPENAPI_AUTH=disabled` skips these sources without claiming credentials are
missing. Incomplete credential pairs still fail startup, and real request or
authentication failures in enabled modes remain errors.

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
      OMADA_TRACK_NETWORK_QUALITY_METRICS: "false"
      OMADA_NETWORK_QUALITY_WINDOW_SECONDS: "86400"
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

## Multiple Bridges With Shared Monitoring

Keep the current installation's `OMADA_BRIDGE_ID` unset. The empty default
preserves its existing MQTT topics, Home Assistant identifiers and unlabelled
Prometheus history. Give every additional bridge a unique, stable
`OMADA_BRIDGE_ID` (CLI: `--bridge-id`). IDs must be 1–64 lowercase letters,
digits or hyphens, starting and ending with a letter or digit; for example,
`slobidska44`. Treat an ID as immutable once deployed: changing it creates new
Home Assistant identities and Prometheus series, not a rename of old data.

A non-empty ID scopes MQTT client/state/discovery/availability identities and
retained-message cleanup to that bridge. It is also published as the
`bridge_id` attribute in Home Assistant and a label on all exported metrics.
Different topic prefixes alone are not a substitute for separate bridge IDs.

Example additional Fusion service, alongside the unchanged existing service:

```yaml
services:
  omada-appt:
    image: rcooler/omada_exporter:latest
    environment:
      OMADA_BRIDGE_ID: "slobidska44"
      OMADA_HOST: "https://10.20.0.1:443"
      OMADA_USER: "service-user"
      OMADA_PASS: "change-me"
      OMADA_SYSTEM_TYPE: "fusion"
      OMADA_OPENAPI_AUTH: "web_session"
      OMADA_CLIENT_ID: ""
      OMADA_SECRET_ID: ""
      OMADA_SITE: "Slobidska 44_28"
      OMADA_PORT: "9040"
      OMADA_INSECURE: "true"
      OMADA_MQTT_ENABLED: "true"
      OMADA_MQTT_BROKER: "tcp://homeassistant.local:1883"
      OMADA_MQTT_USER: "mqtt-user"
      OMADA_MQTT_PASS: "mqtt-password"
    restart: unless-stopped
```

Replace the credential placeholders and join the networks required to reach
the controller, MQTT broker and Prometheus. With a local source deployment,
use the updated build context instead of the published image. Put the ID in
this service's `environment` or a dedicated environment file: **do not add
`OMADA_BRIDGE_ID` to a shared `env_file` inherited by the existing service**.
Fusion `web_session` does not require OpenAPI app credentials; do not inherit
the other controller's client ID/secret.

For a binary deployment, add `--bridge-id slobidska44` to the Fusion command
and use `--host https://10.20.0.1:443 --system-type fusion
--openapi-auth web_session --site "Slobidska 44_28" --port 9040`.

Update the Home Assistant JavaScript bundle and add separate apartment cards
with `bridge_id: slobidska44`; leave existing card YAML unchanged. Both cards
default to legacy entities only when `bridge_id` is omitted. See
[card configuration](../../ha-cards/docs/cards.md#multiple-bridges-in-one-home-assistant).
Add a separate [Prometheus scrape target](./prometheus.md#multiple-bridges)
and reimport the updated sample Grafana dashboards before viewing both sources.
The sample dashboards select one scrape job and bridge endpoint, including
the old source without a `bridge_id` label.

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

## MSP Mode

If the controller runs in **MSP mode**, a single exporter scrapes every MSP customer and
every site and labels all metrics with the customer. It is enabled automatically when
`/api/info` reports `mspMode: true` and `OMADA_SYSTEM_TYPE=auto`, or explicitly with
`OMADA_SYSTEM_TYPE=msp`. Non-MSP controllers behave exactly as before.

Requirements (see [Omada credentials: MSP mode](./omada-credentials.md#msp-mode)):

- An MSP account (`OMADA_USER` / `OMADA_PASS`), verified with the **MSP Owner** role.
- An **MSP-level** Open API app in **Client** mode (`OMADA_CLIENT_ID` / `OMADA_SECRET_ID`).
  The exporter refuses to start in MSP mode without it, or with `OMADA_OPENAPI_AUTH`
  set to `disabled` or `web_session`.

```yaml
services:
  omada_exporter:
    image: rcooler/omada_exporter:latest
    restart: unless-stopped
    ports:
      - "9202:9202"
    environment:
      OMADA_HOST: "https://omada.example.net:8043"
      OMADA_USER: "msp-exporter"
      OMADA_PASS: "change-me"
      OMADA_CLIENT_ID: "msp-level-client-id"
      OMADA_SECRET_ID: "msp-level-client-secret"
      OMADA_SYSTEM_TYPE: "auto"
      OMADA_OPENAPI_AUTH: "auto"
      # Optional:
      # OMADA_MSP_CUSTOMERS: "Alpha Corp,Beta Ltd"
      # OMADA_SITE: "*"
      # OMADA_MSP_DISCOVERY_INTERVAL: "600"
      # OMADA_MSP_SCRAPE_CONCURRENCY: "4"
```

### MSP options

| Variable | Default | Purpose |
| --- | --- | --- |
| `OMADA_MSP_CUSTOMERS` | `all` | `all` (or empty), or a comma-separated list of customer names (case-insensitive) or customer IDs. Entries that match nothing are logged as warnings. |
| `OMADA_SITE` | all sites | In MSP mode the `Default` flag default is **not** applied. Set it to a comma-separated list of site names (case-insensitive) or site IDs to limit the sites of every selected customer; `*` or empty means all. |
| `OMADA_MSP_DISCOVERY_INTERVAL` | `600` | Re-list customers and sites every this many seconds and add or remove targets. `0` discovers only at startup. |
| `OMADA_MSP_SCRAPE_CONCURRENCY` | `4` | How many customer sites are scraped at the same time. The collectors of one site still run in parallel. Raise it if a scrape of all targets gets close to the Prometheus scrape timeout. |

### How it works

1. At startup the exporter logs in with the MSP ID (web session and OpenAPI token) and
   lists customers (`/openapi/v1/msp/{mspId}/customers`) and each customer's sites
   (`/openapi/v1/{customerId}/sites`), then applies the filters.
2. Every (customer, site) pair becomes a target with its own collectors and cache. All
   targets share one web session and one OpenAPI token; data requests put the customer ID
   where the controller ID normally goes.
3. Discovery repeats every `OMADA_MSP_DISCOVERY_INTERVAL`. If the customer list fails,
   all current targets are kept; if one customer's site list fails, that customer's
   targets are kept.
4. One failing customer does not affect the others. Re-logins triggered by a customer
   are limited to one per 30 seconds so a misconfigured customer cannot keep replacing
   the shared credentials.

### Labels

Every collector metric gets two extra labels; `site` and `site_id` carry the real site
name and ID of the target:

| Label | Value |
| --- | --- |
| `customer` | Customer name from the MSP customer list. |
| `customer_id` | Customer ID (the `omadacId` of the customer). |

`omada_collector_*` health metrics additionally carry `customer`, `customer_id`, `site` and
`site_id`. With `OMADA_BRIDGE_ID` set, `bridge_id` is added as usual.

MSP-only exporter metrics:

| Metric | Labels | Meaning |
| --- | --- | --- |
| `omada_msp_target_up` | `customer`, `customer_id`, `site`, `site_id` | `1` when at least one API request for the target succeeded during its last full scrape, `0` when all failed. |
| `omada_msp_target_scrape_duration_seconds` | same | Duration of the target's last full scrape. |
| `omada_msp_target_api_errors` | same | Failed API requests (transport error, HTTP error, non-zero `errorCode`) during the last full scrape. |
| `omada_msp_targets` | - | Number of targets currently scraped. |
| `omada_msp_discovery_last_success_timestamp_seconds` | - | Last discovery in which the customer list and all site lists loaded. |
| `omada_msp_discovery_failures_total` | - | Discovery runs with at least one failed list. |

`/metrics/{collector}` endpoints serve that collector for all targets; the target and
discovery metrics are only on `/metrics`.

Example alert for an unreachable customer site:

```yaml
- alert: OmadaMSPTargetDown
  expr: omada_msp_target_up == 0
  for: 15m
  labels:
    severity: warning
  annotations:
    summary: "Omada customer {{ $labels.customer }} / site {{ $labels.site }} is not answering"
```

### Grafana

The bundled dashboards filter by `site`, which is not unique across customers (many
customers have a site called `Default`). For MSP mode add a `customer` variable and use
it in the queries:

- Variable: `label_values(omada_controller_uptime_seconds{job=~"$job", instance=~"$instance"}, customer)`,
  multi-value, include **All** with custom all value `.*`.
- Make the `site` variable depend on it:
  `label_values(omada_controller_uptime_seconds{customer=~"$customer"}, site)`.
- Add `customer=~"$customer"` to the panel selectors. `=~".*"` also matches series
  without the label, so the change keeps working for non-MSP exporters.

### Limitations

- The Home Assistant MQTT publisher (`OMADA_MQTT_ENABLED`) and device control
  (`OMADA_ALLOW_DEVICE_CONTROL`) are not available in MSP mode; the exporter refuses to
  start with them. Run a separate non-MSP exporter for a site that needs them.
- `OMADA_DUMP_RESPONSES_DIR` writes one dump per target, each in its own `omada-*`
  subdirectory. `manifest.json` names the customer ID (`omadaCid`), site and site ID.
- Verified end to end on a real MSP controller (v5.15.24.19, two customers, sites
  with access points and switches but no gateway): discovery, devices, switch ports and
  LAGs, AP radios, clients (OpenAPI v1; v2 answers 404 and the existing v1 fallback is
  used), controller status and upgrades, alert count, VPN list and tunnel statistics,
  DPI requests (empty without a gateway) and AP wired uplink. Gateway-only endpoints
  (ISP load, WAN status, gateway ports, gateway health detail, site-to-site VPN summary)
  answer HTTP 404 on sites without a gateway and are not verified on an MSP site with a
  gateway yet. EAP port requests were not exercised.
- On sites without a gateway, those gateway endpoints fail on every scrape, so
  `omada_msp_target_api_errors` stays above zero (4 per site with the default
  collectors). Alert on `omada_msp_target_up == 0`, or on increases of the error count.
- Every metric gains the `customer` and `customer_id` labels, so the number of series
  grows with the number of customers and sites. Use the filters to limit it.

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
| `OMADA_BRIDGE_ID` | empty | no | Immutable unique namespace for an additional bridge (`--bridge-id`). Empty preserves legacy identities. Non-empty: 1–64 lowercase letters/digits/hyphens, beginning and ending with a letter or digit. |
| `OMADA_ALLOW_DEVICE_CONTROL` | `false` | no | Opt in to MQTT reboot, Wi-Fi reconnect and PoE power-cycle buttons (`--allow-device-control`). Requires MQTT and controller write permissions; see [Home Assistant controls](./home-assistant.md). |
| `OMADA_HOST` | empty | yes | Controller URL including protocol. |
| `OMADA_USER` | empty | yes | Omada service user. |
| `OMADA_PASS` | empty | yes | Omada service user password. |
| `OMADA_CLIENT_ID` | empty | no | Omada OpenAPI client id for standard client-credentials auth. |
| `OMADA_SECRET_ID` | empty | no | Omada OpenAPI secret for standard client-credentials auth. |
| `OMADA_SYSTEM_TYPE` | `auto` | no | Omada system type: `auto`, `standard`, `fusion`, or `msp`. `auto` detects MSP mode from the controller. |
| `OMADA_OPENAPI_AUTH` | `auto` | no | OpenAPI auth mode: `auto`, `client_credentials`, `web_session`, or `disabled`. |
| `OMADA_SITE` | `Default` | no | Site name to collect. In [MSP mode](#msp-mode): unset, empty or `*` collects every site; otherwise a comma-separated list of site names or IDs. |
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
| `OMADA_TRACK_NETWORK_QUALITY_METRICS` | `false` | Export optional AP wired-uplink counters and gateway WAN quality averages; requires supported OpenAPI responses. |
| `OMADA_NETWORK_QUALITY_WINDOW_SECONDS` | `86400` | Positive rolling-window duration for gateway WAN quality averages. |
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
