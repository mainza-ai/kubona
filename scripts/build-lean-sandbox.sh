#!/usr/bin/env bash
# Build the pinned Lean comparator sandbox and optionally warm its build cache.
#
# The toolchain version comes from the vendored submodules/math/lean/
# lean-toolchain; the Dockerfile must pin the same version (asserted here and by
# platform/tests/test_lean_sandbox_pin.py). Upstream publishes no official
# `leanprover/lean4` Docker image, so the image installs the checksummed GitHub
# release tarball (see the Dockerfile header).
#
# The warm step needs network and can take a long time (mathlib-scale). Runtime
# verification then runs with --network none.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
IMAGE="${KUBONA_LEAN_IMAGE:-kubona-lean:4.34.1}"
TOOLCHAIN_FILE="$ROOT/submodules/math/lean/lean-toolchain"
DOCKERFILE="$ROOT/platform/docker/Dockerfile.lean"
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
if [ ! -f "$TOOLCHAIN_FILE" ]; then
  echo "build-lean-sandbox: missing vendored toolchain $TOOLCHAIN_FILE" >&2
  exit 1
fi

TOOLCHAIN="$(tr -d '[:space:]' < "$TOOLCHAIN_FILE")"
case "$TOOLCHAIN" in
  leanprover/lean4:v*) VERSION="${TOOLCHAIN#leanprover/lean4:v}" ;;
  *) echo "build-lean-sandbox: unsupported lean-toolchain '$TOOLCHAIN'" >&2; exit 1 ;;
esac
DOCKER_PIN="$(sed -n 's/^ARG LEAN_VERSION=//p' "$DOCKERFILE" | head -1)"
if [ "$VERSION" != "$DOCKER_PIN" ]; then
  echo "build-lean-sandbox: lean-toolchain v$VERSION does not match Dockerfile ARG LEAN_VERSION=$DOCKER_PIN" >&2
  echo "update the Dockerfile version AND its LEAN_LINUX_SHA256 together" >&2
  exit 1
fi
echo "build-lean-sandbox: pinned Lean v$VERSION (from lean-toolchain)"

docker build -f "$DOCKERFILE" --build-arg "LEAN_VERSION=$VERSION" -t "$IMAGE" "$ROOT"
echo "build-lean-sandbox: built $IMAGE"

# The image must actually contain the pinned compiler and Lake before anyone
# warms a cache against it.
INSTALLED="$(docker run --rm --entrypoint lean "$IMAGE" --version)"
echo "build-lean-sandbox: $INSTALLED"
case "$INSTALLED" in
  *"version $VERSION,"*|*"version $VERSION "*) ;;
  *) echo "build-lean-sandbox: image reports '$INSTALLED', expected version $VERSION" >&2; exit 1 ;;
esac
docker run --rm --entrypoint lake "$IMAGE" --version >/dev/null

if [ "$WARM" = 1 ]; then
  echo "build-lean-sandbox: warming the Lake cache (network required)…"
  docker run --rm -v "$ROOT/submodules/math/lean:/workspace:rw" -w /workspace \
    --entrypoint lake "$IMAGE" build
fi
echo "Next: scripts/comparator-verify.sh --challenge <name> --session <sid> --out /tmp/attestation.json"
