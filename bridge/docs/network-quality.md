# Network quality

AP radio interference is part of the existing device collector. No new API calls or settings are needed. `omada_ap_interference_percentage` is emitted per online AP and supported radio (`radio_band`: `2.4`, `5`, `5-2`, `6`). A real zero is retained; absent/null, negative, out-of-range and offline readings are omitted.

## Enable AP uplink and WAN quality collection

Add these variables to the bridge container's **environment** (a Compose `.env` file alone does not pass variables into the container unless referenced by the Compose configuration):

```yaml
environment:
  OMADA_TRACK_NETWORK_QUALITY_METRICS: "true"
  OMADA_NETWORK_QUALITY_WINDOW_SECONDS: "86400"
```

Equivalent CLI flags are `--track-network-quality-metrics --network-quality-window-seconds 86400`. These metrics are disabled by default because they require extra OpenAPI requests and controller/device support. The window must be positive. The collector is registered at `/metrics/network-quality` and included in `/metrics`; it also flows through MQTT Discovery when publishing is enabled. Home Assistant sensors retain separate identities for each uplink direction and radio band. No custom-card or dashboard changes are required to collect the metrics.

The collector shares the existing authenticated client, request cache (`OMADA_CACHE_TTL`), and device inventory. It makes one additional request per connected, wired-backhaul AP and one per connected gateway, with a maximum of four concurrent device queries. Reading these endpoints does not change controller configuration.

## Meaning and availability

| Measurements | Source | Meaning |
| --- | --- | --- |
| `omada_ap_uplink_{packets,dropped_packets,error_packets}_total` | OpenAPI AP `wired-uplink` | Cumulative controller-reported packet counters, split by `direction=up/down` as named by Omada. Reset lifetime is not specified by the API; counters can reset and must not be treated as a persistent lifetime total. |
| `omada_gateway_wan_quality_latency_average_seconds`, `omada_gateway_wan_quality_jitter_average_seconds` | Gateway `health/detail` | Gateway-wide averages over the requested window; milliseconds from Omada are converted to seconds. These are not instantaneous measurements or per-WAN-interface readings. |
| `omada_gateway_wan_quality_packet_loss_average_percentage`, `omada_gateway_wan_quality_mos_average` | Gateway `health/detail` | Windowed packet-loss percentage and mean opinion score (MOS), not counters. |
| `omada_gateway_wan_quality_window_seconds` | Bridge configuration | Window used for available gateway quality readings; 24 hours by default. |

The WAN query uses epoch milliseconds and exports only supported, present, finite, nonnegative measurements. Packet loss is restricted to 0–100 and MOS to 0–5. Historical arrays are not replayed into Prometheus; each scrape publishes the API's window averages at scrape time. Existing instantaneous, per-interface `omada_wan_latency` remains available in milliseconds.

`omada_network_quality_scrape_success{source="inventory|ap_uplink|wan"}` reports whether an attempted query succeeded. `omada_network_quality_available{source="ap_uplink|wan"}` reports whether at least one valid value was returned. Empty/unsupported payloads never become healthy-looking zero traffic or perfect quality. Offline devices and wireless-backhaul AP uplinks are skipped (availability 0, no query-success sample because no query was attempted). Legitimate zero drop/error/loss measurements are emitted. A failed quality request does not prevent other devices or existing collectors from exporting their data.

Support varies by controller, device, firmware and permissions. Validated on OC220 firmware 1.7.11 / Controller 6.3.0.45. Unsupported HTTP/API responses produce query-success 0 and availability 0; unsupported/empty successful payloads produce query-success 1 and availability 0. Client health scores and incident telemetry are not part of this collector.

## Example PromQL

The updated Home Assistant network card displays radio interference and cumulative wired-uplink counters in the selected AP's detail panel, and rolling WAN quality averages (including their configured window) in the selected gateway's panel. Missing fields stay unavailable; these values are not converted into synthetic rates or instantaneous quality scores. See [card configuration](../../ha-cards/docs/cards.md#ap-and-gateway-network-quality).

Downstream AP drops per second (use a range covering several scrapes):

```promql
rate(omada_ap_uplink_dropped_packets_total{direction="down"}[5m])
```

Current AP radio interference, in percent:

```promql
omada_ap_interference_percentage
```

Rolling-average WAN jitter displayed in milliseconds:

```promql
1000 * omada_gateway_wan_quality_jitter_average_seconds
```

Do not apply `rate()` to WAN averages or interference gauges. Use availability/query-success to distinguish missing support from real zero values. When displaying a WAN quality panel, show its configured window so a 24-hour average is not mistaken for a current-link alert.
