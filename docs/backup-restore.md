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

## Off-host copy

Copy the verified file and its printed sha256 to storage outside the host.
Recompute the hash after transfer:

```bash
sha256sum /backups/kubona-*.db
```

Encrypt at rest if the storage is shared (for example `age` or `gpg`). The
ledger contains spend, tenant, and provenance records.

## Restore drill

Run this on a scratch host before trusting the procedure in an incident.

```bash
# 1. Stop the daemon (single-writer lock is a file lock, not a DB lock).
systemctl stop kubonad   # or the deployment's stop command

# 2. Preserve the current DB for forensics; never delete it first.
mv /home/kubona/.kubona/kubona.db /home/kubona/.kubona/kubona.db.pre-restore

# 3. Install the verified backup and remove stale WAL sidecars.
cp /backups/kubona-20261029T180000Z.db /home/kubona/.kubona/kubona.db
rm -f /home/kubona/.kubona/kubona.db-wal /home/kubona/.kubona/kubona.db-shm

# 4. Verify before serving.
kubonad verify --db /home/kubona/.kubona/kubona.db

# 5. Start the daemon and check /api/ready.
systemctl start kubonad
```

`kubonad verify` must print `chain clean (all sessions)` before the service
is exposed. If it reports faults, keep the pre-restore file and escalate;
do not start the daemon on a faulted ledger.

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

## Current limits

The schema has no version table yet and the migration path is not
fail-closed (see the 2026-10-29 stale-state audit, P0-3). Until that lands,
take a fresh backup before every upgrade and keep the pre-upgrade file.
