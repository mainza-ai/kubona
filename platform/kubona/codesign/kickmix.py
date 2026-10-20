"""Kickmix trajectory evaluator (Methods Eqs 10-16, SI Eqs 17-21).

Models the paper's circuit class exactly:
- classical reversible: X, CNOT (cx), Toffoli (ccx), SWAP
- diagonal: Z, CZ, CCZ (+ global sign flip)
- destructive X-basis measurement (hmr) with classically-conditioned
  diagonal correction (cz/ccz conditioned on the record).

A computational-basis trajectory tracks (bits, phase) only — never 2^n
amplitudes. Every op maps basis -> basis, possibly flipping phase.

Validity (fail-closed, no vibes):
- output bits match target permutation pi(x)
- required inputs unchanged
- every ancilla allocated via Builder returns to |0> at free()
- phase matches target g(x) exactly for EVERY measurement record
- a measurement record may XOR/combine and control DIAGONAL corrections
  only; any record-controlled basis-changing gate REFUSES.

Statistics: k passes -> eps <= 1-delta^{1/k}; bounded-failure P_{k,b}.
"""

from __future__ import annotations

import math
import random
from dataclasses import dataclass, field
from typing import Any, Callable

from .factories import factory_report


#: Gates the evaluator executes. H is excluded by construction: it creates
#: superposition and is outside kickmix — refusing is the honest answer,
#: not a transpiler gap.
KICKMIX_REVERSIBLE = frozenset({"x", "cx", "ccx", "swap"})
KICKMIX_DIAGONAL = frozenset({"z", "cz", "ccz", "global_flip"})
KICKMIX_MEASURE = frozenset({"hmr"})
KICKMIX_ALL = KICKMIX_REVERSIBLE | KICKMIX_DIAGONAL | KICKMIX_MEASURE

#: Toffoli-equivalent counting: ccx + ccz count equally (paper Methods).
TOFFOLI_GATES = frozenset({"ccx", "ccz"})


@dataclass
class Op:
    name: str
    qubits: tuple[int, ...]
    cond: tuple[int, ...] = ()  # measurement-record ids gating a DIAGONAL op


@dataclass
class TrajectoryResult:
    valid: bool
    reason: str
    toffoli: int = 0
    peak_qubits: int = 0
    phase: int = 0
    final_bits: dict[int, int] = field(default_factory=dict)


@dataclass
class TaskSpec:
    """Hand-written spec (no implementation hint) — the fixed setup.

    - num_qubits: total addressable qubits in the Builder program.
    - inputs: qubit ids that must be preserved.
    - outputs: qubit ids carrying y xor f(x) (checked against target_perm).
    - ancillas: ids that must return to 0 (allocated via Builder).
    - target_perm: bits -> bits (permutation pi).
    - target_phase: bits -> 0/1 (boolean phase g); default 0.
    - input_distribution: callable(rng) -> dict[id, bit] for sampling.
    """

    num_qubits: int
    inputs: tuple[int, ...]
    outputs: tuple[int, ...]
    ancillas: tuple[int, ...] = ()
    target_perm: Callable[[dict[int, int]], dict[int, int]] | None = None
    target_phase: Callable[[dict[int, int]], int] | None = None
    input_distribution: Callable[[random.Random], dict[int, int]] | None = None

    def __post_init__(self) -> None:
        if not isinstance(self.num_qubits, int) or self.num_qubits < 1:
            raise ValueError(f"num_qubits must be positive int, got {self.num_qubits!r}")


class Builder:
    """Circuit-generating program (paper's Python interface).

    alloc/free track liveness for peak-Q; ops record the fixed gate
    sequence. Emission-time classical computation is allowed (plain
    Python before emit); the emitted op list is what the evaluator checks.
    """

    def __init__(self, num_qubits: int):
        if not isinstance(num_qubits, int) or num_qubits < 1:
            raise ValueError("num_qubits must be positive int")
        self.num_qubits = num_qubits
        self.ops: list[Op] = []
        self._alloced: set[int] = set()
        self._freed: set[int] = set()
        self._consumed: set[int] = set()  # allocs released by hmr (no free() needed)
        self._peak = 0
        self._live = 0
        self._next_measure_id = 0

    def _live_allocs(self) -> set[int]:
        return set(self._alloced) - set(self._freed) - set(self._consumed)

    def leaked_allocs(self) -> set[int]:
        """Allocs never freed nor measured away (fail-closed verdict input)."""
        return self._live_allocs()

    # -- allocation -----------------------------------------------------
    def alloc(self, q: int) -> int:
        if q < 0 or q >= self.num_qubits:
            raise ValueError(f"alloc qubit {q} out of range [0,{self.num_qubits})")
        if q in self._live_allocs():
            raise ValueError(f"qubit {q} already live: double-alloc refused")
        self._alloced.add(q)
        # re-alloc after free/consume is a fresh live interval
        self._freed.discard(q)
        self._consumed.discard(q)
        self._live += 1
        self._peak = max(self._peak, self._live)
        self.ops.append(Op("alloc_marker", (q,)))
        return q

    def alloc_many(self, qs: list[int]) -> list[int]:
        return [self.alloc(q) for q in qs]

    def free(self, q: int) -> None:
        if q in self._consumed:
            raise ValueError(f"qubit {q} already released by measurement: do not free")
        if q not in self._alloced or q in self._freed:
            raise ValueError(f"free of non-live qubit {q}: refusing")
        self.ops.append(Op("free_marker", (q,)))
        self._freed.add(q)
        self._live -= 1

    # -- gates ----------------------------------------------------------
    def _gate(self, name: str, qs: tuple[int, ...], cond: tuple[int, ...] = ()) -> None:
        if name not in KICKMIX_ALL:
            raise ValueError(f"gate {name!r} outside kickmix set {sorted(KICKMIX_ALL)}: refusing")
        for q in qs:
            if q < 0 or q >= self.num_qubits:
                raise ValueError(f"qubit {q} out of range")
        if cond and name not in KICKMIX_DIAGONAL:
            raise ValueError(
                f"measurement record gates basis-changing {name!r}: refusing "
                "(records may control diagonal corrections only)"
            )
        self.ops.append(Op(name, qs, cond))

    def x(self, q: int) -> None:
        self._gate("x", (q,))

    def cx(self, c: int, t: int) -> None:
        if c == t:
            raise ValueError("cx control == target: refusing")
        self._gate("cx", (c, t))

    def ccx(self, a: int, b: int, t: int) -> None:
        if len({a, b, t}) != 3:
            raise ValueError("ccx needs 3 distinct qubits: refusing")
        self._gate("ccx", (a, b, t))

    def swap(self, a: int, b: int) -> None:
        if a == b:
            raise ValueError("swap needs 2 distinct qubits: refusing")
        self._gate("swap", (a, b))

    def z(self, q: int, cond: tuple[int, ...] = ()) -> None:
        self._gate("z", (q,), cond)

    def cz(self, a: int, b: int, cond: tuple[int, ...] = ()) -> None:
        self._gate("cz", (a, b), cond)

    def ccz(self, a: int, b: int, c: int, cond: tuple[int, ...] = ()) -> None:
        self._gate("ccz", (a, b, c), cond)

    def global_flip(self, cond: tuple[int, ...] = ()) -> None:
        self._gate("global_flip", (), cond)

    def hmr(self, q: int) -> int:
        """Destructive X-basis measurement: returns record id, releases qubit.

        The qubit's bit is consumed (it held f(y)); the record bit r is
        uniform 0/1 per trajectory. The caller must add the matching
        diagonal correction conditioned on the returned id.
        """
        if q < 0 or q >= self.num_qubits:
            raise ValueError(f"hmr qubit {q} out of range")
        rid = self._next_measure_id
        self._next_measure_id += 1
        self.ops.append(Op("hmr", (q,)))
        # measurement releases a live alloc interval (no free() needed after)
        if q in self._live_allocs():
            self._consumed.add(q)
            self._live -= 1
        return rid

    def num_measurements(self) -> int:
        return sum(1 for o in self.ops if o.name == "hmr")

    def peak_qubits(self) -> int:
        # peak live allocs, floored by total addressable width
        return max(self.num_qubits, self._peak)

    def toffoli_count(self) -> int:
        return sum(1 for o in self.ops if o.name in TOFFOLI_GATES)

    def emit(self) -> list[Op]:
        return list(self.ops)


def emit_xor_cover(b: "Builder", n_ccx: list[int],
                   lits_list: list[list[tuple[int, int]]],
                   temps: list[int], out: int) -> None:
    """XOR-accumulate one output from AND-cubes (shared-temp discipline).

    lits per cube: [(qubit, polarity)]. Negated literals flip for the
    chain and unflip after the mirror (flips persist through the mirror:
    it reuses the same controls). Singletons are CNOTs (double-X wrapped
    when negated); the empty cube is X. Temps shared sequentially across
    cubes; every temp freed by the caller (dirty-free refuses otherwise).
    One logic home for the ANF/decoded/minimized emitters (was three
    copies that could rot apart; all guarded by their exhaustive proofs).
    """
    for lits in lits_list:
        if len(lits) == 0:
            b.x(out)
            continue
        if len(lits) == 1:
            q, pol = lits[0]
            if not pol:
                b.x(q)
            b.cx(q, out)
            if not pol:
                b.x(q)
            continue
        for q, pol in lits:
            if not pol:
                b.x(q)
        b.ccx(lits[0][0], lits[1][0], temps[0])
        n_ccx[0] += 1
        for k in range(2, len(lits)):
            b.ccx(temps[k - 2], lits[k][0], temps[k - 1])
            n_ccx[0] += 1
        b.cx(temps[len(lits) - 2], out)
        for k in range(len(lits) - 1, 1, -1):
            b.ccx(temps[k - 2], lits[k][0], temps[k - 1])
            n_ccx[0] += 1
        b.ccx(lits[0][0], lits[1][0], temps[0])
        n_ccx[0] += 1
        for q, pol in lits:
            if not pol:
                b.x(q)


class _DirtyFree(ValueError):
    """Internal: free_marker hit with the qubit holding a 1 (uncomputed state)."""


def _sim_trajectory(
    ops: list[Op],
    init_bits: dict[int, int],
    rng: random.Random,
    force_record: list[int] | None = None,
) -> tuple[dict[int, int], int, list[int]]:
    """Simulate one basis trajectory. Returns (bits, phase, record).

    force_record fixes the measurement outcomes (exhaustive enumeration);
    otherwise outcomes sample uniformly via rng (Methods: uniform records).
    """
    bits = dict(init_bits)
    phase = 0
    forced = list(force_record) if force_record is not None else None
    record: list[int] = []

    def bit(q: int) -> int:
        return bits.get(q, 0) & 1

    for op in ops:
        n = op.name
        if n == "alloc_marker":
            (q,) = op.qubits
            bits[q] = 0  # fresh allocs start |0>; stale reuse would hide leaks
            continue
        if n == "free_marker":
            (q,) = op.qubits
            if bit(q) != 0:
                raise _DirtyFree(f"qubit {q} freed holding 1 (uncomputed): refusing")
            continue
        if n == "x":
            (q,) = op.qubits
            bits[q] = bit(q) ^ 1
        elif n == "cx":
            c, t = op.qubits
            if bit(c):
                bits[t] = bit(t) ^ 1
        elif n == "ccx":
            a, b, t = op.qubits
            if bit(a) and bit(b):
                bits[t] = bit(t) ^ 1
        elif n == "swap":
            a, b = op.qubits
            bits[a], bits[b] = bit(b), bit(a)
        elif n == "z":
            (q,) = op.qubits
            fire = all(record[i] == 1 for i in op.cond) if op.cond else True
            if fire and bit(q):
                phase ^= 1
        elif n == "cz":
            a, b = op.qubits
            fire = all(record[i] == 1 for i in op.cond) if op.cond else True
            if fire and bit(a) and bit(b):
                phase ^= 1
        elif n == "ccz":
            a, b, c = op.qubits
            fire = all(record[i] == 1 for i in op.cond) if op.cond else True
            if fire and bit(a) and bit(b) and bit(c):
                phase ^= 1
        elif n == "global_flip":
            fire = all(record[i] == 1 for i in op.cond) if op.cond else True
            if fire:
                phase ^= 1
        elif n == "hmr":
            (q,) = op.qubits
            # record the pre-measurement bit into the phase channel:
            # state picks up (-1)^{r*f(y)} with r uniform (Methods Eq 14).
            f = bit(q)
            if forced is not None:
                if not forced:
                    raise ValueError("force_record shorter than hmr ops: refusing")
                r = forced.pop(0)
                if r not in (0, 1):
                    raise ValueError(f"forced record bit must be 0/1, got {r!r}: refusing")
            else:
                r = rng.getrandbits(1)
            record.append(r)
            if r and f:
                phase ^= 1
            bits[q] = 0  # ancilla released to |0>
        else:
            raise ValueError(f"unknown op {n!r}: refusing")
    return bits, phase, record


def transfer_bounds(eps: float) -> dict[str, float]:
    """Methods Eq transfer: trajectory failure prob -> coherent-action bounds.

    Paper: eps = P[(x,m) in B] under D x uniform records. Then
      ||Phi_V - Phi_U|| <= 2*sqrt(eps),
      state fidelity >= (1-2*eps)^2,
      TV distance of any subsequent measurement <= 2*sqrt(eps).
    The fidelity form is stated for small eps; for eps > 0.5 it climbs back
    toward 1, so this returns the vacuous floor 0 there instead of a
    misleading near-1 (fail-closed, never a flattering bound).
    """
    if isinstance(eps, bool) or not isinstance(eps, (int, float)):
        raise ValueError(f"eps must be a number in [0,1], got {eps!r}: refusing")
    e = float(eps)
    if not math.isfinite(e) or not (0.0 <= e <= 1.0):
        raise ValueError(f"eps must be finite in [0,1], got {eps!r}: refusing")
    dist = min(2.0, 2.0 * math.sqrt(e))
    tv = min(1.0, 2.0 * math.sqrt(e))
    fidelity_floor = (1.0 - 2.0 * e) ** 2 if e <= 0.5 else 0.0
    return {"state_dist": dist, "fidelity_floor": fidelity_floor, "tv_bound": tv}


def binom_tail(k: int, b: int, eps: float) -> float:
    """Exact P_{k,b}(eps) = sum_{j=0}^b C(k,j) eps^j (1-eps)^{k-j} (Methods).

    Log-domain evaluation (lgamma + log-sum-exp): no forward-recurrence
    underflow — (1-eps)^k vanishing in float does NOT zero the tail when
    the mass sits at higher j. Cost is O(b) lgammas; k caps at 1e6
    (fail-closed above: quadratic-cost honesty, not a math limit).
    Float-rounded to ~1e-15 relative; stated, not hidden.
    """
    if isinstance(k, bool) or not isinstance(k, int) or k <= 0:
        raise ValueError(f"k must be positive int, got {k!r}: refusing")
    if isinstance(b, bool) or not isinstance(b, int) or not (0 <= b <= k):
        raise ValueError(f"b must be int in [0,k], got {b!r}: refusing")
    if isinstance(eps, bool) or not isinstance(eps, (int, float)):
        raise ValueError(f"eps must be in [0,1], got {eps!r}: refusing")
    e = float(eps)
    if not math.isfinite(e) or not (0.0 <= e <= 1.0):
        raise ValueError(f"eps must be finite in [0,1], got {eps!r}: refusing")
    if k > 1_000_000:
        raise ValueError(f"k={k} exceeds exact-tail cap 1e6: refusing")
    if b >= k:
        return 1.0
    if e <= 0.0:
        return 1.0
    if e >= 1.0:
        return 0.0
    le = math.log(e)
    l1 = math.log1p(-e)  # accurate for tiny eps
    lk1 = math.lgamma(k + 1)
    logs: list[float] = []
    for j in range(b + 1):
        lj = lk1 - math.lgamma(j + 1) - math.lgamma(k - j + 1) + j * le + (k - j) * l1
        logs.append(lj)
    m = max(logs)
    return min(1.0, max(0.0, math.fsum(math.exp(lj - m) for lj in logs) * math.exp(m)))


def eps_upper_exact(k: int, b: int, delta: float = 0.025) -> float:
    """Invert P_{k,b}(eps) = delta: eps upper bound at confidence 1-delta.

    Binary search on the monotone tail (80 iterations, float-exact). b==k
    carries no information -> returns 1.0 (vacuous, never a tight-looking 0).
    Matches the closed form 1-delta^{1/k} for b == 0 exactly.
    """
    if isinstance(k, bool) or not isinstance(k, int) or k <= 0:
        raise ValueError(f"k must be positive int, got {k!r}: refusing")
    if isinstance(b, bool) or not isinstance(b, int) or not (0 <= b <= k):
        raise ValueError(f"b must be int in [0,k], got {b!r}: refusing")
    if isinstance(delta, bool) or not isinstance(delta, (int, float)):
        raise ValueError(f"delta must be in (0,1), got {delta!r}: refusing")
    d = float(delta)
    if not math.isfinite(d) or not (0.0 < d < 1.0):
        raise ValueError(f"delta must be in (0,1), got {delta!r}: refusing")
    if b >= k:
        return 1.0
    lo, hi = 0.0, 1.0
    for _ in range(80):
        mid = 0.5 * (lo + hi)
        if binom_tail(k, b, mid) > d:
            lo = mid
        else:
            hi = mid
    return hi


def _depth_layers(ops: list) -> list[int]:
    """Longest-path layer per gate op (markers excluded from depth).

    Edges: shared-qubit conflict (earlier -> later) + record feedforward
    (the hmr op producing record r -> any later op conditioned on r).
    Returns layer per op index (0 for markers). Fail-closed on non-kickmix.
    """
    if not isinstance(ops, list):
        raise ValueError(f"ops must be a list, got {type(ops).__name__}: refusing")
    for o in ops:
        if o.name in ("alloc_marker", "free_marker"):
            continue
        if o.name not in KICKMIX_ALL:
            raise ValueError(f"non-kickmix op {o.name!r}: refusing")
    rid_to_hmr: dict[int, int] = {}
    rid = 0
    for i, o in enumerate(ops):
        if o.name == "hmr":
            rid_to_hmr[rid] = i
            rid += 1
    layers = [0] * len(ops)
    for i, o in enumerate(ops):
        if o.name in ("alloc_marker", "free_marker"):
            continue
        best = 0
        qi = set(o.qubits)
        for j in range(i):
            p = ops[j]
            if p.name in ("alloc_marker", "free_marker"):
                continue
            dep = False
            if qi & set(p.qubits):
                dep = True
            if o.cond:
                for r in o.cond:
                    if rid_to_hmr.get(r) == j:
                        dep = True
            if dep and layers[j] > best:
                best = layers[j]
        layers[i] = best + 1
    return layers


def circuit_depth(ops: list) -> int:
    """Exact gate depth (longest conflict chain, markers free)."""
    layers = _depth_layers(ops)
    return max(layers) if any(layers) else 0


def toffoli_depth(ops: list) -> int:
    """Exact Toffoli depth: distinct layers containing a ccx/ccz."""
    layers = _depth_layers(ops)
    return len({l for o, l in zip(ops, layers) if o.name in TOFFOLI_GATES})


def active_qubits(ops: list) -> int:
    """Distinct qubits touched by any non-marker op (Litinski-style activity
    count at toy honesty: measured participation, never a volume claim)."""
    if not isinstance(ops, list):
        raise ValueError(f"ops must be a list, got {type(ops).__name__}: refusing")
    touched: set[int] = set()
    for o in ops:
        if o.name in ("alloc_marker", "free_marker"):
            continue
        if o.name not in KICKMIX_ALL:
            raise ValueError(f"non-kickmix op {o.name!r}: refusing")
        touched.update(o.qubits)
    return len(touched)


class KickmixEvaluator:
    """Independent verifier + scorer. Ground truth; never proposes."""

    #: Exhaustive record enumeration cap (inputs x records). Above this,
    #: sampling + Hoeffding bound is the honest route (documented, not silent).
    EXHAUSTIVE_RECORD_CAP = 1024

    def check_one(
        self,
        builder: Builder,
        spec: TaskSpec,
        init_bits: dict[int, int],
        rng: random.Random,
        force_record: list[int] | None = None,
    ) -> TrajectoryResult:
        ops = builder.emit()
        # reject non-kickmix at check time too (defense in depth: a
        # hand-built op list could bypass Builder._gate)
        for o in ops:
            if o.name in ("free_marker", "alloc_marker"):
                continue
            if o.name not in KICKMIX_ALL:
                return TrajectoryResult(False, f"non-kickmix op {o.name!r}: refusing")
            if o.cond and o.name not in KICKMIX_DIAGONAL:
                return TrajectoryResult(
                    False,
                    f"record-controlled basis gate {o.name!r}: refusing",
                )
        leaked = builder.leaked_allocs()
        if leaked:
            return TrajectoryResult(
                False, f"allocs {sorted(leaked)} never freed/measured: refusing")
        try:
            bits, phase, _record = _sim_trajectory(ops, init_bits, rng, force_record)
        except _DirtyFree as e:
            return TrajectoryResult(False, str(e))
        # inputs unchanged
        for q in spec.inputs:
            if bits.get(q, 0) != init_bits.get(q, 0):
                return TrajectoryResult(False, f"input qubit {q} modified: refusing")
        # ancillas clean
        for q in spec.ancillas:
            if bits.get(q, 0) != 0:
                return TrajectoryResult(False, f"ancilla {q} not returned to |0>: refusing")
        # permutation + phase
        if spec.target_perm is not None:
            want = spec.target_perm(dict(init_bits))
            for q in spec.outputs:
                if bits.get(q, 0) != want.get(q, 0):
                    return TrajectoryResult(False, f"output qubit {q} mismatch: refusing")
        if spec.target_phase is not None:
            want_p = spec.target_phase(dict(init_bits)) & 1
            if (phase & 1) != want_p:
                return TrajectoryResult(False, "phase mismatch: refusing")
        return TrajectoryResult(
            True, "ok",
            toffoli=builder.toffoli_count(),
            peak_qubits=builder.peak_qubits(),
            phase=phase & 1,
            final_bits=dict(bits),
        )

    def evaluate(
        self,
        builder: Builder,
        spec: TaskSpec,
        num_tests: int = 256,
        max_failures: int = 0,
        seed: int = 7,
        *,
        factory: str | None = None,
        code_distance: int | None = None,
        cycle_us: float | None = None,
    ) -> dict[str, Any]:
        """Sample trajectories; bound eps per Methods Eqs 20-21.

        For a candidate fixed before the draw, k passes -> eps bound.
        Returns fail-closed verdict + resource counts + bound.

        Factory diagnostics (factories.py, literature-derived, diagnostics
        only — never ranking): pass factory + code_distance + cycle_us to
        attach a factory_report; omit all three for "factory": None
        (explicitly not computed). Partial specification refuses.
        """
        if num_tests <= 0:
            raise ValueError("num_tests must be positive")
        if not (0 <= max_failures <= num_tests):
            raise ValueError("max_failures must be in [0, num_tests]")
        rng = random.Random(seed)
        failures = 0
        first_reason = "ok"
        tests_run = 0
        # exhaustive when the input domain is small enough to enumerate
        n_inputs = len(spec.inputs)
        exhaustive: list[dict[int, int]] | None = None
        if n_inputs <= 10:
            exhaustive = []
            for mask in range(1 << n_inputs):
                b: dict[int, int] = {}
                for i, q in enumerate(spec.inputs):
                    b[q] = (mask >> (n_inputs - 1 - i)) & 1
                # ancillas + other qubits start |0>
                for q in range(spec.num_qubits):
                    b.setdefault(q, 0)
                exhaustive.append(b)
        # Exhaustive record enumeration: with m measurements the trajectory
        # space is inputs x 2^m. Sampling one record per input can MISS the
        # failing record (false valid). Enumerate all when small, the budget
        # covers it, and exactness is demanded (max_failures==0).
        m = builder.num_measurements()
        enumerate_records = (
            exhaustive is not None
            and max_failures == 0
            and (len(exhaustive) * (1 << m)) <= self.EXHAUSTIVE_RECORD_CAP
            and num_tests >= len(exhaustive) * (1 << m)
        )
        if enumerate_records:
            assert exhaustive is not None
            for init in exhaustive:
                for rmask in range(1 << m):
                    force = [(rmask >> (m - 1 - i)) & 1 for i in range(m)] if m else []
                    res = self.check_one(builder, spec, dict(init),
                                         rng, force_record=force)
                    tests_run += 1
                    if not res.valid:
                        failures += 1
                        if first_reason == "ok":
                            first_reason = res.reason
                        break
                if failures > max_failures:
                    break
        else:
            for t in range(num_tests):
                if exhaustive is not None:
                    init = dict(exhaustive[t % len(exhaustive)])
                elif spec.input_distribution is not None:
                    init = dict(spec.input_distribution(rng))
                    for q in range(spec.num_qubits):
                        init.setdefault(q, 0)
                else:
                    # Too many inputs to enumerate and no distribution: sample
                    # inputs uniformly, zero everything else (fresh-target
                    # contract — the same setdefault-0 the exhaustive path
                    # uses). Fully-random inits made accumulate-style tasks
                    # verdict-flaky by input draw (a 12-input scan failed on
                    # seed 7's first draw while every trajectory is correct);
                    # the paths must agree, so sampling matches exhaustive.
                    in_set = set(spec.inputs)
                    init = {q: (rng.getrandbits(1) if q in in_set else 0)
                            for q in range(spec.num_qubits)}
                # fresh per-trajectory measurement sampling (uniform records)
                trng = random.Random(rng.getrandbits(64))
                res = self.check_one(builder, spec, init, trng)
                tests_run += 1
                if not res.valid:
                    failures += 1
                    if first_reason == "ok":
                        first_reason = res.reason
                    if failures > max_failures:
                        break
        valid = failures <= max_failures
        k = tests_run
        b = failures
        # Clopper-Pearson-style upper bound via binomial tail inversion is
        # expensive; the paper's closed form for b=0 plus a Hoeffding-style
        # tail for b>0 keeps the bound honest without pretending exactness.
        # NOTE: exact inversion lives in eps_upper_exact()/binom_tail() (new,
        # tested separately); this path keeps its long-pinned conservative
        # Hoeffding shape so existing receipts do not silently tighten.
        if b == 0:
            # eps <= 1 - delta^{1/k} at confidence 1-delta; report delta=0.025
            # (97.5% as in the paper's DQI gate).
            eps_bound = 1.0 - (0.025 ** (1.0 / k))
        else:
            phat = b / k
            # Hoeffding upper tail at 97.5%: eps <= phat + sqrt(ln(40)/2k)
            eps_bound = min(1.0, phat + math.sqrt(math.log(40.0) / (2.0 * k)))
        transfer = transfer_bounds(eps_bound)
        ops = builder.emit()
        if factory is None and (code_distance is not None or cycle_us is not None):
            raise ValueError(
                "factory diagnostics need factory + code_distance + cycle_us "
                "together: refusing partial specification")
        if factory is not None and (code_distance is None or cycle_us is None):
            raise ValueError(
                "factory diagnostics need factory + code_distance + cycle_us "
                "together: refusing partial specification (no silent physics)")
        toffoli_depth_v = toffoli_depth(ops)
        factory_rep = None
        if factory is not None:
            factory_rep = factory_report(
                toffoli=builder.toffoli_count(),
                qubits=builder.peak_qubits(),
                toffoli_depth=toffoli_depth_v,
                factory=factory,
                code_distance=code_distance,
                cycle_us=cycle_us,
            )
        return {
            "valid": valid,
            "failures": failures,
            "allowed": max_failures,
            "reason": first_reason if valid else first_reason,
            "toffoli": builder.toffoli_count(),
            "peak_qubits": builder.peak_qubits(),
            "eps_bound_97_5": eps_bound,
            "tests": k,
            "transfer": transfer,
            "depth": circuit_depth(ops),
            "toffoli_depth": toffoli_depth_v,
            "active_qubits": active_qubits(ops),
            "factory": factory_rep,
        }
