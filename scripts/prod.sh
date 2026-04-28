#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

backend_addr="${AGENTX_ADDR:-}"
bin_dir="${AGENTX_BIN_DIR:-bin}"
binary="$bin_dir/agentx"
setup_token="${AGENTX_ADMIN_TOKEN:-}"
backend_label="${backend_addr:-config.toml}"

generate_token() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 24
    return
  fi
  od -An -N24 -tx1 /dev/urandom | tr -d ' \n'
}

if [[ -z "$setup_token" ]]; then
  setup_token="$(generate_token)"
fi

if [[ "${AGENTX_PROD_DRY_RUN:-}" == "1" ]]; then
  echo "AGENTX_ADMIN_TOKEN=${AGENTX_ADMIN_TOKEN:-<generated>}"
  echo "AGENTX_ADDR=${backend_addr:-<unset>}"
  echo "bash scripts/build.sh $binary"
  echo "$binary"
  exit 0
fi

bash scripts/build.sh "$binary"

echo "Starting AgentX production server at $backend_label"
echo "Press Ctrl+C to stop."

export AGENTX_ADMIN_TOKEN="$setup_token"
if [[ -n "$backend_addr" ]]; then
  export AGENTX_ADDR="$backend_addr"
fi
exec "$binary"
