#!/bin/sh
set -eu

# The image contract is a read-only bootstrap. Production refreshes must land
# in the writable state volume; seed it on first boot and pass that path to
# every provider/fidelity loader.
STATE="${KUBONA_STATE_DIR:-/home/kubona/.kubona}"
mkdir -p "$STATE/contracts" "$STATE/models"
if [ ! -f "$STATE/contracts/fidelity-devices.json" ]; then
  cp /app/contracts/fidelity-devices.json "$STATE/contracts/fidelity-devices.json"
fi
export KUBONA_FIDELITY_DEVICES="${KUBONA_FIDELITY_DEVICES:-$STATE/contracts/fidelity-devices.json}"

if [ "$#" -gt 0 ]; then
  exec "$@"
fi
exec python server.py
