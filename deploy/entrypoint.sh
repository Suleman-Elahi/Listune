#!/bin/bash
set -euo pipefail

cd /app

mkdir -p /app/data/gtokens /app/data/library

/usr/local/bin/listune-server &
backend_pid=$!

nginx -g 'daemon off;' &
nginx_pid=$!

cleanup() {
  kill -TERM "$backend_pid" "$nginx_pid" 2>/dev/null || true
}

trap cleanup TERM INT

status=0
wait -n "$backend_pid" "$nginx_pid" || status=$?

cleanup
wait "$backend_pid" 2>/dev/null || true
wait "$nginx_pid" 2>/dev/null || true

exit "$status"
