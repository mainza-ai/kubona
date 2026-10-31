# Ledger backup and restore

The ledger is one SQLite WAL database (`~/.kubona/kubona.db` by default, or
`--db /abs/path`). It is the money and provenance authority; a backup that
has not been verified is not a backup.

## Backup (online, daemon may keep running)

```bash
# Absolute paths only. The command refuses relative paths and never
# overwrites an existing output.
kubonad backup \
  --db /home/kubona/.kubona/kubona.db \
  --out /backups/kubona-$(date -u +%Y%m%dT%H%M%SZ).db
```

The command uses SQLite `VACUUM INTO` for a consistent snapshot, opens the
copy read-only, runs `VerifyChain`, and prints:

```
backup: /backups/kubona-20261029T180000Z.db
size: 123456
sha256: <hex>
chain: clean
```

Any non-zero exit means the copy is not trustworthy. Do not ship it.

If the command reports `chain fault(s)` (for example legacy events with
`prev_hash=""`), the SQLite snapshot may still be consistent
(`PRAGMA integrity_check = ok`) but it is **forensic, not a verified restore
source**. Preserve it, record the fault count, and resolve the chain before
using it for restore. FK quarantine can still proceed because it is
transactional and preserves every row in `_fk_orphans`, but state the caveat
in the operator log.

## Off-host copy

Copy the verified file and its printed sha256 to storage outside the host.
Recompute the hash after transfer:

```bash
sha256sum /backups/kubona-*.db
```

Encrypt at rest if the storage is shared (for example `age` or `gpg`). The
ledger contains spend, tenant, and provenance records.

## Restore

Restore is a two-step command. The default is a dry run; `--apply` refuses
while the daemon holds the state lock.

```bash
# Dry run: verify the source and show what would move.
kubonad restore \
  --from /backups/kubona.db.backup-20261031T030000Z \
  --db /home/kubona/.kubona/kubona.db

# Stop the daemon, then apply.
systemctl stop kubonad
kubonad restore \
  --from /backups/kubona.db.backup-20261031T030000Z \
  --db /home/kubona/.kubona/kubona.db \
  --apply
systemctl start kubonad
curl -fsS http://127.0.0.1:8080/api/ready
```

`--apply` verifies the source chain and `PRAGMA integrity_check`, stages the
copy in the target directory, atomically renames it into place, and verifies
the installed ledger again. The current database and its WAL/SHM sidecars
are moved to `<db>.pre-restore-<UTC>/ledger.db` (never deleted). If the
installed copy fails verification, the command restores the preserved state.

The command exits non-zero and changes nothing when the source is faulted,
the daemon lock is held, the paths are relative, or source and target are
the same. Do not start the daemon on a faulted ledger; keep the preservation
directory and escalate.

## Scheduled backups

`compose.prod.yml` includes an opt-in `backup` profile:

```bash
# In .env:
#   KUBONA_BACKUP_AGE_RECIPIENT=age1...     # recommended
#   KUBONA_BACKUP_REQUIRE_ENCRYPTION=1      # fail closed if it is missing
docker compose --env-file .env -f platform/docker/compose.prod.yml \
  --profile backup up -d
```

The backup loop runs `kubonad backup`, optionally encrypts with `age`, writes
a sha256 receipt, and keeps the newest `KUBONA_BACKUP_KEEP` artifacts
(default 7). `kubonad restore` accepts a plaintext verified backup; decrypt an
`.age` artifact first:

```bash
age -d -i /secure/kubona-backup.key \
  /backups/kubona.db.backup-20261031T030000Z.age > /tmp/restore.db
kubonad restore --from /tmp/restore.db --db /home/kubona/.kubona/kubona.db
``` A failed backup leaves the previous verified copy in place and
records `.last-failure`. The healthcheck runs:

```bash
kubonad backup-status --dir /backups --max-age 26h
```

It is unhealthy when no backup exists or the newest is older than the age
budget. The backup directory is the `kubona-backups` volume; copy its
encrypted artifacts off-host as part of the retention policy.

## Manual recovery fallback

If the CLI is unavailable, the previous shell procedure is still documented
here as an emergency fallback: stop the daemon, move the live DB and sidecars
aside, copy the verified backup, remove stale sidecars, run `kubonad verify`,
then start the daemon. Prefer the command path because it verifies before and
after the swap and performs rollback on failure.

## Retention and cadence

- Back up at least daily, and before any migration or release.
- Keep a minimum of 7 daily and 4 weekly verified copies.
- Record the sha256 and verification result in the operator log.
- Test a restore monthly. A backup that has never been restored is a hope,
  not a recovery plan.

## Foreign-key orphan audit (before enabling enforcement)

`PRAGMA foreign_key_check` is read-only and does not take the writer lock:

```bash
kubonad fk-audit --db /home/kubona/.kubona/kubona.db
kubonad fk-audit --db /home/kubona/.kubona/kubona.db --json
```

Exit 0 means clean, 1 means orphan rows were found (the report names the
table, rowid, parent, and fkid), 2 is a usage error. Run this on every
restored copy and before a release that turns on FK enforcement. Do not
enable `PRAGMA foreign_keys=ON` while orphans exist; inserts will start
failing on rows the migration should have repaired first.

## FK orphan repair plan (dry run)

`fk-audit` finds the rows; `fk-repair-plan` classifies them without touching
anything:

```bash
kubonad fk-repair-plan --db /home/kubona/.kubona/kubona.db
kubonad fk-repair-plan --db /home/kubona/.kubona/kubona.db --json
```

It groups orphans into `content` (`chat_messages`), `spend` (`jobs`, `loops`),
and `unknown`, prints counts, and exits 1 when action is needed. It never
deletes or re-parents a row. Jobs and loops may carry reservations or mandate
references, so they need manual review. Take a verified backup first; the
actual quarantine/delete is an explicit operator action.

## Quarantine (explicit apply)

```bash
# Dry run: prints counts, writes nothing.
kubonad fk-quarantine --db /home/kubona/.kubona/kubona.db

# Apply: copies each full row to _fk_orphans, then deletes it from the live
# table in one transaction. Refuses unknown tables. Does not enable FK.
kubonad fk-quarantine --db /home/kubona/.kubona/kubona.db --apply
```

Stop the daemon first (the writer lock is exclusive), take a verified backup,
and inspect the dry run. Jobs/loops keep their spend, estimate, and mandate
fields in `_fk_orphans.row_json`; nothing is discarded. After a clean
`fk-audit`, FK enforcement can be considered.

## Production startup gate

With `KUBONA_ENV=production` (or the explicit `KUBONA_FK_ENFORCE=1`),
`kubonad` opens SQLite with `_foreign_keys=on` and runs
`PRAGMA foreign_key_check` before serving. If any orphan exists, it refuses
to start and names the repair:

```
ledger has N foreign-key orphan row(s) [...]: repair with
`kubonad fk-quarantine --apply` after a verified backup, then restart
```

This is deliberate: enforcement without the audit would only fail later,
on the first insert, with an opaque constraint error. The development/test
open path stays permissive so the audit, plan, and quarantine tools can
run against the broken ledger.

## Retention pruner

```bash
# Dry run: prints would-delete lines, removes nothing.
kubonad backup-prune --dir /backups

# Apply: only files named *.backup-YYYYMMDDTHHMMSSZ are candidates.
kubonad backup-prune --dir /backups --apply
```

Defaults keep the newest 7 backups, the newest backup per UTC day for 7
days, and the newest backup per ISO week for 4 weeks. Directories, symlinks,
and unparseable names are never touched; an all-zero retention set is refused.
Use `--now RFC3339` for a restore drill or a replay.

## Schema upgrades

The ledger records `schema_migrations` and refuses a database written by a
newer binary. When an older schema is opened, the daemon takes a verified
`VACUUM INTO` snapshot at `<db>.pre-migration-<UTC>` before any migration;
a failed snapshot or chain verification refuses the upgrade. The snapshot is
the rollback point for that upgrade. Future migrations must append a version
and keep the same fail-closed behavior.

Take a fresh verified backup before every upgrade as an independent control.
Do not restore a backup over a running daemon.
