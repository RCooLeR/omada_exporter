# Grafana dashboards

These dashboards target the current `omada_exporter` Prometheus metric and label
contract. Both use Grafana's portable Classic dashboard model (`schemaVersion`
42) and selectable Prometheus datasource, scrape job, instance, and site
filters.

- [`dashboard.json`](./dashboard.json) is the comprehensive dashboard. It adds a
  device filter and covers exporter health, clients, device and radio health,
  ports, LAGs, WAN/ISP, VPN peers, controller storage, and DPI insights.
- [`simple-omada-dashboard.json`](./simple-omada-dashboard.json) is the compact
  health and traffic overview.

## Import

1. In Grafana, choose **Dashboards → New → Import**.
2. Upload one of the JSON files.
3. Select the Prometheus datasource and exporter job/instance.
4. Choose one or more sites. The comprehensive dashboard can also filter by
   device MAC.

The dashboards refresh every 30 seconds, matching the scrape interval in the
[Prometheus setup example](../prometheus.md). Rate panels use
`$__rate_interval`, so their sampling window adapts to the selected time range.

Some comprehensive panels intentionally show no data when their collector is
disabled or unsupported by the controller. Per-client, per-port, and DPI panels
depend on `OMADA_TRACK_CLIENT_METRICS`, `OMADA_TRACK_PORT_METRICS`, and
`OMADA_TRACK_INSIGHT_METRICS`, respectively.

## Maintenance

The JSON files are generated from [`generate.go`](./generate.go). From the
`bridge` directory, regenerate both with:

```shell
go generate ./docs/dashboards
```

`go test ./...` validates panel metadata, datasource portability, variables,
metric names, selector and legend labels, and adaptive Prometheus rate windows
against the descriptors emitted by the current collectors. CI also runs
Grafana's pinned `dashboard-linter`; [`.lint`](./.lint) documents the one
intentional exception that imported dashboards remain editable.
