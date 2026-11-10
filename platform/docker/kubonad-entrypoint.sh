#!/bin/sh
set -eu

# Keep the Go and Python rails on the same writable contract. The image copy
# seeds the state volume on first boot; refreshes rewrite the state copy.
STATE="${KUBONA_STATE_DIR:-/home/kubona/.kubona}"
mkdir -p "$STATE/contracts" "$STATE/models"
for f in /contracts/*; do
  base="$(basename "$f")"
  case "$base" in
    fidelity-devices.json|rate-cards.json)
      [ -e "$STATE/contracts/$base" ] || cp "$f" "$STATE/contracts/"
      ;;
    *)
      cp "$f" "$STATE/contracts/"
      ;;
  esac
done
export KUBONA_CONTRACTS_DIR="$STATE/contracts"

exec /usr/local/bin/kubonad "$@"
