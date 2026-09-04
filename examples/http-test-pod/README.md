# HTTP cleartext test pod

Minimal pod that serves **plain HTTP** (no TLS) on port **8080**. Use it to validate NetObserv PCA wire capture: HTTP is readable on the wire without `--enable_openssl`, `--enable_gotls`, or `--enable_ktls`.

## Deploy and traffic

```bash
cd examples/http-test-pod
make images IMAGE_ORG=$USER   # or deploy-local on Kind
make deploy wait traffic
export HTTP_POD_IP=$(make pod-ip)
```

## Capture

No TLS flags — PCA only:

```bash
./build/oc-netobserv packets \
  --port=8080 \
  --peer_ip="${HTTP_POD_IP}" \
  --max-bytes=100000000
```

## Live TUI

- **White wire rows** — encrypted-looking traffic is absent; TCP payloads carry HTTP.
- **Select a row** — when the packet contains cleartext HTTP, the detail panel shows **Wire HTTP (cleartext)** with status line, headers, and body (not hex).
- Non-HTTP wire packets still open the hex view.

Marker in responses: `NETOBSERV-HTTP` (see `GET /message`).

## Compare with TLS examples

| Pod | Port | Capture flags | Detail panel |
|-----|------|---------------|--------------|
| **http-test** (this) | 8080 | none (PCA) | Wire HTTP (cleartext) |
| [openssl-test](../openssl-test-pod/) | 8443 | `--enable_openssl` | TLS Plaintext |
| [gotls-test](../gotls-test-pod/) | 8443 | `--enable_gotls` | TLS Plaintext |
| [ktls-test](../ktls-test-pod/) | 8443 | `--enable_ktls` | TLS Plaintext |

See [examples/README.md](../README.md) for the full TLS showcase.
