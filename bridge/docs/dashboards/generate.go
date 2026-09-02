//go:build ignore

// Command generate writes the distributable Grafana dashboards in this
// directory. Keep dashboard behavior here so all generated JSON files share
// the same portable datasource, variables, PromQL conventions, and styling.
package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	datasourceUID    = "$datasource"
	jobVariable      = "$job"
	instanceVariable = "$instance"
	siteVariable     = "$" + "{Site:regex}"
	deviceVariable   = "$" + "{Device:regex}"
	rateInterval     = "$__rate_interval"
	rateMinStep      = "1m"
	noDataColor      = "#808080"
	deviceDetailsURL = "/d/omada-device-details/omada-device-details?${datasource:queryparam}&${job:queryparam}&${instance:queryparam}&${Site:queryparam}&var-Device=${__field.labels.device_mac}&from=${__from}&to=${__to}"
)

type object = map[string]any

type querySpec struct {
	expr     string
	legend   string
	interval string
	instant  bool
}

type dashboardBuilder struct {
	nextID int
	panels []any
}

func main() {
	outputs := []struct {
		name      string
		dashboard object
	}{
		{"dashboard.json", fullDashboard()},
		{"omada-device-details.json", deviceDetailsDashboard()},
		{"simple-omada-dashboard.json", simpleDashboard()},
	}

	for _, output := range outputs {
		if err := writeJSON(output.name, output.dashboard); err != nil {
			fmt.Fprintf(os.Stderr, "generate %s: %v\n", output.name, err)
			os.Exit(1)
		}
	}
}

func writeJSON(name string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(name, data, 0o644)
}

func fullDashboard() object {
	b := &dashboardBuilder{nextID: 1}
	target := "job=~\"" + jobVariable + "\",instance=~\"" + instanceVariable + "\""
	site := target + ",site=~\"" + siteVariable + "\""
	device := site + ",device_mac=~\"" + deviceVariable + "\""

	b.row("Overview", 0)
	b.stat("Devices online", "Devices whose status starts with Connected, including wireless-backhaul and migrating variants.", 0, 1, 4, 4, "short", object{"min": 0}, nil, onlineThresholds(),
		instant("count(omada_device_uptime_seconds{"+device+",device_status=~\"^Connected.*$\"}) or vector(0)", "Online"))
	b.stat("Devices not online", "Devices in disconnected, provisioning, upgrading, pending, isolated, or other non-connected states.", 4, 1, 4, 4, "short", object{"min": 0}, nil, problemThresholds(),
		instant("count(omada_device_uptime_seconds{"+device+",device_status!~\"^Connected.*$\"}) or vector(0)", "Not online"))
	b.stat("Connected clients", "Current wired and wireless clients reported by the selected sites.", 8, 1, 4, 4, "short", object{"min": 0}, nil, healthyThresholds(),
		instant("sum(omada_client_connected_total{"+site+"}) or vector(0)", "Clients"))
	b.stat("Site alerts", "Current alert count across the selected sites.", 12, 1, 4, 4, "short", object{"min": 0}, nil, problemThresholds(),
		instant("sum(omada_site_alert_num{"+site+"}) or vector(0)", "Alerts"))
	b.stat("Firmware upgrades", "Device and controller firmware upgrades currently advertised by Omada.", 16, 1, 4, 4, "short", object{"min": 0}, nil, warningThresholds(),
		instant("(sum(omada_device_need_upgrade{"+device+"}) or vector(0)) + (sum(omada_controller_upgrade_available{"+site+"}) or vector(0))", "Upgrades"))
	b.stat("Collector failures", "Collectors whose latest scrape did not complete. API errors handled inside a collector are logged separately.", 20, 1, 4, 4, "short", object{"min": 0}, nil, problemThresholds(),
		instant("sum(omada_collector_last_scrape_completed{"+target+"} == 0) or vector(0)", "Failed"))

	b.stat("Controller uptime", "Longest controller uptime reported for the selected sites.", 0, 5, 4, 4, "s", object{"min": 0}, nil, healthyThresholds(),
		instant("max(omada_controller_uptime_seconds{"+site+"}) or vector(0)", "Uptime"))
	b.stat("Controller storage used", "Used controller storage divided by total controller storage.", 4, 5, 4, 4, "percent", object{"min": 0, "max": 100}, nil, utilizationThresholds(),
		instant("100 * sum(omada_controller_storage_used_bytes{"+site+"}) / clamp_min(sum(omada_controller_storage_total_bytes{"+site+"}), 1)", "Used"))
	b.stat("PoE draw", "Current PoE draw summed across every selected device and port.", 8, 5, 4, 4, "watt", object{"min": 0}, nil, healthyThresholds(),
		instant("sum(omada_port_power_watts{"+device+"}) or vector(0)", "Draw"))
	b.stat("Collector panics", "Recovered collector panics during the selected dashboard time range.", 12, 5, 4, 4, "short", object{"min": 0}, nil, problemThresholds(),
		instant("sum(increase(omada_collector_panics_total{"+target+"}[$__range])) or vector(0)", "Panics"))
	b.stat("Slowest collector", "Current duration of the slowest collector scrape.", 16, 5, 4, 4, "s", object{"min": 0}, nil, durationThresholds(),
		instant("max(omada_collector_last_scrape_duration_seconds{"+target+"}) or vector(0)", "Duration"))
	b.stat("Site-to-site peers", "Total configured peers across site-to-site VPN tunnels.", 20, 5, 4, 4, "short", object{"min": 0}, nil, healthyThresholds(),
		instant("sum(max by (site, vpn_id, tunnel_id, name) (omada_site_to_site_vpn_total_peers{"+site+"})) or vector(0)", "Peers"))

	b.row("Traffic", 9)
	b.timeseries("Client throughput", "Aggregate client counter rate. Grafana's adaptive rate interval prevents gaps when the dashboard range changes.", 0, 10, 12, 8, "bps", object{"min": 0},
		rateQuery("sum(rate(omada_client_traffic_down_bytes{"+site+"}["+rateInterval+"])) * 8", "Download"),
		rateQuery("sum(rate(omada_client_traffic_up_bytes{"+site+"}["+rateInterval+"])) * 8", "Upload"))
	b.barGauge("Top clients by throughput", "Ten busiest clients by combined upload and download counter rate.", 12, 10, 12, 8, "bps", object{"min": 0}, nil, healthyThresholds(),
		rateInstant("topk(10, sum by (name, mac) ((rate(omada_client_traffic_down_bytes{"+site+"}["+rateInterval+"]) + rate(omada_client_traffic_up_bytes{"+site+"}["+rateInterval+"])) * 8))", "{{name}} · {{mac}}"))
	b.timeseries("Device traffic", "Traffic counters grouped by current device labels.", 0, 18, 12, 8, "bps", object{"min": 0},
		rateQuery("sum by (device_name, device_mac) (rate(omada_device_download{"+device+"}["+rateInterval+"])) * 8", "{{device_name}} download"),
		rateQuery("sum by (device_name, device_mac) (rate(omada_device_upload{"+device+"}["+rateInterval+"])) * 8", "{{device_name}} upload"))
	b.barGauge("Wireless client link rates", "Current negotiated Wi-Fi link rates; these are link capabilities rather than measured throughput.", 12, 18, 12, 8, "bps", object{"min": 0}, nil, healthyThresholds(),
		instant("topk(10, max by (name, mac, ssid) (omada_client_rx_rate{"+site+",wireless=\"true\"}))", "{{name}} · {{ssid}} RX"),
		instant("topk(10, max by (name, mac, ssid) (omada_client_tx_rate{"+site+",wireless=\"true\"}))", "{{name}} · {{ssid}} TX"))

	b.row("Device and Wi-Fi health", 26)
	b.timeseries("Device CPU", "CPU utilization grouped by stable device identity.", 0, 27, 8, 8, "percent", object{"min": 0, "max": 100},
		rangeQuery("max by (site, device_name, device_mac, device_model) (omada_device_cpu_percentage{"+device+"})", "{{device_name}} · {{device_model}}"))
	b.timeseries("Device memory", "Memory utilization grouped by stable device identity.", 8, 27, 8, 8, "percent", object{"min": 0, "max": 100},
		rangeQuery("max by (site, device_name, device_mac, device_model) (omada_device_mem_percentage{"+device+"})", "{{device_name}} · {{device_model}}"))
	b.timeseries("Device temperature", "Temperature reported by supported gateways and switches.", 16, 27, 8, 8, "celsius", object{},
		rangeQuery("max by (site, device_name, device_mac, device_model) (omada_device_temp{"+device+"})", "{{device_name}} · {{device_model}}"))
	b.barGauge("Device states", "Current count for every Omada device state. Wireless-backhaul Connected variants remain visible here and count as online above.", 0, 35, 8, 8, "short", object{"min": 0}, nil, healthyThresholds(),
		instant("count by (device_status) (omada_device_uptime_seconds{"+device+"})", "{{device_status}}"))
	deviceUptimePanel := b.barGauge("Device uptime", "Current uptime for each selected device. Select a series to open the portable device drill-down.", 8, 35, 8, 8, "s", object{"min": 0}, nil, healthyThresholds(),
		instant("max by (device_name, device_mac) (omada_device_uptime_seconds{"+device+"})", "{{device_name}} · {{device_mac}}"))
	addDataLink(deviceUptimePanel, "Open device details", deviceDetailsURL)
	b.barGauge("Lowest Wi-Fi signal", "Ten lowest client signal-quality readings, useful for finding roaming or coverage problems.", 16, 35, 8, 8, "percent", object{"min": 0, "max": 100}, nil, signalThresholds(),
		instant("bottomk(10, max by (name, mac, ssid) (omada_client_signal_pct{"+site+",wireless=\"true\"}))", "{{name}} · {{ssid}}"))
	b.timeseries("AP radio utilization", "Receive and transmit utilization for every radio band exported by the selected access points.", 0, 43, 12, 8, "percent", object{"min": 0, "max": 100},
		rangeQuery("max by (device_name, device_mac) (omada_device_2g_rx_util{"+device+"})", "{{device_name}} 2.4 GHz RX"),
		rangeQuery("max by (device_name, device_mac) (omada_device_2g_tx_util{"+device+"})", "{{device_name}} 2.4 GHz TX"),
		rangeQuery("max by (device_name, device_mac) (omada_device_5g_rx_util{"+device+"})", "{{device_name}} 5 GHz RX"),
		rangeQuery("max by (device_name, device_mac) (omada_device_5g_tx_util{"+device+"})", "{{device_name}} 5 GHz TX"),
		rangeQuery("max by (device_name, device_mac) (omada_device_5g2_rx_util{"+device+"})", "{{device_name}} 5 GHz-2 RX"),
		rangeQuery("max by (device_name, device_mac) (omada_device_5g2_tx_util{"+device+"})", "{{device_name}} 5 GHz-2 TX"),
		rangeQuery("max by (device_name, device_mac) (omada_device_6g_rx_util{"+device+"})", "{{device_name}} 6 GHz RX"),
		rangeQuery("max by (device_name, device_mac) (omada_device_6g_tx_util{"+device+"})", "{{device_name}} 6 GHz TX"))
	b.timeseries("Wi-Fi RSSI and SNR", "Signal strength and signal-to-noise readings for the ten weakest wireless clients.", 12, 43, 12, 8, "dBm", object{},
		rangeQuery("bottomk(10, max by (name, mac, ssid) (omada_client_rssi_dbm{"+site+",wireless=\"true\"}))", "{{name}} · {{ssid}} RSSI"),
		rangeQuery("bottomk(10, max by (name, mac, ssid) (omada_client_snr_dbm{"+site+",wireless=\"true\"}))", "{{name}} · {{ssid}} SNR"))

	b.row("Switching", 51)
	b.barGauge("Port link state", "Current link state for ports. Requires OMADA_TRACK_PORT_METRICS.", 0, 52, 8, 8, "short", object{"min": 0, "max": 1}, binaryMappings("Disconnected", "Connected"), binaryThresholds(),
		instant("max by (device_name, device_mac, port, name) (omada_port_link_status{"+device+"})", "{{device_name}} · port {{port}} {{name}}"))
	b.barGauge("Port link speed", "Negotiated port link capability in Mbit/s.", 8, 52, 8, 8, "Mbits", object{"min": 0}, linkSpeedMappings(), linkSpeedThresholds(),
		instant("max by (device_name, device_mac, port, name) (omada_port_link_speed_mbps{"+device+"})", "{{device_name}} · port {{port}} {{name}}"))
	b.barGauge("PoE by port", "Current per-port PoE draw.", 16, 52, 8, 8, "watt", object{"min": 0}, nil, healthyThresholds(),
		instant("max by (device_name, device_mac, port, name) (omada_port_power_watts{"+device+"})", "{{device_name}} · port {{port}} {{name}}"))
	b.timeseries("Port throughput", "Per-port byte counters converted to bit/s with the adaptive rate interval.", 0, 60, 12, 8, "bps", object{"min": 0},
		rateQuery("sum by (device_name, device_mac, port, name) (rate(omada_port_link_rx{"+device+"}["+rateInterval+"])) * 8", "{{device_name}} · {{port}} RX"),
		rateQuery("sum by (device_name, device_mac, port, name) (rate(omada_port_link_tx{"+device+"}["+rateInterval+"])) * 8", "{{device_name}} · {{port}} TX"))
	b.timeseries("LAG throughput", "Link-aggregation byte counters converted to bit/s.", 12, 60, 12, 8, "bps", object{"min": 0},
		rateQuery("sum by (device_name, device_mac, lag_id, name) (rate(omada_lag_link_rx{"+device+"}["+rateInterval+"])) * 8", "{{device_name}} · LAG {{lag_id}} RX"),
		rateQuery("sum by (device_name, device_mac, lag_id, name) (rate(omada_lag_link_tx{"+device+"}["+rateInterval+"])) * 8", "{{device_name}} · LAG {{lag_id}} TX"))
	b.barGauge("LAG link state", "Current state of switch link-aggregation groups.", 0, 68, 8, 8, "short", object{"min": 0, "max": 1}, binaryMappings("Disconnected", "Connected"), binaryThresholds(),
		instant("max by (device_name, device_mac, lag_id, name) (omada_lag_link_status{"+device+"})", "{{device_name}} · LAG {{lag_id}} {{name}}"))
	b.barGauge("LAG link speed", "Aggregate negotiated link speed for each LAG.", 8, 68, 8, 8, "Mbits", object{"min": 0}, linkSpeedMappings(), linkSpeedThresholds(),
		instant("max by (device_name, device_mac, lag_id, name) (omada_lag_link_speed_mbps{"+device+"})", "{{device_name}} · LAG {{lag_id}} {{name}}"))
	b.barGauge("PoE remaining", "Remaining PoE budget on switches that report PoE capacity.", 16, 68, 8, 8, "watt", object{"min": 0}, nil, healthyThresholds(),
		instant("max by (device_name, device_mac) (omada_device_poe_remain_watts{"+device+"})", "{{device_name}}"))

	b.row("Controller, WAN, and ISP", 76)
	b.stat("Controller uptime by site", "Controller uptime grouped by site and controller name.", 0, 77, 6, 4, "s", object{"min": 0}, nil, healthyThresholds(),
		instant("max by (site, device_name) (omada_controller_uptime_seconds{"+site+"})", "{{site}} · {{device_name}}"))
	b.stat("Controller free storage", "Available storage grouped by storage volume.", 6, 77, 6, 4, "bytes", object{"min": 0}, nil, healthyThresholds(),
		instant("sum by (site, device_name, storage_name) (omada_controller_storage_available_bytes{"+site+"})", "{{site}} · {{storage_name}}"))
	b.stat("WAN internet state", "Internet reachability state for each WAN interface.", 12, 77, 6, 4, "short", object{"min": 0, "max": 1}, binaryMappings("Offline", "Online"), binaryThresholds(),
		instant("max by (device_name, device_mac, port, name, ip) (omada_wan_internet_state{"+device+"})", "{{device_name}} · {{name}}"))
	b.stat("WAN link state", "Current connected or disconnected state reported for each WAN interface.", 18, 77, 6, 4, "short", object{"min": 0, "max": 1}, binaryMappings("Disconnected", "Connected"), binaryThresholds(),
		instant("max by (device_name, device_mac, port, name, ip) (omada_wan_status{"+device+"})", "{{device_name}} · {{name}}"))
	b.timeseries("WAN throughput", "Omada WAN KB/s gauges converted to bit/s.", 0, 81, 12, 8, "bps", object{"min": 0},
		rangeQuery("sum by (device_name, device_mac, port, name) (omada_wan_rx_rate{"+device+"}) * 8000", "{{device_name}} · {{name}} RX"),
		rangeQuery("sum by (device_name, device_mac, port, name) (omada_wan_tx_rate{"+device+"}) * 8000", "{{device_name}} · {{name}} TX"))
	b.timeseries("WAN latency", "Latency reported for each WAN interface.", 12, 81, 12, 8, "ms", object{"min": 0},
		rangeQuery("max by (device_name, device_mac, port, name) (omada_wan_latency{"+device+"})", "{{device_name}} · {{name}}"))
	b.barGauge("WAN link speed", "Negotiated WAN link capability in Mbit/s.", 0, 89, 8, 8, "Mbits", object{"min": 0}, linkSpeedMappings(), linkSpeedThresholds(),
		instant("max by (device_name, device_mac, port, name) (omada_wan_link_speed_mbps{"+device+"})", "{{device_name}} · {{name}}"))
	b.stat("ISP state", "Current Omada ISP state for each gateway interface.", 8, 89, 8, 8, "short", object{"min": 0, "max": 1}, binaryMappings("Offline", "Online"), binaryThresholds(),
		instant("max by (gateway_name, gateway_mac, port, name, ip) (omada_isp_status{"+site+"})", "{{gateway_name}} · {{name}}"))
	b.barGauge("Configured ISP bandwidth", "Configured ISP download and upload speeds in Mbit/s.", 16, 89, 8, 8, "Mbits", object{"min": 0}, nil, healthyThresholds(),
		instant("max by (gateway_name, gateway_mac, port, name) (omada_isp_download_speed{"+site+"})", "{{gateway_name}} · {{name}} down"),
		instant("max by (gateway_name, gateway_mac, port, name) (omada_isp_upload_speed{"+site+"})", "{{gateway_name}} · {{name}} up"))

	b.row("VPN", 97)
	vpnUptime := "max by (job, instance, site, site_id, name, interface_name, vpn_type) (omada_vpn_uptime{" + site + "})" +
		" or (max by (job, instance, site, site_id, vpn_id, name, vpn_type) (clamp_min(time() - (omada_site_to_site_vpn_peer_login_timestamp{" + site + "} > 0), 0))" +
		" unless on (job, instance, site, site_id, name, vpn_type) max by (job, instance, site, site_id, name, vpn_type) (omada_vpn_uptime{" + site + "}))"
	b.stat("VPN configuration state", "Enabled or disabled state for every configured VPN.", 0, 98, 6, 8, "short", object{"min": 0, "max": 1}, binaryMappings("Disabled", "Enabled"), binaryThresholds(),
		instant("max by (site, name, vpn_type) (omada_vpn_status{"+site+"})", "{{site}} · {{name}} {{vpn_type}}"))
	b.stat("Site-to-site peer state", "Online state returned for peers that expose it.", 6, 98, 6, 8, "short", object{"min": 0, "max": 1}, binaryMappings("Offline", "Online"), binaryThresholds(),
		instant("max by (site, name, peer_name, peer_id) (omada_site_to_site_vpn_peer_status{"+site+"})", "{{name}} · {{peer_name}}"))
	b.barGauge("VPN uptime", "Current generic tunnel uptime, with the longest site-to-site peer session age inferred when generic tunnel stats are unavailable.", 12, 98, 6, 8, "s", object{"min": 0}, nil, healthyThresholds(),
		instant(vpnUptime, "{{site}} · {{name}} · {{vpn_type}}"))
	b.stat("Peer login time", "Most recent peer login timestamp, displayed relative to now.", 18, 98, 6, 8, "dateTimeFromNow", object{}, nil, healthyThresholds(),
		instant("max by (site, name, peer_name, peer_id) (omada_site_to_site_vpn_peer_login_timestamp{"+site+"} > 0) * 1000", "{{name}} · {{peer_name}}"))
	b.timeseries("VPN throughput", "General VPN tunnel byte counters converted to bit/s.", 0, 106, 12, 8, "bps", object{"min": 0},
		rateQuery("sum by (site, name, interface_name, vpn_type) (rate(omada_vpn_down_bytes{"+site+"}["+rateInterval+"])) * 8", "{{name}} · down"),
		rateQuery("sum by (site, name, interface_name, vpn_type) (rate(omada_vpn_up_bytes{"+site+"}["+rateInterval+"])) * 8", "{{name}} · up"))
	b.timeseries("Site-to-site VPN throughput", "Aggregate site-to-site VPN byte counters converted to bit/s.", 12, 106, 12, 8, "bps", object{"min": 0},
		rateQuery("sum by (site, vpn_id, name, vpn_type) (rate(omada_site_to_site_vpn_down_bytes{"+site+"}["+rateInterval+"])) * 8", "{{name}} · down"),
		rateQuery("sum by (site, vpn_id, name, vpn_type) (rate(omada_site_to_site_vpn_up_bytes{"+site+"}["+rateInterval+"])) * 8", "{{name}} · up"))
	b.timeseries("Site-to-site peer throughput", "Per-peer site-to-site counters converted to bit/s.", 0, 114, 24, 8, "bps", object{"min": 0},
		rateQuery("sum by (site, name, peer_name, peer_id) (rate(omada_site_to_site_vpn_peer_down_bytes{"+site+"}["+rateInterval+"])) * 8", "{{name}} · {{peer_name}} down"),
		rateQuery("sum by (site, name, peer_name, peer_id) (rate(omada_site_to_site_vpn_peer_up_bytes{"+site+"}["+rateInterval+"])) * 8", "{{name}} · {{peer_name}} up"))

	b.row("DPI insights", 122)
	b.barGauge("Top DPI categories", "Largest DPI traffic categories within the exporter's configured insight window. Requires OMADA_TRACK_INSIGHT_METRICS=true.", 0, 123, 12, 8, "bytes", object{"min": 0}, nil, healthyThresholds(),
		instant("topk(10, sum by (site, family_id, family_name) (omada_dpi_category_traffic_bytes{"+site+"}))", "{{site}} · {{family_name}}"))
	b.barGauge("Top DPI applications", "Largest DPI-classified applications within the configured insight window. Requires OMADA_TRACK_INSIGHT_METRICS=true and a non-zero application limit.", 12, 123, 12, 8, "bytes", object{"min": 0}, nil, healthyThresholds(),
		instant("topk(10, sum by (site, application_id, application_name) (omada_dpi_application_traffic_bytes{"+site+"}))", "{{site}} · {{application_name}}"))
	b.stat("DPI-classified traffic", "Total classified bytes for the current insight window. No data means insight metrics are disabled or unsupported.", 0, 131, 12, 4, "bytes", object{"min": 0}, nil, healthyThresholds(),
		instant("sum(omada_dpi_total_traffic_bytes{"+site+"})", "Classified"))
	b.stat("DPI insight window", "Window used by the exporter when querying DPI insights. No data means insight metrics are disabled or unsupported.", 12, 131, 12, 4, "s", object{"min": 0}, nil, healthyThresholds(),
		instant("max(omada_dpi_scrape_window_seconds{"+site+"})", "Window"))

	return dashboard("Omada Overview", "omada-overview", 12,
		"Comprehensive TP-Link Omada dashboard for the current omada_exporter metric and label contract.", b.panels, true)
}

func deviceDetailsDashboard() object {
	b := &dashboardBuilder{nextID: 1}
	target := "job=~\"" + jobVariable + "\",instance=~\"" + instanceVariable + "\""
	site := target + ",site=~\"" + siteVariable + "\""
	device := site + ",device_mac=~\"" + deviceVariable + "\""
	apClient := site + ",ap_mac=~\"" + deviceVariable + "\""
	clientThroughput := func(topologyLabel string) string {
		client := site + "," + topologyLabel + "=~\"" + deviceVariable + "\""
		return "sum by (name, mac, device_type) ((rate(omada_client_traffic_down_bytes{" + client + "}[" + rateInterval + "]) + rate(omada_client_traffic_up_bytes{" + client + "}[" + rateInterval + "])) * 8)"
	}
	topClients := "topk(10, (" + clientThroughput("ap_mac") + " or " + clientThroughput("switch_mac") + " or " + clientThroughput("gateway_mac") + "))"

	b.row("Selected device", 0)
	b.stat("Device uptime", "Current uptime for the selected device; the legend also identifies its hardware type.", 0, 1, 4, 4, "s", object{"min": 0}, nil, healthyThresholds(),
		instant("max by (device_name, device_mac, device_type, device_model) (omada_device_uptime_seconds{"+device+"})", "{{device_name}} · {{device_type}} · {{device_model}}"))
	b.stat("CPU", "Current CPU utilization for the selected device.", 4, 1, 4, 4, "percent", object{"min": 0, "max": 100}, nil, utilizationThresholds(),
		instant("max by (device_name, device_mac) (omada_device_cpu_percentage{"+device+"})", "{{device_name}}"))
	b.stat("Memory", "Current memory utilization for the selected device.", 8, 1, 4, 4, "percent", object{"min": 0, "max": 100}, nil, utilizationThresholds(),
		instant("max by (device_name, device_mac) (omada_device_mem_percentage{"+device+"})", "{{device_name}}"))
	b.stat("Temperature", "Current temperature when reported by the selected gateway or switch.", 12, 1, 4, 4, "celsius", object{}, nil, temperatureThresholds(),
		instant("max by (device_name, device_mac) (omada_device_temp{"+device+"})", "{{device_name}}"))
	b.stat("Current RX", "Controller-reported receive rate for the selected access point or switch.", 16, 1, 4, 4, "binBps", object{"min": 0}, nil, healthyThresholds(),
		instant("max by (device_name, device_mac) (omada_device_rx_rate{"+device+"})", "{{device_name}}"))
	b.stat("Current TX", "Controller-reported transmit rate for the selected access point or switch.", 20, 1, 4, 4, "binBps", object{"min": 0}, nil, healthyThresholds(),
		instant("max by (device_name, device_mac) (omada_device_tx_rate{"+device+"})", "{{device_name}}"))
	b.stat("Online", "Whether the selected device status starts with Connected, including wireless-backhaul variants.", 0, 5, 6, 4, "short", object{"min": 0}, nil, onlineThresholds(),
		instant("count(omada_device_uptime_seconds{"+device+",device_status=~\"^Connected.*$\"}) or vector(0)", "Online"))
	b.stat("Not online", "Whether the selected device is disconnected, provisioning, upgrading, pending, isolated, or in another non-connected state.", 6, 5, 6, 4, "short", object{"min": 0}, nil, problemThresholds(),
		instant("count(omada_device_uptime_seconds{"+device+",device_status!~\"^Connected.*$\"}) or vector(0)", "Not online"))
	b.stat("Firmware", "Firmware upgrade state and advertised target version for the selected device.", 12, 5, 6, 4, "short", object{"min": 0, "max": 1}, binaryMappings("Current", "Upgrade available"), warningThresholds(),
		instant("max by (device_name, device_mac, device_version, device_version_upgrade) (omada_device_need_upgrade{"+device+"})", "{{device_name}} · {{device_version}} → {{device_version_upgrade}}"))
	b.stat("PoE remaining", "Remaining PoE budget when the selected switch reports its capacity.", 18, 5, 6, 4, "watt", object{"min": 0}, nil, healthyThresholds(),
		instant("max by (device_name, device_mac, device_total_power) (omada_device_poe_remain_watts{"+device+"})", "{{device_name}} · {{device_total_power}} W total"))

	b.row("Traffic and attached clients", 9)
	b.timeseries("Device throughput", "Selected-device traffic counters converted to bit/s with an adaptive rate window.", 0, 10, 12, 8, "bps", object{"min": 0},
		rateQuery("sum by (device_name, device_mac) (rate(omada_device_download{"+device+"}["+rateInterval+"])) * 8", "{{device_name}} download"),
		rateQuery("sum by (device_name, device_mac) (rate(omada_device_upload{"+device+"}["+rateInterval+"])) * 8", "{{device_name}} upload"))
	b.barGauge("Top clients through selected device", "Ten busiest clients associated through the selected access point, switch, or gateway topology label. Requires client metrics.", 12, 10, 12, 8, "bps", object{"min": 0}, nil, healthyThresholds(),
		rateInstant(topClients, "{{device_type}} · {{name}} · {{mac}}"))
	b.barGauge("Client link rates through selected AP", "Current negotiated link rates for wireless clients associated with the selected access point. No data is expected when the selected device is not an access point or the AP has no associated wireless clients.", 0, 18, 12, 8, "bps", object{"min": 0}, nil, healthyThresholds(),
		instant("topk(10, max by (name, mac, ssid) (omada_client_rx_rate{"+apClient+",wireless=\"true\"}))", "{{name}} · {{ssid}} RX"),
		instant("topk(10, max by (name, mac, ssid) (omada_client_tx_rate{"+apClient+",wireless=\"true\"}))", "{{name}} · {{ssid}} TX"))
	b.barGauge("Client signal through selected AP", "Current signal quality for wireless clients associated with the selected access point. No data is expected when the selected device is not an access point or the AP has no associated wireless clients.", 12, 18, 12, 8, "percent", object{"min": 0, "max": 100}, nil, signalThresholds(),
		instant("bottomk(10, max by (name, mac, ssid) (omada_client_signal_pct{"+apClient+",wireless=\"true\"}))", "{{name}} · {{ssid}}"))

	b.row("Access-point radios (AP only)", 26)
	b.barGauge("2.4 GHz utilization", "Current receive and transmit utilization when the selected device has a 2.4 GHz radio.", 0, 27, 6, 8, "percent", object{"min": 0, "max": 100}, nil, utilizationThresholds(),
		instant("max by (device_name, device_mac, device_wp2g_mode, device_wp2g_band_width) (omada_device_2g_rx_util{"+device+"})", "RX · {{device_wp2g_mode}} · {{device_wp2g_band_width}} MHz"),
		instant("max by (device_name, device_mac, device_wp2g_mode, device_wp2g_band_width) (omada_device_2g_tx_util{"+device+"})", "TX · {{device_wp2g_mode}} · {{device_wp2g_band_width}} MHz"))
	b.barGauge("5 GHz utilization", "Current receive and transmit utilization when the selected device has a 5 GHz radio.", 6, 27, 6, 8, "percent", object{"min": 0, "max": 100}, nil, utilizationThresholds(),
		instant("max by (device_name, device_mac, device_wp5g_mode, device_wp5g_band_width) (omada_device_5g_rx_util{"+device+"})", "RX · {{device_wp5g_mode}} · {{device_wp5g_band_width}} MHz"),
		instant("max by (device_name, device_mac, device_wp5g_mode, device_wp5g_band_width) (omada_device_5g_tx_util{"+device+"})", "TX · {{device_wp5g_mode}} · {{device_wp5g_band_width}} MHz"))
	b.barGauge("5 GHz-2 utilization", "Current receive and transmit utilization when the selected device has a second 5 GHz radio.", 12, 27, 6, 8, "percent", object{"min": 0, "max": 100}, nil, utilizationThresholds(),
		instant("max by (device_name, device_mac, device_wp5g2_mode, device_wp5g2_band_width) (omada_device_5g2_rx_util{"+device+"})", "RX · {{device_wp5g2_mode}} · {{device_wp5g2_band_width}} MHz"),
		instant("max by (device_name, device_mac, device_wp5g2_mode, device_wp5g2_band_width) (omada_device_5g2_tx_util{"+device+"})", "TX · {{device_wp5g2_mode}} · {{device_wp5g2_band_width}} MHz"))
	b.barGauge("6 GHz utilization", "Current receive and transmit utilization when the selected device has a 6 GHz radio.", 18, 27, 6, 8, "percent", object{"min": 0, "max": 100}, nil, utilizationThresholds(),
		instant("max by (device_name, device_mac, device_wp6g_mode, device_wp6g_band_width) (omada_device_6g_rx_util{"+device+"})", "RX · {{device_wp6g_mode}} · {{device_wp6g_band_width}} MHz"),
		instant("max by (device_name, device_mac, device_wp6g_mode, device_wp6g_band_width) (omada_device_6g_tx_util{"+device+"})", "TX · {{device_wp6g_mode}} · {{device_wp6g_band_width}} MHz"))

	b.row("Ports and link aggregation (supported devices only)", 35)
	portSpeeds := b.barGauge("Port link speeds", "Scrollable port list colored by negotiated speed; Down indicates a disconnected port. Requires port metrics.", 0, 36, 24, 8, "Mbits", object{"min": 0}, linkSpeedMappings(), linkSpeedThresholds(),
		instant("max by (device_name, device_mac, port, name) (omada_port_link_speed_mbps{"+device+"})", "Port {{port}} · {{name}}"))
	configureScrollableBarGauge(portSpeeds)
	b.timeseries("Port throughput", "Per-port traffic counters converted to bit/s with an adaptive rate window.", 0, 44, 12, 8, "bps", object{"min": 0},
		rateQuery("sum by (device_name, device_mac, port, name) (rate(omada_port_link_rx{"+device+"}["+rateInterval+"])) * 8", "Port {{port}} · {{name}} RX"),
		rateQuery("sum by (device_name, device_mac, port, name) (rate(omada_port_link_tx{"+device+"}["+rateInterval+"])) * 8", "Port {{port}} · {{name}} TX"))
	b.barGauge("PoE draw by port", "Current PoE draw for every port on the selected device. No data is expected on devices without PoE output.", 12, 44, 12, 8, "watt", object{"min": 0}, nil, healthyThresholds(),
		instant("max by (device_name, device_mac, port, name) (omada_port_power_watts{"+device+"})", "Port {{port}} · {{name}}"))
	lagSpeeds := b.barGauge("LAG link speeds", "Configured link-aggregation groups colored by negotiated aggregate speed. No data is expected when the selected device is not a switch or the switch has no configured LAGs.", 0, 52, 12, 8, "Mbits", object{"min": 0}, linkSpeedMappings(), linkSpeedThresholds(),
		instant("max by (device_name, device_mac, lag_id, name, lag_ports) (omada_lag_link_speed_mbps{"+device+"})", "LAG {{lag_id}} · {{name}} · {{lag_ports}}"))
	configureScrollableBarGauge(lagSpeeds)
	b.timeseries("LAG throughput", "Per-LAG traffic counters converted to bit/s with an adaptive rate window. No data is expected when the selected device is not a switch or the switch has no configured LAGs.", 12, 52, 12, 8, "bps", object{"min": 0},
		rateQuery("sum by (device_name, device_mac, lag_id, name) (rate(omada_lag_link_rx{"+device+"}["+rateInterval+"])) * 8", "LAG {{lag_id}} · {{name}} RX"),
		rateQuery("sum by (device_name, device_mac, lag_id, name) (rate(omada_lag_link_tx{"+device+"}["+rateInterval+"])) * 8", "LAG {{lag_id}} · {{name}} TX"))

	b.row("Gateway WAN (gateway only)", 60)
	b.stat("Internet state", "Current internet reachability for each WAN interface on the selected gateway.", 0, 61, 6, 6, "short", object{"min": 0, "max": 1}, binaryMappings("Offline", "Online"), binaryThresholds(),
		instant("max by (device_name, device_mac, port, name) (omada_wan_internet_state{"+device+"})", "{{name}}"))
	b.stat("Link state", "Current connected or disconnected state reported for each WAN interface on the selected gateway.", 6, 61, 6, 6, "short", object{"min": 0, "max": 1}, binaryMappings("Disconnected", "Connected"), binaryThresholds(),
		instant("max by (device_name, device_mac, port, name) (omada_wan_status{"+device+"})", "{{name}}"))
	b.barGauge("WAN link speed", "Negotiated WAN link capability for each interface on the selected gateway.", 12, 61, 6, 6, "Mbits", object{"min": 0}, nil, linkSpeedThresholds(),
		instant("max by (device_name, device_mac, port, name) (omada_wan_link_speed_mbps{"+device+"})", "{{name}}"))
	b.barGauge("WAN latency", "Current latency for each WAN interface on the selected gateway.", 18, 61, 6, 6, "ms", object{"min": 0}, nil, durationThresholds(),
		instant("max by (device_name, device_mac, port, name) (omada_wan_latency{"+device+"})", "{{name}}"))
	b.timeseries("WAN throughput", "Controller-reported WAN receive and transmit rates converted from KB/s to bit/s.", 0, 67, 24, 8, "bps", object{"min": 0},
		rangeQuery("sum by (device_name, device_mac, port, name) (omada_wan_rx_rate{"+device+"}) * 8000", "{{name}} RX"),
		rangeQuery("sum by (device_name, device_mac, port, name) (omada_wan_tx_rate{"+device+"}) * 8000", "{{name}} TX"))

	result := dashboard("Omada Device Details", "omada-device-details", 2,
		"Portable per-device TP-Link Omada drill-down inspired by compact device rows without hard-coded sites, MAC addresses, or port layouts.", b.panels, false)
	result["tags"] = []string{"omada", "network", "prometheus", "device-details"}
	result["templating"] = object{"list": deviceDetailsVariables()}
	return result
}

func simpleDashboard() object {
	b := &dashboardBuilder{nextID: 1}
	target := "job=~\"" + jobVariable + "\",instance=~\"" + instanceVariable + "\""
	site := target + ",site=~\"" + siteVariable + "\""

	b.row("Overview", 0)
	b.stat("Devices online", "Connected devices, including wireless-backhaul Connected variants.", 0, 1, 4, 4, "short", object{"min": 0}, nil, onlineThresholds(),
		instant("count(omada_device_uptime_seconds{"+site+",device_status=~\"^Connected.*$\"}) or vector(0)", "Online"))
	b.stat("Devices not online", "Devices whose status does not start with Connected.", 4, 1, 4, 4, "short", object{"min": 0}, nil, problemThresholds(),
		instant("count(omada_device_uptime_seconds{"+site+",device_status!~\"^Connected.*$\"}) or vector(0)", "Not online"))
	b.stat("Connected clients", "Current wired and wireless client total.", 8, 1, 4, 4, "short", object{"min": 0}, nil, healthyThresholds(),
		instant("sum(omada_client_connected_total{"+site+"}) or vector(0)", "Clients"))
	b.stat("Site alerts", "Current alert count.", 12, 1, 4, 4, "short", object{"min": 0}, nil, problemThresholds(),
		instant("sum(omada_site_alert_num{"+site+"}) or vector(0)", "Alerts"))
	b.stat("Firmware upgrades", "Available device and controller firmware upgrades.", 16, 1, 4, 4, "short", object{"min": 0}, nil, warningThresholds(),
		instant("(sum(omada_device_need_upgrade{"+site+"}) or vector(0)) + (sum(omada_controller_upgrade_available{"+site+"}) or vector(0))", "Upgrades"))
	b.stat("Collector failures", "Collectors whose latest scrape did not complete.", 20, 1, 4, 4, "short", object{"min": 0}, nil, problemThresholds(),
		instant("sum(omada_collector_last_scrape_completed{"+target+"} == 0) or vector(0)", "Failed"))

	b.row("Traffic", 5)
	b.timeseries("Client throughput", "Aggregate client traffic using Grafana's adaptive Prometheus rate interval.", 0, 6, 12, 8, "bps", object{"min": 0},
		rateQuery("sum(rate(omada_client_traffic_down_bytes{"+site+"}["+rateInterval+"])) * 8", "Download"),
		rateQuery("sum(rate(omada_client_traffic_up_bytes{"+site+"}["+rateInterval+"])) * 8", "Upload"))
	b.timeseries("WAN throughput", "Omada WAN KB/s gauges converted to bit/s.", 12, 6, 12, 8, "bps", object{"min": 0},
		rangeQuery("sum by (site, device_name, device_mac, port, name) (omada_wan_rx_rate{"+site+"}) * 8000", "{{site}} · {{device_name}} · {{name}} RX"),
		rangeQuery("sum by (site, device_name, device_mac, port, name) (omada_wan_tx_rate{"+site+"}) * 8000", "{{site}} · {{device_name}} · {{name}} TX"))
	b.barGauge("Top clients", "Ten busiest clients by combined upload and download rate.", 0, 14, 12, 8, "bps", object{"min": 0}, nil, healthyThresholds(),
		rateInstant("topk(10, sum by (name, mac) ((rate(omada_client_traffic_down_bytes{"+site+"}["+rateInterval+"]) + rate(omada_client_traffic_up_bytes{"+site+"}["+rateInterval+"])) * 8))", "{{name}} · {{mac}}"))
	b.barGauge("Device states", "Current count by Omada status, with wireless Connected variants shown separately.", 12, 14, 12, 8, "short", object{"min": 0}, nil, healthyThresholds(),
		instant("count by (device_status) (omada_device_uptime_seconds{"+site+"})", "{{device_status}}"))

	b.row("Health", 22)
	deviceCPU := b.barGauge("Device CPU", "Current CPU utilization by device. Select a series to open the portable device drill-down.", 0, 23, 8, 8, "percent", object{"min": 0, "max": 100}, nil, utilizationThresholds(),
		instant("max by (device_name, device_mac, device_model) (omada_device_cpu_percentage{"+site+"})", "{{device_name}} · {{device_model}}"))
	addDataLink(deviceCPU, "Open device details", deviceDetailsURL)
	b.barGauge("Device memory", "Current memory utilization by device.", 8, 23, 8, 8, "percent", object{"min": 0, "max": 100}, nil, utilizationThresholds(),
		instant("max by (device_name, device_mac, device_model) (omada_device_mem_percentage{"+site+"})", "{{device_name}} · {{device_model}}"))
	b.barGauge("WAN latency", "Current latency by WAN interface.", 16, 23, 8, 8, "ms", object{"min": 0}, nil, durationThresholds(),
		instant("max by (device_name, device_mac, port, name) (omada_wan_latency{"+site+"})", "{{device_name}} · {{name}}"))
	b.stat("PoE draw", "Current PoE draw across all ports in the selected sites.", 0, 31, 8, 4, "watt", object{"min": 0}, nil, healthyThresholds(),
		instant("sum(omada_port_power_watts{"+site+"}) or vector(0)", "Draw"))
	b.stat("Controller storage used", "Used storage divided by total controller storage.", 8, 31, 8, 4, "percent", object{"min": 0, "max": 100}, nil, utilizationThresholds(),
		instant("100 * sum(omada_controller_storage_used_bytes{"+site+"}) / clamp_min(sum(omada_controller_storage_total_bytes{"+site+"}), 1)", "Used"))
	b.stat("WAN internet state", "Current internet reachability for each WAN interface.", 16, 31, 8, 4, "short", object{"min": 0, "max": 1}, binaryMappings("Offline", "Online"), binaryThresholds(),
		instant("max by (device_name, device_mac, port, name) (omada_wan_internet_state{"+site+"})", "{{device_name}} · {{name}}"))

	return dashboard("Simple Omada dashboard", "ad5dtmf", 8,
		"Focused TP-Link Omada health and traffic dashboard for the current omada_exporter metrics.", b.panels, false)
}

func dashboard(title, uid string, version int, description string, panels []any, includeDevice bool) object {
	return object{
		"annotations": object{"list": []any{
			object{
				"builtIn":    1,
				"datasource": object{"type": "datasource", "uid": "grafana"},
				"enable":     true,
				"hide":       true,
				"iconColor":  "rgba(0, 211, 255, 1)",
				"name":       "Annotations & Alerts",
				"target":     object{"limit": 100, "matchAny": false, "tags": []any{}, "type": "dashboard"},
				"type":       "dashboard",
			},
		}},
		"description":          description,
		"editable":             true,
		"fiscalYearStartMonth": 0,
		"graphTooltip":         1,
		"id":                   nil,
		"links":                []any{},
		"liveNow":              false,
		"panels":               panels,
		"preload":              false,
		"refresh":              "30s",
		"schemaVersion":        42,
		"tags":                 []string{"omada", "network", "prometheus"},
		"templating":           object{"list": dashboardVariables(includeDevice)},
		"time":                 object{"from": "now-6h", "to": "now"},
		"timepicker": object{
			"refresh_intervals": []string{"30s", "1m", "5m", "15m", "30m", "1h"},
			"time_options":      []string{"5m", "15m", "1h", "6h", "12h", "24h", "2d", "7d", "30d"},
		},
		"timezone":  "browser",
		"title":     title,
		"uid":       uid,
		"version":   version,
		"weekStart": "",
	}
}

func dashboardVariables(includeDevice bool) []any {
	variables := []any{
		object{
			"current":    object{},
			"includeAll": false,
			"label":      "Prometheus data source",
			"multi":      false,
			"name":       "datasource",
			"options":    []any{},
			"query":      "prometheus",
			"refresh":    1,
			"regex":      "",
			"type":       "datasource",
		},
		object{
			"allValue":   ".+",
			"current":    object{"text": "All", "value": "$__all"},
			"datasource": prometheusDatasource(),
			"definition": "label_values(omada_device_uptime_seconds, job)",
			"includeAll": true,
			"label":      "Job",
			"multi":      true,
			"name":       "job",
			"options":    []any{},
			"query": object{
				"label":   "job",
				"metric":  "omada_device_uptime_seconds",
				"query":   "label_values(omada_device_uptime_seconds, job)",
				"refId":   "PrometheusVariableQueryEditor-Job",
				"qryType": 1,
			},
			"refresh": 2,
			"regex":   "",
			"sort":    1,
			"type":    "query",
		},
		object{
			"allValue":   ".+",
			"current":    object{"text": "All", "value": "$__all"},
			"datasource": prometheusDatasource(),
			"definition": "label_values(omada_device_uptime_seconds{job=~\"" + jobVariable + "\"}, instance)",
			"includeAll": true,
			"label":      "Instance",
			"multi":      true,
			"name":       "instance",
			"options":    []any{},
			"query": object{
				"label":   "instance",
				"metric":  "omada_device_uptime_seconds{job=~\"" + jobVariable + "\"}",
				"query":   "label_values(omada_device_uptime_seconds{job=~\"" + jobVariable + "\"}, instance)",
				"refId":   "PrometheusVariableQueryEditor-Instance",
				"qryType": 1,
			},
			"refresh": 2,
			"regex":   "",
			"sort":    1,
			"type":    "query",
		},
		object{
			"allValue":   ".*",
			"current":    object{"text": "All", "value": "$__all"},
			"datasource": prometheusDatasource(),
			"definition": "label_values(omada_device_uptime_seconds{job=~\"" + jobVariable + "\",instance=~\"" + instanceVariable + "\"}, site)",
			"includeAll": true,
			"label":      "Site",
			"multi":      true,
			"name":       "Site",
			"options":    []any{},
			"query": object{
				"label":   "site",
				"metric":  "omada_device_uptime_seconds{job=~\"" + jobVariable + "\",instance=~\"" + instanceVariable + "\"}",
				"query":   "label_values(omada_device_uptime_seconds{job=~\"" + jobVariable + "\",instance=~\"" + instanceVariable + "\"}, site)",
				"refId":   "PrometheusVariableQueryEditor-Site",
				"qryType": 1,
			},
			"refresh": 2,
			"regex":   "",
			"sort":    1,
			"type":    "query",
		},
	}
	if includeDevice {
		metric := "omada_device_uptime_seconds{job=~\"" + jobVariable + "\",instance=~\"" + instanceVariable + "\",site=~\"" + siteVariable + "\"}"
		variables = append(variables, object{
			"allValue":   ".*",
			"current":    object{"text": "All", "value": "$__all"},
			"datasource": prometheusDatasource(),
			"definition": "label_values(" + metric + ", device_mac)",
			"includeAll": true,
			"label":      "Device MAC",
			"multi":      true,
			"name":       "Device",
			"options":    []any{},
			"query": object{
				"label":   "device_mac",
				"metric":  metric,
				"query":   "label_values(" + metric + ", device_mac)",
				"refId":   "PrometheusVariableQueryEditor-Device",
				"qryType": 1,
			},
			"refresh": 2,
			"regex":   "",
			"sort":    1,
			"type":    "query",
		})
	}
	return variables
}

func deviceDetailsVariables() []any {
	variables := dashboardVariables(false)
	metric := "omada_device_uptime_seconds{job=~\"" + jobVariable + "\",instance=~\"" + instanceVariable + "\",site=~\"" + siteVariable + "\"}"
	query := "query_result(max by (device_mac, device_name) (" + metric + "))"
	return append(variables, object{
		"current":    object{},
		"datasource": prometheusDatasource(),
		"definition": query,
		"includeAll": false,
		"label":      "Device",
		"multi":      false,
		"name":       "Device",
		"options":    []any{},
		"query": object{
			"query":   query,
			"refId":   "PrometheusVariableQueryEditor-Device",
			"qryType": 3,
		},
		"refresh": 2,
		"regex":   `/device_mac="(?<value>[^"]+)".*device_name="(?<text>[^"]+)"/`,
		"sort":    1,
		"type":    "query",
	})
}

func (b *dashboardBuilder) row(title string, y int) {
	b.panels = append(b.panels, object{
		"collapsed": false,
		"gridPos":   object{"h": 1, "w": 24, "x": 0, "y": y},
		"id":        b.id(),
		"panels":    []any{},
		"title":     title,
		"type":      "row",
	})
}

func (b *dashboardBuilder) stat(title, description string, x, y, w, h int, unit string, bounds object, mappings, thresholds []any, queries ...querySpec) {
	panel := b.panel(title, description, "stat", x, y, w, h, fieldDefaults(unit, bounds, "thresholds", mappings, thresholds), queries)
	panel["options"] = object{
		"colorMode":              "value",
		"graphMode":              "area",
		"justifyMode":            "auto",
		"orientation":            "auto",
		"percentChangeColorMode": "standard",
		"reduceOptions":          reduceOptions(),
		"showPercentChange":      false,
		"textMode":               "auto",
		"wideLayout":             true,
	}
	b.panels = append(b.panels, panel)
}

func (b *dashboardBuilder) timeseries(title, description string, x, y, w, h int, unit string, bounds object, queries ...querySpec) {
	defaults := fieldDefaults(unit, bounds, "palette-classic", nil, healthyThresholds())
	defaults["custom"] = object{
		"axisBorderShow":    false,
		"axisCenteredZero":  false,
		"axisColorMode":     "text",
		"axisLabel":         "",
		"axisPlacement":     "auto",
		"barAlignment":      0,
		"drawStyle":         "line",
		"fillOpacity":       10,
		"gradientMode":      "none",
		"hideFrom":          object{"legend": false, "tooltip": false, "viz": false},
		"insertNulls":       false,
		"lineInterpolation": "linear",
		"lineWidth":         2,
		"pointSize":         5,
		"scaleDistribution": object{"type": "linear"},
		"showPoints":        "never",
		"spanNulls":         false,
		"stacking":          object{"group": "A", "mode": "none"},
		"thresholdsStyle":   object{"mode": "off"},
	}
	panel := b.panel(title, description, "timeseries", x, y, w, h, defaults, queries)
	panel["options"] = object{
		"legend":  object{"calcs": []any{}, "displayMode": "list", "placement": "bottom", "showLegend": true},
		"tooltip": object{"hideZeros": false, "mode": "multi", "sort": "desc"},
	}
	b.panels = append(b.panels, panel)
}

func (b *dashboardBuilder) barGauge(title, description string, x, y, w, h int, unit string, bounds object, mappings, thresholds []any, queries ...querySpec) object {
	panel := b.panel(title, description, "bargauge", x, y, w, h, fieldDefaults(unit, bounds, "thresholds", mappings, thresholds), queries)
	panel["options"] = object{
		"displayMode":   "gradient",
		"legend":        object{"calcs": []any{}, "displayMode": "list", "placement": "bottom", "showLegend": false},
		"maxVizHeight":  300,
		"minVizHeight":  16,
		"minVizWidth":   8,
		"namePlacement": "auto",
		"orientation":   "horizontal",
		"reduceOptions": reduceOptions(),
		"showUnfilled":  true,
		"sizing":        "auto",
		"valueMode":     "color",
	}
	b.panels = append(b.panels, panel)
	return panel
}

func (b *dashboardBuilder) panel(title, description, panelType string, x, y, w, h int, defaults object, queries []querySpec) object {
	return object{
		"datasource":  prometheusDatasource(),
		"description": description,
		"fieldConfig": object{"defaults": defaults, "overrides": []any{}},
		"gridPos":     object{"h": h, "w": w, "x": x, "y": y},
		"id":          b.id(),
		"options":     object{},
		"targets":     targets(queries),
		"title":       title,
		"type":        panelType,
	}
}

func (b *dashboardBuilder) id() int {
	id := b.nextID
	b.nextID++
	return id
}

func targets(queries []querySpec) []any {
	result := make([]any, 0, len(queries))
	for i, query := range queries {
		result = append(result, object{
			"datasource":   prometheusDatasource(),
			"editorMode":   "code",
			"exemplar":     !query.instant,
			"expr":         query.expr,
			"instant":      query.instant,
			"interval":     query.interval,
			"legendFormat": query.legend,
			"range":        !query.instant,
			"refId":        string(rune('A' + i)),
		})
	}
	return result
}

func prometheusDatasource() object {
	return object{"type": "prometheus", "uid": datasourceUID}
}

func addDataLink(panel object, title, url string) {
	fieldConfig := panel["fieldConfig"].(object)
	defaults := fieldConfig["defaults"].(object)
	defaults["links"] = []any{object{
		"targetBlank": false,
		"title":       title,
		"url":         url,
	}}
}

func configureScrollableBarGauge(panel object) {
	options := panel["options"].(object)
	options["displayMode"] = "basic"
	options["maxVizHeight"] = 32
	options["minVizHeight"] = 24
	options["namePlacement"] = "left"
	options["sizing"] = "manual"
	options["valueMode"] = "text"
}

func fieldDefaults(unit string, bounds object, colorMode string, mappings, thresholds []any) object {
	if mappings == nil {
		mappings = []any{}
	}
	if thresholds == nil {
		thresholds = healthyThresholds()
	}
	defaults := object{
		"color":      object{"mode": colorMode},
		"mappings":   mappings,
		"thresholds": object{"mode": "absolute", "steps": thresholds},
		"unit":       unit,
	}
	for key, value := range bounds {
		defaults[key] = value
	}
	return defaults
}

func reduceOptions() object {
	return object{"calcs": []string{"lastNotNull"}, "fields": "", "values": false}
}

func rangeQuery(expr, legend string) querySpec {
	return querySpec{expr: expr, legend: legend}
}

func rateQuery(expr, legend string) querySpec {
	return querySpec{expr: expr, legend: legend, interval: rateMinStep}
}

func instant(expr, legend string) querySpec {
	return querySpec{expr: expr, legend: legend, instant: true}
}

func rateInstant(expr, legend string) querySpec {
	return querySpec{expr: expr, legend: legend, interval: rateMinStep, instant: true}
}

func binaryMappings(zero, one string) []any {
	return []any{object{
		"options": object{
			"0": object{"color": "red", "index": 0, "text": zero},
			"1": object{"color": "green", "index": 1, "text": one},
		},
		"type": "value",
	}}
}

func linkSpeedMappings() []any {
	return []any{object{
		"options": object{
			"0": object{"color": "red", "index": 0, "text": "Down"},
		},
		"type": "value",
	}}
}

func onlineThresholds() []any {
	return []any{
		object{"color": noDataColor, "value": nil},
		object{"color": "red", "value": 0},
		object{"color": "green", "value": 1},
	}
}

func problemThresholds() []any {
	return []any{
		object{"color": noDataColor, "value": nil},
		object{"color": "green", "value": 0},
		object{"color": "red", "value": 1},
	}
}

func warningThresholds() []any {
	return []any{
		object{"color": noDataColor, "value": nil},
		object{"color": "green", "value": 0},
		object{"color": "orange", "value": 1},
	}
}

func healthyThresholds() []any {
	return []any{
		object{"color": noDataColor, "value": nil},
		object{"color": "green", "value": 0},
	}
}

func binaryThresholds() []any {
	return []any{
		object{"color": noDataColor, "value": nil},
		object{"color": "red", "value": 0},
		object{"color": "green", "value": 1},
	}
}

func utilizationThresholds() []any {
	return []any{
		object{"color": noDataColor, "value": nil},
		object{"color": "green", "value": 0},
		object{"color": "yellow", "value": 70},
		object{"color": "red", "value": 90},
	}
}

func temperatureThresholds() []any {
	return []any{
		object{"color": noDataColor, "value": nil},
		object{"color": "green", "value": -100},
		object{"color": "yellow", "value": 70},
		object{"color": "red", "value": 90},
	}
}

func signalThresholds() []any {
	return []any{
		object{"color": noDataColor, "value": nil},
		object{"color": "red", "value": 0},
		object{"color": "yellow", "value": 50},
		object{"color": "green", "value": 70},
	}
}

func durationThresholds() []any {
	return []any{
		object{"color": noDataColor, "value": nil},
		object{"color": "green", "value": 0},
		object{"color": "yellow", "value": 5},
		object{"color": "red", "value": 15},
	}
}

func linkSpeedThresholds() []any {
	return []any{
		object{"color": noDataColor, "value": nil},
		object{"color": "red", "value": 0},
		object{"color": "orange", "value": 10},
		object{"color": "yellow", "value": 100},
		object{"color": "green", "value": 1000},
		object{"color": "blue", "value": 2500},
		object{"color": "purple", "value": 10000},
	}
}
