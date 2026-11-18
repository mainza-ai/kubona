#!/usr/bin/env bash
# Build the pinned Lean comparator sandbox and optionally warm its build cache.
#
# The warm step needs network and can take a long time (mathlib-scale). Runtime
# verification then runs with --network none.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IMAGE="${KUBONA_LEAN_IMAGE:-kubona-lean:4.34.1}"
WARM=0
for arg in "$@"; do
  case "$arg" in
    --warm) WARM=1 ;;
    *) echo "usage: $0 [--warm]" >&2; exit 2 ;;
  esac
done
if ! command -v docker >/dev/null 2>&1; then
  echo "build-lean-sandbox: docker is required" >&2
  exit 1
fi
docker build -f "$ROOT/platform/docker/Dockerfile.lean" -t "$IMAGE" "$ROOT"
echo "build-lean-sandbox: built $IMAGE"
if [ "$WARM" = 1 ]; then
  echo "build-lean-sandbox: warming the Lake cache (network required)…"
  docker run --rm -v "$ROOT/submodules/math/lean:/workspace:rw" -w /workspace \
    --entrypoint lake "$IMAGE" build
fi
echo "Next: scripts/comparator-verify.sh --challenge <name> --session <sid> --out /tmp/attestation.json"
