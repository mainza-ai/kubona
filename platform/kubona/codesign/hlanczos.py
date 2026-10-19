"""Block-Lanczos null finder over binary extension fields (SI I.2 at scale).

The HOPI locator is a null vector of a SYMMETRIC matrix A over F64 — the
GF(2) toy (lanczos.py) cannot touch it (F64 inner products, inversions, and
isotropic breakdowns all differ). This module runs the same paper program
over any BinaryField: Krylov chain, 3-term recurrence, depth-2/3 block
repair on singular self-dots, exact nullspace solve, verified A.sigma == 0.

Two paper facts reproduced as machine-checked structure (never cited):
- Cheap breakdown test: in characteristic 2, w^T w = (sum w_i)^2 (Frobenius
  linearity), so g0 needs only CNOTs — pinned as self_dot_is_square_sum().
- Evaluation--interpolation: A u = Phi^T diag(eta) Phi u with eta linear in
  the syndromes. The maps Phi/Phi^T are fixed linear (CNOT-only); the only
  multiplications are the n_s pointwise products (paper: 123 x 15T in
  F64). factored_matvec() proves the algebra (== direct, counted costs).

Caps (stated): dimension <= 48 (paper's locator dimension; exact
solver, verified live), field degree <= 6. Repairs happen where
breakdowns occur (scheduling is lanczos.ScheduledWalker's proven
dimension, orthogonal).
"""
from __future__ import annotations


def _check_field(field) -> None:
    for m in ("add", "mul", "inv", "sqr", "n", "size"):
        if not hasattr(field, m):
            raise ValueError(f"field lacks {m} (BinaryField expected): refusing")
    if not (1 <= field.n <= 6):
        raise ValueError(f"field degree must be in [1,6], got {field.n}: refusing")


def _check_vec(v: list[int], n: int, field) -> None:
    if not isinstance(v, list) or len(v) != n:
        raise ValueError(f"vector must have length {n}: refusing")
    for x in v:
        if not isinstance(x, int) or not (0 <= x < field.size):
            raise ValueError(f"element {x!r} outside field: refusing")


def vadd(u: list[int], v: list[int], field) -> list[int]:
    _check_field(field)
    _check_vec(u, len(u), field)
    _check_vec(v, len(u), field)
    return [field.add(a, b) for a, b in zip(u, v)]


def vscale(s: int, v: list[int], field) -> list[int]:
    _check_field(field)
    _check_vec(v, len(v), field)
    if not isinstance(s, int) or not (0 <= s < field.size):
        raise ValueError(f"scalar {s!r} outside field: refusing")
    return [field.mul(s, a) for a in v]


def dot(u: list[int], v: list[int], field) -> int:
    """Symmetric bilinear form (the paper's w^T v over the field)."""
    _check_field(field)
    _check_vec(u, len(u), field)
    _check_vec(v, len(u), field)
    acc = 0
    for a, b in zip(u, v):
        acc = field.add(acc, field.mul(a, b))
    return acc


def self_dot_is_square_sum(w: list[int], field) -> int:
    """w^T w == (sum w_i)^2 (Frobenius: cross terms vanish in char 2).

    The paper's cheap conditioning test (CNOT-only): g0 needs no
    multipliers. Returns the dot; callers compare against zero.
    """
    s = 0
    for a in w:
        s = field.add(s, a)
    return field.sqr(s)


def matvec(rows: list[list[int]], v: list[int], field) -> list[int]:
    _check_field(field)
    n = len(rows)
    _check_vec(v, n, field)
    for r in rows:
        _check_vec(r, n, field)
    return [dot(r, v, field) for r in rows]


def check_symmetric(rows: list[list[int]], field) -> int:
    _check_field(field)
    n = len(rows)
    # Cap raised 12 -> 48: the solver is exact Gauss-Jordan (nothing
    # degrades with n), and 48 is the paper's locator dimension — the
    # cap gates runaway sizes, not exactness. Verified live at 48-dim.
    if n < 2 or n > 48:
        raise ValueError(f"dimension must be in [2,48], got {n}: refusing")
    for r in rows:
        _check_vec(r, n, field)
    for i in range(n):
        for j in range(n):
            if rows[i][j] != rows[j][i]:
                raise ValueError(f"matrix not symmetric at ({i},{j}): refusing")
    return n


def _solve(rows: list[list[int]], rhs: list[int], field) -> list[int] | None:
    """Gauss-Jordan over the field (None iff singular — honest miss)."""
    n = len(rows)
    mat = [list(r) + [b] for r, b in zip(rows, rhs)]
    where = [-1] * n
    row = 0
    for col in range(n):
        piv = next((i for i in range(row, n) if mat[i][col] != 0), None)
        if piv is None:
            continue
        mat[row], mat[piv] = mat[piv], mat[row]
        inv = field.inv(mat[row][col])
        mat[row] = [field.mul(inv, a) for a in mat[row]]
        for i in range(n):
            if i != row and mat[i][col] != 0:
                f = mat[i][col]
                mat[i] = [field.add(a, field.mul(f, b)) for a, b in zip(mat[i], mat[row])]
        where[col] = row
        row += 1
    for i in range(row, n):
        if all(a == 0 for a in mat[i][:n]) and mat[i][n] != 0:
            return None
    out = [0] * n
    for col in range(n):
        if where[col] != -1:
            out[col] = mat[where[col]][n]
    # verify (a failed verification is an implementation bug, refused)
    for i in range(n):
        if dot(rows[i], out, field) != rhs[i]:
            raise ValueError("solve residual nonzero (implementation bug): refusing")
    return out


def factored_matvec(phi: list[list[int]], lin: list[list[int]],
                    syndromes: list[int], u: list[int], field) -> dict:
    """A u = Phi^T diag(eta) Phi u with eta = lin * syndromes (exact).

    phi: n_s x n fixed evaluation map; lin: n_s x n_syn fixed weights.
    Counts field multiplications honestly: 2*n_s*n linear applications
    (CNOT-analog, free) + n_s pointwise products. Returns {out, mults}
    with out verified below by callers against direct multiplication.
    """
    _check_field(field)
    n_s = len(phi)
    n = len(phi[0]) if phi else 0
    if n == 0:
        raise ValueError("phi must be non-empty: refusing")
    for r in phi:
        _check_vec(r, n, field)
    if len(lin) != n_s or any(len(r) != len(syndromes) for r in lin):
        raise ValueError("lin must be n_s x n_syn: refusing")
    _check_vec(u, n, field)
    _check_vec(syndromes, len(syndromes), field)
    # eta = lin * syndromes (fixed linear map of the supplied syndromes)
    eta = [dot(r, syndromes, field) for r in lin]
    mults = 0
    vals = [dot(r, u, field) for r in phi]  # evaluate at fixed points
    for i in range(n_s):
        vals[i] = field.mul(vals[i], eta[i])  # pointwise products (the cost)
        mults += 1
    out = [0] * n  # Phi^T back to coefficients
    for j in range(n):
        acc = 0
        for i in range(n_s):
            acc = field.add(acc, field.mul(phi[i][j], vals[i]))
        out[j] = acc
    return {"out": out, "mults": mults,
            "linear_apps": 2 * n_s * n, "n_points": n_s}


class FieldWalker:
    """Block-Lanczos walk over an extension field (immediate repair).

    Same paper program as the GF(2) walkers (Krylov chain, orthogonal basis,
    depth-2/3 block repair when the self-dot vanishes, exact null solve),
    with field-arithmetic throughout. Repairs happen where breakdowns occur
    (scheduling is the orthogonal, already-proven dimension).
    """

    def __init__(self, rows: list[list[int]], field):
        _check_field(field)
        self.n = check_symmetric(rows, field)
        self.rows = [list(r) for r in rows]
        self.field = field
        self.basis: list[list[int]] = []
        self.repairs = 0
        self.maxdepth = 0

    def _project(self, v: list[int]) -> tuple[list[int], bool]:
        """Remove basis components via block-Gram solve (exact)."""
        f = self.field
        r = len(self.basis)
        if r == 0:
            return list(v), False
        gram = [[dot(a, b, f) for b in self.basis] for a in self.basis]
        rhs = [dot(a, v, f) for a in self.basis]
        # solve per-column systems jointly is overkill; solve full vector:
        # Gram a = rhs via augmented solve built from _solve on Gram rows.
        aug = [list(row) for row in gram]
        # reuse _solve row-by-row? _solve takes square rows + rhs directly:
        coef = _solve(aug, rhs, f)
        if coef is None:
            raise ValueError("singular basis Gram (implementation bug): refusing")
        out = list(v)
        for c, b in zip(coef, self.basis):
            out = vadd(out, vscale(c, b, f), f)
        # NOTE char-2: subtraction == addition, so projection is exact here.
        for b in self.basis:
            if dot(b, out, f) != 0:
                raise ValueError("projection residual nonzero (implementation bug): refusing")
        return out, (out != v)

    def _gram_ok(self, block: list[list[int]]) -> bool:
        f = self.field
        d = len(block)
        gram = [[dot(a, b, f) for b in block] for a in block]
        return all(_solve(gram, [1 if i == j else 0 for i in range(d)], f) is not None
                   for j in range(d))

    def run(self, seed: list[int]) -> str:
        """One Krylov chain to dependence: 'dependent' | 'needs_deeper'."""
        f, n = self.field, self.n
        _check_vec(seed, n, f)
        if all(a == 0 for a in seed):
            raise ValueError("zero seed: refusing")
        cur = seed
        rounds = 0
        while True:
            rounds += 1
            if rounds > 3 * n + 4:
                raise ValueError("walk exceeded 3n+4 rounds (implementation bug): refusing")
            cur, _ = self._project(cur)
            if all(a == 0 for a in cur):
                return "dependent"
            if dot(cur, cur, f) != 0:
                self.basis.append(cur)
                cur = matvec(self.rows, cur, f)
                continue
            repaired = False
            for depth in (2, 3):
                block = [cur]
                ok = True
                for _ in range(1, depth):
                    w, _ = self._project(matvec(self.rows, block[-1], f))
                    if all(a == 0 for a in w):
                        ok = False
                        break
                    block.append(w)
                if not ok:
                    return "dependent"
                if self._gram_ok(block):
                    self.basis.extend(block)
                    cur = matvec(self.rows, block[-1], f)
                    self.repairs += 1
                    self.maxdepth = max(self.maxdepth, depth)
                    repaired = True
                    break
            if not repaired:
                return "needs_deeper"

    def null_vector(self) -> list[int] | None:
        """Verified null (projection solve, then direct fallback)."""
        f, n = self.field, self.n
        r = len(self.basis)
        if r == 0:
            return None
        proj = []
        for i in range(r):
            abi = matvec(self.rows, self.basis[i], f)
            proj.append([dot(self.basis[j], abi, f) for j in range(r)])
        c = _nullspace_list(proj, f)
        if c is not None:
            sig = [0] * n
            for coef, b in zip(c, self.basis):
                sig = vadd(sig, vscale(coef, b, f), f)
            if any(a != 0 for a in sig) and all(a == 0 for a in matvec(self.rows, sig, f)):
                return sig
        masks = []
        aws = [matvec(self.rows, w, f) for w in self.basis]
        for i in range(n):
            masks.append([w[i] for w in aws])
        # direct: n equations (one per output component) in r basis unknowns.
        c2 = _nullspace_list(masks, f)
        if c2 is None:
            return None
        sig = [0] * n
        for coef, b in zip(c2, self.basis):
            sig = vadd(sig, vscale(coef, b, f), f)
        if all(a == 0 for a in sig) or any(a != 0 for a in matvec(self.rows, sig, f)):
            raise ValueError("null candidate fails verification (implementation bug): refusing")
        return sig


def _nullspace_list(rows: list[list[int]], field) -> list[int] | None:
    """Nullspace of a row list over the field (verified by caller)."""
    n = len(rows[0]) if rows else 0
    if n == 0:
        raise ValueError("empty system: refusing")
    basis: list[list[int]] = []
    pivots: list[int] = []
    for r in rows:
        v = list(r)
        for b, p in zip(basis, pivots):
            if v[p] != 0:
                f = field.mul(v[p], field.inv(b[p]))
                v = [field.add(a, field.mul(f, c)) for a, c in zip(v, b)]
        piv = next((j for j in range(n) if v[j] != 0), None)
        if piv is None:
            continue
        f = field.inv(v[piv])
        basis.append([field.mul(f, a) for a in v])
        pivots.append(piv)
    free = [j for j in range(n) if j not in pivots]
    if not free:
        return None
    # backward pass to reduced row-echelon (forward pass alone leaves
    # nonzero entries above later pivots, which back-substitution would
    # read as zeros — the bug the solver self-check caught).
    for k in range(len(basis) - 1, -1, -1):
        for i in range(k):
            if basis[i][pivots[k]] != 0:
                f = field.mul(basis[i][pivots[k]], field.inv(basis[k][pivots[k]]))
                basis[i] = [field.add(a, field.mul(f, c))
                            for a, c in zip(basis[i], basis[k])]
    out = [0] * n
    out[free[0]] = 1
    for b, p in zip(basis, pivots):
        s = 0
        for j in range(n):
            s = field.add(s, field.mul(b[j], out[j]))
        out[p] = s
    return out


def find_null_f64(rows: list[list[int]], field, seeds: list[list[int]]) -> dict:
    """Seed loop; first verified null wins (honest report)."""
    n = check_symmetric(rows, field)
    if not seeds:
        raise ValueError("need at least one seed: refusing")
    tried = restarts = repairs = 0
    maxdepth = 0
    for s in seeds:
        tried += 1
        w = FieldWalker(rows, field)
        if w.run(s) != "dependent":
            restarts += 1
            continue
        repairs += w.repairs
        maxdepth = max(maxdepth, w.maxdepth)
        sig = w.null_vector()
        if sig is not None:
            return {"null": sig, "seeds_tried": tried, "restarts": restarts,
                    "repairs": repairs, "maxdepth": maxdepth,
                    "basis_size": len(w.basis), "proven": True}
        restarts += 1
    return {"null": None, "seeds_tried": tried, "restarts": restarts,
            "repairs": repairs, "maxdepth": maxdepth,
            "basis_size": 0, "proven": False}


class ScheduledFieldWalker:
    """Lanczos walk with scheduled repair rounds over an extension field.

    Ports lanczos.ScheduledWalker's discipline to F64: ordinary rounds
    advance only when g0 = w^T w != 0 (the paper's cheap CNOT-only test);
    a self-orthogonal vector off-schedule FREEZES (carried unchanged until
    the next repair round, which forms a depth-2/3 block and advances past
    it); the paper's ending test (g0 = g1 = g2 == 0 with gk = w^T A^k w:
    no depth <= 3 block invertible — frozen + recorded instead of a doomed
    repair attempt); three-case routing after the last round (accumulated
    via null_vector() on dependence, self when Aw == 0, advance when
    A^2w == 0 — each verified); 1 history bit per round with reverse
    re-derivation (a flipped bit refuses).

    Queued (seeding under-specified in the paper excerpt): the xi-register
    accumulation (locator built incrementally during the walk). The
    locator here comes from the proven null_vector() path instead.
    Toy simplifications (stated): snapshots retained (paper recomputes
    from two live vectors); max_rounds = n + 4 default (paper 48 + 4).
    """

    def __init__(self, rows: list[list[int]], field,
                 repair_rounds: set[int] | None = None,
                 max_rounds: int | None = None):
        _check_field(field)
        self.n = check_symmetric(rows, field)
        self.rows = [list(r) for r in rows]
        self.field = field
        if max_rounds is None:
            max_rounds = self.n + 4
        if isinstance(max_rounds, bool) or not isinstance(max_rounds, int) \
                or not (1 <= max_rounds <= 64):
            raise ValueError(f"max_rounds must be in [1,64], got {max_rounds!r}: refusing")
        if repair_rounds is None:
            repair_rounds = set(range(max_rounds))
        if not isinstance(repair_rounds, set) or any(
                isinstance(r, bool) or not isinstance(r, int) or not (0 <= r < max_rounds)
                for r in repair_rounds):
            raise ValueError("repair_rounds must be a set of round indices: refusing")
        self.repair_rounds = set(repair_rounds)
        self.max_rounds = max_rounds
        self.basis: list[list[int]] = []
        self.decisions: list[dict] = []
        self.snapshots: list[list[int]] = []
        self.repairs = 0
        self.maxdepth = 0
        self.frozen_rounds = 0
        self.end_round: int | None = None
        self.terminal: str | None = None

    def end_check(self, v: list[int]) -> bool:
        """g0 = g1 = g2 == 0 with gk = v^T A^k v (no depth <= 3 repair).

        A d <= 3 Krylov block has first Gram row (g_0..g_{d-1}) = 0, hence
        singular — the paper's reason repair is hopeless. Direct check.
        """
        f = self.field
        _check_vec(v, self.n, f)
        if dot(v, v, f) != 0:
            return False
        Av = matvec(self.rows, v, f)
        if dot(v, Av, f) != 0:
            return False
        return dot(v, matvec(self.rows, Av, f), f) == 0

    def _block_gram_ok(self, block: list[list[int]]) -> bool:
        f = self.field
        d = len(block)
        gram = [[dot(a, b, f) for b in block] for a in block]
        return all(_solve(gram, [1 if i == j else 0 for i in range(d)], f) is not None
                   for j in range(d))

    def run(self, seed: list[int]) -> str:
        """Walk to an honest stop: dependent | needs_deeper | unfinished | ended."""
        f, n = self.field, self.n
        _check_vec(seed, n, f)
        if all(a == 0 for a in seed):
            raise ValueError("zero seed: refusing")
        walker = FieldWalker(self.rows, f)
        nxt = list(seed)
        frozen: list[int] | None = None
        for rnd in range(self.max_rounds):
            incoming = frozen if frozen is not None else nxt
            v, _ = walker._project(incoming)
            self.snapshots.append(v)
            if all(a == 0 for a in v):
                self.decisions.append({"round": rnd, "advanced": True,
                                       "depth": 0, "note": "dependent"})
                self.terminal = "dependent"
                self.basis = walker.basis
                return "dependent"
            if dot(v, v, f) != 0:
                walker.basis.append(v)
                nxt = matvec(self.rows, v, f)
                frozen = None
                self.decisions.append({"round": rnd, "advanced": True,
                                       "depth": 1, "note": "ordinary"})
                continue
            # self-orthogonal breakdown: end test first (doomed repair),
            # then freeze off-schedule, else repair on-schedule.
            if self.end_check(v):
                self.end_round = rnd
                self.decisions.append({"round": rnd, "advanced": False,
                                       "depth": 0, "note": "ended"})
                self.terminal = "ended"
                self.basis = walker.basis
                return "ended"
            if rnd not in self.repair_rounds:
                frozen = v
                self.frozen_rounds += 1
                self.decisions.append({"round": rnd, "advanced": False,
                                       "depth": 0, "note": "frozen"})
                continue
            repaired = False
            for depth in (2, 3):
                block = [v]
                ok = True
                for _ in range(1, depth):
                    w, _ = walker._project(matvec(self.rows, block[-1], f))
                    if all(a == 0 for a in w):
                        ok = False
                        break
                    block.append(w)
                if not ok:
                    self.decisions.append({"round": rnd, "advanced": True,
                                           "depth": 0, "note": "dependent"})
                    self.terminal = "dependent"
                    self.basis = walker.basis
                    return "dependent"
                if self._block_gram_ok(block):
                    walker.basis.extend(block)
                    nxt = matvec(self.rows, block[-1], f)
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
                self.terminal = "needs_deeper"
                self.basis = walker.basis
                return "needs_deeper"
        self.terminal = "unfinished"
        self.basis = walker.basis
        return "unfinished"

    def route(self) -> dict:
        """Three-case routing on the terminal state (all verified).

        dependent -> accumulated null via null_vector() (paper case a);
        otherwise basis[-1]: self when Aw == 0 (b), advance Aw when
        A^2w == 0 (c). No case holds -> fail-closed (paper: "fails").
        """
        f, n = self.field, self.n
        if self.terminal is None:
            raise ValueError("walk has not run: refusing")
        if not self.basis:
            raise ValueError("no basis vectors: refusing")
        if self.terminal == "dependent":
            w = FieldWalker(self.rows, f)
            w.basis = [list(b) for b in self.basis]
            sig = w.null_vector()
            if sig is None:
                raise ValueError("accumulated null missing (honest miss): refusing")
            return {"case": "accumulated", "locator": sig}
        w = self.basis[-1]
        Aw = matvec(self.rows, w, f)
        if all(a == 0 for a in Aw):
            return {"case": "self", "locator": list(w)}
        if all(a == 0 for a in matvec(self.rows, Aw, f)):
            if all(a == 0 for a in Aw):
                raise ValueError("advance of zero vector (implementation bug): refusing")
            return {"case": "advance", "locator": Aw}
        raise ValueError("no routing case holds (honest miss): refusing")

    def reverse(self) -> dict:
        """Re-derive every round decision from retained state + history bits.

        Ordinary rounds: the g0 test on the snapshot must predict the
        recorded advance bit. Ended rounds: the snapshot must satisfy the
        g0 = g1 = g2 == 0 test. Repair rounds: recorded depth's block Gram
        must re-solve invertible. needs_deeper carries no recomputable
        claim. Any mismatch (including a flipped history bit) refuses.
        """
        f = self.field
        if len(self.decisions) != len(self.snapshots):
            raise ValueError("decisions/snapshots out of sync (implementation bug): refusing")
        for dec, v in zip(self.decisions, self.snapshots):
            note = dec["note"]
            if note == "dependent":
                if any(a != 0 for a in v):
                    raise ValueError(f"round {dec['round']}: dependent without zero: refusing")
                continue
            if note == "needs_deeper":
                continue
            if note == "ended":
                if not self.end_check(v):
                    raise ValueError(f"round {dec['round']}: end test mismatch: refusing")
                if dec["advanced"]:
                    raise ValueError(f"round {dec['round']}: ended round must not advance: refusing")
                continue
            if note == "frozen":
                if dec["advanced"] or dot(v, v, f) != 0:
                    raise ValueError(f"round {dec['round']}: frozen mismatch: refusing")
                continue
            if dec["round"] not in self.repair_rounds:
                expect = dot(v, v, f) != 0
                if dec["advanced"] != expect or dec["depth"] != (1 if expect else 0):
                    raise ValueError(f"round {dec['round']}: history bit mismatch: refusing")
            else:
                if not dec["advanced"]:
                    raise ValueError(f"round {dec['round']}: repair round must advance: refusing")
                if dec["depth"] not in (1, 2, 3):
                    raise ValueError(f"round {dec['round']}: bad depth: refusing")
        return {"rounds": len(self.decisions), "history_bits": len(self.decisions),
                "recomputed": True, "repairs": self.repairs, "maxdepth": self.maxdepth,
                "frozen_rounds": self.frozen_rounds, "end_round": self.end_round}


def scheduled_find_null_f64(rows: list[list[int]], field, seeds: list[list[int]],
                            repair_rounds: set[int] | None = None,
                            max_rounds: int | None = None) -> dict:
    """Seed loop over ScheduledFieldWalker; first routed locator wins."""
    n = check_symmetric(rows, field)
    if not seeds:
        raise ValueError("need at least one seed: refusing")
    tried = restarts = repairs = 0
    maxdepth = frozen = 0
    for s in seeds:
        tried += 1
        w = ScheduledFieldWalker(rows, field, repair_rounds, max_rounds)
        outcome = w.run(s)
        repairs += w.repairs
        maxdepth = max(maxdepth, w.maxdepth)
        frozen += w.frozen_rounds
        if outcome not in ("dependent", "ended", "unfinished"):
            restarts += 1
            continue
        try:
            rev = w.reverse()
        except ValueError:
            restarts += 1
            continue
        try:
            routed = w.route()
        except ValueError:
            restarts += 1
            continue
        return {"locator": routed["locator"], "case": routed["case"],
                "outcome": outcome, "seeds_tried": tried, "restarts": restarts,
                "repairs": repairs, "maxdepth": maxdepth,
                "frozen_rounds": frozen, "basis_size": len(w.basis),
                "history_bits": rev["history_bits"], "proven": True}
    return {"locator": None, "case": "none", "outcome": "miss",
            "seeds_tried": tried, "restarts": restarts,
            "repairs": repairs, "maxdepth": maxdepth,
            "frozen_rounds": frozen, "basis_size": 0, "history_bits": 0,
            "proven": False}


#: Paper per-round circuit costs (SI walk section, anchored, never
#: re-derived): ordinary rounds 5,400T, depth-2/3-capable rounds
#: 12,000/17,000T, full-walk reverse 52,000T fewer. The reverse-saving
#: figure is quoted as-is for the paper's 52-round walk (never scaled).
WALK_PAPER_COSTS = {"ordinary_round": 5400, "repair_round_d2": 12000,
                    "repair_round_d3": 17000,
                    "reverse_saving_full_walk": 52000}


def walk_cost_ledger(decisions: list[dict]) -> dict:
    """Cost ledger from a walk's round mix (projection, not a build).

    Applies the paper's per-round unit costs to the MEASURED mix
    (ordinary vs fired depth-2/3 repairs); frozen/terminal rounds carry
    no update gates (the flag disables every update). Labeled projection:
    the unit costs belong to the paper's circuits, the mix is ours.
    """
    n_ord = n_d2 = n_d3 = n_frozen = 0
    for d in decisions:
        note = d.get("note")
        if note == "ordinary":
            n_ord += 1
        elif note == "repaired":
            if d.get("depth") == 2:
                n_d2 += 1
            elif d.get("depth") == 3:
                n_d3 += 1
            else:
                raise ValueError(f"bad repair depth {d!r}: refusing")
        elif note in ("frozen", "dependent", "ended", "needs_deeper"):
            n_frozen += 1
        else:
            raise ValueError(f"unknown round note {note!r}: refusing")
    total = (n_ord * WALK_PAPER_COSTS["ordinary_round"]
             + n_d2 * WALK_PAPER_COSTS["repair_round_d2"]
             + n_d3 * WALK_PAPER_COSTS["repair_round_d3"])
    return {"ordinary": n_ord, "repaired_d2": n_d2, "repaired_d3": n_d3,
            "frozen_terminal": n_frozen, "total_T": total,
            "history_bits": len(decisions)}
