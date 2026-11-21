#!/bin/sh
# Regenerate type-safe query code from backend/db/{migrations,queries}.
# Usage: scripts/sqlc.sh [generate|diff|vet]   (default: generate)
set -eu
ROOT=$(cd "$(dirname "$0")/.." && pwd)
exec docker run --rm -v "$ROOT/backend:/src" -w /src sqlc/sqlc:1.31.1 "${1:-generate}"
