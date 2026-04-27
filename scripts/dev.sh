#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

backend_addr="${AGENTX_ADDR:-127.0.0.1:8080}"
web_host="${AGENTX_WEB_HOST:-127.0.0.1}"
web_port="${AGENTX_WEB_PORT:-5173}"
data_dir="${AGENTX_DATA_DIR:-.agentx}"
sqlite_path="${AGENTX_SQLITE_PATH:-$data_dir/agentx.db}"
setup_token="${AGENTX_ADMIN_TOKEN:-dev-token}"

if [[ "${AGENTX_DEV_DRY_RUN:-}" == "1" ]]; then
  echo "AGENTX_ADMIN_TOKEN=$setup_token"
  echo "AGENTX_ADDR=$backend_addr"
  echo "AGENTX_DATA_DIR=$data_dir"
  echo "AGENTX_SQLITE_PATH=$sqlite_path"
  echo "go run ./cmd/agentx"
  echo "pnpm exec vite --host $web_host --port $web_port --strictPort"
  exit 0
fi

if [[ ! -d web/node_modules ]]; then
  echo "Installing web dependencies..."
  (cd web && pnpm install)
fi

pids=()

cleanup() {
  trap - EXIT INT TERM
  for pid in "${pids[@]}"; do
    if kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
    fi
  done
  wait "${pids[@]}" 2>/dev/null || true
}

handle_signal() {
  cleanup
  exit 0
}

trap cleanup EXIT
trap handle_signal INT TERM

echo "Starting AgentX API at http://$backend_addr"
(
  export AGENTX_ADMIN_TOKEN="$setup_token"
  export AGENTX_ADDR="$backend_addr"
  export AGENTX_DATA_DIR="$data_dir"
  export AGENTX_SQLITE_PATH="$sqlite_path"
  go run ./cmd/agentx
) &
pids+=("$!")

echo "Starting AgentX web at http://$web_host:$web_port"
(
  cd web
  pnpm exec vite --host "$web_host" --port "$web_port" --strictPort
) &
pids+=("$!")

echo "Setup token: $setup_token"
echo "Press Ctrl+C to stop both processes."

set +e
wait -n "${pids[@]}"
status="$?"
set -e
if [[ "$status" -eq 130 || "$status" -eq 143 ]]; then
  cleanup
  exit 0
fi
cleanup
exit "$status"
