from datetime import datetime, timedelta, timezone
from pathlib import Path

from kubona.run import _doctor_backups


def _write_backup(directory: Path, at: datetime, suffix: str = "") -> Path:
    path = directory / f"kubona.db.backup-{at.strftime('%Y%m%dT%H%M%SZ')}{suffix}"
    path.write_bytes(b"verified")
    return path


def test_missing_backup_dir_is_optional(tmp_path, monkeypatch):
    monkeypatch.setenv("KUBONA_BACKUP_DIR", str(tmp_path / "absent"))
    text, ok = _doctor_backups()
    assert ok and "not enabled" in text


def test_empty_backup_dir_is_a_warning(tmp_path, monkeypatch):
    monkeypatch.setenv("KUBONA_BACKUP_DIR", str(tmp_path))
    text, ok = _doctor_backups()
    assert not ok and "no verified backups" in text


def test_fresh_and_stale_backups(tmp_path, monkeypatch):
    now = datetime.now(timezone.utc)
    _write_backup(tmp_path, now - timedelta(hours=1))
    monkeypatch.setenv("KUBONA_BACKUP_DIR", str(tmp_path))
    monkeypatch.setenv("KUBONA_BACKUP_MAX_AGE", "26h")
    text, ok = _doctor_backups()
    assert ok and "verified artifact" in text


def test_stale_backup_fails(tmp_path, monkeypatch):
    now = datetime.now(timezone.utc)
    _write_backup(tmp_path, now - timedelta(hours=48))
    monkeypatch.setenv("KUBONA_BACKUP_DIR", str(tmp_path))
    monkeypatch.setenv("KUBONA_BACKUP_MAX_AGE", "26h")
    text, ok = _doctor_backups()
    assert not ok and "STALE" in text


def test_newer_failure_marker_fails(tmp_path, monkeypatch):
    now = datetime.now(timezone.utc)
    _write_backup(tmp_path, now - timedelta(hours=1))
    success = tmp_path / ".last-success"
    failure = tmp_path / ".last-failure"
    success.write_text(now.isoformat())
    failure.write_text(now.isoformat())
    old = now.timestamp() - 3600
    failure.touch()
    import os

    os.utime(failure, (now.timestamp(), now.timestamp()))
    os.utime(success, (old, old))
    monkeypatch.setenv("KUBONA_BACKUP_DIR", str(tmp_path))
    text, ok = _doctor_backups()
    assert not ok and "FAILED" in text


def test_invalid_max_age_fails(tmp_path, monkeypatch):
    now = datetime.now(timezone.utc)
    _write_backup(tmp_path, now)
    monkeypatch.setenv("KUBONA_BACKUP_DIR", str(tmp_path))
    monkeypatch.setenv("KUBONA_BACKUP_MAX_AGE", "26")
    text, ok = _doctor_backups()
    assert not ok and "invalid" in text
