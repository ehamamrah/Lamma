#!/bin/sh
# Run Go tooling in a pinned container (no local Go install needed).
# Usage: scripts/go.sh go test ./...      (runs inside backend/)
set -eu
ROOT=$(cd "$(dirname "$0")/.." && pwd)
exec docker run --rm \
  -v lamma-gomod:/go/pkg/mod -v lamma-gocache:/root/.cache/go-build \
  -v "$ROOT/backend:/src" -w /src \
  -e GOFLAGS="${GOFLAGS:--p=2}" ${GO_DOCKER_ARGS:-} golang:1.27-bookworm "$@"
