#!/bin/bash
set -euo pipefail

echo "OpenSSL: $(openssl version)"
echo "nginx: $(nginx -v 2>&1)"
echo "libssl (nginx): $(ldd "$(command -v nginx)" 2>/dev/null | grep -E 'libssl\.so' || echo 'not found')"

if ! nginx -t 2>&1; then
  echo "nginx config test failed" >&2
  exit 1
fi

exec nginx -g 'daemon off;'
