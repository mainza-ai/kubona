#!/bin/sh
set -eu

# Keep the Go and Python rails on the same writable contract. The image copy
# seeds the state volume on first boot; refreshes rewrite the state copy.
STATE="${KUBONA_STATE_DIR:-/home/kubona/.kubona}"
mkdir -p "$STATE/contracts" "$STATE/models"
for f in /contracts/*; do
  [ -e "$STATE/contracts/$(basename "$f")" ] || cp "$f" "$STATE/contracts/"
done
export KUBONA_CONTRACTS_DIR="$STATE/contracts"

exec /usr/local/bin/kubonad "$@"
