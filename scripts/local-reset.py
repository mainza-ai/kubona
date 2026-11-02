#!/usr/bin/env python3
"""Safely reset a local Kubona ledger, after a verified online backup.

This is an operator command, never an API surface. It refuses while the daemon
holds the ledger lock, snapshots through SQLite's backup API, verifies the
snapshot, then removes the ledger and WAL/SHM files. The backup is kept.

Usage:
  python3 scripts/local-reset.py --db /abs/path/kubona.db --yes
  KUBONA_DB=/abs/path/kubona.db python3 scripts/local-reset.py --yes
"""
from __future__ import annotations

import argparse
import datetime as dt
import fcntl
import os
import sqlite3
import sys
from pathlib import Path


def fail(message: str) -> int:
    print(f"local-reset: {message}", file=sys.stderr)
    return 1


def verify_backup(path: Path) -> None:
    conn = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    try:
        rows = conn.execute("PRAGMA integrity_check").fetchall()
    finally:
        conn.close()
    if not rows or any(str(row[0]).lower() != "ok" for row in rows):
        raise RuntimeError(f"backup integrity check failed: {rows!r}")


def main() -> int:
    parser = argparse.ArgumentParser(description="Reset a local Kubona ledger after backup.")
    parser.add_argument("--db", default=os.environ.get("KUBONA_DB") or str(Path.home() / ".kubona" / "kubona.db"))
    parser.add_argument("--yes", action="store_true", help="confirm destructive reset")
    parser.add_argument("--keep-backup", default=True, action=argparse.BooleanOptionalAction)
    args = parser.parse_args()

    db = Path(args.db).expanduser().resolve()
    if not db.is_absolute():
        return fail("--db must resolve to an absolute path")
    if not db.exists():
        return fail(f"ledger not found: {db}")
    if not args.yes:
        return fail("refusing destructive reset without --yes")

    lock_path = db.with_name(db.name + ".lock")
    lock_handle = None
    try:
        lock_handle = open(lock_path, "a+")
        fcntl.flock(lock_handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
    except BlockingIOError:
        if lock_handle:
            lock_handle.close()
        return fail(f"ledger is in use by the running daemon ({lock_path}); stop kubonad first")
    except OSError as exc:
        if lock_handle:
            lock_handle.close()
        return fail(f"cannot inspect ledger lock {lock_path}: {exc}")

    stamp = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    backup = db.with_name(f"{db.name}.reset-backup-{stamp}")
    try:
        src = sqlite3.connect(f"file:{db}?mode=ro", uri=True)
        dst = sqlite3.connect(str(backup))
        try:
            src.backup(dst)
        finally:
            src.close()
            dst.close()
        verify_backup(backup)
    except Exception as exc:  # noqa: BLE001 - operator command reports the concrete cause
        try:
            backup.unlink(missing_ok=True)
        finally:
            if lock_handle:
                fcntl.flock(lock_handle.fileno(), fcntl.LOCK_UN)
                lock_handle.close()
        return fail(f"backup failed; nothing deleted: {exc}")

    removed: list[str] = []
    for suffix in ("", "-wal", "-shm"):
        target = Path(str(db) + suffix)
        if target.exists():
            target.unlink()
            removed.append(str(target))
    if not args.keep_backup:
        backup.unlink(missing_ok=True)
    if lock_handle:
        fcntl.flock(lock_handle.fileno(), fcntl.LOCK_UN)
        lock_handle.close()
    print(f"local-reset: backed up to {backup}")
    print(f"local-reset: removed {', '.join(removed) or 'no files'}")
    print("local-reset: restart kubonad to create a clean ledger")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
