#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: $0 <branch-name>"
  echo ""
  echo "Create a temporary git worktree for the given branch and start"
  echo "the full dev stack (API + Web UI) inside it. The worktree is"
  echo "automatically removed when you press Ctrl+C."
  exit 1
}

[[ $# -lt 1 ]] && usage

branch="$1"
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

if ! git -C "$repo_root" rev-parse --verify "$branch" >/dev/null 2>&1; then
  echo "Error: branch '$branch' does not exist"
  exit 1
fi

existing_worktree="$(git -C "$repo_root" worktree list --porcelain \
  | awk -v b="$branch" '
    /^worktree /{ wt=$2 }
    /^branch /{ if ($2 == "refs/heads/" b) print wt }
  ')"

created_worktree=""

if [[ -n "$existing_worktree" ]]; then
  worktree_dir="$existing_worktree"
  echo "Reusing existing worktree for '$branch' at $worktree_dir"
else
  worktree_dir="$repo_root/.worktrees/dev-$(echo "$branch" | tr '/' '-')"
  echo "Creating worktree for '$branch' at $worktree_dir"
  git -C "$repo_root" worktree add "$worktree_dir" "$branch"
  created_worktree="$worktree_dir"
fi

cleanup() {
  trap - EXIT INT TERM
  echo ""
  echo "Stopping dev stack..."
  for pid in "${pids[@]}"; do
    if kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
    fi
  done
  wait "${pids[@]}" 2>/dev/null || true
  if [[ -n "$created_worktree" ]]; then
    echo "Removing worktree at $created_worktree"
    git -C "$repo_root" worktree remove --force "$created_worktree" 2>/dev/null || true
  fi
}

handle_signal() {
  cleanup
  exit 0
}

cd "$worktree_dir"

backend_addr="${AGENTX_ADDR:-}"
web_host="${AGENTX_WEB_HOST:-127.0.0.1}"
web_port="${AGENTX_WEB_PORT:-5173}"
data_dir="${AGENTX_DATA_DIR:-.agentx-worktree}"
sqlite_path="${AGENTX_SQLITE_PATH:-$data_dir/agentx.db}"
setup_token="${AGENTX_ADMIN_TOKEN:-dev-token}"
backend_label="${backend_addr:-config.toml}"

if [[ ! -d web/node_modules ]]; then
  echo "Installing web dependencies..."
  (cd web && pnpm install)
fi

pids=()
trap cleanup EXIT
trap handle_signal INT TERM

echo "Starting AgentX API at $backend_label"
(
  export AGENTX_ADMIN_TOKEN="$setup_token"
  if [[ -n "$backend_addr" ]]; then
    export AGENTX_ADDR="$backend_addr"
  fi
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

echo ""
echo "Branch:          $branch"
echo "Worktree:        $worktree_dir"
echo "Press Ctrl+C to stop and clean up."

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
