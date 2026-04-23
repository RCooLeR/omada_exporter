## [Unreleased]
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
