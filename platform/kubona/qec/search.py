"""Phase 3 QEC discovery search engine: Islands, MAP-Elites, Verifier-in-Loop & Scratchpads.

Implements the two proven 2026 loop shapes:
- Agentic FunSearch (S4): verifier inside the proposer loop (propose -> verify ->
  error report -> refine), 4-6 islands with migration, and per-island scratchpads.
- OmniQEC slow/fast ladder (S2): fast GF(2) rank + decomposability -> BP-OSD ->
  top-x% HiGHS MILP exact distance -> top-y% Stim circuit-level LER feedback.

Also provides DiscoveryController (fixes B8) to pick the next search region from
the current frontier and failure history within budget.
"""

from __future__ import annotations

import math
import random
from dataclasses import dataclass, field
from typing import Any, Callable, Sequence, Tuple

from kubona.qec.codes import build_bb_code
from kubona.qec.distance import estimate_distance_bposd, prove_distance_milp
from kubona.qec.gf2 import css_parameters, is_decomposable_css


@dataclass
class QecCandidate:
    ell: int
    m: int
    a_terms: Tuple[Tuple[int, int], ...]
    b_terms: Tuple[Tuple[int, int], ...]
    n: int = 0
    k: int = 0
    d: int | None = None
    d_kind: str = "upper_bound"
    fom: float = 0.0
    decomposable: bool = False
    valid: bool = False
    failure_reason: str = ""
    island_id: int = 0
    generation: int = 0

    def key(self) -> Tuple[int, int, Tuple[Tuple[int, int], ...], Tuple[Tuple[int, int], ...]]:
        return (self.ell, self.m, tuple(sorted(self.a_terms)), tuple(sorted(self.b_terms)))


class MapElitesQecArchive:
    """2D MAP-Elites diversity archive indexed by (qubit_bucket, rate_bucket)."""

    def __init__(self, qubit_step: int = 36, rate_step: float = 0.05):
        self.qubit_step = max(1, int(qubit_step))
        self.rate_step = max(0.01, float(rate_step))
        self.cells: dict[Tuple[int, int], QecCandidate] = {}

    def niche_of(self, cand: QecCandidate) -> Tuple[int, int]:
        rate = (cand.k / cand.n) if cand.n > 0 else 0.0
        return (cand.n // self.qubit_step, int(math.floor(rate / self.rate_step)))

    def insert(self, cand: QecCandidate) -> bool:
        if not cand.valid or cand.decomposable or cand.k <= 0:
            return False
        niche = self.niche_of(cand)
        inc = self.cells.get(niche)
        if inc is None or cand.fom > inc.fom:
            self.cells[niche] = cand
            return True
        return False

    def top_k(self, k: int = 5) -> list[QecCandidate]:
        return sorted(self.cells.values(), key=lambda c: (c.fom, c.k), reverse=True)[:k]

    def pareto_frontier(self) -> list[QecCandidate]:
        """Non-dominated frontier maximizing (rate = k/n, fom)."""
        items = list(self.cells.values())
        front: list[QecCandidate] = []
        for c in items:
            rc = c.k / max(1, c.n)
            dominated = any(
                (o.k / max(1, o.n) >= rc and o.fom >= c.fom)
                and (o.k / max(1, o.n) > rc or o.fom > c.fom)
                for o in items
                if o is not c
            )
            if not dominated:
                front.append(c)
        return sorted(front, key=lambda c: (c.fom, c.k), reverse=True)


@dataclass
class Island:
    island_id: int
    population: list[QecCandidate] = field(default_factory=list)
    scratchpad: list[str] = field(default_factory=list)
    recent_failures: list[str] = field(default_factory=list)

    def record_note(self, note: str, max_notes: int = 12) -> None:
        self.scratchpad.append(note)
        if len(self.scratchpad) > max_notes:
            self.scratchpad = self.scratchpad[-max_notes:]

    def record_failure(self, reason: str, max_failures: int = 8) -> None:
        self.recent_failures.append(reason)
        if len(self.recent_failures) > max_failures:
            self.recent_failures = self.recent_failures[-max_failures:]


def evaluate_bb_candidate_fast(
    ell: int,
    m: int,
    a_terms: Sequence[Tuple[int, int]],
    b_terms: Sequence[Tuple[int, int]],
    *,
    bposd_trials: int = 12,
    prove_if_fom_ge: float | None = None,
    milp_time_limit_s: float = 10.0,
    seed: int = 7,
) -> QecCandidate:
    """Multi-fidelity cascade: GF(2) rank -> decomposability -> BP-OSD -> optional MILP."""
    a_t = tuple(sorted({(int(a) % ell, int(b) % m) for a, b in a_terms}))
    b_t = tuple(sorted({(int(a) % ell, int(b) % m) for a, b in b_terms}))
    cand = QecCandidate(ell=ell, m=m, a_terms=a_t, b_terms=b_t)
    if len(a_t) < 2 or len(b_t) < 2:
        cand.failure_reason = "degenerate polynomial (<2 distinct terms)"
        return cand
    try:
        hx, hz = build_bb_code(ell, m, a_t, b_t)
        params = css_parameters(hx, hz)
    except Exception as exc:
        cand.failure_reason = f"invalid CSS construction: {exc}"
        return cand

    cand.n = params["n"]
    cand.k = params["k"]
    if cand.k <= 0:
        cand.failure_reason = f"k=0 (no logical qubits for n={cand.n})"
        return cand

    decomp = is_decomposable_css(hx, hz)
    cand.decomposable = bool(decomp["decomposable"])
    if cand.decomposable:
        cand.failure_reason = f"decomposable into {decomp['component_qubit_sizes']}"
        return cand

    dist = estimate_distance_bposd(hx, hz, num_trials=bposd_trials, seed=seed)
    d_val = dist.get("value")
    if d_val is None or d_val <= 1:
        cand.failure_reason = f"trivial distance bound d={d_val}"
        return cand

    cand.d = int(d_val)
    cand.d_kind = "upper_bound"
    cand.fom = round((cand.k * (cand.d ** 2)) / cand.n, 6)
    cand.valid = True

    if prove_if_fom_ge is not None and cand.fom >= prove_if_fom_ge:
        exact = prove_distance_milp(hx, hz, time_limit_s=milp_time_limit_s)
        if exact.get("value") is not None:
            cand.d = int(exact["value"])
            cand.d_kind = str(exact.get("kind") or "upper_bound")
            cand.fom = round((cand.k * (cand.d ** 2)) / cand.n, 6)
    return cand


def mutate_bb_terms(
    ell: int,
    m: int,
    a_terms: Tuple[Tuple[int, int], ...],
    b_terms: Tuple[Tuple[int, int], ...],
    rng: random.Random,
    *,
    term_count: int = 3,
) -> Tuple[Tuple[Tuple[int, int], ...], Tuple[Tuple[int, int], ...]]:
    """Mutate one polynomial exponent in A or B while keeping distinct terms."""
    a_list = list(a_terms)
    b_list = list(b_terms)
    target = a_list if rng.random() < 0.5 else b_list
    if target:
        idx = rng.randrange(len(target))
        if rng.random() < 0.7:
            # Small step mutation on one coordinate
            da = rng.choice((-1, 0, 1))
            db = rng.choice((-1, 0, 1))
            target[idx] = ((target[idx][0] + da) % ell, (target[idx][1] + db) % m)
        else:
            target[idx] = (rng.randrange(ell), rng.randrange(m))
    while len(set(a_list)) < term_count:
        a_list.append((rng.randrange(ell), rng.randrange(m)))
    while len(set(b_list)) < term_count:
        b_list.append((rng.randrange(ell), rng.randrange(m)))
    return tuple(sorted(set(a_list))[:term_count]), tuple(sorted(set(b_list))[:term_count])


def verifier_in_loop_refine(
    proposer_fn: Callable[[list[str]], Tuple[Tuple[Tuple[int, int], ...], Tuple[Tuple[int, int], ...]]],
    ell: int,
    m: int,
    *,
    max_retries: int = 3,
    bposd_trials: int = 12,
    seed: int = 7,
) -> Tuple[QecCandidate, list[str]]:
    """Agentic FunSearch verifier-in-the-loop pattern (S4).

    Calls ``proposer_fn(feedback_history)``, runs the deterministic verifier,
    and feeds structured failure reasons back into the proposer up to ``max_retries``.
    """
    feedback: list[str] = []
    last_cand: QecCandidate | None = None
    for attempt in range(max_retries + 1):
        a_terms, b_terms = proposer_fn(list(feedback))
        cand = evaluate_bb_candidate_fast(ell, m, a_terms, b_terms, bposd_trials=bposd_trials, seed=seed + attempt)
        last_cand = cand
        if cand.valid:
            return cand, feedback
        feedback.append(f"attempt {attempt + 1} rejected: {cand.failure_reason}")
    assert last_cand is not None
    return last_cand, feedback


class IslandSearchPool:
    """Multi-island evolutionary QEC search with MAP-Elites, migration, and scratchpads."""

    def __init__(
        self,
        ell: int,
        m: int,
        *,
        num_islands: int = 4,
        migration_interval: int = 6,
        use_scratchpads: bool = True,
        seed: int = 7,
    ):
        self.ell = int(ell)
        self.m = int(m)
        self.rng = random.Random(seed)
        self.migration_interval = max(1, int(migration_interval))
        self.use_scratchpads = bool(use_scratchpads)
        self.islands = [Island(island_id=i) for i in range(max(1, num_islands))]
        self.archive = MapElitesQecArchive()
        self.seen: set[Tuple[Any, ...]] = set()
        self.evaluations = 0

    def _random_pair(self) -> Tuple[Tuple[Tuple[int, int], ...], Tuple[Tuple[int, int], ...]]:
        # Bias toward structured axis-aligned terms (matching BB literature) plus random terms
        a_set: set[Tuple[int, int]] = set()
        b_set: set[Tuple[int, int]] = set()
        while len(a_set) < 3:
            a_set.add((self.rng.randrange(self.ell), self.rng.randrange(self.m)))
        while len(b_set) < 3:
            b_set.add((self.rng.randrange(self.ell), self.rng.randrange(self.m)))
        return tuple(sorted(a_set)), tuple(sorted(b_set))

    def seed_island(self, island_id: int, a_terms: Sequence[Tuple[int, int]], b_terms: Sequence[Tuple[int, int]]) -> QecCandidate:
        cand = evaluate_bb_candidate_fast(self.ell, self.m, a_terms, b_terms, bposd_trials=12, seed=self.evaluations + 7)
        cand.island_id = island_id
        self.evaluations += 1
        self.seen.add(cand.key())
        if cand.valid:
            self.islands[island_id].population.append(cand)
            self.archive.insert(cand)
            if self.use_scratchpads:
                self.islands[island_id].record_note(f"seed k={cand.k} d<={cand.d} fom={cand.fom}")
        else:
            self.islands[island_id].record_failure(cand.failure_reason)
        return cand

    def step(self, generation: int, *, bposd_trials: int = 12) -> list[QecCandidate]:
        round_results: list[QecCandidate] = []
        for isl in self.islands:
            if isl.population:
                parent = max(isl.population, key=lambda c: c.fom)
                a_t, b_t = mutate_bb_terms(self.ell, self.m, parent.a_terms, parent.b_terms, self.rng)
            else:
                a_t, b_t = self._random_pair()

            cand = evaluate_bb_candidate_fast(
                self.ell,
                self.m,
                a_t,
                b_t,
                bposd_trials=bposd_trials,
                seed=self.evaluations + 17,
            )
            cand.island_id = isl.island_id
            cand.generation = generation
            self.evaluations += 1
            self.seen.add(cand.key())
            round_results.append(cand)
            if cand.valid:
                isl.population.append(cand)
                # Keep top 8 per island
                isl.population.sort(key=lambda c: c.fom, reverse=True)
                isl.population = isl.population[:8]
                improved = self.archive.insert(cand)
                if self.use_scratchpads and improved:
                    isl.record_note(f"gen {generation}: new niche best k={cand.k} d<={cand.d} fom={cand.fom}")
            else:
                isl.record_failure(cand.failure_reason)

        if generation > 0 and generation % self.migration_interval == 0 and len(self.islands) > 1:
            self._migrate()
        return round_results

    def _migrate(self) -> None:
        """Ring-migrate the best candidate from island i -> island (i+1) % N."""
        bests = [isl.population[0] if isl.population else None for isl in self.islands]
        n = len(self.islands)
        for i, migrant in enumerate(bests):
            if migrant is None:
                continue
            dst = self.islands[(i + 1) % n]
            if all(c.key() != migrant.key() for c in dst.population):
                dst.population.append(migrant)
                dst.population.sort(key=lambda c: c.fom, reverse=True)
                dst.population = dst.population[:8]
                if self.use_scratchpads:
                    dst.record_note(f"migrated fom={migrant.fom} from island {i}")

    def best(self) -> QecCandidate | None:
        top = self.archive.top_k(1)
        return top[0] if top else None


class DiscoveryController:
    """Closed-loop controller (fixes B8): selects next search action from frontier & failures."""

    def next_action(self, pool: IslandSearchPool, *, target_fom: float = 6.0, max_evals: int = 200) -> dict[str, Any]:
        best = pool.best()
        if pool.evaluations >= max_evals:
            return {"action": "stop", "reason": "evaluation_budget_reached", "best_fom": best.fom if best else 0.0}
        if best is not None and best.fom >= target_fom and best.d_kind != "proved":
            return {
                "action": "promote_milp",
                "reason": f"candidate fom={best.fom} >= target {target_fom}; promote to exact MILP distance",
                "candidate": best,
            }
        if best is not None and best.fom >= target_fom and best.d_kind == "proved":
            return {
                "action": "promote_circuit_sim",
                "reason": f"proved candidate [[{best.n},{best.k},{best.d}]] ready for held-out circuit-level LER",
                "candidate": best,
            }
        # Pick the island with lowest best FOM or most failures for focused exploration
        weakest = min(pool.islands, key=lambda isl: isl.population[0].fom if isl.population else -1.0)
        return {
            "action": "explore_island",
            "island_id": weakest.island_id,
            "recent_failures": list(weakest.recent_failures[-3:]),
            "scratchpad": list(weakest.scratchpad[-3:]),
        }
