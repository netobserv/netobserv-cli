# Network Observability CLI

network-observability-cli is a lightweight flow, packet, and metrics visualization tool.
It deploys the [NetObserv eBPF agent](https://github.com/netobserv/netobserv-ebpf-agent) on your Kubernetes cluster and streams data to a collector for analysis and visualization.
Flow and packet files are copied to local `output/flow`, `output/pcap`, and (when TLS plaintext capture is enabled) `output/plaintext` directories.

On OpenShift environments, you can also capture metrics in the monitoring stack and display a preconfigured dashboard.

## Prerequisites

To run this CLI, you will need:
- A running Kubernetes or OpenShift cluster
- Either `oc` or `kubectl` installed and connected to your cluster
- Cluster admin rights

## Getting started with Krew

The CLI is available as a [Krew](https://krew.sigs.k8s.io/) plugin, which is the fastest way to get it up and running if you already use Krew.
If not, please refer to its [installation guide](https://krew.sigs.k8s.io/docs/user-guide/setup/install/).

To install the NetObserv CLI, run:

```bash
kubectl krew install netobserv
```

To update the Krew index and upgrade an existing installation:

```bash
kubectl krew update
kubectl krew upgrade netobserv
```

Then, run it as a `kubectl` plugin, such as:

```bash
kubectl netobserv version
# output example: 
# NetObserv CLI version main
```

You can get detailed help using these commands:

```bash
kubectl netobserv --help
kubectl netobserv flows --help
kubectl netobserv packets --help
kubectl netobserv metrics --help
```

## Run

### Flow Capture

For instance, to capture all flows on the cluster that go through the `br-ex` network interface:

- As a `kubectl` plugin:

  ```bash
  kubectl netobserv flows --interfaces=br-ex
  ```

- Or from the repository `Makefile`:

  Run the following command to start capturing flows, replacing `IMAGE_ORG`, `VERSION` and `COMMAND_ARGS` accordingly:

  ```bash
  IMAGE_ORG=netobserv VERSION=dev COMMAND_ARGS=--interfaces=br-ex make flows
  ```

![flows](./img/flow-table.png)

It displays a live table of recently collected flows. By default, the CLI asks whether to copy the capture output locally when the capture ends; use `--copy=true` to enable this without prompting or `--copy=false` to skip it. Press Ctrl-C to stop a foreground capture.

When copied, a flow capture produces two files:
- `./output/flow/<CAPTURE_DATE_TIME>.json` containing json array of received data such as:
```json
{
  "AgentIP": "10.0.1.76",
  "Bytes": 561,
  "DnsErrno": 0,
  "Dscp": 20,
  "DstAddr": "f904:ece9:ba63:6ac7:8018:1e5:7130:0",
  "DstMac": "0A:58:0A:80:00:37",
  "DstPort": 9999,
  "Duplicate": false,
  "Etype": 2048,
  "Flags": 16,
  "FlowDirection": 0,
  "IfDirection": 0,
  "Interface": "ens5",
  "K8S_FlowLayer": "infra",
  "Packets": 1,
  "Proto": 6,
  "SrcAddr": "3e06:6c10:6440:2:a80:37:b756:270f",
  "SrcMac": "0A:58:0A:80:00:01",
  "SrcPort": 46934,
  "TimeFlowEndMs": 1709741962111,
  "TimeFlowRttNs": 121000,
  "TimeFlowStartMs": 1709741962111,
  "TimeReceived": 1709741964
}
```
- `./output/flow/<CAPTURE_DATE_TIME>.db` database that can be inspected using `sqlite3` for example: 
```bash
bash-5.1$ sqlite3 ./output/flow/<CAPTURE_DATE_TIME>.db 
SQLite version 3.34.1 2021-01-20 14:10:07
Enter ".help" for usage hints.
sqlite> SELECT DnsLatencyMs, DnsFlagsResponseCode, DnsId, DstAddr, DstPort, Interface, Proto, SrcAddr, SrcPort, Bytes, Packets FROM flow WHERE DnsLatencyMs >10 LIMIT 10;
12|NoError|58747|10.128.0.63|57856||17|172.30.0.10|53|284|1
11|NoError|20486|10.128.0.52|56575||17|169.254.169.254|53|225|1
11|NoError|59544|10.128.0.103|51089||17|172.30.0.10|53|307|1
13|NoError|32519|10.128.0.52|55241||17|169.254.169.254|53|254|1
12|NoError|32519|10.0.0.3|55241||17|169.254.169.254|53|254|1
15|NoError|57673|10.128.0.19|59051||17|172.30.0.10|53|313|1
13|NoError|35652|10.0.0.3|46532||17|169.254.169.254|53|183|1
32|NoError|37326|10.0.0.3|52718||17|169.254.169.254|53|169|1
14|NoError|14530|10.0.0.3|58203||17|169.254.169.254|53|246|1
15|NoError|40548|10.0.0.3|45933||17|169.254.169.254|53|174|1
```
or `dbeaver`:
![dbeaver](./img/dbeaver.png)


### Packet Capture

For instance, to capture all TCP packets on the cluster that go through port 80:

- As a `kubectl` plugin:

  ```bash
  kubectl netobserv packets --protocol=TCP --port=80
  ```

- Or from the repository `Makefile`:

  Run the following command to start capturing packets, replacing `IMAGE_ORG`, `VERSION` and `COMMAND_ARGS` accordingly:

  ```bash
  IMAGE_ORG=netobserv VERSION=dev COMMAND_ARGS="--protocol=TCP --port=80" make packets
  ```

It displays a live packet table while collecting packets. To stop a foreground capture, press Ctrl-C. Packet capture requires at least one filter; this avoids an unbounded capture.

When copied, the capture writes a [pcapng](https://wiki.wireshark.org/Development/PcapNg) file to `./output/pcap/<CAPTURE_DATE_TIME>.pcapng`, which can be opened with Wireshark:

![wireshark](./img/wireshark.png)

We use the pcapng format to add contextual metadata, such as the k8s pods and service names.

With `--enable_openssl`, the CLI correlates plaintext events with wire packets
using tuples, endpoints, direction, timing, payload and capture filters. Complete
kernel-captured tuples are deterministic; partial or legacy records use
conservative heuristics and may remain unannotated (`PcapAnnotated: false`) when
the match is ambiguous. An annotation identifies the connection, not an exact
plaintext-to-packet byte mapping.

### Metrics dashboard (OpenShift only)

For instance, to capture metrics including packet drops, DNS statistics, and TCP RTT:

- As a `kubectl` plugin:

  ```bash
  kubectl netobserv metrics --enable_pkt_drop --enable_dns --enable_rtt
  ```

- Or from a locally built plugin:

  ```bash
  ./build/oc-netobserv metrics --enable_pkt_drop --enable_dns --enable_rtt
  ```

![metrics](./img/metrics-dashboard.png)

It generates a monitoring dashboard called `NetObserv / On Demand` in your OpenShift cluster and prints its URL.

### Cleanup

The `cleanup` function will automatically remove the eBPF programs when the CLI exits. However you may need to run it manually if running in background or an error occurs.

- As a `kubectl` plugin:

  ```bash
  kubectl netobserv cleanup
  ```

- Or from the repository `Makefile`:

  ```bash
  IMAGE_ORG=netobserv VERSION=dev make cleanup
  ```

## Build

To check and build both project binaries locally:

### Build collector and plugin

```bash
make build
```

This formats and lints the repository, refreshes the vendor directory, and builds both the collector (`build/network-observability-cli`) and the client plugin.

To build only one binary:

```bash
make build-collector              # build/network-observability-cli
make build-plugin K8S_CLI_BIN=oc  # build/oc-netobserv
```

### Build the client plugin

```bash
make oc-commands       # or: make kubectl-commands
```

These compatibility targets build the `oc-netobserv`/`kubectl-netobserv` client plugin binary into the `build` directory. `make commands` remains available for CI and scripts.

### Regenerate the CLI reference

The AsciiDoc command reference is generated from the live Cobra command tree:

```bash
make docs
```

### Tests

Run the unit tests and generate `cover.out` with:

```bash
make test
```

Cluster integration tests are separate and require a configured cluster and an installed `oc-netobserv` binary:

```bash
KUBECONFIG=/path/to/kubeconfig make tests-int
```

### Images

To build your own CLI image, run the following command replacing `IMAGE_ORG` and `VERSION` accordingly:
```bash
IMAGE_ORG=netobserv VERSION=dev make images
```

`make images` builds one collector image per `MULTIARCH_TARGETS` architecture and publishes a manifest. Local builds default to `amd64`; use `MULTIARCH_TARGETS="amd64 arm64"` for a multi-architecture build. The release workflows additionally build `ppc64le` and `s390x`.

### Krew release artifacts

Krew releases contain one client plugin archive per supported operating system and architecture:

```bash
make release
# release-assets/netobserv-cli-<VERSION>-linux-amd64.tar.gz
# release-assets/netobserv-cli-<VERSION>-linux-arm64.tar.gz
# release-assets/netobserv-cli-<VERSION>-darwin-amd64.tar.gz
# release-assets/netobserv-cli-<VERSION>-darwin-arm64.tar.gz
```

The plugin is cross-compiled with `CGO_ENABLED=0`; the collector remains in the multi-architecture container image.

## Extending OpenShift or Kubernetes CLI with plugins

You can add this plugin to your favorite `oc` or `kubectl` commands using the following steps:

```bash
K8S_CLI_BIN=oc make install-commands
```
OR 
```bash
K8S_CLI_BIN=kubectl make install-commands
```

This will add `netobserv` commands to your CLI.
You can verify the commands are available using:

```bash
oc plugin list
```
OR
```bash
kubectl plugin list
```

It will display as result:

```bash
The following compatible plugins are available:
...
/usr/bin/<oc|kubectl>-netobserv
```

More info [on official OpenShift documentation](https://docs.openshift.com/container-platform/4.14/cli_reference/openshift_cli/extending-cli-plugins.html).
