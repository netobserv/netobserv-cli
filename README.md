# Network Observability CLI

network-observability-cli is a lightweight Flow, Packet and Metrics visualization tool.
It deploys [NetObserv eBPF agent](https://github.com/netobserv/netobserv-ebpf-agent) on your k8s cluster to collect flows or packets from nodes network interfaces
and streams data to a local collector for analysis and visualization.
Output files are generated under `output/flow` and `output/pcap` directories per host name.

On Openshift environments, you can also capture metrics in your monitoring stack and display a fully configured dashboard.

## Prerequisites

To run this CLI, you will need:
- A running kubernetes / OpenShift cluster
- A kubeconfig with valid cluster credentials (`KUBECONFIG` or `~/.kube/config`)
- Cluster admin rights

## Standalone Go CLI

`oc-netobserv` is a native Go executable. Running it directly does not require
`oc`, `kubectl`, Bash, or `yq` on your machine. It uses the Kubernetes Go client
libraries and your kubeconfig. If your kubeconfig uses an external credential
plugin, that credential plugin must still be available.

Build and run it with Go (the version in `go.mod`):

```sh
make oc-commands
./build/oc-netobserv flows --protocol=TCP --port=8080
./build/oc-netobserv packets --port=443 --background
./build/oc-netobserv follow
./build/oc-netobserv copy
./build/oc-netobserv cleanup
```

Existing commands, capture flags, `or` filter groups, image overrides, and
`NETOBSERV_NAMESPACE` remain supported. Optional `--kubeconfig`, `--context`,
`--namespace`, and `--output-dir` arguments can be supplied after the command.
Use the same connection and namespace arguments for follow-up commands.
`--yaml` generates manifests without contacting the cluster, unless
`--get-subnets` explicitly requests cluster network information.

`make kubectl-commands` builds the same executable as `kubectl-netobserv`.
Installing either binary on `PATH` also allows the existing `oc netobserv` or
`kubectl netobserv` invocation when those tools are installed.
Release archives are specific to the OS and architecture; `make release-all`
builds Linux amd64/arm64/ppc64le/s390x and macOS amd64/arm64 artifacts.

## Getting started with Krew

The CLI is available as a [Krew](https://krew.sigs.k8s.io/) plugin, which is the fastest way to get it up and running if you already use Krew.
If not, please refer to its [installation guide](https://krew.sigs.k8s.io/docs/user-guide/setup/install/).

To install the NetObserv CLI, run:

```bash
kubectl krew install netobserv
```

Or if you had an older version, to update it:

```bash
kubectl krew update netobserv
```

Then, run it as a `kubectl` plugin, such as:

```bash
kubectl netobserv --version
# output example: 
# NetObserv CLI version v0.0.8
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

  Run the following command to start capturing flows, replacing `USER`, `VERSION` and `COMMAND_ARGS` accordingly:

  ```bash
  USER=netobserv VERSION=dev COMMAND_ARGS=--interfaces=br-ex make flows
  ```

![flows](./img/flow-table.png)

It will display a table view with latest flows collected and write data under output/flow directory.
To stop capturing press Ctrl-C.

This will write data into two separate files:
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

  Run the following command to start capturing packets, replacing `USER`, `VERSION` and `COMMAND_ARGS` accordingly:

  ```bash
  USER=netobserv VERSION=dev COMMAND_ARGS="--protocol=TCP --port=80" make packets
  ```

Similarly to flow capture, it will display a table view with latest flows. However, it will collect packets and write data under output/pcap directory.
To stop capturing press Ctrl-C.

This will write [pcapng](https://wiki.wireshark.org/Development/PcapNg) into a single file located in `./output/pcap/<CAPTURE_DATE_TIME>.pcapng` that can be opened with Wireshark for example:

![wireshark](./img/wireshark.png)

We use the pcapng format to add contextual metadata, such as the k8s pods and service names.

### Metrics dashboard (OpenShift only)

For instance, to capture many available metrics, including Packet drops, DNS stats and latenties:

- As a `kubectl` plugin:

  ```bash
  kubectl netobserv metrics --enable_pkt_drop --enable_dns --enable_rtt
  ```

- Or from the repository `Makefile`:

  Run the following command to start capturing metrics, replacing `USER`, `VERSION` and `COMMAND_ARGS` accordingly:

  ```bash
  USER=netobserv VERSION=dev COMMAND_ARGS='--enable_pkt_drop --enable_dns --enable_rtt' make metrics
  ```

![metrics](./img/metrics-dashboard.png)

It will generate a monitoring dashboard called "NetObserv / On Demand" in your Openshift cluster.
The url to access it is automatically generated from the CLI. Simply click on the link to open the page.

### Cleanup

The `cleanup` function will automatically remove the eBPF programs when the CLI exits. However you may need to run it manually if running in background or an error occurs.

- As a `kubectl` plugin:

  ```bash
  kubectl netobserv cleanup
  ```

- Or from the repository `Makefile`:

  ```bash
  USER=netobserv VERSION=dev make cleanup
  ```

## Build

To build the project locally:

### Install `shellcheck` package

```bash
sudo dnf install -y shellcheck
```

### Build the project

```bash
make build
```

This will also copy resources and commands to the `build` directory.

### Images

To build your own images of CLI, run the following command replacing `USER` and `VERSION` accordingly:
```bash
USER=netobserv VERSION=dev make images
```

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
