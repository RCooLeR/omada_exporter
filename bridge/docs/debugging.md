# API response diagnostics

Response dumping is an optional, disabled-by-default diagnostic mode. It captures
raw responses from the existing Web API and OpenAPI status, device, port, client,
VPN, firmware-information, and health queries. It does not change controller
configuration, start an upgrade, or modify devices. The two POST requests are
read-only alert-count and client-list queries.

Use your normal controller credentials and add these flags for a one-shot run:

```sh
omada-exporter --dump-responses-dir /private/omada-dumps --dump-responses-only
```

Alternatively, set `OMADA_DUMP_RESPONSES_DIR` and
`OMADA_DUMP_RESPONSES_ONLY=true`. Without `--dump-responses-only`, the bridge
captures once at startup, then starts normal monitoring. Dump-only requires a
nonblank output directory and exits before starting HTTP or MQTT publishing.
Remove the settings after diagnosis to avoid collecting more snapshots on each
container restart. A container needs a writable mount for the chosen directory.

Each run creates a unique `omada-<UTC timestamp>-<random suffix>` subdirectory.
Earlier runs are not overwritten. New directories use mode `0700` and files use
`0600` on platforms supporting POSIX permissions; existing parent permissions are
not changed. On Windows, restrict the directory's ACL yourself. `manifest.json`
lists the queried controller/site context, 24-hour health window, response files,
and number of failed endpoints. Each response file records the request method,
URL, query body (when present), HTTP status, retrieval time, and raw response or
error. JSON integers retain their original precision. Responses are limited to
32 MiB each.

Some exploratory endpoints are unsupported on particular controller versions,
models, authentication modes, or accounts. These failures are recorded and the
remaining queries continue. A completed dump with failed endpoints is still a
successful diagnostic capture, not a declaration that every API is supported.
OpenAPI queries record errors when OpenAPI authentication is disabled. Device
inventory must be valid; invalid inventory, cancellation, and output-file errors
stop the run. A partial manifest is written where possible. Grid queries capture
the original first page, with up to 1,000 rows; this is not a complete controller
backup or an exhaustive API audit.

**Treat all dump files as private.** Raw responses can contain device/client
identifiers, IP/MAC addresses, names, topology, VPN configuration, or other
sensitive values. Request authentication headers are not included, but response
payloads are not redacted. Review and redact files before sharing them; do not
commit them to Git, publish them, or store them in a web-served directory.
