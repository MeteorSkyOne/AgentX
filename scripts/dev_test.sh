#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="$repo_root/scripts/dev.sh"
worktree_script="$repo_root/scripts/dev-worktree.sh"
prod_script="$repo_root/scripts/prod.sh"
build_script="$repo_root/scripts/build.sh"

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
if [[ ! -f "$build_script" ]]; then
  echo "missing scripts/build.sh" >&2
  exit 1
fi

bash -n "$script"
bash -n "$worktree_script"
bash -n "$prod_script"
bash -n "$build_script"

output="$(
  cd "$repo_root"
  AGENTX_DEV_DRY_RUN=1 "$script"
)"

case "$output" in
  *"AGENTX_ADMIN_TOKEN=dev-token"* ) ;;
  * )
    echo "dry run did not include default setup token" >&2
    echo "$output" >&2
    exit 1
    ;;
esac

case "$output" in
  *"AGENTX_ADDR=<unset>"* ) ;;
  * )
    echo "dry run unexpectedly set default backend address" >&2
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
    echo "prod dry run did not include generated setup token placeholder" >&2
    echo "$prod_output" >&2
    exit 1
    ;;
esac

case "$prod_output" in
  *"AGENTX_ADDR=<unset>"* ) ;;
  * )
    echo "prod dry run unexpectedly set default backend address" >&2
    echo "$prod_output" >&2
    exit 1
    ;;
esac

case "$prod_output" in
  *"bash scripts/build.sh bin/agentx"* ) ;;
  * )
    echo "prod dry run did not include embedded production build" >&2
    echo "$prod_output" >&2
    exit 1
    ;;
esac
