#!/bin/sh
# Scheduled verified local backups for the compose stack.
#
# Enabled with:  docker compose ... --profile backup up -d
# One iteration: kubonad backup (VACUUM INTO + chain verification), optional
# age encryption, sha256 receipt, then retention. The backup-status healthcheck
# fails when the newest backup is older than KUBONA_BACKUP_MAX_AGE.
set -eu

DB_PATH="${KUBONA_DB_PATH:-/home/kubona/.kubona/kubona.db}"
BACKUP_DIR="${KUBONA_BACKUP_DIR:-/backups}"
INTERVAL="${KUBONA_BACKUP_INTERVAL_SECONDS:-86400}"
KEEP="${KUBONA_BACKUP_KEEP:-7}"
RECIPIENT="${KUBONA_BACKUP_AGE_RECIPIENT:-}"
REQUIRE_ENCRYPTION="${KUBONA_BACKUP_REQUIRE_ENCRYPTION:-0}"
ALERT_WEBHOOK="${KUBONA_ALERT_WEBHOOK_URL:-}"

notify() {
  # Keep failures visible off-host without making the backup loop depend on
  # the alert rail. Never print the webhook URL (it may contain a token).
  if [ -z "$ALERT_WEBHOOK" ]; then return 0; fi
  curl -fsS --max-time 5 -X POST -H 'Content-Type: application/json' \
    --data-binary "{\"text\":\"$1\"}" "$ALERT_WEBHOOK" >/dev/null 2>&1 || true
}

mkdir -p "$BACKUP_DIR"
if [ -n "$RECIPIENT" ] && ! command -v age >/dev/null 2>&1; then
  echo "backup-loop: KUBONA_BACKUP_AGE_RECIPIENT is set but age is not installed" >&2
  exit 1
fi
if [ "$REQUIRE_ENCRYPTION" = "1" ] && [ -z "$RECIPIENT" ]; then
  echo "backup-loop: encryption is required but KUBONA_BACKUP_AGE_RECIPIENT is empty" >&2
  exit 1
fi

while :; do
  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  base="$BACKUP_DIR/$(basename "$DB_PATH").backup-$ts"
  echo "backup-loop: starting $ts"
  if kubonad backup --db "$DB_PATH" --out "$base"; then
    if [ -n "$RECIPIENT" ]; then
      if ! age -r "$RECIPIENT" -o "$base.age" "$base"; then
        rm -f "$base" "$base.age"
        echo "backup-loop: encryption failed; removed plaintext copy" >&2
        exit 1
      fi
      sha256sum "$base.age" > "$base.age.sha256"
      rm -f "$base"
      latest="$base.age"
    else
      sha256sum "$base" > "$base.sha256"
      latest="$base"
    fi
    printf '%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$BACKUP_DIR/.last-success"
    printf '%s\n' "$latest" > "$BACKUP_DIR/.last-backup"
    echo "backup-loop: wrote $latest"
  else
    printf '%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$BACKUP_DIR/.last-failure"
    echo "backup-loop: backup failed; leaving the previous verified copy in place" >&2
    notify "Kubona backup failed on $(hostname) at $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  fi

  # Retention: newest KEEP backup artifacts (plaintext or .age). Sidecars are
  # removed with their artifact. The last-success/failure markers are kept.
  if [ -d "$BACKUP_DIR" ]; then
    ls -1t "$BACKUP_DIR"/*.backup-* 2>/dev/null | grep -v '\.sha256$' | tail -n +$((KEEP + 1)) | while IFS= read -r old; do
      rm -f "$old" "$old.sha256"
    done || true
  fi

  sleep "$INTERVAL"
done
