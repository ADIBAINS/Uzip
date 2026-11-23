#!/usr/bin/env bash
set -euo pipefail

: "${POSTGRES_URL:?POSTGRES_URL is required}"
: "${REDIS_URL:?REDIS_URL is required}"
export PGCONNECT_TIMEOUT=5
export PGSSLMODE="${PGSSLMODE:-require}"
for attempt in {1..12}; do
  if render-gateway migrate; then
    break
  fi
  if [[ "$attempt" == 12 ]]; then
    exit 1
  fi
  sleep 2
done

export PGOPTIONS="-c role=uzip_app"
export UZIP_RENDER_DEMO=true DEV_AUTH=true
export REDIRECT_BASE_URL="${RENDER_EXTERNAL_URL:-http://localhost:${PORT:-10000}}/s"
export ALLOWED_ORIGINS="${RENDER_EXTERNAL_URL:-http://localhost:${PORT:-10000}}"
pids=()

stop() {
  if (( ${#pids[@]} )); then
    kill -TERM "${pids[@]}" 2>/dev/null || true
    wait "${pids[@]}" 2>/dev/null || true
    pids=()
  fi
}
trap stop EXIT
trap 'exit 0' TERM INT

BIND_HOST=127.0.0.1 PORT=8080 control-api & pids+=("$!")
BIND_HOST=127.0.0.1 PORT=8081 redirect & pids+=("$!")
BIND_HOST=127.0.0.1 PORT=8083 analytics-api & pids+=("$!")
node node_modules/next/dist/bin/next start --hostname 127.0.0.1 --port 3000 & pids+=("$!")
render-gateway & pids+=("$!")

status=1
wait -n "${pids[@]}" || status=$?
exit "$status"
