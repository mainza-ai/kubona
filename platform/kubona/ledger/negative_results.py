"""Tamper-evident Negative Results & Refutations Ledger (Ring 1).

Prevents wasteful re-exploration by indexing refuted hypotheses, barren plateau traps,
and non-advantaged variational trials in an append-only hash-chained record file.

Honesty note: records are HASH-CHAINED (each links the previous record hash)
and optionally Ed25519-SIGNED when a signing key is supplied. Without a key the
record is labeled 'tamper-evident, unsigned' — detection of later edits, not
prevention. The words 'immutable' and 'cryptographic' (as a blanket claim)
do not appear here on purpose.
"""

from __future__ import annotations

import enum
import hashlib
import json
import math
import re
import time
import uuid
from dataclasses import asdict, dataclass, field
from pathlib import Path
from typing import Any, List, Optional


UNSIGNED_MODE = "tamper-evident, unsigned"
SIGNED_MODE = "ed25519-signed"


class FailureMode(str, enum.Enum):
    BARREN_PLATEAU = "BARREN_PLATEAU"
    CLASSICAL_SUPERIOR = "CLASSICAL_SUPERIOR"
    NOISE_CORRUPTION = "NOISE_CORRUPTION"
    BUDGET_EXHAUSTED = "BUDGET_EXHAUSTED"


def normalize_hamiltonian(hamiltonian: str) -> str:
    """Canonical form for Hamiltonian comparison: collapse all whitespace,
    split into +-separated terms, drop empties, sort, rejoin.

    "Z0Z1 + 0.5 X0X1" and "0.5X0X1+Z0Z1" normalize identically, so
    search_similar_failures matches on physics, not on spacing/ordering.
    """
    terms = [re.sub(r"\s+", "", t) for t in hamiltonian.split("+")]
    terms = [t for t in terms if t]
    return "+".join(sorted(terms))


def hamiltonian_hash(hamiltonian: str) -> str:
    return hashlib.sha256(normalize_hamiltonian(hamiltonian).encode("utf-8")).hexdigest()


@dataclass
class NegativeResultRecord:
    record_id: str
    timestamp: float
    prev_hash: str
    content_hash: str
    record_hash: str
    attributor: str
    signing_mode: str
    signature_hex: Optional[str]
    signer_pubkey_hex: Optional[str]
    problem_type: str  # e.g., "vqe_ground_state", "qaoa_maxcut", "circuit_synthesis"
    hamiltonian_or_graph: str
    hamiltonian_hash: str
    ansatz_family: str
    failure_mode: str  # FailureMode value
    variational_energy: float
    classical_anchor_energy: float
    delta_hartree: float
    objective_direction: str  # "min" | "max" — which way was "better"
    shots_wasted: int
    cost_usd: float
    falsification_notes: str
    raw_parameters: List[float] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return asdict(self)


class NegativeResultsLedger:
    def __init__(
        self,
        storage_dir: str | Path | None = None,
        *,
        signing_key: bytes | None = None,
    ):
        if storage_dir is not None:
            base = Path(storage_dir)
        else:
            # Default route: config.state_path (survives reboots under
            # ~/.kubona). Never /tmp: a ledger that evaporates is not a ledger.
            from kubona.config import state_path

            base = state_path("negative_results.jsonl").parent
            self.ledger_path = state_path("negative_results.jsonl")
            self.ledger_path.parent.mkdir(parents=True, exist_ok=True)
            self._signing_key = self._check_key(signing_key)
            self._last_skipped_corrupt = 0
            return
        self.ledger_path = base / "negative_results.jsonl"
        self.ledger_path.parent.mkdir(parents=True, exist_ok=True)
        self._signing_key = self._check_key(signing_key)
        self._last_skipped_corrupt = 0

    @staticmethod
    def _check_key(signing_key: bytes | None) -> bytes | None:
        if signing_key is None:
            return None
        if not isinstance(signing_key, (bytes, bytearray)) or len(signing_key) != 32:
            raise ValueError("signing_key must be a 32-byte Ed25519 seed")
        return bytes(signing_key)

    def _read_tail(self) -> str:
        if self.ledger_path.exists() and self.ledger_path.stat().st_size:
            lines = [
                ln for ln in self.ledger_path.read_text().splitlines() if ln.strip()
            ]
            # Walk back to the last DECODABLE record (tail of a file whose
            # last line is corrupt still chains to the last good record).
            for ln in reversed(lines):
                try:
                    d = json.loads(ln)
                    if isinstance(d.get("record_hash"), str):
                        return d["record_hash"]
                except (ValueError, AttributeError):
                    continue
        return "0" * 64

    def record_refutation(
        self,
        problem_type: str,
        hamiltonian: str,
        ansatz_family: str,
        failure_mode: str | FailureMode,
        variational_energy: float,
        classical_anchor_energy: float,
        *,
        attributor: str,
        objective_direction: str,
        shots_wasted: int = 0,
        cost_usd: float = 0.0,
        falsification_notes: str = "",
        raw_parameters: List[float] | None = None,
        signing_key: bytes | None = None,
    ) -> NegativeResultRecord:
        """Append a hash-chained (and optionally signed) refutation record.

        attributor is REQUIRED (every record names who attributes the
        failure — unattributed records refuse). objective_direction ("min" |
        "max") records which way was 'better' for this objective. signing_key
        overrides the ledger-level key for this record; with no key anywhere
        the record is labeled tamper-evident, unsigned.
        """
        try:
            fm = (
                failure_mode
                if isinstance(failure_mode, FailureMode)
                else FailureMode(str(failure_mode))
            )
        except ValueError:
            raise ValueError(
                f"unknown failure_mode {failure_mode!r}; "
                f"known: {[m.value for m in FailureMode]}"
            ) from None
        if not isinstance(attributor, str) or not attributor.strip():
            raise ValueError("attributor is required: unattributed refutations refuse")
        if objective_direction not in ("min", "max"):
            raise ValueError(
                f"objective_direction must be 'min' or 'max', got {objective_direction!r}"
            )
        for name, val in (("variational_energy", variational_energy),
                          ("classical_anchor_energy", classical_anchor_energy),
                          ("cost_usd", cost_usd)):
            if not isinstance(val, (int, float)) or not math.isfinite(float(val)):
                raise ValueError(f"{name} must be a finite number, got {val!r}")
        if float(cost_usd) < 0:
            raise ValueError(f"cost_usd must be >= 0, got {cost_usd!r}")
        if not isinstance(shots_wasted, int) or isinstance(shots_wasted, bool) or shots_wasted < 0:
            raise ValueError(f"shots_wasted must be a non-negative int, got {shots_wasted!r}")

        prev = self._read_tail()
        norm = normalize_hamiltonian(hamiltonian)
        content = {
            "problem_type": problem_type,
            "hamiltonian": norm,
            "hamiltonian_hash": hamiltonian_hash(hamiltonian),
            "ansatz_family": ansatz_family,
            "failure_mode": fm.value,
            "variational_energy": float(variational_energy),
            "classical_anchor_energy": float(classical_anchor_energy),
            "delta_hartree": float(variational_energy) - float(classical_anchor_energy),
            "objective_direction": objective_direction,
            "shots_wasted": shots_wasted,
            "cost_usd": float(cost_usd),
            "falsification_notes": falsification_notes,
            "raw_parameters": list(raw_parameters or []),
            "attributor": attributor.strip(),
        }
        content_hash = hashlib.sha256(
            json.dumps(content, sort_keys=True).encode("utf-8")
        ).hexdigest()
        record_hash = hashlib.sha256(f"{prev}:{content_hash}".encode("utf-8")).hexdigest()

        key = self._check_key(signing_key) if signing_key is not None else self._signing_key
        if key is not None:
            from nacl.signing import SigningKey

            sk = SigningKey(key)
            sig = sk.sign(record_hash.encode("utf-8")).signature.hex()
            mode, pub = SIGNED_MODE, sk.verify_key.encode().hex()
        else:
            sig, mode, pub = None, UNSIGNED_MODE, None

        rec = NegativeResultRecord(
            record_id=uuid.uuid4().hex[:16],
            timestamp=time.time(),
            prev_hash=prev,
            content_hash=content_hash,
            record_hash=record_hash,
            attributor=attributor.strip(),
            signing_mode=mode,
            signature_hex=sig,
            signer_pubkey_hex=pub,
            problem_type=problem_type,
            hamiltonian_or_graph=hamiltonian,
            hamiltonian_hash=hamiltonian_hash(hamiltonian),
            ansatz_family=ansatz_family,
            failure_mode=fm.value,
            variational_energy=float(variational_energy),
            classical_anchor_energy=float(classical_anchor_energy),
            delta_hartree=float(variational_energy) - float(classical_anchor_energy),
            objective_direction=objective_direction,
            shots_wasted=shots_wasted,
            cost_usd=float(cost_usd),
            falsification_notes=falsification_notes,
            raw_parameters=list(raw_parameters or []),
        )
        with open(self.ledger_path, "a", encoding="utf-8") as f:
            f.write(json.dumps(rec.to_dict(), sort_keys=True) + "\n")
        return rec

    def load(self) -> tuple[List[NegativeResultRecord], int]:
        """All decodable records + count of skipped corrupt lines.

        Corrupt lines are SKIPPED AND COUNTED (never crash the read, never
        silently dropped without a number).
        """
        records: List[NegativeResultRecord] = []
        skipped = 0
        if not self.ledger_path.exists():
            self._last_skipped_corrupt = 0
            return records, 0
        with open(self.ledger_path, "r", encoding="utf-8") as f:
            for line in f:
                if not line.strip():
                    continue
                try:
                    d = json.loads(line)
                    records.append(NegativeResultRecord(**d))
                except (ValueError, TypeError, KeyError):
                    skipped += 1
        self._last_skipped_corrupt = skipped
        return records, skipped

    def all(self) -> List[NegativeResultRecord]:
        records, _ = self.load()
        return records

    @property
    def last_skipped_corrupt(self) -> int:
        return self._last_skipped_corrupt

    def verify_chain(self) -> tuple[bool, str]:
        """Recompute the hash chain (and Ed25519 signatures where present)."""
        prev = "0" * 64
        n = 0
        records, skipped = self.load()
        for rec in records:
            if rec.prev_hash != prev:
                return False, f"chain broken at {rec.record_id}: prev mismatch"
            content = {
                "problem_type": rec.problem_type,
                "hamiltonian": normalize_hamiltonian(rec.hamiltonian_or_graph),
                "hamiltonian_hash": rec.hamiltonian_hash,
                "ansatz_family": rec.ansatz_family,
                "failure_mode": rec.failure_mode,
                "variational_energy": rec.variational_energy,
                "classical_anchor_energy": rec.classical_anchor_energy,
                "delta_hartree": rec.delta_hartree,
                "objective_direction": rec.objective_direction,
                "shots_wasted": rec.shots_wasted,
                "cost_usd": rec.cost_usd,
                "falsification_notes": rec.falsification_notes,
                "raw_parameters": rec.raw_parameters,
                "attributor": rec.attributor,
            }
            content_hash = hashlib.sha256(
                json.dumps(content, sort_keys=True).encode("utf-8")
            ).hexdigest()
            if content_hash != rec.content_hash:
                return False, f"chain broken at {rec.record_id}: content hash mismatch"
            if hashlib.sha256(f"{rec.prev_hash}:{content_hash}".encode()).hexdigest() != rec.record_hash:
                return False, f"chain broken at {rec.record_id}: record hash mismatch"
            if rec.signing_mode == SIGNED_MODE:
                try:
                    from nacl.signing import VerifyKey

                    VerifyKey(bytes.fromhex(rec.signer_pubkey_hex or "")).verify(
                        bytes.fromhex(rec.signature_hex or "") + rec.record_hash.encode()
                    )
                except Exception as e:
                    return False, f"chain broken at {rec.record_id}: bad signature ({e})"
            prev = rec.record_hash
            n += 1
        return True, f"{n} records, chain intact ({skipped} corrupt lines skipped)"

    def search_similar_failures(self, hamiltonian: str, ansatz_family: str) -> List[NegativeResultRecord]:
        want = hamiltonian_hash(hamiltonian)
        return [
            r for r in self.all()
            if r.hamiltonian_hash == want and r.ansatz_family == ansatz_family
        ]
