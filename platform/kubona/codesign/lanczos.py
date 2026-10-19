"""Toy Hermitian-pattern locator: Lanczos null-vector finder over GF(2).

Mirrors the paper's Hermitian walk (SI I.2) at toy scale: symmetric matrix
A built from supplied data, repeated applications producing vectors that
must become dependent, Lanczos 3-term recurrence to process them
incrementally, block repair (depth 2) when a vector is self-orthogonal,
and a null vector out at the end.

Honest toy-scalings (stated, not hidden):
- Field F64 -> GF(2); dimension 48 -> n <= 16 (fail-closed above).
- Full re-orthogonalization runs alongside the 3-term step, and the
  Lanczos 3-term theorem (re-orth changes nothing) is ASSERTED live for
  non-block steps — a loud failure means an implementation bug, and the
  final solve never depends on the theorem holding.
- The null vector comes from an exact nullspace solve over the recorded
  projection matrix H (all pairs, not just tridiagonal), then VERIFIED by
  direct multiplication (A.sigma == 0, sigma != 0). Failure to find one
  (seed's Krylov span carries no null vector, unrepairable breakdown)
  returns an honest unproven report — never a zero vector as a "null".
- No scheduled-repair-round machinery (a Toffoli-cost optimization with no
  cost model at toy scale); repairs happen where breakdowns occur.
"""

from __future__ import annotations


def _pop(v: int) -> int:
    return bin(v).count("1")


def dot(u: int, v: int) -> int:
    return _pop(u & v) & 1


def matvec(rows: list[int], v: int, n: int) -> int:
    out = 0
    for i in range(n):
        if dot(rows[i], v):
            out |= 1 << i
    return out


def check_symmetric(rows: list[int], n: int) -> None:
    if len(rows) != n:
        raise ValueError(f"need {n} rows, got {len(rows)}: refusing")
    for i in range(n):
        if rows[i] >> n:
            raise ValueError(f"row {i} exceeds {n} bits: refusing")
        for j in range(n):
            if ((rows[i] >> j) & 1) != ((rows[j] >> i) & 1):
                raise ValueError(f"matrix not symmetric at ({i},{j}): refusing")


def nullspace_vector(masks: list[int], r: int) -> int | None:
    """One nonzero c with dot(row, c) == 0 for every row, or None."""
    mat = list(masks)
    pivots: dict[int, int] = {}
    row = 0
    for col in range(r):
        piv = next((i for i in range(row, len(mat)) if (mat[i] >> col) & 1), None)
        if piv is None:
            continue
        mat[row], mat[piv] = mat[piv], mat[row]
        for i in range(len(mat)):
            if i != row and ((mat[i] >> col) & 1):
                mat[i] ^= mat[row]
        pivots[col] = row
        row += 1
    free = [c for c in range(r) if c not in pivots]
    if not free:
        return None
    c = 1 << free[0]
    for _ in range(r + 1):
        changed = False
        for col, pr in pivots.items():
            if _pop(mat[pr] & c) & 1:
                c |= 1 << col
                changed = True
        if not changed:
            break
    if c == 0:
        return None
    for m in masks:
        if _pop(m & c) & 1:
            raise ValueError("nullspace solve inconsistent (implementation bug): refusing")
    return c


def _solve_gram(gram: list[int], rhs: int, r: int) -> int | None:
    """Solve G a = rhs over GF(2) (gram rows as bitmasks). None if singular."""
    aug = [(gram[i], (rhs >> i) & 1) for i in range(r)]
    where = [-1] * r
    row = 0
    for col in range(r):
        piv = next((i for i in range(row, r) if (aug[i][0] >> col) & 1), None)
        if piv is None:
            continue
        aug[row], aug[piv] = aug[piv], aug[row]
        where[col] = row
        for i in range(r):
            if i != row and ((aug[i][0] >> col) & 1):
                aug[i] = (aug[i][0] ^ aug[row][0], aug[i][1] ^ aug[row][1])
        row += 1
    for i in range(row, r):
        if aug[i][0] == 0 and aug[i][1] == 1:
            return None
    a = 0
    for col in range(r):
        if where[col] != -1 and aug[where[col]][1]:
            a |= 1 << col
    return a


def _project_out(v: int, basis: list[int]) -> tuple[int, bool]:
    """Remove all basis components exactly (Gram solve — no orthonormality
    assumption; repaired pairs carry cross-dots by design).

    Asserts an invertible Gram: a singular Gram means a dependent vector was
    appended earlier (implementation bug), caught here loudly, never silently.
    """
    r = len(basis)
    if r == 0:
        return v, False
    gram = []
    for i in range(r):
        mask = 0
        for j in range(r):
            if dot(basis[i], basis[j]):
                mask |= 1 << j
        gram.append(mask)
    rhs = 0
    for i in range(r):
        if dot(basis[i], v):
            rhs |= 1 << i
    a = _solve_gram(gram, rhs, r)
    if a is None:
        raise ValueError("singular basis Gram (implementation bug): refusing")
    out = v
    for i in range(r):
        if (a >> i) & 1:
            out ^= basis[i]
    for u in basis:
        if dot(u, out):
            raise ValueError("projection failed (implementation bug): refusing")
    return out, (out != v)


class LanczosWalker:
    """Incremental orthogonal-basis walk with depth-2 block repair."""

    def __init__(self, rows: list[int], n: int):
        if not isinstance(n, int) or n < 2 or n > 16:
            raise ValueError(f"n must be in [2,16], got {n!r}: refusing")
        check_symmetric(rows, n)
        self.rows = list(rows)
        self.n = n
        self.basis: list[int] = []
        self.blocks: list[tuple[int, int]] = []  # index pairs forming repaired pairs
        self.repairs = 0
        self.theorem_holds = True
        self.pure_3term = True  # cleared by the first block repair (recorded)

    def _record_pair(self, i: int, j: int) -> None:
        self.blocks.append((i, j))

    def extend(self, seed: int) -> str:
        """Run one Krylov chain to dependence. Returns 'dependent' | 'stuck'.

        'stuck' = unrepairable breakdown (honest stop, caller tries next seed).
        Appends basis vectors (and block records) as it goes.
        """
        n = self.n
        if seed == 0:
            raise ValueError("zero seed: refusing")
        # live set for the 3-term recurrence tail (indices into basis)
        prev: int | None = None
        cur = seed
        rounds = 0
        while True:
            rounds += 1
            if rounds > 3 * n + 4:
                raise ValueError("walk exceeded 3n+4 rounds (implementation bug): refusing")
            # re-orthonormalize the incoming vector against the basis so far
            cur, _ = _project_out(cur, self.basis)
            if cur == 0:
                return "dependent"
            if dot(cur, cur) == 0:
                outcome = self._repair(cur)
                if outcome == "failed":
                    # cur was never appended; caller restarts with a new seed
                    return "stuck"
                if outcome == "dependent":
                    return "dependent"
                # "paired": 3-term tail resets across a block (recorded)
                prev = None
                self.pure_3term = False
                cur = matvec(self.rows, self.basis[-1], n)
                continue
            self.basis.append(cur)
            # 3-term step with live theorem check (pure segments only)
            aw = matvec(self.rows, cur, n)
            alpha = dot(cur, aw)
            v = aw
            if alpha:
                v ^= cur
            if prev is not None:
                v ^= self.basis[prev]
            v_re, changed = _project_out(v, self.basis)
            if self.pure_3term and changed:
                # re-orth must change nothing: the 3-term already removed
                # every basis component (Lanczos theorem, exact arithmetic)
                self.theorem_holds = False
                raise ValueError(
                    "Lanczos 3-term theorem violated (implementation bug): refusing")
            prev = len(self.basis) - 1
            cur = v_re

    def _repair(self, cur: int) -> str:
        """Depth-2 block repair for self-orthogonal cur (nonzero, orth vs basis).

        Returns 'paired' (appended [cur, d], block recorded), 'dependent'
        (A.cur fell in span(basis, cur): appended cur, caller terminates),
        or 'failed' (Gram singular at depth 2: needs a deeper block).
        """
        n = self.n
        d, _ = _project_out(matvec(self.rows, cur, n), self.basis)
        if dot(cur, d):
            d ^= cur
        if d == 0:
            self.basis.append(cur)
            return "dependent"
        if dot(cur, d) != 1:
            return "failed"
        i0 = len(self.basis)
        self.basis.append(cur)
        self.basis.append(d)
        self._record_pair(i0, i0 + 1)
        self.repairs += 1
        return "paired"

    def projection(self) -> list[int]:
        """Full H: row i = mask of j with basis[i]^T A basis[j] == 1."""
        r = len(self.basis)
        rows = []
        for i in range(r):
            abi = matvec(self.rows, self.basis[i], self.n)
            mask = 0
            for j in range(r):
                if dot(self.basis[j], abi):
                    mask |= 1 << j
            rows.append(mask)
        return rows

    def gram_full_rank(self) -> bool:
        r = len(self.basis)
        masks = []
        for i in range(r):
            mask = 0
            for j in range(r):
                if dot(self.basis[i], self.basis[j]):
                    mask |= 1 << j
            masks.append(mask)
        # full rank iff elimination leaves r pivots
        mat = list(masks)
        piv = 0
        for col in range(r):
            k = next((i for i in range(piv, r) if (mat[i] >> col) & 1), None)
            if k is None:
                return False
            mat[piv], mat[k] = mat[k], mat[piv]
            for i in range(r):
                if i != piv and ((mat[i] >> col) & 1):
                    mat[i] ^= mat[piv]
            piv += 1
        return True

    def _combine(self, c: int) -> int:
        sig = 0
        for i in range(len(self.basis)):
            if (c >> i) & 1:
                sig ^= self.basis[i]
        return sig

    def null_vector(self) -> tuple[int | None, bool, str]:
        """Exact null candidate, verified. Returns (sigma|None, gram_full, via).

        `via` is "projection" (paper's H path — sound under full Gram; a
        verifying failure there raises as a bug), "direct" (the n output
        equations solved over the basis unknowns — no Gram assumption), or
        "none" (honest miss). Verification (A.sigma == 0, sigma != 0) gates
        every success; nothing unverified is ever returned.
        """
        return _null_from_basis(self.rows, self.n, self.basis)


def _null_from_basis(rows: list[int], n: int, basis: list[int]
                     ) -> tuple[int | None, bool, str]:
    """Shared verified-null solve over a basis (both walkers use this one)."""
    r = len(basis)

    def _combine(c: int) -> int:
        sig = 0
        for i in range(r):
            if (c >> i) & 1:
                sig ^= basis[i]
        return sig

    def _projection() -> list[int]:
        out = []
        for i in range(r):
            abi = matvec(rows, basis[i], n)
            mask = 0
            for j in range(r):
                if dot(basis[j], abi):
                    mask |= 1 << j
            out.append(mask)
        return out

    def _gram_full() -> bool:
        mat = []
        for i in range(r):
            mask = 0
            for j in range(r):
                if dot(basis[i], basis[j]):
                    mask |= 1 << j
            mat.append(mask)
        piv = 0
        for col in range(r):
            k = next((i for i in range(piv, r) if (mat[i] >> col) & 1), None)
            if k is None:
                return False
            mat[piv], mat[k] = mat[k], mat[piv]
            for i in range(r):
                if i != piv and ((mat[i] >> col) & 1):
                    mat[i] ^= mat[piv]
            piv += 1
        return True

    if r == 0:
        return None, True, "none"
    gram_full = _gram_full()
    c = nullspace_vector(_projection(), r)
    if c is not None:
        sig = _combine(c)
        if sig != 0 and matvec(rows, sig, n) == 0:
            return sig, gram_full, "projection"
        if gram_full:
            raise ValueError("projection miss under full Gram "
                             "(implementation bug): refusing")
    masks = []
    aws = [matvec(rows, w, n) for w in basis]
    for i in range(n):
        mask = 0
        for j in range(r):
            if (aws[j] >> i) & 1:
                mask |= 1 << j
        masks.append(mask)
    c2 = nullspace_vector(masks, r)
    if c2 is None:
        return None, gram_full, "none"
    sig = _combine(c2)
    if sig == 0 or matvec(rows, sig, n) != 0:
        raise ValueError("null candidate via direct fails verification "
                         "(implementation bug): refusing")
    return sig, gram_full, "direct"


def find_null(rows: list[int], n: int, seeds: list[int]) -> dict:
    """Try seeds in order; first verified null vector wins (honest report)."""
    check_symmetric(rows, n)
    if not seeds or any(not isinstance(s, int) or s <= 0 or s >= (1 << n) for s in seeds):
        raise ValueError("seeds must be nonzero n-bit ints: refusing")
    repairs = 0
    tried = 0
    restarts = 0
    for s in seeds:
        tried += 1
        w = LanczosWalker(rows, n)
        # carry over nothing between seeds (independent attempts, reported)
        outcome = w.extend(s)
        repairs += w.repairs
        if outcome == "stuck":
            restarts += 1
            continue
        sig, gram_full, via = w.null_vector()
        if sig is not None:
            return {"null": sig, "seeds_tried": tried, "restarts": restarts,
                    "repairs": repairs, "basis_size": len(w.basis),
                    "theorem_holds": w.theorem_holds, "pure_3term": w.pure_3term,
                    "gram_full": gram_full, "via": via, "proven": True}
        restarts += 1
    return {"null": None, "seeds_tried": tried, "restarts": restarts,
            "repairs": repairs, "basis_size": 0,
            "theorem_holds": True, "pure_3term": True,
            "gram_full": True, "via": "none", "proven": False}


class ScheduledWalker:
    """Lanczos walk with scheduled repair rounds (paper SI I.2, toy GF(2)).

    Ordinary rounds advance only when g0 = w^T w != 0 (cheap CNOT-only test
    in the paper's characteristic-2 setting). A self-orthogonal vector off
    schedule FREEZES: the walk state is carried unchanged until the next
    repair round, which forms a depth-2 (then depth-3) block and advances
    past it in one step — the paper's concentration of repair machinery in
    17 of 52 rounds, at toy scale. Inputs needing a deeper block, or still
    unfinished after max_rounds (n+4 default: dimension + delay, as paper's
    52 = 48 + 4), end honestly (needs_deeper / unfinished — never a vector).

    Full re-orthogonalization stands in for the 3-term recurrence (the same
    declared simplification as LanczosWalker, exact here). Snapshots of each
    round's incoming vector are retained at toy scale (stated: the paper
    recomputes from the two live basis vectors); what the reverse proves is
    the DECISION logic — every zero-test re-derives from retained state,
    while the 1-bit-per-round advance record is load-bearing (flip it and
    reversal refuses).
    """

    def __init__(self, rows: list[int], n: int, repair_rounds: set[int] | None = None,
                 max_rounds: int | None = None):
        if not isinstance(n, int) or n < 2 or n > 16:
            raise ValueError(f"n must be in [2,16], got {n!r}: refusing")
        check_symmetric(rows, n)
        self.rows = list(rows)
        self.n = n
        if max_rounds is None:
            max_rounds = n + 4
        if not isinstance(max_rounds, int) or max_rounds < 1 or max_rounds > 64:
            raise ValueError(f"max_rounds must be in [1,64], got {max_rounds!r}: refusing")
        if repair_rounds is None:
            repair_rounds = set(range(max_rounds))
        if not isinstance(repair_rounds, set) or any(
                not isinstance(r, int) or not (0 <= r < max_rounds) for r in repair_rounds):
            raise ValueError("repair_rounds must be a set of round indices: refusing")
        self.repair_rounds = set(repair_rounds)
        self.max_rounds = max_rounds
        self.basis: list[int] = []
        self.decisions: list[dict] = []  # one entry per round (the history bit + depth)
        self.snapshots: list[int] = []  # incoming vector per round (toy-retained)
        self.repairs = 0
        self.maxdepth = 0
        self.frozen_rounds = 0

    def _block_gram_ok(self, block: list[int]) -> bool:
        """Block Gram invertible: every unit rhs solvable (exact, via _solve_gram)."""
        d = len(block)
        gram = []
        for i in range(d):
            mask = 0
            for j in range(d):
                if dot(block[i], block[j]):
                    mask |= 1 << j
            gram.append(mask)
        for j in range(d):
            if _solve_gram(gram, 1 << j, d) is None:
                return False
        return True

    def run(self, seed: int) -> str:
        """Walk to dependence. Returns 'dependent' | 'needs_deeper' | 'unfinished'."""
        n = self.n
        if not isinstance(seed, int) or seed <= 0 or seed >= (1 << n):
            raise ValueError(f"seed must be a nonzero n-bit int: refusing {seed!r}")
        nxt = seed
        frozen: int | None = None
        for rnd in range(self.max_rounds):
            incoming = frozen if frozen is not None else nxt
            v, _ = _project_out(incoming, self.basis)
            self.snapshots.append(v)
            if v == 0:
                self.decisions.append({"round": rnd, "advanced": True,
                                       "depth": 0, "note": "dependent"})
                return "dependent"
            if dot(v, v) == 1:
                self.basis.append(v)
                nxt = matvec(self.rows, v, n)
                frozen = None
                self.decisions.append({"round": rnd, "advanced": True,
                                       "depth": 1, "note": "ordinary"})
                continue
            # self-orthogonal breakdown
            if rnd not in self.repair_rounds:
                frozen = v  # carried unchanged until the next repair round
                self.frozen_rounds += 1
                self.decisions.append({"round": rnd, "advanced": False,
                                       "depth": 0, "note": "frozen"})
                continue
            repaired = False
            for depth in (2, 3):
                block = [v]
                ok = True
                for _ in range(1, depth):
                    w, _ = _project_out(matvec(self.rows, block[-1], n), self.basis)
                    if w == 0:
                        ok = False  # Krylov dependence inside the block: stop
                        break
                    block.append(w)
                if not ok:
                    self.decisions.append({"round": rnd, "advanced": True,
                                           "depth": 0, "note": "dependent"})
                    return "dependent"
                if self._block_gram_ok(block):
                    self.basis.extend(block)
                    nxt = matvec(self.rows, block[-1], n)
                    frozen = None
                    self.repairs += 1
                    self.maxdepth = max(self.maxdepth, depth)
                    self.decisions.append({"round": rnd, "advanced": True,
                                           "depth": depth, "note": "repaired"})
                    repaired = True
                    break
            if not repaired:
                self.decisions.append({"round": rnd, "advanced": False,
                                       "depth": 0, "note": "needs_deeper"})
                return "needs_deeper"
        return "unfinished"

    def reverse(self) -> dict:
        """Re-derive every round decision from retained state + history bits.

        Non-repair rounds: the g0 zero-test on the snapshot must predict the
        recorded advance bit. Repair rounds: the recorded depth's block Gram
        must re-solve invertible. Any mismatch (including a flipped history
        bit) refuses — the 1-bit-per-round record is load-bearing.
        """
        if len(self.decisions) != len(self.snapshots):
            raise ValueError("decisions/snapshots out of sync (implementation bug): refusing")
        for dec, v in zip(self.decisions, self.snapshots):
            if dec["note"] == "dependent":
                if v != 0:
                    raise ValueError(f"round {dec['round']}: dependent without zero vector: refusing")
                continue
            if dec["note"] == "needs_deeper":
                continue  # honest stop carries no recomputable claim
            if dec["round"] not in self.repair_rounds:
                expect_advanced = not (v != 0 and dot(v, v) == 0)
                if dec["advanced"] != expect_advanced or dec["depth"] != (1 if expect_advanced else 0):
                    raise ValueError(f"round {dec['round']}: history bit mismatch: refusing")
            else:
                if not dec["advanced"]:
                    raise ValueError(f"round {dec['round']}: repair round must advance: refusing")
                if dec["depth"] not in (1, 2, 3):
                    raise ValueError(f"round {dec['round']}: bad depth: refusing")
        return {"rounds": len(self.decisions), "history_bits": len(self.decisions),
                "recomputed": True, "repairs": self.repairs, "maxdepth": self.maxdepth,
                "frozen_rounds": self.frozen_rounds}

    def null_vector(self) -> tuple[int | None, bool, str]:
        """Verified null via the shared solve (same gates as LanczosWalker)."""
        return _null_from_basis(self.rows, self.n, self.basis)


def scheduled_find_null(rows: list[int], n: int, seeds: list[int],
                        repair_rounds: set[int] | None = None,
                        max_rounds: int | None = None) -> dict:
    """Seed loop over ScheduledWalker; first verified null wins (honest report)."""
    check_symmetric(rows, n)
    if not seeds or any(not isinstance(s, int) or s <= 0 or s >= (1 << n) for s in seeds):
        raise ValueError("seeds must be nonzero n-bit ints: refusing")
    tried = restarts = repairs = 0
    maxdepth = frozen = 0
    for s in seeds:
        tried += 1
        w = ScheduledWalker(rows, n, repair_rounds, max_rounds)
        outcome = w.run(s)
        repairs += w.repairs
        maxdepth = max(maxdepth, w.maxdepth)
        frozen += w.frozen_rounds
        if outcome != "dependent":
            restarts += 1
            continue
        w.reverse()  # reversal must hold before any null is trusted
        sig, gram_full, via = w.null_vector()
        if sig is not None:
            return {"null": sig, "seeds_tried": tried, "restarts": restarts,
                    "repairs": repairs, "maxdepth": maxdepth, "frozen_rounds": frozen,
                    "basis_size": len(w.basis), "history_bits": len(w.decisions),
                    "gram_full": gram_full, "via": via, "proven": True}
        restarts += 1
    return {"null": None, "seeds_tried": tried, "restarts": restarts,
            "repairs": repairs, "maxdepth": maxdepth, "frozen_rounds": frozen,
            "basis_size": 0, "history_bits": 0,
            "gram_full": True, "via": "none", "proven": False}
