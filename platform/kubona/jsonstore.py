"""Small shared helpers for the JSON-backed stores.

Atomic writes (temp + fsync + replace) and a per-path cross-process exclusive
lock. These stores were load-modify-write with a plain write_text: two
processes could lose records or leave a half-written file.
"""

from __future__ import annotations

import fcntl
import json
import os
import threading
from contextlib import contextmanager
from pathlib import Path
from typing import Any, Iterator

_locks: dict[str, threading.Lock] = {}
_locks_guard = threading.Lock()


def _thread_lock(path: Path) -> threading.Lock:
    key = str(path.resolve())
    with _locks_guard:
        lock = _locks.get(key)
        if lock is None:
            lock = threading.Lock()
            _locks[key] = lock
        return lock


@contextmanager
def exclusive(path: Path) -> Iterator[None]:
    lock = _thread_lock(path)
    lock_path = path.parent / (path.name + ".lock")
    with lock:
        with open(lock_path, "a+b") as lf:
            fcntl.flock(lf.fileno(), fcntl.LOCK_EX)
            try:
                yield
            finally:
                fcntl.flock(lf.fileno(), fcntl.LOCK_UN)


def atomic_write_json(path: Path, data: Any) -> None:
    tmp = path.with_suffix(path.suffix + ".tmp")
    with open(tmp, "w", encoding="utf-8") as fh:
        fh.write(json.dumps(data, indent=2, sort_keys=True) + "\n")
        fh.flush()
        os.fsync(fh.fileno())
    os.replace(tmp, path)
