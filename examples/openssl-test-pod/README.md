# OpenSSL test pod

Minimal pod that serves HTTPS with **nginx + dynamically linked OpenSSL 3** (`libssl.so`). Use it to validate NetObserv packet capture with `--enable_openssl`.

This is the right workload for OpenSSL uprobes. It does **not** use Go `crypto/tls` (see [gotls-test-pod](../gotls-test-pod/)) and does **not** enable kernel TLS offload (see [ktls-test-pod](../ktls-test-pod/)).

## Why not use gotls-test or ktls-test with `--enable_openssl`?

| Test pod | TLS stack | Flag for green plaintext rows |
|----------|-----------|-------------------------------|
| **openssl-test** (this) | nginx + `libssl.so` | `--enable_openssl` |
| [gotls-test](../gotls-test-pod/) | Go `crypto/tls` | `--enable_gotls` (not `--enable_openssl`) |
| [ktls-test](../ktls-test-pod/) | nginx + kTLS offload | `--enable_ktls` when kernel offloads; if kTLS is active, OpenSSL uprobes often see **no** application data |

## 1. Build and push (Quay)

Default image: `quay.io/$(USER)/openssl-test:latest`

```bash
cd examples/openssl-test-pod

podman login quay.io   # or docker login

make images

make images IMAGE_ORG=netobserv
# => quay.io/netobserv/openssl-test:latest
```

**Local Kind cluster** (no registry pull):

```bash
make image-build USER=jpinsonn VERSION=test
make deploy-local USER=jpinsonn VERSION=test
```

## 2. Deploy

```bash
make deploy IMAGE_ORG=netobserv
make wait
export OPENSSL_POD_IP=$(make pod-ip)
echo "Pod IP: $OPENSSL_POD_IP"
```

Verify nginx links libssl dynamically:

```bash
kubectl logs -n openssl-test deploy/openssl-test | grep libssl
# expect: libssl (nginx): libssl.so.3 => /lib64/libssl.so.3
```

## 3. Generate HTTPS traffic

```bash
make traffic
# or one-off:
kubectl run -n openssl-test curl-once --rm -it --restart=Never \
  --image=curlimages/curl:8.11.1 --command -- \
  curl --http1.1 -sk "https://${OPENSSL_POD_IP}:8443/message"
```

Expected response body (each workload uses a unique `NETOBSERV-*` marker):

```text
NETOBSERV-OPENSSL plaintext probe
TLS stack: nginx + OpenSSL 3 (userspace libssl.so)
Workload: openssl-test-pod
Endpoint: GET /message
Capture hint: look for "NETOBSERV-OPENSSL" in PlaintextPreview
```

`make traffic` also hits `/api/items` (JSON with `"sku":"openssl-alpha"`) and `POST /api/echo` with a client body `NETOBSERV-OPENSSL client request seq=N`.

Fake HTTP headers are included on purpose (not real credentials):

- **Response** (visible in server `SSL_write` plaintext): `X-NetObserv-Stack`, `X-Fake-Authorization`, `X-Fake-Trace-Id`, …
- **Request** (sent by the traffic Job): `Authorization`, `X-Fake-Trace-Id: openssl-req-*`, `X-NetObserv-Client: openssl-traffic-pod`

In the live TUI, `PlaintextPreview` should show lines like `HTTP/1.1 200 OK` and `X-Fake-Trace-Id: openssl-resp-message` ahead of the body.

## 4. Capture with NetObserv

Agent image must include OpenSSL uprobe support (`ENABLE_OPENSSL_TRACKING`).

```bash
export OPENSSL_POD_IP=$(make pod-ip)
NETOBSERV_AGENT_IMAGE=quay.io/jpinsonn/netobserv-ebpf-agent:plaintext3 \
NETOBSERV_COLLECTOR_IMAGE=quay.io/jpinsonn/network-observability-cli:plaintext2 \
  ./build/oc-netobserv packets \
  --port=8443 \
  --peer_ip="${OPENSSL_POD_IP}" \
  --enable_openssl \
  --privileged \
  --max-bytes=100000000
```

`--peer_ip` scopes uprobes and helps plaintext pass agent filters when the 5-tuple is not enriched yet. Match **both** agent and collector image tags to your local builds (`plaintext3` / `plaintext2` above are examples).

`--enable_openssl` sets `hostPID`, host `/usr`/`/lib`/`/lib64` mounts, and `ENABLE_OPENSSL_TRACKING=true` on the agent DaemonSet.

Keep traffic running during capture (`make traffic` or repeated `curl`).

### What to expect

- **Green TUI rows** with HTTP response bodies containing `NETOBSERV-OPENSSL`
- **JSONL** (`output/plaintext/*.jsonl`): `"TLSSource": "openssl"`, `"Direction": "write"` for server responses
- **Agent logs** on the node running the pod:

```bash
kubectl logs -n netobserv-cli -l app=netobserv-cli --tail=200 | rg -i 'openssl|SSL_write|plaintext'
```

Look for `attached SSL_write uprobe` and `TLS plaintext event captured` with `source=openssl`.

`--peer_ip` is optional for OpenSSL discovery but recommended to reduce noise from other node processes.

## Makefile reference

| Variable | Default | Example |
|----------|---------|---------|
| `IMAGE_REGISTRY` | `quay.io` | `docker.io` |
| `IMAGE_ORG` | `$(USER)` | `netobserv` |
| `VERSION` | `latest` | `test` |
| `IMAGE` | `$(IMAGE_REGISTRY)/$(IMAGE_ORG)/openssl-test:$(VERSION)` | full override |
| `PULL_POLICY` | `Always` | `IfNotPresent` (local) |

Run `make help` for targets.

## Troubleshooting

| Symptom | Check |
|---------|--------|
| CLI warns about missing `--peer_ip` | Expected without peer scope — add `--peer_ip=$(make pod-ip)` to narrow hooks and reduce noise |
| No green rows with `--enable_openssl` on **gotls-test** | Wrong flag — use `--enable_gotls` for Go TLS |
| No green rows on **ktls-test** with `--enable_openssl` | kTLS may offload TLS to the kernel; use this **openssl-test** pod, or `--enable_ktls` on ktls-test |
| No green rows, agent logs show `TLS plaintext event captured` but JSONL is empty | **`--port` without `--peer_ip`** used to drop plaintext before export when no 5-tuple (fixed in agent `plaintext3+`); workaround: add `--peer_ip=<pod-ip>`; rebuild agent |
| Green rows delayed ~30s | Plaintext waits for wire-packet correlation; pause capture or wait for buffer flush |
| `no libssl.so libraries discovered` in agent logs | Agent needs `hostPID=true` and `--privileged`; redeploy capture with `--enable_openssl --privileged` |
| `attached SSL_write` but no plaintext | Custom agent image with TLS capture; keep `make traffic` running; check `FLOW_FILTER_RULES` port matches 8443 |
| `peer_ip` scoped but no hooks | `hostPID` must be true; pod IP must match `--peer_ip`; agent DaemonSet on the **same node** as the workload |
| Only health-probe lines (`GET /healthz`) | Normal — also hit `/` and `/api/items` via `make traffic` |
| `ImagePullBackOff` | `make images` + public Quay repo or pull secret in `openssl-test` namespace |

## Cleanup

```bash
make undeploy
```

## Related examples

- [GoTLS test pod](../gotls-test-pod/) — `--enable_gotls`
- [kTLS test pod](../ktls-test-pod/) — `--enable_ktls`
