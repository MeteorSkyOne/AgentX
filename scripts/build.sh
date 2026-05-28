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
version="$(git describe --tags --always --dirty 2>/dev/null || echo "dev")"
version="$(echo "$version" | sed -E 's/^v//; s/-([0-9]+)-g[a-f0-9]+$/-dev.\1/; s/-([0-9]+)-g[a-f0-9]+-dirty$/-dev.\1-dirty/')"
commit="$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")"
build_date="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
version_pkg="github.com/meteorsky/agentx/internal/version"
ldflags="-s -w -X ${version_pkg}.Version=${version} -X ${version_pkg}.Commit=${commit} -X ${version_pkg}.Date=${build_date}"

go build -tags agentx_embed_web -ldflags "$ldflags" -o "$binary" ./cmd/agentx
