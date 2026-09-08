#!/bin/bash
set -euo pipefail

echo "nginx: $(nginx -v 2>&1)"

if ! nginx -t 2>&1; then
  echo "nginx config test failed" >&2
  exit 1
fi

exec nginx -g 'daemon off;'
