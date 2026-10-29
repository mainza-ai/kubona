"""Content-addressed, hash-chained, append-only provenance ledger.

The Phase-0 provenance record: circuit hash -> backend -> shots -> results -> cost,
chained so any tampering breaks the chain. Agents query it as an MCP resource
(Phase 3); here it is the write path the P0 gate proves.
"""

from __future__ import annotations

import fcntl
import hashlib
import json
import os
import tempfile
import time
import uuid
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Callable, Iterator

from kubona.session import strict_tenant_slug


@dataclass(frozen=True)
class LedgerRecord:
    record_id: str
    prev_hash: str
    content_hash: str
    record_hash: str
    ts: float
    kind: str
    payload: dict[str, Any]
    # Confidentiality label: "public" or "tenant/<id>". Defaults to "" for
    # pre-label rows, which read as the file's own tenant (files are already
    # sharded per tenant). Use derive_label() for derived records.
    label: str = ""

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
                "label": self.label,
            },
            sort_keys=True,
        )

    @classmethod
    def from_json(cls, line: str) -> "LedgerRecord":
        d = json.loads(line)
        return cls(**d)


def shard_path_for(base: str | os.PathLike, tenant: str) -> Path:
    """Shard file for a tenant (Phase-2 physical sharding).

    Strict: non-canonical tenants refuse instead of collapsing onto a
    neighbor file (the old `_safe` stripped hostile chars — `"..."`
    collapsed onto the default namespace). The default tenant keeps the
    base path (no suffix); every other tenant gets
    `<stem>.<slug><suffix>` beside it.
    """
    slug = strict_tenant_slug(tenant)
    base_p = Path(base)
    if slug == "default":
        return base_p
    return base_p.parent / f"{base_p.stem}.{slug}{base_p.suffix}"


def base_legacy_default_shard(base: str | os.PathLike) -> Path:
    """Pre-Phase-2 `ledger.default.jsonl` beside `base`, if any.

    Transitional back-compat only: old code mapped `tenant="default"` to a
    suffixed shard; Phase-2 maps it to the base file itself. New code must
    never write here.
    """
    base_p = Path(base)
    return base_p.parent / f"{base_p.stem}.default{base_p.suffix}"


def _content_hash(payload: dict[str, Any], label: str = "") -> str:
    # Versioned binding: pre-label rows hash the payload alone (chain stays
    # valid for every existing file); labeled rows bind payload+label so a
    # forged label breaks validation exactly like forged content.
    if label:
        return hashlib.sha256(
            (json.dumps(payload, sort_keys=True) + "\nlabel:" + label).encode()
        ).hexdigest()
    return hashlib.sha256(json.dumps(payload, sort_keys=True).encode()).hexdigest()


def _record_hash(prev: str, content: str) -> str:
    return hashlib.sha256(f"{prev}:{content}".encode()).hexdigest()


def _contract_path() -> Path:
    # R1a: single-source closed taxonomy shared with the Go rail
    # (platform/contracts/event-types.json). Explicit override first (tests,
    # installs), else the repo tree relative to this file. Missing contract
    # = fail-closed at import: running without the registry would fork the
    # audit trail with untyped kinds.
    override = os.environ.get("KUBONA_EVENT_TYPES")
    if override:
        return Path(override)
    return Path(__file__).resolve().parents[2] / "contracts" / "event-types.json"


def _load_taxonomy() -> dict[str, dict[str, Any]]:
    path = _contract_path()
    try:
        contract = json.loads(path.read_text())
    except OSError as e:
        raise RuntimeError(
            f"event taxonomy contract missing at {path}: refusing to run "
            f"without the closed registry ({e})"
        ) from e
    if contract.get("version") != 1:
        raise RuntimeError(
            f"event taxonomy contract version {contract.get('version')!r}: "
            "this code knows v1 — refusing unknown major"
        )
    return contract["types"]


KNOWN_EVENT_TYPES: dict[str, dict[str, Any]] = _load_taxonomy()


class Ledger:
    """Append-only JSONL ledger with a hash chain (genesis = 64 zeroes).

    Multi-tenant (Phase 5): a `tenant`/`namespace` shards records into a
    tenant-scoped file (`<stem>.<slug><suffix>` beside the base path, via
    `shard_path_for`), so different laboratories never share namespace or
    files. The default file (tenant None, or the `"default"` slug) holds
    only unscoped/public rows for new writes; tenant rows go to their
    shard. Migration (transitional): pre-sharding `tenant/<T>` rows still
    physically sit in the default file and stay readable through the
    back-compat read path (`iter_as`/`query_as` union the default file with
    the requester's shard); cross-tenant reads of another tenant's shard
    refuse outright. Appends use an atomic O_APPEND write so concurrent
    writers cannot corrupt the chain.
    """

    # Registry: tenant slug -> shard Ledger for one base path. Process-local
    # cache so repeated `for_tenant` lookups share the object; the files
    # themselves are the source of truth (no in-memory state to drift).
    _registry: dict[tuple[str, str], "Ledger"] = {}

    def __init__(self, path: str | os.PathLike | None = None, tenant: str | None = None):
        if tenant is not None:
            # Strict entry: hostile/non-canonical tenants refuse instead of
            # collapsing onto a neighbor file (the old `_safe` stripped
            # characters — `"a/b"` and `"a b"` landed in the same shard).
            tenant = strict_tenant_slug(tenant)
        if path is None:
            from kubona.config import ledger_path

            base = ledger_path()
        else:
            base = Path(path)
        if tenant is None:
            target = base
            tenant = "default"
        else:
            target = shard_path_for(base, tenant)
        self.path = Path(target)
        self.base = Path(base)
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self.tenant = tenant
        self._tail: str = "0" * 64
        self._lock = __import__("threading").Lock()  # serialize tail-read + append

    @classmethod
    def for_tenant(cls, tenant: str, path: str | os.PathLike | None = None) -> "Ledger":
        """Registry lookup: the shard Ledger for `tenant` under `path`.

        Strict (non-canonical tenants refuse); cached per (base, slug) so
        callers share one object per shard. New tenant writes belong here —
        never hand-built `tenant/<T>` labels in the default file.
        """
        slug = strict_tenant_slug(tenant)
        if path is None:
            from kubona.config import ledger_path

            base = ledger_path()
        else:
            base = Path(path)
        key = (str(base), slug)
        hit = cls._registry.get(key)
        if hit is None:
            hit = cls(path=base, tenant=slug)
            cls._registry[key] = hit
        return hit

    @classmethod
    def _open_file(cls, base: str | os.PathLike, path: str | os.PathLike,
                   tenant: str) -> "Ledger":
        """Open an exact file as a Ledger without shard-path derivation.

        Internal (transitional back-compat reads only): `tenant` labels the
        file's tenant context for `readable`, `path` is used verbatim.
        """
        obj = cls.__new__(cls)
        obj.path = Path(path)
        obj.base = Path(base)
        obj.path.parent.mkdir(parents=True, exist_ok=True)
        obj.tenant = tenant
        obj._tail = "0" * 64
        obj._lock = __import__("threading").Lock()
        return obj

    def _read_tail(self) -> str:
        if self.path.exists() and self.path.stat().st_size:
            last = self.path.read_text().strip().splitlines()[-1]
            return LedgerRecord.from_json(last).record_hash
        return "0" * 64

    def append(self, kind: str, payload: dict[str, Any], label: str | None = None) -> LedgerRecord:
        # R1a closed taxonomy (C1): unknown kinds refuse here — a typo'd kind
        # used to silently fork the cross-rail audit trail, and the MCP
        # write_record tool passes agent-supplied kinds straight through, so
        # this is also the tool-boundary enforcement. Register the kind in
        # platform/contracts/event-types.json first.
        # R5: the refusal is typed (bad_request, non-retryable) so MCP
        # callers can implement retry-vs-escalate from the catalog.
        if kind not in KNOWN_EVENT_TYPES:
            from kubona.errors import KubonaError

            raise KubonaError(
                "bad_request",
                f"unknown ledger event kind {kind!r}: not in the shared "
                "event taxonomy (platform/contracts/event-types.json)",
                {"kind": kind},
            )
        with self._lock:  # the hash chain is inherently sequential: guard tail-read + append
            return self._append_locked(kind, payload, label)

    def _append_locked(self, kind: str, payload: dict[str, Any], label: str | None = None) -> LedgerRecord:
        # Cross-process single-writer: self._lock serializes threads, but
        # concurrent PROCESSES share this file (gateway + tests, two suites
        # at once) — tail-read + append must hold an flock, mirroring the
        # Go ledger's <db>.lock (E-1a). Proven necessary 2026-10-12: two
        # concurrent pytest runs forked the chain with identical prev_hashes.
        lock_path = self.path.parent / (self.path.name + ".lock")
        with open(lock_path, "a+b") as lf:
            fcntl.flock(lf.fileno(), fcntl.LOCK_EX)
            try:
                prev = self._read_tail()
                # Default label is the file's own lab tenant (files are sharded per
                # tenant); explicit "public" is the only way to widen readership.
                resolved = label if label else f"tenant/{self.tenant or 'default'}"
                if resolved.startswith("tenant/"):
                    # Strict label derivation: a hostile suffix refuses instead
                    # of collapsing (the old `_safe` path rule never applied
                    # here — a hand-built "tenant/a b" label hashed cleanly
                    # and read back as nobody's tenant).
                    strict_tenant_slug(resolved[len("tenant/"):])
                elif resolved not in ("public", ""):
                    raise ValueError(f"unknown ledger label shape {resolved!r}: refusing")
                content = _content_hash(payload, resolved)
                record = LedgerRecord(
                    record_id=uuid.uuid4().hex[:16],
                    prev_hash=prev,
                    content_hash=content,
                    record_hash=_record_hash(prev, content),
                    ts=time.time(),
                    kind=kind,
                    payload=payload,
                    label=resolved,
                )
                # O_APPEND single write, under the flock above (the old
                # "atomic O_APPEND is enough" comment was wrong: the
                # tail-read + append sequence is what must be exclusive).
                fd = os.open(self.path, os.O_WRONLY | os.O_CREAT | os.O_APPEND, 0o644)
                try:
                    os.write(fd, (record.to_json() + "\n").encode())
                    os.fsync(fd)
                finally:
                    os.close(fd)
                return record
            finally:
                fcntl.flock(lf.fileno(), fcntl.LOCK_UN)

    def __iter__(self) -> Iterator[LedgerRecord]:
        if not self.path.exists():
            return
        # Shared flock: a concurrent append must not be observed as a torn
        # final line (the writer holds LOCK_EX across tail-read + write).
        lock_path = self.path.parent / (self.path.name + ".lock")
        with open(lock_path, "a+b") as lf:
            fcntl.flock(lf.fileno(), fcntl.LOCK_SH)
            try:
                raw = self.path.read_text()
            finally:
                fcntl.flock(lf.fileno(), fcntl.LOCK_UN)
        for line in raw.splitlines():
            if line.strip():
                yield LedgerRecord.from_json(line)

    def validate_chain(self) -> tuple[bool, str]:
        prev = "0" * 64
        for rec in self:
            if rec.prev_hash != prev:
                return False, f"chain broken at {rec.record_id}: prev mismatch"
            if rec.record_hash != _record_hash(rec.prev_hash, rec.content_hash):
                return False, f"chain broken at {rec.record_id}: record hash mismatch"
            if rec.content_hash != _content_hash(rec.payload, rec.label):
                return False, f"chain broken at {rec.record_id}: content hash mismatch"
            prev = rec.record_hash
        return True, f"{sum(1 for _ in self)} records, chain intact"

    def query(self, predicate: Callable[[LedgerRecord], bool] | None = None) -> list[LedgerRecord]:
        return [r for r in self if predicate is None or predicate(r)]

    def count(self) -> int:
        return sum(1 for _ in self)

    def iter_as(self, requester: str | None) -> Iterator[LedgerRecord]:
        """Readership-scoped read (Phase-2 physical sharding).

        File-local rows are filtered by `readable` against their own file
        tenant — never another tenant's rows. A shard (`tenant !=
        "default"`) additionally refuses the OPEN itself when `requester !=
        shard tenant` (fail-closed physical boundary, not just a filter:
        a cross-tenant `Ledger.for_tenant("lab-b").iter_as("lab-a")`
        raises instead of yielding zero rows). The default file unions its
        own rows (transitional: pre-sharding `tenant/<T>` rows still sit
        here) with the requester's shard so one read covers both; other
        tenants' shards are never touched.
        """
        if self.tenant != "default":
            if requester != self.tenant:
                raise PermissionError(
                    f"cross-tenant shard read refused: shard {self.tenant!r} "
                    f"requested as {requester!r}")
            for rec in self:
                if readable(rec, requester, self.tenant):
                    yield rec
            return
        # Default file: own rows first (transitional: pre-sharding
        # tenant/<T> rows still physically sit here and stay readable).
        # OPERATOR DECISION (ledger-legacy residual, closed): NO data
        # migration, ever. Sealed rows are content-addressed history —
        # moving/rewriting them would re-hash and fork the audit trail, so
        # pre-sharding `tenant/<T>` rows stay in place in the default file
        # and the transitional `ledger.default.jsonl` (if any) stays in
        # place. The union below is the PERMANENT read path, not a migration
        # shim. If a future operator wants physical consolidation it must be
        # a COPY-with-provenance-link (new rows pointing back, originals
        # untouched), never a rewrite — and that needs a new decision, not
        # this one.
        seen_shard = self.path.resolve() if self.path.exists() else self.path
        for rec in self:
            if readable(rec, requester, "default"):
                yield rec
        if requester is None:
            return  # ambient reads never touch a named shard
        try:
            want = strict_tenant_slug(requester)
        except ValueError:
            return  # malformed requester sees default-file rows only (filtered above)
        if want == "default":
            # Transitional: pre-Phase-2 `Ledger(base, tenant="default")`
            # writers created a `ledger.default.jsonl` shard beside the base
            # file. Union it so those rows stay readable; new "default"
            # writes land in the base file itself.
            legacy = base_legacy_default_shard(self.base)
            if legacy.exists() and legacy.resolve() != seen_shard:
                for rec in Ledger._open_file(self.base, legacy, "default"):
                    if readable(rec, requester, "default"):
                        yield rec
            return
        shard = shard_path_for(self.base, want)
        if shard.resolve() == seen_shard or not shard.exists():
            return
        other = Ledger(path=self.base, tenant=want)
        for rec in other:
            if readable(rec, requester, want):
                yield rec

    def query_as(self, requester: str | None,
                 predicate: Callable[[LedgerRecord], bool] | None = None) -> list[LedgerRecord]:
        """`iter_as` plus an optional predicate (back-compat read path)."""
        return [r for r in self.iter_as(requester)
                if predicate is None or predicate(r)]


def readable(record: LedgerRecord, requester: str | None, file_tenant: str | None = None) -> bool:
    """Pre-serialization readership check (IFC seam 1 of 3).

    Public and pre-label ("") records are world-readable; tenant records are
    readable inside their own lab tenant only. requester None means
    unauthenticated ambient access: public rows only... except pre-label rows,
    which predate labels and read as their file's tenant context. Callers that
    know their tenant must pass it; ambient MCP resources pass None and see
    public rows (never another tenant's).
    """
    label = record.label
    if label == "public":
        return True
    if not label:
        # Legacy pre-label rows: the file's own tenant is the boundary. An
        # ambient reader (requester None) must NOT inherit that tenant just
        # because the row predates labels; a named reader must match it.
        if requester is None or file_tenant is None:
            return False
        return requester == file_tenant
    if requester is None:
        return False
    return label == f"tenant/{requester}"


def derive_label(input_labels: list[str], ctx_tenant: str) -> str:
    """Derivation label for a record built from inputs (IFC join, v1).

    All-public inputs stay public; any tenant-scoped input restricts the
    derivative to the deriving tenant's label. Cross-tenant derivation is
    refused upstream (no grant model yet) — this function never widens.
    """
    labels = [l for l in input_labels if l]
    if not labels or all(l == "public" for l in labels):
        return "public"
    return f"tenant/{ctx_tenant}"
