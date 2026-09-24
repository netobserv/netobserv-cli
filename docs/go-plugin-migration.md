# Standalone Go plugin compatibility

The local plugin is built from `cmd/oc-netobserv`. The existing root binary and
`cmd` package remain the in-cluster collector. Cluster access follows the
operator's live-client pattern: Kubernetes typed clients, dynamic clients for
platform APIs, and API discovery. Pod execution and copying use client-go
WebSocket/SPDY streams; local shell commands are not used.

## Preserved interface

| Area | Commands or options |
| --- | --- |
| Capture | `flows`, `packets`, `metrics` |
| Lifecycle | `follow`, `stop`, `copy`, `cleanup` |
| Information | no arguments, `help`, `--help`, command-specific help, `version`, `--version` |
| Execution | `background`, `copy`, `yaml`, `log-level`, `max-time`, `max-bytes` |
| Features | `enable_all`, `enable_dns`, `enable_ipsec`, `enable_network_events`, `enable_pkt_translation`, `enable_pkt_drop`, `enable_rtt`, `enable_udn_mapping`, `privileged`, `get-subnets`, `sampling` |
| Interfaces and nodes | `interfaces`, `exclude_interfaces`, `node-selector` |
| Filters | `direction`, `cidr`, `protocol`, `sport`, `dport`, `port`, `sport_range`, `dport_range`, `port_range`, `sports`, `dports`, `ports`, `icmp_type`, `icmp_code`, `peer_ip`, `peer_cidr`, `action`, `tcp_flags`, `drops`, `query`, ordered `or` groups |
| Metrics selection | `include_list` |
| Environment | `NETOBSERV_NAMESPACE`, `NETOBSERV_COLLECTOR_IMAGE`, `NETOBSERV_AGENT_IMAGE`; existing test controls `isE2E`, `copy`, `runBackground`, `outputYAML`, `dateName` |

Both `--key=value` and the existing bare `key=value` spelling work. Boolean
switches accept bare switches and explicit `=true`/`=false`. The original option
sequence, including repeated filters and `or`, is forwarded to the collector.
Capture defaults remain five minutes / 50,000,000 bytes for flows and packets,
one hour for metrics, foreground execution, and prompting before copying.
Packet captures still require an eBPF filter. Feature flags preserve the shell's
additive behavior, including the explicit `enable_ipsec=false` override.

The existing manifests and pipeline JSON are embedded from `res`. OpenShift
collector captures retain serving certificates, service CA injection, and the
`resolve-tls` init container. Agents start after the collector is ready so its
TLS ConfigMap exists. Metrics retain the existing ServiceMonitor and dashboard.
YAML mode retains the agent/resource file plus printed collector deployment
instructions, and requires no cluster connection unless subnets are requested.

The optional `--kubeconfig`, `--context`, `--namespace`, and `--output-dir`
arguments select the connection and local destination. Authentication follows
kubeconfig, including external credential plugins and OIDC refresh.

Capture namespaces are created exclusively. New resources have namespace owner
references so cluster-scoped resources and dashboards are garbage-collected.
Legacy RBAC left by shell captures is reconciled, but resources owned by another
capture are not overwritten. Failed file copies retain the capture for retry.
Explicit cleanup accepts the dedicated `netobserv-cli` namespace even without
the capture label, preserving shell compatibility for manually created namespaces.
Custom namespaces must carry `app=netobserv-cli` to be removed.

## Verification

`e2e/testdata` contains DaemonSet fixtures produced by the
original Bash implementation. Unit tests compare complete agent specifications,
normalizing only embedded JSON whitespace. They cover filters, features, repeated
switches, queries, and metrics selection. Separate tests cover collector argument
forwarding, help, TLS/subnets, cleanup, legacy RBAC, and safe output extraction.

Run the tests without a cluster:

```sh
KUBECONFIG= go test -mod=vendor ./...
```

The historical fixtures and flag lists are fixed compatibility references saved
before removing the Bash capture implementation. They are not regenerated from
the Go implementation. Intentional behavior changes require explicit fixture
review. The legacy entry point and its helper/injection scripts have been
removed; independent documentation, config-update, and cluster-setup scripts
remain. Documentation now reads help from the compiled Go binary.

`make commands`, `make oc-commands`, and `make kubectl-commands` build Go binaries.
Release archives and Krew selectors are OS/architecture specific.

Before release, run the existing Kind/OpenShift capture integration suites to
exercise actual eBPF capture, terminal interaction, TLS, copying, and metrics.
Unit tests do not replace those live-cluster checks.

## Live validation (2026-09-23)

Validated against an OpenShift 5.0 nightly cluster, restricting captures to one
worker. The standalone plugin ran with `PATH=/nonexistent`, so local `oc`,
`kubectl`, Bash, and yq were unavailable to it. Final cluster and Prometheus
checks used Go clients directly.

- TLS flow capture produced 573 JSON records, all with sampling 1, and a valid
  SQLite database. Background logs, copy, automatic byte-limit stopping, and
  cleanup succeeded.
- Foreground packet capture with a terminal produced a readable PCAP containing
  567 packets; every packet matched TCP port 6443. Headless operation uses the
  existing background mode; the collector's interactive display needs a terminal.
- Metrics deployment created its ServiceMonitor and dashboard, resolved three
  subnet groups, and produced seven node-egress metric series in Prometheus.
  The collector reported nonzero traffic rates. Stop and cleanup succeeded.
- Copying revealed a trailing-tar-padding/stream-close warning. The Go copy path
  now drains the stream before closing it, with a regression test; subsequent
  packet copying completed without the warning.

These are targeted live smoke tests, not a complete run of every integration
scenario or a guarantee for all supported cluster versions and architectures.

## Full OpenShift integration suite (2026-09-23)

The existing nine-spec `e2e/integration-tests` suite passed against the native Go
plugin: **9 passed, 0 failed, 0 skipped**, in 608.558 seconds. It covered deployment
across all six nodes, port-8080 packet capture, namespace regex filtering,
sampling 1, interface exclusion, dashboard/Prometheus metrics, and default,
explicit, and packet-drop privilege settings.

Tests used the supplied kubeconfig and `NETOBSERV_NAMESPACE=netobserv-go-integration`.
The compiled test executable ran from a separate scratch directory with a PATH
containing only the native `oc-netobserv` binary. No `oc` or `kubectl` was available.
The harness now uses `exec.LookPath`, honors `NETOBSERV_NAMESPACE`, and propagates
command exit errors. Existing test commands and assertions were retained.

Go-client checks confirmed removal of the test namespace, owned cluster-scoped
RBAC/SCC resources, and dashboard while preserving the original `netobserv-cli`
namespace. Repository Go tests and lint also passed. The separate Kind suite and
other cluster versions/architectures were not run.

## Capture lifecycle and download isolation

Flow and packet duration limits use a timer independent of incoming records.
Empty captures finish at the deadline, closing SQLite/text output and flushing
PCAPNG headers before completion is signaled. Detached captures stop collection
and retain the collector for downloading, as before.

Metrics dashboards in `openshift-config-managed` use the capture namespace as
the ConfigMap name and remain owned by that namespace. Separate capture
namespaces can therefore create and clean up dashboards independently; the CLI
prints the URL for the corresponding dashboard.

`oc-netobserv serve` defaults to `127.0.0.1:8080`. The operator explicitly uses
this loopback address. Download via Kubernetes port forwarding, which requires
API authentication and `pods/portforward` authorization. Direct pod-IP and pod
proxy downloads are unavailable. Overriding `--listen` with a non-loopback
address exposes an unauthenticated HTTP server and requires external access
controls.

Concurrent metrics captures reserve separate host ports in the range 9401–9500
using namespace-owned ConfigMaps in `openshift-config-managed`. Atomic creation
prevents captures from choosing the same port; ports already requested by pods
are skipped. The agent listener and Service use the reserved port together.
Reservations are garbage-collected with the capture namespace. This bounds the
pool to 100 captures and does not detect host processes outside Kubernetes.
Offline YAML retains the default port; change it consistently before manually
applying multiple metrics manifests on the same nodes.
