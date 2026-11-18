#!/usr/bin/env bash
# Run the pinned Lean comparator and emit a signed attestation.
#
# Prerequisites:
#   - built sandbox image (scripts/build-lean-sandbox.sh)
#   - a comparator key (scripts/comparator-keygen.sh) and the matching public
#     key configured on the daemon
#   - Docker and the vendored Lean project (submodules/math/lean)
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PYTHON="${KUBONA_PYTHON:-$ROOT/platform/.venv/bin/python}"
if [ ! -x "$PYTHON" ]; then PYTHON=python3; fi
KEY_FILE="${KUBONA_COMPARATOR_KEY_FILE:-$HOME/.kubona/comparator.key}"

if [ -z "${KUBONA_COMPARATOR_PRIVKEY_HEX:-}" ]; then
  if [ ! -r "$KEY_FILE" ]; then
    echo "comparator-verify: no KUBONA_COMPARATOR_PRIVKEY_HEX and no key at $KEY_FILE" >&2
    exit 1
  fi
  export KUBONA_COMPARATOR_PRIVKEY_HEX="$(tr -d '[:space:]' < "$KEY_FILE")"
fi

cd "$ROOT/platform"
exec "$PYTHON" -m kubona.eval.comparator_cli \
  --lean-dir "$ROOT/submodules/math/lean" \
  "$@"
