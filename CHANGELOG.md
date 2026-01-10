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