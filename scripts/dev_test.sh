#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="$repo_root/scripts/dev.sh"

if [[ ! -f "$script" ]]; then
  echo "missing scripts/dev.sh" >&2
  exit 1
fi

bash -n "$script"

output="$(
  cd "$repo_root"
  AGENTX_DEV_DRY_RUN=1 "$script"
)"

case "$output" in
  *"AGENTX_ADMIN_TOKEN=dev-token"* ) ;;
  * )
    echo "dry run did not include default admin token" >&2
    echo "$output" >&2
    exit 1
    ;;
esac

case "$output" in
  *"AGENTX_ADDR=127.0.0.1:8080"* ) ;;
  * )
    echo "dry run did not include default backend address" >&2
    echo "$output" >&2
    exit 1
    ;;
esac

case "$output" in
  *"npm run dev -- --host 127.0.0.1 --port 5173"* ) ;;
  * )
    echo "dry run did not include default vite host and port" >&2
    echo "$output" >&2
    exit 1
    ;;
esac
