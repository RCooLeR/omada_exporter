## [Unreleased]

### Fixed
- Give Grafana rate queries a one-minute Min step so dashboards work when Omada is scraped every 60 seconds, show site-to-site peer session age when generic VPN uptime is unavailable, and stop presenting disabled DPI insights as real zero values.

## [2.4.1] - 2026-09-02

### Added
- Publish multi-platform `linux/amd64` and `linux/arm64` container images with SBOMs, build provenance, OCI annotations, and GitHub release attestations.
- Add weekly Dependabot updates for Go, npm, Docker, and GitHub Actions dependencies, plus Go vulnerability and npm audit checks in CI.
- Support OpenMetrics negotiation, zstd-compressed metric responses, and `name[]` metric-family filtering on aggregate and per-collector scrape endpoints.
- Add keyboard-accessible Home Assistant card selection/filter controls and descriptive ECharts ARIA output.

### Changed
- Upgrade the bridge to Go 1.27 and current Go modules, migrate the CLI from `urfave/cli` v2 to v3, and use newer standard-library helpers and graceful process cancellation.
- Upgrade the Home Assistant cards to the native TypeScript 7 compiler, Vite 8.2, and current Vitest, with stricter type checking and Vite's native Rolldown configuration.
- Build containers from Go 1.27.0 and Alpine 3.24.1, refresh installed Alpine security packages during image construction, run the final image as a non-root user, and use BuildKit cache mounts and reproducible base-image digests.
- Upgrade and SHA-pin CI/release actions, add race-enabled Go tests, verify downloaded modules, and validate multi-platform container builds and GoReleaser configuration in CI.
- Isolate exporter metrics in a dedicated Prometheus registry, coalesce overlapping gathers, and bound scrape concurrency and duration so slow controllers cannot create unbounded scrape work.
- Use Go 1.27 parameterized methods for typed API caching and paginated OpenAPI requests, and fetch per-device port, LAG, and WAN enrichment concurrently within a fixed limit.
- Validate controller URLs, ports, enums, timeouts, cache windows, insight limits, and MQTT intervals through `urfave/cli` v3 before startup; reject duplicate flags and suggest likely flag names for typos.
- Bound Omada response bodies, return structured HTTP/API failures, and apply cancellable MQTT connection handling with bounded writes and concurrent retained-message callbacks.
- Manage ECharts through a Lit reactive controller, coalesce resize work, use dirty-rectangle and lazy rendering, and skip card updates caused only by unrelated Home Assistant entities.
- Document partial metric availability on older Omada Controller releases and distinguish unsupported controller API paths from Grafana or Prometheus configuration errors.
- Regenerate the bundled Grafana dashboards from source for the current metric and label contract, portable datasource selection, adaptive rate windows, and Grafana schema 42.

### Fixed
- Encode Omada web and OpenAPI credentials with the JSON encoder so quotes, backslashes, and newlines cannot corrupt login requests.
- Shut down the HTTP server and MQTT publisher cleanly on termination, and apply explicit HTTP request/header limits.
- Remove the browser-global `process` shim from the Home Assistant card bundle.
- Serialize web and OpenAPI authentication transitions, keep controller/site context snapshots synchronized during reauthentication, and prevent stale in-flight fetches from repopulating invalidated caches.
- Make custom-card registration idempotent across Home Assistant resource reloads.
- Recreate charts after a card reconnect, apply changed card configuration immediately, normalize MAC formats across topology joins, keep unnamed WAN/ISP rows distinct, and report missing Wi-Fi quality as unknown.
- Treat APs reported as `Connected(Wireless)` as online in the Home Assistant device list.
- Export gateway temperature as a gauge and derive gateway connectivity from the correct ISP response field.
- Coalesce duplicate site-to-site VPN statistics returned by overlapping Omada API queries without dropping distinct tunnel or peer identities.

## [2.4.0] - 2026-08-05

### Changed
- Treat client `port` and `lag_id` labels as attachment metadata; LAG ownership remains with the parent switch/gateway.
- Use metric-specific Home Assistant MQTT identities so mutable client topology, gateway, site display names, and Wi-Fi properties do not create duplicate entities.
- Publish client tracker attributes only when labels or presence metadata change; `last_seen` is emitted once when a client leaves instead of being refreshed on every collection interval.

### Fixed
- Show only clients whose Home Assistant device tracker is currently `home`; retained client metrics no longer recreate inactive rows or display their stale traffic values, and retained tracker inventory now lets a restarted publisher mark absent clients `not_home`.
- Stop labelling wired clients as `802.11a` or `LAG 0`, and clear wireless-only AP/SSID properties from wired clients.
- Prevent Omada infrastructure devices that also appear in the active-client feed from creating duplicate client trackers/devices.
- Keep switch LAG metrics owned by switches in the Home Assistant card instead of merging them into clients/controllers.
- Give each DPI category and application its own MQTT entity identity.
- Reconcile retained MQTT records only when a current semantic counterpart confirms that the old record was superseded; unrelated or unconfirmed records are preserved.
- Reuse the newest retained counterpart during MQTT identity migration so existing Home Assistant entity IDs, history, automations, and customizations remain attached.
- Retry superseded retained-entity cleanup when either MQTT tombstone publish fails instead of forgetting partially removed records.
- Replay tracker attributes on each discovery cycle when MQTT retention is disabled, so Home Assistant and broker restarts do not lose client metadata.

## [2.3.2] - 2026-07-10
### Added
- Expose optional VPN detail labels from Omada OpenAPI responses, including local IP, local/remote networks, allowed IPs, endpoint, and endpoint IP.
- Add optional site-to-site VPN peer status and packet metrics when Omada returns peer-level data.
- Show site-to-site VPN metrics and optional peer rows in the Home Assistant cards.

### Fixed
- Use site-to-site VPN aggregate and peer metrics in Home Assistant card VPN totals instead of showing empty totals for WireGuard/site-to-site rows.
- Improve Home Assistant card VPN display fallbacks for remote/allowed IP, mode, and uptime.

## [2.3.1] - 2026-07-05
### Fixed
- Keep controller metrics available when the optional software update channel endpoint is unreachable, and avoid unnecessary session reauthentication after transport timeouts.

## [2.3.0] - 2026-07-05
### Added
- Add optional DPI insight metrics behind `OMADA_TRACK_INSIGHT_METRICS`, with configurable query window and application-series limit.
- Add `omada_controller_storage_total_bytes`.

### Changed
- Fetch paginated OpenAPI client and VPN tunnel stats instead of only the first 1000 rows.
- Convert Omada client RX/TX negotiation rates from Kbit/s to bit/s before export, preserving Home Assistant's existing unit.

### Fixed
- Fix controller storage byte conversion and make `omada_controller_storage_available_bytes` report free storage.
- Fix AP 5 GHz-2 radio labels to use the 5 GHz-2 radio data.
- Return errors for empty alert responses, device decode failures, and non-zero Omada API JSON error envelopes.

## [2.2.1] - 2026-06-12
### Fixed
- Fix startup panic from duplicate collector self-metric descriptors when multiple Omada collectors are registered in the same Prometheus registry.

## [2.2.0] - 2026-06-11
### Added
- Add `OMADA_MQTT_TRACKED_CLIENT_MACS` / `--mqtt-tracked-client-macs` to publish Home Assistant client device trackers as `not_home` even when configured clients are already offline.
- Add GitHub Actions CI for Go tests, vet, build, Home Assistant card typecheck, tests, and build.
- Add collector self-metrics for last scrape completion, scrape duration, and recovered panics.
- Add focused tests for API cache behavior, secret redaction, collector panic recovery, MQTT tracked client discovery/state payloads, and Home Assistant card model/format helpers.

### Changed
- Improve MQTT publisher connection handling so it retries broker connection failures and publishes availability state more reliably.
- Harden Web API and Open API authentication by deduplicating concurrent login/refresh attempts and redacting token-like query parameters from URL-shaped errors.
- Upgrade Go module dependencies, Home Assistant card dependencies, and Docker base images.
- Reorganize repository documentation, release packaging, and Home Assistant card docs for the OmadaBridge project structure.

### Fixed
- Fix root page links for VPN stats and ISP per-collector metrics.
- Remove the typo `github.com/RCooLeR/omada_exporte` module replacement.
- Fix Home Assistant card typing issues exposed by upgraded ECharts, Vite, and TypeScript.

### Docs
- Document collector self-metrics and local development checks.
- Add/update legal, trademark, disclaimer, Docker, Home Assistant, and dashboard documentation.

## [2.1.6] - 2026-04-29
### Fixed
- Fetch site-to-site VPN peer stats with the tunnel stats `id` required by the peer endpoint, and export peer metrics with the peer-specific identifier from the peer stats response.

## [2.1.5] - 2026-04-29
### Fixed
- Include site-to-site VPNs in `omada_vpn_status` when they are not present in the legacy VPN summary endpoint.
- Treat omitted site-to-site VPN peer/count fields as missing data instead of exporting misleading zero values.

### Changed
- Add aggregated per-tunnel site-to-site traffic metrics: `omada_site_to_site_vpn_down_bytes` and `omada_site_to_site_vpn_up_bytes`.
- Remove unsupported site-to-site metrics for peer status, peer packet counters, and connected/disconnected peer counts.

## [2.1.4] - 2026-04-29
### Fixed
- Build Linux release binaries with `CGO_ENABLED=0` so Docker release images run correctly on Alpine.
- Add a release workflow smoke test that builds the release-style image and verifies the container starts before publishing artifacts.

## [2.1.3] - 2026-04-29
### Added
- Add support for site-to-site VPN.
- Add additional metrics for site-to-site VPN.
- Add `siteId` and `siteName` to client labels.

## [2.1.2] - 2026-04-25
### Added
- Add Docker container health checks with dedicated `/healthz` and `/readyz` endpoints.
### Fixed
- Accept larger client VLAN IDs from the Omada API by widening `NetworkClient.vid`, preventing JSON unmarshal failures for values like `999`.

## [2.1.1] - 2026-04-24
### Added
- Home Assistant custom cards in `ha-cards/`, including Omada network and links cards, build tooling, and example dashboard configs.
### Changed
- Add client-side API request caching with a configurable TTL to reduce repeated Omada requests and improve collector/API performance.
- Optimize Home Assistant custom cards with better filtering, visible-record handling, and more efficient list rendering for large device and client datasets.
- Improve ISP display names in Home Assistant custom cards with fallback formatting.
### Docs
- Update README and related docs to present `omada_exporter` as both a Prometheus exporter and a Home Assistant MQTT integration.

## [2.1.0] - 2026-04-20
### Added
- Home Assistant MQTT Discovery support with configurable broker, discovery prefix, state prefix, publish interval, retained messages, and sensor expiration.
- MQTT entities for existing Omada metrics, including controller, alerts, devices, WAN, ports, LAGs, AP radios, clients, VPN, VPN stats, ISP, and active client device trackers.
- `ha.md` with Home Assistant MQTT setup, Docker Compose example, topic examples, and published entity coverage.

## [2.0.4] - 2026-04-09
### Changed
- Fix pulling info for clients (starting to fail after upgrading controller to 6.2.0.17)

## [2.0.3] - 2026-04-05
### Changed
- removed device_uptime_seconds from device labels (thanks to [@lauer](https://github.com/lauer) for reporting)
- updated dependencies
- re-auth on auth/request failures after controller restart
- added config/env toggles for optional port activity label, per-port metrics, and per-client metrics

## [2.0.2] - 2026-01-19
### Fixed
- add Access Point port metrics for those having ports (Wall, desktop)

## [2.0.1] - 2026-01-11
### Changed
- match device label across metrics
- fix some bugs in metrics calculation
- added gateway temp
- added label like "⚡ 9w ⇅ 2.5 Gbps" for ports

## [2.0.0] - 2026-01-10
### Changed
- full refactoring of the App 🤦
- some labels names changed to match api field names
### Added
- A lot of Labels
- Alert metric
  - omada_site_alert_num 
- Controller metric
  - omada_controller_upgrade_available
- Device Band Utilization Metrics (depends from device)
  - omada_device_2g_rx_util
  - omada_device_2g_tx_util
  - omada_device_5g_rx_util
  - omada_device_5g_tx_util
  - omada_device_5g1_rx_util
  - omada_device_5g1_tx_util
  - omada_device_5g2_rx_util
  - omada_device_5g2_tx_util
  - omada_device_6g_rx_util
  - omada_device_6g_tx_util
- ISP Metrics
  - omada_isp_status
  - omada_isp_download_speed
  - omada_isp_upload_speed
- LAG (Link Aggregation Group) Metrics
  - omada_lag_link_status
  - omada_lag_link_speed_mbps
  - omada_lag_link_rx
  - omada_lag_link_tx
- endpoints for collectors (thanks to MaJaHa95) which will allow to make jobs for your needs only
  - /metrics/controller 
  - /metrics/alert 
  - /metrics/device (all devices with gateway WAN's, Switch ports & lags, AP radio stats)
  - /metrics/client
  - /metrics/vpn 
  - /metrics/vpn-stats 
  - /metrics/isp 
### Fixed
- duplicated slow requests
- repeated auth requests
- info logging level so we can see what is going on in docker logs

### ⚠️ `omada_client_upload_activity_bytes` API is buggy and does not return correct values.  
  Use:
  ```promql
  rate(omada_client_traffic_up_bytes[3m])
  rate(omada_client_traffic_down_bytes[3m])
  ```
   
## [1.0.0] - 2026-01-08
### Added
- Open API support
- Metrics
### Fixed 
- omada_client_traffic_down_bytes
- omada_client_traffic_up_bytes
- omada_client_tx_rate
- omada_client_rx_rate

## [0.13.1] - 2024-08-05
### Fixed
- fix getCid on new omada
---
Old history: check git commits
