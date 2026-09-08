# TLS plaintext showcase workloads

Three minimal HTTPS pods demo NetObserv TLS plaintext capture. A fourth **cleartext HTTP** pod demos PCA wire readability (no TLS flags).

| Pod | TLS stack | CLI flag | Marker | Example header |
|-----|-----------|----------|--------|----------------|
| [http-test-pod](http-test-pod/) | none (HTTP) | _(PCA only)_ | `NETOBSERV-HTTP` | `X-NetObserv-Stack: http` |
| [openssl-test-pod](openssl-test-pod/) | nginx + `libssl.so` | `--enable_openssl` | `NETOBSERV-OPENSSL` | `X-NetObserv-Stack: openssl` |
| [gotls-test-pod](gotls-test-pod/) | Go `crypto/tls` | `--enable_gotls` | `NETOBSERV-GOTLS` | `X-NetObserv-Stack: gotls` |
| [ktls-test-pod](ktls-test-pod/) | nginx + kTLS (`sk_msg`) | `--enable_ktls` | `NETOBSERV-KTLS` | `X-NetObserv-Stack: ktls` |

## Cleartext HTTP (PCA)

```bash
cd examples/http-test-pod && make deploy-local wait traffic
export HTTP_POD_IP=$(make pod-ip)
./build/oc-netobserv packets --port=8080 --peer_ip="${HTTP_POD_IP}"
```

Select a wire row — the detail panel shows **Wire HTTP (cleartext)** when the TCP payload is HTTP (status line + headers + body). TLS pods still use the green **TLS Plaintext** panel.

## Quick start (TLS stacks)

```bash
cd examples

# Build, deploy, wait, generate traffic
make images-all IMAGE_ORG=$USER
make deploy-all wait-all traffic-all

# Pod IPs for scoped capture
make pod-ips
make capture-hint   # prints ready-to-paste commands
```

Kind without a registry:

```bash
make deploy-local-all wait-all traffic-all
```

## What each endpoint showcases

| Endpoint | Purpose |
|----------|---------|
| `GET /message` | Multi-line plaintext + fake response headers (`HTTP/1.1 200 OK`, `X-Fake-Trace-Id`, …) |
| `GET /api/items` | JSON body with stack-specific `"sku"` values |
| `POST /api/echo` | POST body from traffic Job; **gotls** also echoes `received_request_headers` in the response |
| `GET /healthz` | Short probe response (kubelet noise — keep for readiness only) |

Traffic Jobs send fake **request** headers (`Authorization`, `X-Fake-Trace-Id: <stack>-req-*`, `X-NetObserv-Client`) on every call.

## Capture commands

**Recommended:** one stack + `--peer_ip` (less noise, reliable 5-tuple):

```bash
export OPENSSL_POD_IP=$(make -sC openssl-test-pod pod-ip)

NETOBSERV_AGENT_IMAGE=quay.io/you/netobserv-ebpf-agent:plaintext \
  ./build/oc-netobserv packets \
  --port=8443 \
  --peer_ip="${OPENSSL_POD_IP}" \
  --enable_openssl \
  --privileged
```

**Stress test:** all flags, port only (no `--peer_ip`):

```bash
./build/oc-netobserv packets --port=8443 --enable_openssl --enable_gotls --enable_ktls --privileged
```

Annotated plaintext (`PcapAnnotated: true`) picks up `SrcK8S_*` / `DstK8S_*` from the correlated wire packet (PCA rows are FLP-enriched). For better agent 5-tuple without a single pod IP, use `--peer_cidr=10.244.0.0/16` (Kind default pod network). See `make capture-hint`.

Keep `make traffic-all` running during capture.

## What to look for in the live TUI

- **Green rows** — TLS plaintext (`RecordType` column shows `openssl` / `gotls` / `ktls`)
- **`PlaintextPreview`** — should include `HTTP/1.1 200 OK`, fake `X-Fake-*` headers, then `NETOBSERV-<STACK> …`
- **Binary noise** — kTLS/gRPC framing shows as `<ktls write 8190B binary>` when not HTTP-shaped
- **Row select** — opens hex panel with full decoded payload (pauses live ingest)
- **`+` / `-`** — increase “Showing last: N” if the table feels sparse

## Verify JSONL output

```bash
# Count by stack
jq -r '.TLSSource' output/plaintext/*.jsonl | sort | uniq -c

# Distinct markers in previews
rg 'NETOBSERV-(OPENSSL|GOTLS|KTLS)' output/plaintext/

# Wire ↔ plaintext correlation
jq 'select(.PcapAnnotated == true) | {TLSSource, PlaintextPreview}' output/plaintext/*.jsonl | head
```

## Optional CLI knobs to demo

| Flag | Showcase |
|------|----------|
| `--tls_plaintext_min_bytes=4` | Drop tiny TLS fragments; cleaner table, fewer binary rows |
| `--tls_plaintext_preview_bytes=512` | Longer `PlaintextPreview` column (includes more headers) |
| `--peer_cidr=10.244.0.0/16` | Scope to pod network instead of single `--peer_ip` |

See [docs/tls-decryption-coverage.md](../docs/tls-decryption-coverage.md) for limits, `PcapAnnotated`, and fd→inode 5-tuple enrichment.

## Ideas not covered yet (future)

- **Large response** (`/large`, 8–16 KiB) — preview truncation vs hex panel
- **HTTP errors** (`418` / `404`) — non-200 status lines in plaintext
- **Read-direction capture** — inbound HTTP requests on server (GoTLS read uprobes disabled today)
- **SSLKEYLOGFILE** sidecar — `--tls-keylog` wire decryption path (requires app change)
- **Java / BoringSSL** workloads — Envoy/Istio mesh scenarios
