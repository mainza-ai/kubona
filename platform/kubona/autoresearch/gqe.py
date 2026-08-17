"""GQE circuit-generation harness — the 4th harness (\u00a711.5, v2).

GQE (Generative Quantum Eigensolver) is a transformer-driven proposal strategy,
slated as a v2 upgrade AFTER Search Mode validates the sandboxed write-scope
pattern (§13.3: the library retrains GQE, which becomes the proposal strategy —
the flywheel). Production training runs via Unsloth (wiki/entities/unsloth.md).

This reference implements the SAME four-part harness contract with a
n-gram-over-good-circuits proposal strategy (a stand-in for the transformer),
so the engine, eval, and library machinery are exercised end to end; swapping
in the trained transformer is a drop-in replacement of `propose`.
"""

from __future__ import annotations

import collections
import random

from kubona.autoresearch.base import Harness
from kubona.circuits import Circuit, Gate
from kubona.providers import get_provider

# A small gate-vocabulary "good circuit" corpus (the library's historical data).
_VOCABULARY = ["h", "ry", "rz", "cx", "cz", "x"]


class GQEHarness(Harness):
    """Mutable: a proposed circuit (gate sequence). Fixed: the target amplitude
    (here: produce a Bell-like entangled distribution)."""

    def __init__(self, seed: int = 7, max_gates: int = 6):
        super().__init__("gqe_circuit")
        self.rng = random.Random(seed)
        self.max_gates = max_gates
        # a bigram model "trained" on good circuits (the flywheel's training data)
        self._bigram = collections.Counter()
        for seq in [["h", "cx"], ["h", "ry", "cx"], ["h", "h", "cz"]]:
            for i in range(len(seq) - 1):
                self._bigram[(seq[i], seq[i + 1])] += 1

    def fixed_setup(self) -> dict:
        return {"target": "entangled (00/11 distribution)", "vocab_size": len(_VOCABULARY)}

    def mutable_artifact(self):
        first = self.rng.choice(_VOCABULARY)
        return [first]

    def propose(self, seq):
        """n-gram + random proposal (transformer stand-in)."""
        if not seq:
            return [self.rng.choice(_VOCABULARY)]
        nxt = self.rng.choice(_VOCABULARY)
        candidates = [nxt]
        for (a, b), w in self._bigram.items():
            if a == seq[-1] and self.rng.random() < 0.2 * w:
                candidates.append(b)
        seq = seq + [self.rng.choice(candidates)]
        return seq[: self.max_gates]

    def metric(self, result: dict) -> float:
        # higher = more entangled (closer to 00/11 50/50), capped at 1
        return min(1.0, result["entanglement"])

    def validity_check(self, result: dict) -> bool:
        return 0 <= len(result["sequence"]) <= self.max_gates

    def run(self, seq):
        gates = []
        q = 0
        for name in seq:
            if name == "cx":
                gates.append(Gate("cx", (0, 1)))
            elif name == "cz":
                gates.append(Gate("cz", (0, 1)))
            elif name in ("ry", "rz", "rx"):
                gates.append(Gate(name, (q,), (0.5,)))  # parameterized gates need an angle
            else:
                gates.append(Gate(name, (q,)))
            q = 1 - q
        cic = Circuit("gqe", 2, gates)
        counts = get_provider("local-simulator").submit(cic, 4096, seed=3)["counts"]
        tot = sum(counts.values()) or 1
        ent = (counts.get("00", 0) + counts.get("11", 0)) / tot
        return {"sequence": seq, "entanglement": min(1.0, abs(ent - 0.5) * 2.0)}


def build_gqe_harness() -> Harness:
    return GQEHarness()
