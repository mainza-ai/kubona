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

target="${1:-${KUBONA_BACKUP_DIR:-/backups}}"
kubonad="${KUBONAD_BIN:-kubonad}"

if [ -f "$target" ]; then
  source="$target"
elif [ -d "$target" ]; then
  source="$(find "$target" -maxdepth 1 -type f -name '*.backup-*' ! -name '*.sha256' -print0 2>/dev/null \
    | xargs -0 ls -1t 2>/dev/null | head -1 || true)"
  if [ -z "$source" ]; then
    echo "backup-restore-check: no backup artifact in $target" >&2
    exit 1
  fi
else
  echo "backup-restore-check: no such backup file or directory: $target" >&2
  exit 1
fi

if [ -f "${source}.sha256" ]; then
  if command -v sha256sum >/dev/null 2>&1; then
    expected="$(awk '{print $1}' "${source}.sha256")"
    actual="$(sha256sum "$source" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    expected="$(awk '{print $1}' "${source}.sha256")"
    actual="$(shasum -a 256 "$source" | awk '{print $1}')"
  else
    echo "backup-restore-check: sha256 sidecar present but neither sha256sum nor shasum is installed" >&2
    exit 1
  fi
  if [ -z "$expected" ] || [ "$expected" != "$actual" ]; then
    echo "backup-restore-check: sha256 mismatch for $source (expected $expected, got $actual)" >&2
    exit 1
  fi
  echo "backup-restore-check: sha256 verified ($actual)"
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
