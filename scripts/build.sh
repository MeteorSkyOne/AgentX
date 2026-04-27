#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

binary="${1:-agentx}"
embed_dir="internal/webdist/dist"

if ! command -v pnpm >/dev/null 2>&1; then
  echo "pnpm is required. If Node.js Corepack is available, run: corepack enable" >&2
  exit 1
fi

echo "Building AgentX web..."
(
  cd web
  pnpm install --frozen-lockfile
  pnpm run build
)

echo "Preparing embedded web assets..."
rm -rf "$embed_dir"
mkdir -p "$(dirname "$embed_dir")"
cp -R web/dist "$embed_dir"

echo "Building AgentX server..."
mkdir -p "$(dirname "$binary")"
go build -tags agentx_embed_web -o "$binary" ./cmd/agentx
