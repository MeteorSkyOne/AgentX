#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="$repo_root/scripts/dev.sh"
worktree_script="$repo_root/scripts/dev-worktree.sh"
prod_script="$repo_root/scripts/prod.sh"

if [[ ! -f "$script" ]]; then
  echo "missing scripts/dev.sh" >&2
  exit 1
fi
if [[ ! -f "$worktree_script" ]]; then
  echo "missing scripts/dev-worktree.sh" >&2
  exit 1
fi
if [[ ! -f "$prod_script" ]]; then
  echo "missing scripts/prod.sh" >&2
  exit 1
fi

bash -n "$script"
bash -n "$worktree_script"
bash -n "$prod_script"

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
  *"pnpm exec vite --host 127.0.0.1 --port 5173 --strictPort"* ) ;;
  * )
    echo "dry run did not include default vite host and port" >&2
    echo "$output" >&2
    exit 1
    ;;
esac

prod_output="$(
  cd "$repo_root"
  AGENTX_PROD_DRY_RUN=1 "$prod_script"
)"

case "$prod_output" in
  *"AGENTX_ADMIN_TOKEN=<generated>"* ) ;;
  * )
    echo "prod dry run did not include generated admin token placeholder" >&2
    echo "$prod_output" >&2
    exit 1
    ;;
esac

case "$prod_output" in
  *"pnpm run build"* ) ;;
  * )
    echo "prod dry run did not include web production build" >&2
    echo "$prod_output" >&2
    exit 1
    ;;
esac

case "$prod_output" in
  *"go build -o bin/agentx ./cmd/agentx"* ) ;;
  * )
    echo "prod dry run did not include server build" >&2
    echo "$prod_output" >&2
    exit 1
    ;;
esac
