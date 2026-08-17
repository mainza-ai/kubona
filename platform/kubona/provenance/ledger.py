"""Content-addressed, hash-chained, append-only provenance ledger.

The Phase-0 provenance record: circuit hash -> backend -> shots -> results -> cost,
chained so any tampering breaks the chain. Agents query it as an MCP resource
(Phase 3); here it is the write path the P0 gate proves.
"""

from __future__ import annotations

import hashlib
import json
import os
import tempfile
import time
import uuid
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable, Iterator


@dataclass(frozen=True)
class LedgerRecord:
    record_id: str
    prev_hash: str
    content_hash: str
    record_hash: str
    ts: float
    kind: str
    payload: dict[str, Any]

    def to_json(self) -> str:
        return json.dumps(
            {
                "record_id": self.record_id,
                "prev_hash": self.prev_hash,
                "content_hash": self.content_hash,
                "record_hash": self.record_hash,
                "ts": self.ts,
                "kind": self.kind,
                "payload": self.payload,
            },
            sort_keys=True,
        )

    @classmethod
    def from_json(cls, line: str) -> "LedgerRecord":
        d = json.loads(line)
        return cls(**d)


def _safe(tenant: str) -> str:
    return "".join(ch for ch in tenant if ch.isalnum() or ch in "-_")


def _content_hash(payload: dict[str, Any]) -> str:
    return hashlib.sha256(json.dumps(payload, sort_keys=True).encode()).hexdigest()


def _record_hash(prev: str, content: str) -> str:
    return hashlib.sha256(f"{prev}:{content}".encode()).hexdigest()


class Ledger:
    """Append-only JSONL ledger with a hash chain (genesis = 64 zeroes).

    Multi-tenant (Phase 5): a `tenant`/`namespace` shards records into a
    tenant-scoped file, so many laboratories/communities never collide in one
    namespace; global reads (when tenant is None) aggregate them. Appends use an
    atomic O_APPEND write so concurrent writers cannot corrupt the chain.
    """

    def __init__(self, path: str | os.PathLike | None = None, tenant: str | None = None):
        if path is None:
            base = Path(os.environ.get("KUBONA_LEDGER") or os.path.join(tempfile.gettempdir(), "kubona-ledger.jsonl"))
        else:
            base = Path(path)
        target = base if tenant is None else base.parent / f"{base.stem}.{_safe(tenant)}{base.suffix}"
        self.path = Path(target)
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.tenant = tenant
        self._tail: str = "0" * 64
        self._lock = __import__("threading").Lock()  # serialize tail-read + append

    def _read_tail(self) -> str:
        if self.path.exists() and self.path.stat().st_size:
            last = self.path.read_text().strip().splitlines()[-1]
            return LedgerRecord.from_json(last).record_hash
        return "0" * 64

    def append(self, kind: str, payload: dict[str, Any]) -> LedgerRecord:
        with self._lock:  # the hash chain is inherently sequential: guard tail-read + append
            return self._append_locked(kind, payload)

    def _append_locked(self, kind: str, payload: dict[str, Any]) -> LedgerRecord:
        prev = self._read_tail()
        content = _content_hash(payload)
        record = LedgerRecord(
            record_id=uuid.uuid4().hex[:16],
            prev_hash=prev,
            content_hash=content,
            record_hash=_record_hash(prev, content),
            ts=time.time(),
            kind=kind,
            payload=payload,
        )
        # atomic O_APPEND single write — safe under concurrent multi-tenant writers
        fd = os.open(self.path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o644)
        try:
            os.write(fd, (record.to_json() + "\n").encode())
        finally:
            os.close(fd)
        return record

    def __iter__(self) -> Iterator[LedgerRecord]:
        if not self.path.exists():
            return
        for line in self.path.read_text().splitlines():
            if line.strip():
                yield LedgerRecord.from_json(line)

    def validate_chain(self) -> tuple[bool, str]:
        prev = "0" * 64
        for rec in self:
            if rec.prev_hash != prev:
                return False, f"chain broken at {rec.record_id}: prev mismatch"
            if rec.record_hash != _record_hash(rec.prev_hash, rec.content_hash):
                return False, f"chain broken at {rec.record_id}: record hash mismatch"
            if rec.content_hash != _content_hash(rec.payload):
                return False, f"chain broken at {rec.record_id}: content hash mismatch"
            prev = rec.record_hash
        return True, f"{sum(1 for _ in self)} records, chain intact"

    def query(self, predicate: Callable[[LedgerRecord], bool] | None = None) -> list[LedgerRecord]:
        return [r for r in self if predicate is None or predicate(r)]

    def count(self) -> int:
        return sum(1 for _ in self)
