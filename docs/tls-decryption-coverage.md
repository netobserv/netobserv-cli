# TLS Decryption Coverage Matrix

Reference for NetObserv CLI on-demand packet capture with TLS plaintext visibility on OpenShift.

## Approaches

| Approach | Coverage on OpenShift | Requires app changes | CLI flag | Agent env |
|----------|----------------------|----------------------|----------|-----------|
| PCA wire capture | Cleartext HTTP only | No | (default in packets mode) | `ENABLE_PCA=true` |
| OpenSSL uprobes | Apps using libssl (nginx, curl, Python, Ruby, PHP) | No | `--enable_openssl` | `ENABLE_OPENSSL_TRACKING=true` |
| GoTLS uprobes | Go binaries (`crypto/tls`) | No | `--enable_gotls` | `ENABLE_GOTLS_TRACKING=true` (write path only by default) |
| kTLS sk_msg | Kernel TLS offload (nginx+kTLS, niche) | No | `--enable_ktls` | `ENABLE_KTLS_TRACKING=true` |

Test workloads: [openssl-test-pod](../examples/openssl-test-pod/) (`--enable_openssl`), [gotls-test-pod](../examples/gotls-test-pod/) (`--enable_gotls`), [ktls-test-pod](../examples/ktls-test-pod/) (`--enable_ktls`).
| SSLKEYLOGFILE + pcapng DSB | Any TLS when customer sets env var | Yes | `--tls-keylog` | (none) |

## OpenShift scenarios

| Scenario | Wire PCAP | Recommended path |
|----------|-----------|------------------|
| HTTP on port 80 | Readable | PCA |
| Route terminates TLS, pod sees HTTP | Readable | PCA |
| Pod HTTPS (OpenSSL) | Encrypted | OpenSSL uprobes (`--enable_openssl`); test workload: [examples/openssl-test-pod](../examples/openssl-test-pod/) |
| Go microservice | Encrypted | GoTLS uprobes (`--enable_gotls`) |
| Mixed OpenSSL + Go workloads | Encrypted | Both flags together (`--enable_openssl --enable_gotls`) |
| Service mesh mTLS (Istio/CSM) | Encrypted | BoringSSL on Envoy (future) |
| Customer can set `SSLKEYLOGFILE` | Encrypted | CLI `--tls-keylog` |
| nginx with kTLS | Encrypted on wire | kTLS tracking (`--enable_ktls`); test workload: [examples/ktls-test-pod](../examples/ktls-test-pod/) |

## CLI outputs

| File | Path | Contents |
|------|------|----------|
| Wire capture | `./output/pcap/<timestamp>.pcapng` | Encrypted TLS on the wire; when a plaintext event matches a buffered frame (5-tuple + time), `PlaintextPreview` is appended as an EPB comment on that **real** packet |
| Plaintext sidecar | `./output/plaintext/<timestamp>.jsonl` | One JSON object per TLS plaintext event (when TLS flags enabled) |

Plaintext JSONL fields: `RecordType`, `PacketID`, `PcapAnnotated`, `Time`, `TimeFlowStartMs`, `Pid`, `Tgid`, `Direction`, `TLSSource`, `Plaintext` (base64), `PlaintextLen`, `PlaintextPreview`, `SSLType`, and when available `SrcAddr`, `DstAddr`, `SrcPort`, `DstPort`, `Protocol`.

Deploy a collector image that includes the TLS correlation code. The default `quay.io/netobserv/network-observability-cli:main` image does not yet ship this feature; build locally and set `NETOBSERV_COLLECTOR_IMAGE` before running `oc netobserv packets`.

`PcapAnnotated` is `true` when the CLI matched the event to a wire packet (strict 5-tuple + time). When multiple workloads share a capture port, port-only correlation is refused; use `--peer_ip` or rely on agent socket-fd 5-tuple enrichment.

## Flow filters vs plaintext

`--port`, `--peer_ip`, `--peer_cidr`, and other `FLOW_FILTER_RULES` apply to **pcapng wire packets** (PCA / TC hook).

For plaintext, the agent applies matching rules via `PlaintextScope`:

| Filter | Wire (pcapng) | Plaintext JSONL | Uprobe discovery |
|--------|---------------|-----------------|------------------|
| `--peer_ip` / `--peer_cidr` | Yes | Yes (PID scope + 5-tuple match) | Yes (limits which PIDs are hooked) |
| `--port` | Yes | Yes (when 5-tuple enriched) | No (OpenSSL hooks per libssl path, not per-PID) |
| Port-only, no peer IP | Yes | Yes (port match on enriched 5-tuple) | No |

PID scope refreshes every 5s by scanning `/proc/<pid>/net/tcp` for sockets involving the peer IP/CIDR. Override with `TLS_PLAINTEXT_PID_ALLOWLIST` (comma-separated PIDs).

Duplicate plaintext events (same PID, direction, payload prefix) are dropped within `TLS_PLAINTEXT_DEDUP_WINDOW` (default 500ms).

Set `TLS_PLAINTEXT_MIN_BYTES` on the agent (default `0`) to drop short TLS fragments before export. For on-demand CLI captures, `4` or `8` removes most WebSocket/binary framing noise (~18–37% of events in typical router HTTPS traffic) while keeping readable HTTP payloads.

CLI: `--tls_plaintext_min_bytes=4` (packets mode only) sets the agent env on the capture DaemonSet.

`PlaintextPreview` length defaults to 256 bytes. Set `--tls_plaintext_preview_bytes=0` for the full captured payload in preview (max 16 KiB per event), or another positive value to customize. The TUI also decodes the full base64 `Plaintext` field when the preview is shorter than `PlaintextLen`.

## OpenShift deployment requirements

TLS plaintext capture requires elevated privileges on the agent DaemonSet:

- `--privileged` or `SYS_PTRACE` capability
- `hostPID: true` on the **pod template** (`spec.template.spec.hostPID`) for per-process library discovery
- Host `/proc` access for libssl path scanning

Agent mounts host `/usr`, `/lib`, `/lib64` under `/host/` when using `--enable_openssl` so uprobes can open discovered `libssl.so` paths. Container workloads are hooked via `/proc/<pid>/root/.../libssl.so` (same path in maps, different inode than host libssl).

## Limitations

- **5-tuple enrichment** — OpenSSL/GoTLS resolve the socket **fd** (`SSL_set_fd` map + `SSL*` BIO fallback; GoTLS `*tls.Conn` walk), map fd→inode via `/proc/<pid>/fd`, then match `/proc/net/tcp`. kTLS carries the kernel 5-tuple in the eBPF event. Falls back to netns IP + connection affinity when fd is unknown.
- **No K8s pod metadata on plaintext** — FLP enrichment keys off `SrcAddr`/`DstAddr`; pod/namespace labels require future enrichment once 5-tuple is reliable
- Java JSSE (non-OpenSSL) is not covered by OpenSSL uprobes
- Statically linked OpenSSL may need explicit library path via `OPENSSL_PATH`
- GoTLS auto-discovers Go executables from `/proc/*/exe` and resolves `writeRecordLocked` / `Read` offsets from `.gopclntab` (Go 1.17+ register ABI)
- GoTLS auto-discovery skips node infrastructure binaries (kubelet, crio, ovnkube, multus, console, host `/usr/bin/kube-*`, `/usr/bin/openshift-*`); OpenSSL discovery uses the same skip list and only attaches per-container `libssl.so` under `/proc/<pid>/root` (never the host default `OPENSSL_PATH`, which would hook every process on the node). **Hard-denied** node/CNI binaries (kubelet, crio, multus, ovnkube, …) are never hooked, even with `--peer_ip`. **Soft-excluded** workloads (e.g. openshift-console) are skipped in broad scans but allowed when `--peer_ip` / `--peer_cidr` scopes discovery to that PID.
- Recommended with TLS plaintext flags: `--peer_ip=<pod-ip>` (or `--peer_cidr`) **and** `--port=<service-port>`. They solve different problems:
  - **`--peer_ip` / `--peer_cidr`**: scopes **which processes get uprobes** (OpenSSL libssl per container, GoTLS binary discovery, kTLS PID allowlist). Without peer scope the CLI warns; the agent hooks broader targets on the node.
  - **`--port`**: filters **exported plaintext and wire capture** to matching src/dst ports; also helps 5-tuple enrichment when addresses are partial. It does **not** reduce uprobe attachment — only narrows what you see in the TUI/JSONL/pcap.
- Optional GoTLS overrides: `GOTLS_ELF_PATH`, `GOTLS_WRITE_OFFSET`, `GOTLS_READ_OFFSET`
- kTLS: IPv6 supported in agent; cgroup attach required on RHEL CoreOS nodes; niche workloads only
- PCA wire capture truncates frames to 256 bytes; plaintext uses a separate 16 KiB ringbuf per event

## Troubleshooting empty plaintext JSONL

1. Confirm capture command includes `--enable_openssl --privileged` (sets `hostPID`, host lib mounts, `ENABLE_OPENSSL_TRACKING`).
2. Check agent logs for `attached SSL_write uprobe to /proc/<pid>/root/.../libssl.so` — host-only `/host/usr/...` hooks system processes, not pod containers.
3. Look for `TLS plaintext event captured` in agent logs during HTTPS traffic.
4. TLS on the wire (visible in pcapng) is not enough: decryption requires in-process OpenSSL in the pod under capture on that node.
5. Go (`crypto/tls`) — use `--enable_gotls --privileged` (auto-discovery). Java apps need SSLKEYLOGFILE.
6. With `--peer_ip`, confirm the target pod has an established TCP socket to that IP (PID scope is refreshed every 5s).

### GoTLS setup

`--enable_gotls` enables auto-discovery: the agent scans `/proc` for Go binaries, parses `.gopclntab`, and attaches a uprobe to `crypto/tls.(*Conn).writeRecordLocked` (outbound application data — HTTP responses on TLS servers). **Read/inbound capture** (`GOTLS_CAPTURE_READ`) is off by default because uprobes on `(*Conn).Read` crash Go targets; do not enable unless you accept that risk.

Optional overrides when auto-discovery fails (stripped/custom builds):

| Env var | Purpose |
|---------|---------|
| `GOTLS_ELF_PATH` | Pin a single Go binary path |
| `GOTLS_WRITE_OFFSET` | File offset for write hook |
| `GOTLS_READ_OFFSET` | File offset for a single Read RET hook |

Requires `hostPID: true` (set automatically with `--enable_gotls --privileged`). **Recommended:** `--peer_ip=<pod-ip>` and `--port=<service-port>`; unscoped peer discovery hooks all non-excluded Go binaries on the node.

## MVP status (validated)

- OpenSSL plaintext on OpenShift console traffic (HTTPS + WebSocket JSON) via `--enable_openssl`
- Output: pcapng + `output/plaintext/<timestamp>.jsonl`
- TUI shows `PlaintextPreview` when a plaintext record is selected

Live packet TUI behavior with plaintext capture (`--enable_openssl`, `--enable_gotls`, and/or `--enable_ktls`):

- The table lists **TLS plaintext rows only** (wire packets stay in the pcapng); green rows are meaningful plaintext (HTTP/JSON-like payloads)
- When multiple TLS sources are active, the table keeps the newest row per source (`openssl`, `gotls`, `ktls`) visible alongside recent events
- **Event / Type** shows the TLS source for plaintext rows; use **PlaintextPreview** for the decoded payload
- Pause the capture, then click a green row to open the **TLS Plaintext** text panel
- Press Esc to resume live capture
- P0–P3: wall-clock timestamps, PID scoping, userspace 5-tuple enrichment, deduplication, wire↔plaintext correlation (`PcapAnnotated`, pcapng comments)

## Remaining work

| Priority | Item | Why |
|----------|------|-----|
| P4 | BoringSSL / Envoy (service mesh) | Phase 5 in plan |
| P5 | kTLS validation on OpenShift | Phase 4 in plan |
| — | ~~eBPF 5-tuple on `ssl_data_event_t`~~ | kTLS kernel tuple + fd→inode for OpenSSL/GoTLS (done) |
| — | K8s pod/namespace metadata on plaintext | TUI join with workload identity |
