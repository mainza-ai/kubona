"""Direct squared-distance accumulation (Coulomb front-end as a circuit).

The paper forms the exact squared distance d^2 by adding the partial
products of all three coordinate squares DIRECTLY into one accumulator —
no separate product registers, no summation stage. This module is that
dataflow at toy scale: three 2-bit coordinate differences (non-negative
inputs; difference formation itself is outside this kernel, exactly as
coefficients ride as loads in horner.py) accumulate dx^2+dy^2+dz^2 into
one 5-bit zeroed accumulator (3*9 = 27 < 32: exact, model-checked bound).

Squaring reuses the proven schoolbook multiplier via a copy (CNOTs into
fresh temps, multiply copy x original, uncopy): uniform code path with
emit_mul_rows — no second squarer to rot apart, diagonal idempotence
(a_i^2 = a_i) falls out of the AND array with A != B qubits, never a
special case. Uncompute is the SUB-based unplan (horner-proven).
"""

from __future__ import annotations

from .horner import _exec, emit_add, emit_mul_rows
from .kickmix import Builder

C_BITS, ACC_BITS = 2, 5


def coulomb_d2_model(dx: int, dy: int, dz: int) -> int:
    """Exact squared distance (the accumulator's target value)."""
    for name, v in (("dx", dx), ("dy", dy), ("dz", dz)):
        if not isinstance(v, int) or not (0 <= v < (1 << C_BITS)):
            raise ValueError(f"{name} must fit in {C_BITS} bits: refusing {v!r}")
    return dx * dx + dy * dy + dz * dz


def build_coulomb_acc() -> tuple[Builder, dict]:
    """Three squares, one accumulator, no product registers (proven 64/64)."""
    dx, dy, dz = [0, 1], [2, 3], [4, 5]
    acc = list(range(6, 11))
    CPY = [11, 12]
    PROD = [13, 14, 15, 16]
    ROW = [17, 18]
    CARR = [19, 20, 21, 22]
    ZERO = 23
    temps = CPY + PROD + ROW + CARR + [ZERO]
    b = Builder(24)
    for q in temps:
        b.alloc(q)
    n_ccx = 0
    for coord in (dx, dy, dz):
        for i, q in enumerate(coord):  # copy (Clifford fan-out into CPY)
            b.cx(q, CPY[i])
        # NOTE: emit_mul_rows with A=CPY(2), B=coord(2), OUT=PROD(4) needs
        # CARR lenOUT-1 = 3 -> CARR[:3]; the accumulator add below needs the
        # full 4 (shared pool, sequential reuse, each op leaves them zero).
        t_fwd, un = emit_mul_rows(b, CPY, coord, PROD, ROW, CARR[:3], ZERO, 0)
        n_ccx += t_fwd
        n_ccx += emit_add(b, acc, PROD, CARR)
        n_ccx += _exec(b, un)  # clear PROD (SUB-based unplan, horner-proven)
        for i, q in enumerate(coord):  # uncopy
            b.cx(q, CPY[i])
    for q in temps:
        b.free(q)
    return b, {"coords": (tuple(dx), tuple(dy), tuple(dz)), "acc": tuple(acc),
               "ccx": n_ccx}


def coulomb_acc_spec():
    """TaskSpec: zeroed acc gets dx^2+dy^2+dz^2 (6 inputs, exhaustive 64)."""
    from .kickmix import TaskSpec

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        v = coulomb_d2_model(bits.get(0, 0) | ((bits.get(1, 0) & 1) << 1),
                             bits.get(2, 0) | ((bits.get(3, 0) & 1) << 1),
                             bits.get(4, 0) | ((bits.get(5, 0) & 1) << 1))
        for i in range(5):
            out[6 + i] = (v >> i) & 1
        return out

    anc = tuple(list(range(11, 24)))
    return TaskSpec(num_qubits=24, inputs=(0, 1, 2, 3, 4, 5), outputs=tuple(range(6, 11)),
                    ancillas=anc, target_perm=_perm)


#: Slope fractional bits for integer line coefficients (circuit-loadable).
LINE_SLOPE_BITS = 8


def attainable_d2(nbits: int = 8):
    """Sorted unique nonzero dx^2+dy^2+dz^2 over n-bit coordinates.

    The paper's promised domain (distinct points in {0..255}^3); numpy-
    vectorized (pinned dependency), deterministic order. Count-gated at
    nbits=8 (139637 attainable distances — reproduced, not cited).
    """
    import numpy as np
    if not isinstance(nbits, int) or not (1 <= nbits <= 8):
        raise ValueError(f"nbits must be in [1,8], got {nbits!r}: refusing")
    c = np.arange(1 << nbits, dtype=np.int32)
    d2 = np.unique((c[:, None, None] ** 2 + c[None, :, None] ** 2
                    + c[None, None, :] ** 2).ravel())
    d2 = d2[d2 > 0]
    if nbits == 8 and len(d2) != 139637:
        raise ValueError(f"attainable count {len(d2)}, want 139637: refusing")
    return d2


def fit_coulomb_lines(nbits: int = 8) -> dict:
    """128-line table over mantissa slices (paper SI III.2 pattern, LS fit).

    Normalizes d^2 = 2^e u (u in [1,2)); parity(e) + 6 leading mantissa bits
    select the line, next 6 bits are the local coordinate. Even lines fit
    C/sqrt(u), odd lines C/(sqrt(2) sqrt(u)) (the 2^-1/2 lives in the line,
    the remaining 2^{-(e-1)/2} is an exact shift) with C = 2^24; results
    scale back by exact shifts — the parity bookkeeping above is load-
    bearing (a uniform 2^-e/2 scale understates odd lines by sqrt(2),
    caught in review). Integer coefficients (A, B at 256 slope steps) from
    least-squares rounding, then scale-weighted minimax coordinate descent:
    the objective is max TRUE error (per-point 2^{-e/2} weights), because
    small-e points dominate the bound while large-e points attenuate — an
    unweighted H-space minimax optimizes the wrong objective (measured no
    gain) and a uniform scale misstates odd lines. This mirrors the paper's
    offline search (fit to the error the circuit commits); deterministic
    (fixed schedules, no randomness). Error measured on the integer law vs
    true 1/sqrt(d^2), worst over the full attainable domain.
    """
    import numpy as np
    d2 = attainable_d2(nbits).astype(np.float64)
    e = np.floor(np.log2(d2)).astype(int)
    u = d2 / 2.0**e
    sel = ((e & 1) << 6) | (((u - 1.0) * 64).astype(int).clip(0, 63))
    t6 = ((((u - 1.0) * 64) % 1) * 64).astype(int).clip(0, 63)
    C = 2.0**24
    H = np.where((e & 1) == 0, C / np.sqrt(u), C / (np.sqrt(2.0) * np.sqrt(u)))
    S = 1 << LINE_SLOPE_BITS
    lines: dict[int, tuple[int, int, int]] = {}
    worst = 0.0
    worst_at = 0
    worst_line = 0
    true_inv = 1.0 / np.sqrt(d2)
    for L in range(128):
        m = sel == L
        if int(m.sum()) < 2:
            raise ValueError(f"line {L} has <2 points (implementation bug): refusing")
        tt = t6[m].astype(float)
        slope, intercept = np.polyfit(tt, H[m], 1)  # highest-degree first
        A, B = round(float(intercept)), round(float(slope) * S)
        w = np.where((e[m] & 1) == 0,
                     2.0 ** (-e[m] / 2.0) / C, 2.0 ** (-(e[m] - 1) / 2.0) / C)
        best = float((np.abs((A + B * tt / S) - H[m]) * w).max())
        for _ in range(6):  # scale-weighted minimax (deterministic schedule)
            moved = False
            for step in (4096, 1024, 256, 64, 16, 4, 1):
                for dA, dB in ((step, 0), (-step, 0), (0, step), (0, -step)):
                    cand = float((np.abs((A + dA + (B + dB) * tt / S) - H[m]) * w).max())
                    if cand < best:
                        A, B, best = A + dA, B + dB, cand
                        moved = True
                        break
                if moved:
                    break
            if not moved:
                break
        got = np.where((e[m] & 1) == 0,
                       (A + B * tt / S) * 2.0 ** (-e[m] / 2.0) / C,
                       (A + B * tt / S) * 2.0 ** (-(e[m] - 1) / 2.0) / C)
        err = np.abs(got - true_inv[m])
        k = int(np.argmax(err))
        if float(err[k]) > worst:
            worst, worst_at, worst_line = float(err[k]), int(d2[m][k]), L
        lines[L] = (A, B, int(m.sum()))
    return {"lines": lines, "worst": worst, "worst_at_d2": worst_at,
            "worst_line": worst_line, "n_lines": len(lines),
            "n_points": len(d2), "tol": 2e-6,
            "within_tol": worst < 2e-6}
