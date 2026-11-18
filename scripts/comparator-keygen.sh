#!/usr/bin/env bash
# Generate the Ed25519 keypair for signed comparator attestations.
#
# The private key stays on the operator host (0600); only the public key is
# configured on the daemon (KUBONA_COMPARATOR_PUBKEY_HEX). Refuses to
# overwrite an existing key: re-keying invalidates prior attestations.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
KEY_FILE="${KUBONA_COMPARATOR_KEY_FILE:-$HOME/.kubona/comparator.key}"
PYTHON="${KUBONA_PYTHON:-$ROOT/platform/.venv/bin/python}"
if [ ! -x "$PYTHON" ]; then PYTHON=python3; fi
if [ -e "$KEY_FILE" ]; then
  echo "comparator-keygen: refusing to overwrite $KEY_FILE" >&2
  exit 1
fi
mkdir -p "$(dirname "$KEY_FILE")"
umask 077
PUB="$("$PYTHON" - "$KEY_FILE" <<'PY'
import sys
from pathlib import Path
from nacl.signing import SigningKey
key = SigningKey.generate()
path = Path(sys.argv[1])
path.write_text(key.encode().hex())
path.chmod(0o600)
print(key.verify_key.encode().hex())
PY
)"
echo "comparator-keygen: wrote $KEY_FILE (0600)"
echo "KUBONA_COMPARATOR_PUBKEY_HEX=$PUB"
echo "Keep the private key host-side; set the public line in .env."
