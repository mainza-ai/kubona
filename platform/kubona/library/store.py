"""Provenance-backed algorithm library (§11.4).

Entries carry: harness, artifact, metric, cost, ledger reference, and validation
status. Doubles as the monetizable resource (agent-resource-economy) and the
training data for the §13.3 GQE flywheel.

Production bar:
- validated defaults False: validation requires an explicit proof path
  (validation_proof); validated=True without one refuses.
- ledger_ref is resolved against the ledger at add-time (phantoms rejected).
  Pass ledger= for existence checks or ledger_resolver= for custom policy;
  the default is a strict local check (rejects "", "none", "n/a", ...).
- best() takes an explicit per-harness objective direction ("min"/"max") —
  the library never guesses from energy-like names.
- Artifacts must be JSON-serializable before append (fail-closed, not at read).
- Entries carry owner+tenant; read/sell paths enforce tenant scope.
"""

from __future__ import annotations

import json
import math
import os
import time
import uuid
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any, Callable

from kubona.config import state_path


PHANTOM_REFS = frozenset({"", "none", "n/a", "unknown", "null", "nil"})


class LibraryError(RuntimeError):
    pass


class LibraryAccessError(LibraryError):
    pass


def strict_local_ref_check(ref: str) -> str:
    """Default ledger-ref policy: reject phantoms, accept everything else.

    This is the fallback when no ledger/resolver is wired — it cannot prove
    existence, only refuse the known-bad values. Production callers pass
    ledger= so refs resolve against the ledger at add-time.
    """
    if not isinstance(ref, str) or not ref.strip() or ref.strip().lower() in PHANTOM_REFS:
        raise LibraryError(
            f"ledger_ref {ref!r} is a phantom — refusing (wire ledger= for existence checks)"
        )
    return ref.strip()


@dataclass
class LibraryEntry:
    entry_id: str
    harness: str
    artifact: Any
    metric: float
    cost_usd: float
    ledger_ref: str
    validated: bool = False
    validation_proof: str | None = None
    owner: str = ""
    tenant: str = "public"
    ts: float = field(default_factory=time.time)
    meta: dict[str, Any] = field(default_factory=dict)


class AlgorithmLibrary:
    def __init__(
        self,
        path: str | Path | None = None,
        *,
        ledger: Any | None = None,
        ledger_resolver: Callable[[str], str] | None = None,
    ):
        self.path = Path(path) if path else state_path("library.json")
        self.path.parent.mkdir(parents=True, exist_ok=True)
        self._has_ledger = ledger_resolver is not None or ledger is not None
        if ledger_resolver is not None:
            self._resolve_ref = ledger_resolver
        elif ledger is not None:
            self._resolve_ref = self._ledger_resolver(ledger)
        else:
            self._resolve_ref = strict_local_ref_check
        self._entries: list[LibraryEntry] = []
        if self.path.exists():
            self._entries = [LibraryEntry(**e) for e in json.loads(self.path.read_text())]

    @staticmethod
    def _ledger_resolver(ledger: Any) -> Callable[[str], str]:
        def resolve(ref: str) -> str:
            if not isinstance(ref, str) or not ref.strip() or ref.strip().lower() in PHANTOM_REFS:
                raise LibraryError(f"ledger_ref {ref!r} is a phantom — refusing")
            want = ref.strip()
            for rec in ledger:
                if rec.record_id == want:
                    return want
            raise LibraryError(
                f"ledger_ref {want!r} resolves against no ledger record — refusing phantom"
            )

        return resolve

    def add(self, harness: str, artifact: Any, metric: float, cost_usd: float,
            ledger_ref: str, validated: bool = False,
            validation_proof: str | None = None,
            owner: str = "", tenant: str = "public",
            **meta: Any) -> LibraryEntry:
        if not isinstance(metric, (int, float)) or not math.isfinite(float(metric)):
            raise LibraryError(f"metric must be finite, got {metric!r}")
        if not isinstance(cost_usd, (int, float)) or not math.isfinite(float(cost_usd)):
            raise LibraryError(f"cost_usd must be finite, got {cost_usd!r}")
        if float(cost_usd) < 0:
            raise LibraryError(f"cost_usd must be >= 0, got {cost_usd!r}")
        # Serializability BEFORE append (fail-closed, not discovered at read).
        try:
            json.dumps({"artifact": artifact, "meta": meta})
        except (TypeError, ValueError) as e:
            raise LibraryError(f"artifact/meta not JSON-serializable — refusing: {e}") from e
        # Ledger resolution at add-time (reject phantoms).
        resolved_ref = self._resolve_ref(ledger_ref)
        # Validation requires an explicit, non-phantom proof. With a ledger
        # wired the proof must resolve to a real record; without one the
        # strict local policy can only reject known-bad values.
        if validated:
            proof = validation_proof.strip() if isinstance(validation_proof, str) else ""
            if not proof or proof.lower() in PHANTOM_REFS:
                raise LibraryError(
                    "validated=True requires validation_proof (explicit proof path) — refusing"
                )
            if len(proof) < 8:
                raise LibraryError(
                    f"validation_proof {proof!r} is too short to be a proof — refusing"
                )
            if self._has_ledger:
                proof = self._resolve_ref(proof)
            validation_proof = proof
        entry = LibraryEntry(entry_id=uuid.uuid4().hex[:16], harness=harness,
                             artifact=artifact, metric=float(metric), cost_usd=float(cost_usd),
                             ledger_ref=resolved_ref, validated=bool(validated),
                             validation_proof=validation_proof, owner=owner, tenant=tenant,
                             meta=meta)
        self._entries.append(entry)
        self._save()
        return entry

    def best(self, harness: str | None = None, *, direction: str,
             tenant: str | None = None) -> LibraryEntry | None:
        """Best validated entry. direction is REQUIRED ("min" or "max") — the
        library never guesses the objective from harness/energy-like names."""
        if direction not in ("min", "max"):
            raise LibraryError(
                f"direction must be 'min' or 'max' (explicit per-harness objective), got {direction!r}"
            )
        pool = [e for e in self._entries
                if (harness is None or e.harness == harness) and e.validated
                and (tenant is None or e.tenant in ("public", tenant))]
        if not pool:
            return None
        key = lambda e: e.metric
        return max(pool, key=key) if direction == "max" else min(pool, key=key)

    def get(self, entry_id: str, *, requester_tenant: str | None = None) -> LibraryEntry:
        """Tenant-scoped read: public entries are world-readable; tenant
        entries require a matching requester_tenant (fail-closed)."""
        for e in self._entries:
            if e.entry_id == entry_id:
                if e.tenant != "public" and requester_tenant != e.tenant:
                    raise LibraryAccessError(
                        f"entry {entry_id} is tenant-scoped ({e.tenant!r}) — read refused"
                    )
                return e
        raise LibraryError(f"unknown library entry {entry_id}")

    def authorize_sale(self, entry_id: str, *, buyer_tenant: str | None = None) -> LibraryEntry:
        """Sell-path enforcement: same tenant rule as read. A missing buyer
        identity refuses non-public sales (anonymous purchase of scoped
        inventory is not a sale)."""
        for e in self._entries:
            if e.entry_id == entry_id:
                if e.tenant == "public":
                    return e
                if not buyer_tenant or buyer_tenant != e.tenant:
                    raise LibraryAccessError(
                        f"entry {entry_id} is tenant-scoped ({e.tenant!r}) — sale refused"
                    )
                return e
        raise LibraryError(f"unknown library entry {entry_id}")

    def all(self) -> list[LibraryEntry]:
        return list(self._entries)

    def _save(self) -> None:
        """Atomic replace: a crash never leaves a half-written library."""
        tmp = self.path.with_suffix(self.path.suffix + ".tmp")
        with open(tmp, "w", encoding="utf-8") as fh:
            fh.write(json.dumps([asdict(e) for e in self._entries], indent=2))
            fh.flush()
            os.fsync(fh.fileno())
        os.replace(tmp, self.path)

    def __len__(self) -> int:
        return len(self._entries)
