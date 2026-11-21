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
      # Refresh when missing or when an image upgrade ships a newer contract,
      # unless the operator explicitly pinned the state copy.
      if [ ! -e "$STATE/contracts/$base" ] || { [ "${KUBONA_KEEP_STATE_CONTRACTS:-0}" != "1" ] && [ "$f" -nt "$STATE/contracts/$base" ]; }; then
        cp -p "$f" "$STATE/contracts/$base"
      fi
      ;;
    *)
      cp -p "$f" "$STATE/contracts/$base"
      ;;
  esac
done
export KUBONA_CONTRACTS_DIR="$STATE/contracts"

exec /usr/local/bin/kubonad "$@"
