#!/bin/sh
# Build portable artifacts without Docker or native backend dependencies.
set -eu
cd "$(dirname "$0")/.."
mkdir -p dist
for target_os in linux darwin windows; do
  for target_arch in amd64 arm64; do
    suffix=""
    if [ "$target_os" = windows ]; then suffix=".exe"; fi
    output_dir="dist/${target_os}-${target_arch}"
    mkdir -p "$output_dir"
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -ldflags='-s -w' -o "$output_dir/mcp-host-bridge$suffix" ./cmd/mcp-host-bridge
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -ldflags='-s -w' -o "$output_dir/mcp-bridge-demo$suffix" ./cmd/mcp-bridge-demo
  done
done
