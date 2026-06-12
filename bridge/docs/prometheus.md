# Prometheus

OmadaBridge exposes Prometheus metrics on the HTTP server. Default endpoint:

```text
http://localhost:9202/metrics
```

The root `/metrics` endpoint contains every enabled collector. Per-collector endpoints are also registered:

```text
/metrics/controller
/metrics/alert
/metrics/device
/metrics/client
/metrics/vpn
/metrics/vpn-stats
/metrics/isp
```

When optional DPI insight metrics are enabled, `/metrics/insights` is also registered.

## Scrape Config

```yaml
scrape_configs:
  - job_name: omada_exporter
    scrape_interval: 30s
    scrape_timeout: 30s
    static_configs:
      - targets:
          - omada_exporter:9202
```

Use the hostname or IP address reachable from Prometheus. If Prometheus runs outside Docker, the target may be `HOST_IP:9202`.

## Collector Controls

| Variable | Default | Effect |
| --- | --- | --- |
| `OMADA_TRACK_PORT_METRICS` | `true` | Disable to suppress per-port metrics. |
| `OMADA_TRACK_CLIENT_METRICS` | `true` | Disable to suppress per-client metrics while keeping `omada_client_connected_total`. |
| `OMADA_TRACK_INSIGHT_METRICS` | `false` | Enable optional DPI insight metrics from Omada Web API. |
| `OMADA_INSIGHT_WINDOW_SECONDS` | `86400` | Query window for DPI insight metrics. |
| `OMADA_INSIGHT_APPLICATION_LIMIT` | `50` | Maximum DPI application series to export. Set `0` to disable application metrics. |
| `OMADA_INCLUDE_PORT_ACTIVITY_LABEL` | `true` | Disable to reduce label churn from Omada port activity text. |
| `OMADA_DISABLE_GO_COLLECTOR` | `true` | Enable Go runtime metrics by setting this to `false`. |
| `OMADA_DISABLE_PROCESS_COLLECTOR` | `true` | Enable process metrics by setting this to `false`. |

## Metric Conventions

- Metrics are prefixed with `omada_`.
- Most metric values are gauges.
- Boolean-style metrics use `1` for active/online/true and `0` for inactive/offline/false.
- Byte counters ending in `_bytes` are raw bytes.
- Link speeds ending in `_mbps` are Mbit/s.
- Client RX/TX negotiation rates are reported by Omada in Kbit/s.
- WAN RX/TX rate metrics are reported by the controller as KB/s.
- DPI insight traffic metrics are gauges for the configured query window, not monotonic counters.
- Labels carry Omada identity and topology metadata. High-cardinality labels are expected for per-client and per-port metrics.

## Exporter Self-Metrics

OmadaBridge wraps each Omada collector with a small layer of scrape-health metrics:

| Metric | Meaning | Labels |
| --- | --- | --- |
| `omada_collector_last_scrape_completed` | `1` when the collector returned without panicking, `0` when a panic was recovered. API errors handled inside individual collectors are still logged by that collector. | collector |
| `omada_collector_last_scrape_duration_seconds` | Duration of the most recent collector scrape. | collector |
| `omada_collector_panics_total` | Total recovered panics for the collector since process start. | collector |

## Metric Reference

| Metric | Meaning | Main labels |
| --- | --- | --- |
| `omada_controller_uptime_seconds` | Controller uptime in seconds. | controller device labels, site, site_id |
| `omada_controller_storage_used_bytes` | Controller storage used. | storage_name, controller device labels, site, site_id |
| `omada_controller_storage_available_bytes` | Controller free storage available. | storage_name, controller device labels, site, site_id |
| `omada_controller_storage_total_bytes` | Controller total storage capacity. | storage_name, controller device labels, site, site_id |
| `omada_controller_upgrade_available` | Firmware upgrade availability by channel. | upgrade_channel, latest_version, controller device labels, site, site_id |
| `omada_site_alert_num` | Number of site alerts. | obscured, site, site_id |
| `omada_device_uptime_seconds` | Device uptime in seconds. | device labels, site, site_id |
| `omada_device_cpu_percentage` | Device CPU usage percentage. | device labels, site, site_id |
| `omada_device_mem_percentage` | Device memory usage percentage. | device labels, site, site_id |
| `omada_device_need_upgrade` | Device firmware upgrade required. | device labels, site, site_id |
| `omada_device_download` | Device download traffic. | device labels, site, site_id |
| `omada_device_upload` | Device upload traffic. | device labels, site, site_id |
| `omada_device_temp` | Device temperature. | device labels, site, site_id |
| `omada_device_tx_rate` | Device TX rate. | device labels, site, site_id |
| `omada_device_rx_rate` | Device RX rate. | device labels, site, site_id |
| `omada_device_poe_remain_watts` | Remaining device PoE power in watts. | device labels, PoE labels, site, site_id |
| `omada_device_2g_tx_util` | 2.4 GHz TX utilization. | AP radio labels, device labels, site, site_id |
| `omada_device_2g_rx_util` | 2.4 GHz RX utilization. | AP radio labels, device labels, site, site_id |
| `omada_device_5g_tx_util` | 5 GHz TX utilization. | AP radio labels, device labels, site, site_id |
| `omada_device_5g_rx_util` | 5 GHz RX utilization. | AP radio labels, device labels, site, site_id |
| `omada_device_5g2_tx_util` | 5 GHz-2 TX utilization. | AP radio labels, device labels, site, site_id |
| `omada_device_5g2_rx_util` | 5 GHz-2 RX utilization. | AP radio labels, device labels, site, site_id |
| `omada_device_6g_tx_util` | 6 GHz TX utilization. | AP radio labels, device labels, site, site_id |
| `omada_device_6g_rx_util` | 6 GHz RX utilization. | AP radio labels, device labels, site, site_id |
| `omada_port_link_status` | Port link status. | device labels, port, max_speed, name, type, operation, link_status, link_speed, poe, port_activity_label |
| `omada_port_power_watts` | Current PoE usage for a port. | device labels, port labels |
| `omada_port_link_speed_mbps` | Port link speed in Mbit/s. | device labels, port labels |
| `omada_port_link_rx` | Bytes received on a port. | device labels, port labels |
| `omada_port_link_tx` | Bytes transmitted on a port. | device labels, port labels |
| `omada_lag_link_status` | LAG link status. | device labels, lag_id, lag_type, name, link_status, link_speed, lag_ports |
| `omada_lag_link_speed_mbps` | LAG link speed in Mbit/s. | device labels, LAG labels |
| `omada_lag_link_rx` | Bytes received on a LAG. | device labels, LAG labels |
| `omada_lag_link_tx` | Bytes transmitted on a LAG. | device labels, LAG labels |
| `omada_wan_status` | WAN connected/disconnected status. | device labels, port, name, desc, type, ip, proto |
| `omada_wan_internet_state` | WAN internet connected/disconnected state. | device labels, WAN labels |
| `omada_wan_link_speed_mbps` | WAN link speed in Mbit/s. | device labels, WAN labels |
| `omada_wan_rx_rate` | WAN RX rate reported by Omada. | device labels, WAN labels |
| `omada_wan_tx_rate` | WAN TX rate reported by Omada. | device labels, WAN labels |
| `omada_wan_latency` | WAN latency in milliseconds. | device labels, WAN labels |
| `omada_client_connected_total` | Total connected clients. | site, site_id, connection_mode, wifi_mode |
| `omada_client_download_activity_bytes` | Current client download activity. | client identity, topology, SSID, site, site_id |
| `omada_client_upload_activity_bytes` | Current client upload activity. | client identity, topology, SSID, site, site_id |
| `omada_client_signal_pct` | Wireless client signal quality percentage. | client identity, topology, SSID, site, site_id |
| `omada_client_snr_dbm` | Wireless client signal-to-noise ratio. | client identity, topology, SSID, site, site_id |
| `omada_client_rssi_dbm` | Wireless client RSSI. | client identity, topology, SSID, site, site_id |
| `omada_client_traffic_down_bytes` | Total bytes received by a client. | client identity, topology, SSID, site, site_id |
| `omada_client_traffic_up_bytes` | Total bytes sent by a client. | client identity, topology, SSID, site, site_id |
| `omada_client_tx_rate` | Client TX negotiation rate in Kbit/s. | client identity, topology, SSID, site, site_id |
| `omada_client_rx_rate` | Client RX negotiation rate in Kbit/s. | client identity, topology, SSID, site, site_id |
| `omada_vpn_status` | VPN enabled/disabled status. | vpn_id, name, purpose, vpn_mode, vpn_type, remote_ip, site, site_id |
| `omada_vpn_uptime` | VPN tunnel uptime. | name, interface_name, vpn_mode, vpn_type, local_ip, remote_ip, site, site_id |
| `omada_vpn_down_packets` | VPN downlink packets. | VPN tunnel labels |
| `omada_vpn_down_bytes` | VPN downlink bytes. | VPN tunnel labels |
| `omada_vpn_up_packets` | VPN uplink packets. | VPN tunnel labels |
| `omada_vpn_up_bytes` | VPN uplink bytes. | VPN tunnel labels |
| `omada_site_to_site_vpn_down_bytes` | Site-to-site VPN aggregate downlink bytes. | vpn_id, name, vpn_type, site_vpn_type, site, site_id |
| `omada_site_to_site_vpn_up_bytes` | Site-to-site VPN aggregate uplink bytes. | vpn_id, name, vpn_type, site_vpn_type, site, site_id |
| `omada_site_to_site_vpn_total_peers` | Site-to-site VPN peer count. | vpn_id, tunnel_id, name, vpn_type, direction, local/remote IP labels, site, site_id |
| `omada_site_to_site_vpn_peer_down_bytes` | Site-to-site VPN peer downlink bytes. | vpn_id, name, peer_id, peer_name, vpn_type, local_ip, remote_ip, port, site, site_id |
| `omada_site_to_site_vpn_peer_up_bytes` | Site-to-site VPN peer uplink bytes. | site-to-site peer labels |
| `omada_site_to_site_vpn_peer_login_timestamp` | Unix login timestamp for a site-to-site VPN peer. | site-to-site peer labels |
| `omada_isp_status` | ISP enabled/disabled status. | gateway_name, gateway_mac, gateway_status, name, port, status, ip, load_balance, max_bandwidth, download_speed_set, site, site_id |
| `omada_isp_download_speed` | Configured ISP download speed. | ISP labels |
| `omada_isp_upload_speed` | Configured ISP upload speed. | ISP labels |
| `omada_dpi_scrape_window_seconds` | Configured DPI insight query window. | site, site_id |
| `omada_dpi_total_traffic_bytes` | Total DPI-classified traffic for the configured window. | site, site_id |
| `omada_dpi_category_traffic_bytes` | DPI-classified traffic by category for the configured window. | family_id, family_name, site, site_id |
| `omada_dpi_application_traffic_bytes` | DPI-classified traffic by application for the configured window. | family_id, family_name, application_id, application_name, site, site_id |

## Label Sets

Common device labels:

```text
device_mac, device_type, device_subtype, device_model, device_show_model,
device_version, device_version_upgrade, device_hw_version,
device_firmware_version, device_ip, device_name, device_status,
site, site_id
```

Common client labels:

```text
mac, ip, vlan_id, connect_type, name, system_name, host_name,
device_type, device_category, vendor, connect_dev_type,
gateway_mac, gateway_name, switch_mac, switch_name, port, lag_id,
wireless, ap_mac, ap_name, wifi_mode, ssid, site, site_id
```

## Query Examples

Connected clients:

```promql
sum(omada_client_connected_total)
```

Offline Omada devices:

```promql
omada_device_uptime_seconds unless omada_device_uptime_seconds > 0
```

Ports with active links:

```promql
sum by (device_name) (omada_port_link_status == 1)
```

Devices needing firmware updates:

```promql
omada_device_need_upgrade == 1
```

WAN latency:

```promql
omada_wan_latency
```
