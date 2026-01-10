# omada_exporter

![docker-publish](https://github.com/RCooLeR/omada_exporter/actions/workflows/release.yml/badge.svg)

<p align="center" style="text-align: center">
    <img src="./docs/images/logo-dark-mode.svg#gh-dark-mode-only" width="70%"><br/>
    <img src="./docs/images/logo-light-mode.svg#gh-light-mode-only" width="70%"><br/>
    Prometheus Exporter for TP-Link Omada Controller SDN. <br/>
</p>

>“RCooLeR/omada_exporter is a maintained fork of charlie-haley/omada_exporter with additional metrics and Open API support.”

### 📈 Dashboard

There are [default dashboards in this repo](docs/dashboards/), which is a good starting point for visualizing your metrics.

You can also find it on [Grafana.com](https://grafana.com/grafana/dashboards/16343).

<p align="center" style="text-align: center">
    <img src="./docs/images/simple-omada-dashboard.png" width="70%"><br/>
</p>
<p align="center" style="text-align: center">
    <img src="./docs/images/dashboard.png" width="70%"><br/>
</p>

## Installation

### Omada Authentication Setup

- OpenAPI Client – Created via: `Settings -> Platform Integration`.
  Assign admin role for full API access.
- Service User – Create under: `Account section` at `Global level`.
  Assign viewer role for read-only access.

### 🚀 Docker Run Example

```bash
docker run -d \
    -p 9202:9202 \
    -e OMADA_HOST='https://192.168.1.20' \
    -e OMADA_USER='exporter' \
    -e OMADA_PASS='mypassword' \
    -e OMADA_SITE='Default' \
    -e OMADA_CLIENT_ID='' \
    -e OMADA_SECRET_ID='' \
    chhaley/omada_exporter
```

### 📦 Docker Compose Example

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
      OMADA_PASS: "mypassword"
      OMADA_SITE: "Default"
      OMADA_CLIENT_ID: ""
      OMADA_SECRET_ID: ""
    restart: unless-stopped
```

### 🖥️ Command Line

[You can download the latest binary release here.](https://github.com/RCooLeR/omada_exporter/releases/latest)

```
NAME:
   omada-exporter - Prometheus Exporter for TP-Link Omada Controller SDN.

USAGE:
   main [global options] command [command options] [arguments...]

VERSION:
   development

AUTHOR:
   Charlie Haley <charlie-haley@users.noreply.github.com>
   Roman Derevianko <RCooLeR@users.noreply.github.com>

COMMANDS:
   version, v  prints the current version.
   help, h     Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --host value                 The hostname of the Omada Controller, including protocol. [$OMADA_HOST]
   --username value             Username of the Omada user you'd like to use to fetch metrics. [$OMADA_USER]
   --password value             Password for your Omada user. [$OMADA_PASS]
   --port value                 Port on which to expose the Prometheus metrics. (default: "9202") [$OMADA_PORT]
   --site value                 Omada site to scrape metrics from. (default: "Default") [$OMADA_SITE]
   --client-id value            Optional Client ID for Open API authentication. [$OMADA_CLIENT_ID]
   --secret-id value            Optional Secret ID for Open API authentication. [$OMADA_SECRET_ID]
   --log-level value            Application log level. (default: "error") [$LOG_LEVEL]
   --timeout value              Timeout when making requests to the Omada Controller. (default: 15) [$OMADA_REQUEST_TIMEOUT]
   --insecure                   Whether to skip verifying the SSL certificate on the controller. (default: false) [$OMADA_INSECURE]
   --disable-go-collector       Disable Go collector metrics. (default: true) [$OMADA_DISABLE_GO_COLLECTOR]
   --disable-process-collector  Disable process collector metrics. (default: true) [$OMADA_DISABLE_PROCESS_COLLECTOR]
   --help, -h                   show help (default: false)
   --version, -v                print the version (default: false)
```

## ⚙️ Configuration


### 📡 Prometheus Scrape Job Example

Add the following job to your `prometheus.yml` configuration:

```yaml
  - job_name: 'Omada'
    scrape_interval: 30s
    scrape_timeout: 30s
    static_configs:
      - targets: ['omada_exporter:9202']
```

> Make sure `omada_exporter` resolves to your container or host running `omada_exporter`.

### Environment Variables

| Variable                        | Purpose                                                                           |
|---------------------------------|-----------------------------------------------------------------------------------|
| OMADA_HOST                      | The hostname of the Omada Controller, including protocol.                         |
| OMADA_USER                      | Username of the Omada user you'd like to use to fetch metrics.                    |
| OMADA_PASS                      | Password for your Omada user.                                                     |
| OMADA_SITE                      | Site you'd like to get metrics from. (default: "Default")                         |
| OMADA_PORT                      | Port on which to expose the Prometheus metrics. (default: 9202)                   |
| OMADA_INSECURE                  | Whether to skip verifying the SSL certificate on the controller. (default: false) |
| OMADA_REQUEST_TIMEOUT           | Timeout when making requests to the Omada Controller. (default: 15)               |
| OMADA_DISABLE_GO_COLLECTOR      | Disable Go collector metrics. (default: true)                                     |
| OMADA_DISABLE_PROCESS_COLLECTOR | Disable process collector metrics. (default: true)                                |
| LOG_LEVEL                       | Application log level. (default: "error")                                         |
| OMADA_CLIENT_ID                 | Optional Client ID for Open API authentication (WAN & VPN metrics)                |
| OMADA_SECRET_ID                 | Optional Secret ID for Open API authentication (WAN & VPN metrics)                |

## 📊 Metrics
| Name                                     | Description                                                                                   | Labels                                                                                                                                                                                                                                                                                                                                                                   | Value Unit   |
|:-----------------------------------------|:----------------------------------------------------------------------------------------------|:-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|:-------------|
| omada_client_connected_total             | Total number of connected clients.                                                            | connection_mode,site,site_id,wifi_mode                                                                                                                                                                                                                                                                                                                                   | count        |
| omada_client_download_activity_bytes     | The current download activity for the client in bytes.                                        | ApMac,ApName,ConnectDevType,DeviceCategory,DeviceType,GatewayMac,GatewayName,HostName,Ip,LagId,Mac,Name,Port,Ssid,SwitchMac,SwitchName,SystemName,Vendor,VlanId,Wireless,connectType,wifiMode                                                                                                                                                                            | byte         |
| omada_client_rssi_dbm                    | The RSSI for the wireless client in dBm.                                                      | ApMac,ApName,ConnectDevType,DeviceCategory,DeviceType,GatewayMac,GatewayName,HostName,Ip,LagId,Mac,Name,Port,Ssid,SwitchMac,SwitchName,SystemName,Vendor,VlanId,Wireless,connectType,wifiMode                                                                                                                                                                            | value        |
| omada_client_rx_rate                     | RX rate of wireless client.                                                                   | ApMac,ApName,ConnectDevType,DeviceCategory,DeviceType,GatewayMac,GatewayName,HostName,Ip,LagId,Mac,Name,Port,Ssid,SwitchMac,SwitchName,SystemName,Vendor,VlanId,Wireless,connectType,wifiMode                                                                                                                                                                            | bps          |
| omada_client_signal_pct                  | The signal quality for the wireless client in percent.                                        | ApMac,ApName,ConnectDevType,DeviceCategory,DeviceType,GatewayMac,GatewayName,HostName,Ip,LagId,Mac,Name,Port,Ssid,SwitchMac,SwitchName,SystemName,Vendor,VlanId,Wireless,connectType,wifiMode                                                                                                                                                                            | percent      |
| omada_client_snr_dbm                     | The signal to noise ratio for the wireless client in dBm.                                     | ApMac,ApName,ConnectDevType,DeviceCategory,DeviceType,GatewayMac,GatewayName,HostName,Ip,LagId,Mac,Name,Port,Ssid,SwitchMac,SwitchName,SystemName,Vendor,VlanId,Wireless,connectType,wifiMode                                                                                                                                                                            | value        |
| omada_client_traffic_down_bytes          | Total bytes received by wireless client.                                                      | ApMac,ApName,ConnectDevType,DeviceCategory,DeviceType,GatewayMac,GatewayName,HostName,Ip,LagId,Mac,Name,Port,Ssid,SwitchMac,SwitchName,SystemName,Vendor,VlanId,Wireless,connectType,wifiMode                                                                                                                                                                            | byte         |
| omada_client_traffic_up_bytes            | Total bytes sent by wireless client.                                                          | ApMac,ApName,ConnectDevType,DeviceCategory,DeviceType,GatewayMac,GatewayName,HostName,Ip,LagId,Mac,Name,Port,Ssid,SwitchMac,SwitchName,SystemName,Vendor,VlanId,Wireless,connectType,wifiMode                                                                                                                                                                            | byte         |
| omada_client_tx_rate                     | TX rate of wireless client.                                                                   | ApMac,ApName,ConnectDevType,DeviceCategory,DeviceType,GatewayMac,GatewayName,HostName,Ip,LagId,Mac,Name,Port,Ssid,SwitchMac,SwitchName,SystemName,Vendor,VlanId,Wireless,connectType,wifiMode                                                                                                                                                                            | bps          |
| omada_client_upload_activity_bytes       | The current upload activity for the client in bytes.                                          | ApMac,ApName,ConnectDevType,DeviceCategory,DeviceType,GatewayMac,GatewayName,HostName,Ip,LagId,Mac,Name,Port,Ssid,SwitchMac,SwitchName,SystemName,Vendor,VlanId,Wireless,connectType,wifiMode                                                                                                                                                                            | byte         |
| omada_controller_storage_available_bytes | Total storage available for the controller.                                                   | controller_name,controller_version,firmware_version,mac,model,site,site_id,storage_name                                                                                                                                                                                                                                                                                  | count        |
| omada_controller_storage_used_bytes      | Storage used on the controller.                                                               | controller_name,controller_version,firmware_version,mac,model,site,site_id,storage_name                                                                                                                                                                                                                                                                                  | value        |
| omada_controller_uptime_seconds          | Uptime of the controller.                                                                     | controller_name,controller_version,firmware_version,mac,model,site,site_id                                                                                                                                                                                                                                                                                               | seconds      |
| omada_device_2g_rx_util                  | The tx rate of the device on 2.4Ghz.                                                          | any_poe_enable,firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version,wireless_linked,wlan_group,wp2g_band_width,wp2g_mode,wp2g_tx_max_rate,wp5g1_band_width,wp5g1_mode,wp5g1_tx_max_rate,wp5g2_band_width,wp5g2_mode,wp5g2_tx_max_rate,wp5g_band_width,wp5g_mode,wp5g_tx_max_rate,wp6g_band_width,wp6g_mode,wp6g_tx_max_rate | bps          |
| omada_device_2g_tx_util                  | The tx rate of the device on 2.4Ghz.                                                          | any_poe_enable,firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version,wireless_linked,wlan_group,wp2g_band_width,wp2g_mode,wp2g_tx_max_rate,wp5g1_band_width,wp5g1_mode,wp5g1_tx_max_rate,wp5g2_band_width,wp5g2_mode,wp5g2_tx_max_rate,wp5g_band_width,wp5g_mode,wp5g_tx_max_rate,wp6g_band_width,wp6g_mode,wp6g_tx_max_rate | bps          |
| omada_device_5g_rx_util                  | The tx rate of the device on 5Ghz.                                                            | any_poe_enable,firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version,wireless_linked,wlan_group,wp2g_band_width,wp2g_mode,wp2g_tx_max_rate,wp5g1_band_width,wp5g1_mode,wp5g1_tx_max_rate,wp5g2_band_width,wp5g2_mode,wp5g2_tx_max_rate,wp5g_band_width,wp5g_mode,wp5g_tx_max_rate,wp6g_band_width,wp6g_mode,wp6g_tx_max_rate | bps          |
| omada_device_5g_tx_util                  | The tx rate of the device on 5Ghz.                                                            | any_poe_enable,firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version,wireless_linked,wlan_group,wp2g_band_width,wp2g_mode,wp2g_tx_max_rate,wp5g1_band_width,wp5g1_mode,wp5g1_tx_max_rate,wp5g2_band_width,wp5g2_mode,wp5g2_tx_max_rate,wp5g_band_width,wp5g_mode,wp5g_tx_max_rate,wp6g_band_width,wp6g_mode,wp6g_tx_max_rate | bps          |
| omada_device_5g1_rx_util                 | The tx rate of the device on 5Ghz (5180–5320 MHz).                                            | any_poe_enable,firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version,wireless_linked,wlan_group,wp2g_band_width,wp2g_mode,wp2g_tx_max_rate,wp5g1_band_width,wp5g1_mode,wp5g1_tx_max_rate,wp5g2_band_width,wp5g2_mode,wp5g2_tx_max_rate,wp5g_band_width,wp5g_mode,wp5g_tx_max_rate,wp6g_band_width,wp6g_mode,wp6g_tx_max_rate | bps          |
| omada_device_5g1_tx_util                 | The tx rate of the device on 5Ghz (5180–5320 MHz).                                            | any_poe_enable,firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version,wireless_linked,wlan_group,wp2g_band_width,wp2g_mode,wp2g_tx_max_rate,wp5g1_band_width,wp5g1_mode,wp5g1_tx_max_rate,wp5g2_band_width,wp5g2_mode,wp5g2_tx_max_rate,wp5g_band_width,wp5g_mode,wp5g_tx_max_rate,wp6g_band_width,wp6g_mode,wp6g_tx_max_rate | bps          |
| omada_device_5g2_rx_util                 | The tx rate of the device on 5Ghz (5500–5700 MHz).                                            | any_poe_enable,firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version,wireless_linked,wlan_group,wp2g_band_width,wp2g_mode,wp2g_tx_max_rate,wp5g1_band_width,wp5g1_mode,wp5g1_tx_max_rate,wp5g2_band_width,wp5g2_mode,wp5g2_tx_max_rate,wp5g_band_width,wp5g_mode,wp5g_tx_max_rate,wp6g_band_width,wp6g_mode,wp6g_tx_max_rate | bps          |
| omada_device_5g2_tx_util                 | The tx rate of the device on 5Ghz (5500–5700 MHz).                                            | any_poe_enable,firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version,wireless_linked,wlan_group,wp2g_band_width,wp2g_mode,wp2g_tx_max_rate,wp5g1_band_width,wp5g1_mode,wp5g1_tx_max_rate,wp5g2_band_width,wp5g2_mode,wp5g2_tx_max_rate,wp5g_band_width,wp5g_mode,wp5g_tx_max_rate,wp6g_band_width,wp6g_mode,wp6g_tx_max_rate | bps          |
| omada_device_6g_rx_util                  | The tx rate of the device on 6Ghz.                                                            | any_poe_enable,firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version,wireless_linked,wlan_group,wp2g_band_width,wp2g_mode,wp2g_tx_max_rate,wp5g1_band_width,wp5g1_mode,wp5g1_tx_max_rate,wp5g2_band_width,wp5g2_mode,wp5g2_tx_max_rate,wp5g_band_width,wp5g_mode,wp5g_tx_max_rate,wp6g_band_width,wp6g_mode,wp6g_tx_max_rate | bps          |
| omada_device_6g_tx_util                  | The tx rate of the device on 6Ghz..                                                           | any_poe_enable,firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version,wireless_linked,wlan_group,wp2g_band_width,wp2g_mode,wp2g_tx_max_rate,wp5g1_band_width,wp5g1_mode,wp5g1_tx_max_rate,wp5g2_band_width,wp5g2_mode,wp5g2_tx_max_rate,wp5g_band_width,wp5g_mode,wp5g_tx_max_rate,wp6g_band_width,wp6g_mode,wp6g_tx_max_rate | bps          |
| omada_device_cpu_percentage              | Percentage of device CPU used.                                                                | firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version                                                                                                                                                                                                                                                                        | percent      |
| omada_device_download                    | Device download traffic.                                                                      | firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version                                                                                                                                                                                                                                                                        | value        |
| omada_device_mem_percentage              | Percentage of device Memory used.                                                             | firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version                                                                                                                                                                                                                                                                        | percent      |
| omada_device_need_upgrade                | A boolean on whether the device needs an upgrade.                                             | firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version                                                                                                                                                                                                                                                                        | value        |
| omada_device_poe_remain_watts            | The remaining amount of PoE power for the device in watts.                                    | firmware_version,hw_version,ip,mac,model,name,poe_support,port_number,show_model,site,site_id,status,subtype,total_power,type,version                                                                                                                                                                                                                                    | watt         |
| omada_device_rx_rate                     | The rx rate of the device.                                                                    | any_poe_enable,firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version,wireless_linked,wlan_group,wp2g_band_width,wp2g_mode,wp2g_tx_max_rate,wp5g1_band_width,wp5g1_mode,wp5g1_tx_max_rate,wp5g2_band_width,wp5g2_mode,wp5g2_tx_max_rate,wp5g_band_width,wp5g_mode,wp5g_tx_max_rate,wp6g_band_width,wp6g_mode,wp6g_tx_max_rate | bps          |
| omada_device_tx_rate                     | The tx rate of the device.                                                                    | any_poe_enable,firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version,wireless_linked,wlan_group,wp2g_band_width,wp2g_mode,wp2g_tx_max_rate,wp5g1_band_width,wp5g1_mode,wp5g1_tx_max_rate,wp5g2_band_width,wp5g2_mode,wp5g2_tx_max_rate,wp5g_band_width,wp5g_mode,wp5g_tx_max_rate,wp6g_band_width,wp6g_mode,wp6g_tx_max_rate | bps          |
| omada_device_upload                      | Device upload traffic.                                                                        | firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version                                                                                                                                                                                                                                                                        | value        |
| omada_device_uptime_seconds              | Uptime of the device.                                                                         | firmware_version,hw_version,ip,mac,model,name,show_model,site,site_id,status,subtype,type,version                                                                                                                                                                                                                                                                        | seconds      |
| omada_isp_download_speed                 | The download speed of the ISP                                                                 | gateway_mac,gateway_name,gateway_status,ip,load_balance,max_bandwidth,name,port,site,site_id,status                                                                                                                                                                                                                                                                      | value        |
| omada_isp_status                         | The current status of the ISP enabled/disabled                                                | gateway_mac,gateway_name,gateway_status,ip,load_balance,max_bandwidth,name,port,site,site_id,status                                                                                                                                                                                                                                                                      | value        |
| omada_isp_upload_speed                   | The upload speed of the ISP                                                                   | gateway_mac,gateway_name,gateway_status,ip,load_balance,max_bandwidth,name,port,site,site_id,status                                                                                                                                                                                                                                                                      | value        |
| omada_lag_link_rx                        | Bytes recieved on a lag.                                                                      | 19",lag_id,lag_ports,lag_type,link_speed,link_status,name,switch_firmware_version,switch_hw_version,switch_ip,switch_mac,switch_model,switch_name,switch_poe_support,switch_port_number,switch_show_model,switch_site,switch_site_id,switch_status,switch_subtype,switch_total_power,switch_type,switch_version                                                          | byte         |
| omada_lag_link_speed_mbps                | Lag link speed in mbps. This is the capability of the connection, not the active throughput.  | 19",lag_id,lag_ports,lag_type,link_speed,link_status,name,switch_firmware_version,switch_hw_version,switch_ip,switch_mac,switch_model,switch_name,switch_poe_support,switch_port_number,switch_show_model,switch_site,switch_site_id,switch_status,switch_subtype,switch_total_power,switch_type,switch_version                                                          | value        |
| omada_lag_link_status                    | A boolean representing the link status of the lag.                                            | 19",lag_id,lag_ports,lag_type,link_speed,link_status,name,switch_firmware_version,switch_hw_version,switch_ip,switch_mac,switch_model,switch_name,switch_poe_support,switch_port_number,switch_show_model,switch_site,switch_site_id,switch_status,switch_subtype,switch_total_power,switch_type,switch_version                                                          | value        |
| omada_lag_link_tx                        | Bytes transmitted on a lag.                                                                   | 19",lag_id,lag_ports,lag_type,link_speed,link_status,name,switch_firmware_version,switch_hw_version,switch_ip,switch_mac,switch_model,switch_name,switch_poe_support,switch_port_number,switch_show_model,switch_site,switch_site_id,switch_status,switch_subtype,switch_total_power,switch_type,switch_version                                                          | byte         |
| omada_port_link_rx                       | Bytes recieved on a port.                                                                     | link_speed,link_status,max_speed,name,operation,poe,port,switch_firmware_version,switch_hw_version,switch_ip,switch_mac,switch_model,switch_name,switch_poe_support,switch_port_number,switch_show_model,switch_site,switch_site_id,switch_status,switch_subtype,switch_total_power,switch_type,switch_version,type                                                      | byte         |
| omada_port_link_speed_mbps               | Port link speed in mbps. This is the capability of the connection, not the active throughput. | link_speed,link_status,max_speed,name,operation,poe,port,switch_firmware_version,switch_hw_version,switch_ip,switch_mac,switch_model,switch_name,switch_poe_support,switch_port_number,switch_show_model,switch_site,switch_site_id,switch_status,switch_subtype,switch_total_power,switch_type,switch_version,type                                                      | value        |
| omada_port_link_status                   | A boolean representing the link status of the port.                                           | link_speed,link_status,max_speed,name,operation,poe,port,switch_firmware_version,switch_hw_version,switch_ip,switch_mac,switch_model,switch_name,switch_poe_support,switch_port_number,switch_show_model,switch_site,switch_site_id,switch_status,switch_subtype,switch_total_power,switch_type,switch_version,type                                                      | value        |
| omada_port_link_tx                       | Bytes transmitted on a port.                                                                  | link_speed,link_status,max_speed,name,operation,poe,port,switch_firmware_version,switch_hw_version,switch_ip,switch_mac,switch_model,switch_name,switch_poe_support,switch_port_number,switch_show_model,switch_site,switch_site_id,switch_status,switch_subtype,switch_total_power,switch_type,switch_version,type                                                      | byte         |
| omada_port_power_watts                   | The current PoE usage of the port in watts.                                                   | link_speed,link_status,max_speed,name,operation,poe,port,switch_firmware_version,switch_hw_version,switch_ip,switch_mac,switch_model,switch_name,switch_poe_support,switch_port_number,switch_show_model,switch_site,switch_site_id,switch_status,switch_subtype,switch_total_power,switch_type,switch_version,type                                                      | watt         |
| omada_vpn_down_bytes                     | VPN downlink traffic in bytes                                                                 | interface_name,local_ip,name,remote_ip,site,site_id,vpn_mode,vpn_type                                                                                                                                                                                                                                                                                                    | byte         |
| omada_vpn_status                         | The current status of the VPN enabled/disabled                                                | name,purpose,remote_ip,site,site_id,vpn_id,vpn_mode,vpn_type                                                                                                                                                                                                                                                                                                             | value        |
| omada_vpn_up_bytes                       | VPN uplink traffic in bytes                                                                   | interface_name,local_ip,name,remote_ip,site,site_id,vpn_mode,vpn_type                                                                                                                                                                                                                                                                                                    | byte         |
| omada_vpn_uptime                         | The current uptime of the VPN                                                                 | interface_name,local_ip,name,remote_ip,site,site_id,vpn_mode,vpn_type                                                                                                                                                                                                                                                                                                    | seconds      |
| omada_wan_internet_state                 | The current status of the WAN internet state connected/disconnected                           | desc,gateway_firmware_version,gateway_hw_version,gateway_ip,gateway_mac,gateway_model,gateway_name,gateway_show_model,gateway_site,gateway_site_id,gateway_status,gateway_subtype,gateway_type,gateway_version,ip,name,port,proto,type                                                                                                                                   | value        |
| omada_wan_latency                        | Wan latency (ms)                                                                              | desc,gateway_firmware_version,gateway_hw_version,gateway_ip,gateway_mac,gateway_model,gateway_name,gateway_show_model,gateway_site,gateway_site_id,gateway_status,gateway_subtype,gateway_type,gateway_version,ip,name,port,proto,type                                                                                                                                   | value        |
| omada_wan_link_speed_mbps                | Wan link speed in mbps. This is the capability of the connection, not the active throughput.  | desc,gateway_firmware_version,gateway_hw_version,gateway_ip,gateway_mac,gateway_model,gateway_name,gateway_show_model,gateway_site,gateway_site_id,gateway_status,gateway_subtype,gateway_type,gateway_version,ip,name,port,proto,type                                                                                                                                   | value        |
| omada_wan_rx_rate                        | Wan RX rate (KB/s)                                                                            | desc,gateway_firmware_version,gateway_hw_version,gateway_ip,gateway_mac,gateway_model,gateway_name,gateway_show_model,gateway_site,gateway_site_id,gateway_status,gateway_subtype,gateway_type,gateway_version,ip,name,port,proto,type                                                                                                                                   | bps          |
| omada_wan_status                         | The current status of the WAN connected/disconnected                                          | desc,gateway_firmware_version,gateway_hw_version,gateway_ip,gateway_mac,gateway_model,gateway_name,gateway_show_model,gateway_site,gateway_site_id,gateway_status,gateway_subtype,gateway_type,gateway_version,ip,name,port,proto,type                                                                                                                                   | value        |
| omada_wan_tx_rate                        | Wan TX rate (KB/s)                                                                            | desc,gateway_firmware_version,gateway_hw_version,gateway_ip,gateway_mac,gateway_model,gateway_name,gateway_show_model,gateway_site,gateway_site_id,gateway_status,gateway_subtype,gateway_type,gateway_version,ip,name,port,proto,type                                                                                                                                   | bps          |

### PS

Last tested on [OC200](https://www.omadanetworks.com/us/business-networking/omada-controller-hardware/oc200/), firmware 6.0.0.36 (ER8411,SG3428X-M2,SG3210XHP-M2,SG2210MP,EAP772-Outdoor,EAP650-Outdoor,EAP225-Outdoor,EAP235-Wall)

OpenApi docs: [https://use1-omada-northbound.tplinkcloud.com/doc.html](https://use1-omada-northbound.tplinkcloud.com/doc.html)
WebApi: no docs. Login to your controller and use Chrome debug tools  🤦