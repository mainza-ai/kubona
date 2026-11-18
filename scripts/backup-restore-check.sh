#!/usr/bin/env bash
# Prove the newest SCHEDULED backup file is a usable restore source.
#
# `kubonad restore-drill` snapshots the live ledger and proves that round-trip;
# it does not read the backup volume. This script restores the actual backup
# artifact into a scratch path and re-verifies the chain, which is the check
# an operator needs before trusting retention.
#
# Usage:
#   scripts/backup-restore-check.sh [/backups]        (inside the backup image)
#   KUBONAD_BIN=/usr/local/bin/kubonad scripts/backup-restore-check.sh /backups
set -euo pipefail

dir="${1:-${KUBONA_BACKUP_DIR:-/backups}}"
kubonad="${KUBONAD_BIN:-kubonad}"

if [ ! -d "$dir" ]; then
  echo "backup-restore-check: no such backup directory: $dir" >&2
  exit 1
fi

source="$(find "$dir" -maxdepth 1 -type f -name '*.backup-*' ! -name '*.sha256' -print0 2>/dev/null \
  | xargs -0 ls -1t 2>/dev/null | head -1 || true)"
if [ -z "$source" ]; then
  echo "backup-restore-check: no backup artifact in $dir" >&2
  exit 1
fi

# Encrypted artifacts need the private age key; restoring ciphertext as a
# SQLite file would fail confusingly. Refuse with the remedy instead.
case "$source" in
  *.age)
    if ! command -v age >/dev/null 2>&1 || [ -z "${KUBONA_AGE_IDENTITY:-}" ]; then
      echo "backup-restore-check: newest artifact is encrypted ($source)." >&2
      echo "  Decrypt first: age -d -i \"\$KUBONA_AGE_IDENTITY\" -o /tmp/kubona.db \"$source\"" >&2
      echo "  then run: $0 <directory containing the decrypted .db>" >&2
      exit 1
    fi
    scratch="$(mktemp -d)"
    trap 'rm -rf "$scratch"' EXIT
    decrypted="$scratch/decrypted.db"
    age -d -i "$KUBONA_AGE_IDENTITY" -o "$decrypted" "$source"
    source="$decrypted"
    ;;
  *)
    scratch="$(mktemp -d)"
    trap 'rm -rf "$scratch"' EXIT
    ;;
esac

echo "backup-restore-check: source $source"
"$kubonad" verify --db "$source"
"$kubonad" restore --from "$source" --db "$scratch/restored.db" --apply
"$kubonad" verify --db "$scratch/restored.db"
echo "backup-restore-check: clean"
