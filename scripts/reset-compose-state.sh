#!/usr/bin/env bash
# Archive and reset the compose ledger (destructive operator command).
#
# Preserves, in this order:
#   1. a verified SQLite backup copied out of the state volume;
#   2. a full copy of the state volume in a new named volume;
#   3. every existing backup artifact (the backup volume is untouched).
# Then removes only the live DB/WAL/SHM so the daemon starts a clean ledger.
#
# WARNING: sessions, labs, sealed UI provider keys, claims, jobs, and campaign
# state live in the DB. Export/re-enter what you need before running --yes.
# This does NOT remove the encrypted backup volume.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE=(docker compose --env-file "$ROOT/.env" -f "$ROOT/platform/docker/compose.prod.yml")
TS="$(date -u +%Y%m%dT%H%M%SZ)"
APPLY=0
ARCHIVE_DIR="${ARCHIVE_DIR:-$HOME/.kubona/reset-$TS}"

usage() { echo "usage: $0 [--yes] [--archive-dir /abs/path]" >&2; }

while [ $# -gt 0 ]; do
  case "$1" in
    --yes) APPLY=1 ;;
    --archive-dir) shift; ARCHIVE_DIR="${1:-}" ;;
    --archive-dir=*) ARCHIVE_DIR="${1#*=}" ;;
    -h|--help) usage; exit 0 ;;
    *) usage; exit 2 ;;
  esac
  shift
done

case "$ARCHIVE_DIR" in
  /*) ;;
  *) echo "reset-compose-state: --archive-dir must be absolute" >&2; exit 1 ;;
esac

PROJECT="$(docker inspect kubona-prod-kubonad-1 --format '{{index .Config.Labels "com.docker.compose.project"}}' 2>/dev/null || true)"
if [ -z "$PROJECT" ]; then
  echo "reset-compose-state: the kubona-prod kubonad container is not running" >&2
  exit 1
fi
STATE_VOL="$(docker inspect kubona-prod-kubonad-1 --format '{{range .Mounts}}{{if eq .Destination "/home/kubona/.kubona"}}{{.Name}}{{end}}{{end}}')"
if [ -z "$STATE_VOL" ]; then
  echo "reset-compose-state: cannot identify the state volume" >&2
  exit 1
fi
ARCHIVE_VOL="${STATE_VOL}-archive-${TS}"

cat <<PLAN
reset-compose-state plan:
  state volume:   $STATE_VOL
  archive volume: $ARCHIVE_VOL
  archive dir:    $ARCHIVE_DIR
  steps: verified backup -> copy state volume -> stop kubonad ->
         remove kubona.db{,-wal,-shm} -> start stack -> assert single-lab
PLAN
if [ "$APPLY" != 1 ]; then
  echo "reset-compose-state: dry run (pass --yes to execute)"
  exit 0
fi

mkdir -p "$ARCHIVE_DIR"
INNER="/home/kubona/.kubona/reset-$TS.db"

echo "reset-compose-state: taking a verified backup"
docker exec kubona-prod-kubonad-1 kubonad backup \
  --db /home/kubona/.kubona/kubona.db --out "$INNER"
docker exec kubona-prod-kubonad-1 kubonad verify --db "$INNER"
docker cp "kubona-prod-kubonad-1:$INNER" "$ARCHIVE_DIR/kubona.db"
docker exec kubona-prod-kubonad-1 rm -f "$INNER"

echo "reset-compose-state: archiving the state volume"
docker volume create "$ARCHIVE_VOL" >/dev/null
docker run --rm -v "$STATE_VOL:/from:ro" -v "$ARCHIVE_VOL:/to" \
  --entrypoint sh kubona/kubonad:prod -c 'cp -a /from/. /to/'

echo "reset-compose-state: stopping kubonad"
"${COMPOSE[@]}" stop kubonad >/dev/null

echo "reset-compose-state: removing the live ledger files"
docker run --rm -v "$STATE_VOL:/state" --entrypoint sh kubona/kubonad:prod \
  -c 'rm -f /state/kubona.db /state/kubona.db-wal /state/kubona.db-shm'

echo "reset-compose-state: starting the stack"
"${COMPOSE[@]}" up -d >/dev/null
for _ in $(seq 1 60); do
  if curl -fsS --max-time 3 http://127.0.0.1:8080/api/ready >/dev/null 2>&1; then break; fi
  sleep 2
done
curl -fsS http://127.0.0.1:8080/api/access
echo
echo "reset-compose-state: archived DB: $ARCHIVE_DIR/kubona.db"
echo "reset-compose-state: archived volume: $ARCHIVE_VOL"
echo "reset-compose-state: re-enter sealed provider keys and reviewer key settings as needed"
