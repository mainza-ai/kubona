"""Variable-spaced QROM pruning (paper SI III.3, Sanders et al. [66]).

A conventional QROM spends gates on every interval. The variable-spaced
QROM stops whenever an entire address subtree shares one value: the cost
is the number of DISTINCT subtrees, not entries. The paper's Gaussian
lookup (1024 intervals: 455 quadratics + 59 constants + 510 zeros) drops
from 1,022 to 495 Toffolis this way.

What is exact here (and tested): the COMBINATORICS — distinct-subtree
counts for a given table, and the theorem that pruning never costs more
than the naive tree. The Toffoli MAPPING (1022/495) depends on the QROM
construction constants and is cited as the paper's anchor, not reproduced.
"""

from __future__ import annotations


def distinct_subtrees(table: list) -> int:
    """Cost of the pruned address tree: distinct leaf values plus distinct
    internal nodes whose two children DIFFER (paper SI IV: "visits one copy
    and skips the address bit" — identical-child nodes cost nothing).

    table: 2^k values (leaf data). A full tree has 2^{k+1}-1 nodes; pruning
    keeps <= that, with equality only for all-distinct leaves.
    """
    if not table or len(table) & (len(table) - 1):
        raise ValueError(f"table needs 2^k entries, got {len(table)}: refusing")
    level = [("leaf", v) for v in table]
    total = len(set(table))  # distinct leaf values
    while len(level) > 1:
        nxt = []
        seen: set = set()
        for i in range(0, len(level), 2):
            left, right = level[i], level[i + 1]
            key = (left, right)
            if left != right and key not in seen:
                seen.add(key)
                total += 1
            nxt.append(key)
        level = nxt
    return total


def naive_nodes(n_entries: int) -> int:
    """Full binary-tree node count for n_entries leaves (power of two)."""
    if n_entries <= 0 or n_entries & (n_entries - 1):
        raise ValueError(f"need positive power of two, got {n_entries}: refusing")
    return 2 * n_entries - 1


def gaussian_regimes() -> list[str]:
    """Paper's Gaussian interval classes: 455 quad + 59 const + 510 zero."""
    return ["quad"] * 455 + ["const"] * 59 + ["zero"] * 510


def qrom_vs_direct(n_entries: int, n_values: int, word_bits: int,
                   addr_bits: int | None = None) -> dict:
    """QROM lookup+add+unlookup vs direct flag-readout+add (paper SI V).

    Cost model (explicit units, all 1T here): pruned-QROM lookup costs one
    Toffoli per tree node (cf. paper's 216-entry ~214T), paid TWICE (lookup
    + unlookup); direct readout costs one AND-tree per nonzero value over
    the address bits (overestimates shared substructure — conservative);
    modular addition costs one Toffoli per word bit on both sides.
    Paper anchor: 478 (QROM path) vs 76/75 (readout path) per loop; the
    model reproduces the DIRECTION and order, not the exact constants
    (their flags share substructure and their adder is tuned).
    """
    for name, v in (("n_entries", n_entries), ("n_values", n_values),
                    ("word_bits", word_bits)):
        if isinstance(v, bool) or not isinstance(v, int) or v < 1:
            raise ValueError(f"{name} must be a positive int: refusing")
    if addr_bits is None:
        addr_bits = max(1, (n_entries - 1).bit_length())
    if n_values < 1 or n_values > n_entries:
        raise ValueError(f"need 1 <= n_values <= n_entries: refusing {n_values!r}")
    lookup = n_entries  # pruned worst case: distinct entries (use distinct_subtrees when known)
    qrom = 2 * lookup + word_bits
    direct = (n_values - 1) * addr_bits + word_bits
    return {"qrom": qrom, "direct": direct,
            "winner": "direct" if direct < qrom else "qrom",
            "units": "Toffoli @ node=1T, flag-bit=1T, add-bit=1T (stated, not measured)"}


def _plan_subtree(entries: list[int], abits: list[int], target: int,
                   temps: list[int], level: int, share: bool) -> list[tuple]:
    """Op plan for target ^= F over this span (all primitives self-inverse).

    Contract (induction): with target starting at 0, the plan leaves
    target = F evaluated on the span's sub-addresses. Constant span needs one
    X iff 1; a fresh temp target makes the unconditional form exact at every
    level, and the caller muxes with the path bit. Identical halves (share):
    one copy plus fan-out, path bit skipped (paper SI IV). Split halves:
    t ^= L, u ^= R, u ^= t, out ^= bit&u, out ^= t, then the exact reverse
    (every primitive self-inverse) returns temps to zero.
    """
    if not entries or len(entries) & (len(entries) - 1):
        raise ValueError(f"span needs 2^j entries, got {len(entries)}: refusing")
    if any(v not in (0, 1) for v in entries):
        raise ValueError("table values must be single bits 0/1: refusing")
    if entries and all(v == entries[0] for v in entries):
        return [("x", (target,))] if entries[0] else []
    if not abits:
        raise ValueError("span varies with no address bits left (implementation bug): refusing")
    # Split bit MUST be the most significant remaining bit: entries[:half]
    # are the indices with that bit clear (list order counts binary on abits
    # in order). Muxing any other bit here mistargets the path (caught live
    # on the alternating table, which is maximally bit-sensitive).
    bit, rest = abits[-1], abits[:-1]
    half = len(entries) // 2
    left, right = entries[:half], entries[half:]
    t, u = temps[2 * level], temps[2 * level + 1]
    if share and left == right:
        sub = _plan_subtree(left, rest, t, temps, level + 1, share)
        return sub + [("cx", (t, target))] + sub[::-1]
    pl = _plan_subtree(left, rest, t, temps, level + 1, share)
    pr = _plan_subtree(right, rest, u, temps, level + 1, share)
    return (pl + pr + [("cx", (t, u)), ("ccx", (bit, u, target)), ("cx", (t, target))]
            + [("cx", (t, u))] + pr[::-1] + pl[::-1])


def _exec_plan(b, plan: list[tuple]) -> int:
    """Execute a planner op list (single code path for all layouts)."""
    from .kickmix import Builder
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    n_ccx = 0
    for op in plan:
        if op[0] == "x":
            b.x(op[1][0])
        elif op[0] == "cx":
            b.cx(op[1][0], op[1][1])
        elif op[0] == "ccx":
            b.ccx(op[1][0], op[1][1], op[1][2])
            n_ccx += 1
        else:
            raise ValueError(f"planner emitted {op[0]!r} (implementation bug): refusing")
    return n_ccx


def _check_table(table: list[int]) -> int:
    if not isinstance(table, list) or not table or len(table) > 16 \
            or len(table) & (len(table) - 1):
        raise ValueError(f"table needs 2^k entries with k<=4, got {len(table) if isinstance(table, list) else table!r}: refusing")
    if any(not isinstance(v, int) or v not in (0, 1) for v in table):
        raise ValueError("table values must be single bits 0/1: refusing")
    return len(table).bit_length() - 1


def emit_pruned_qrom_into(b, table: list[int], share: bool,
                            addr: list[int], out: int, temps: list[int]) -> int:
    """Emit a pruned-QROM load into an existing builder (composition mode).

    Same planner, same contract (out ^= table[a]); lifecycle (alloc/free of
    addr/out/temps) belongs to the caller — this emits gates only. Callers
    unload by re-emitting (XOR re-accumulation clears). Using
    build_pruned_qrom and dropping its builder (as a first composition
    draft did) silently drops the loads — caught live as all-zero
    coefficients with a correct count.
    """
    from .kickmix import Builder
    if not isinstance(b, Builder):
        raise ValueError("b must be a Builder: refusing")
    k = _check_table(table)
    if len(addr) != k or len(temps) < 2 * k:
        raise ValueError(f"need {k} addr bits and >= {2 * k} temps: refusing")
    for name, qs in (("addr", addr), ("temps", temps)):
        if len(qs) != len(set(qs)) or any(not isinstance(q, int) or q < 0
                                          or q >= b.num_qubits for q in qs):
            raise ValueError(f"{name} must be distinct qubits in range: refusing")
    if out in list(addr) + list(temps) or not (0 <= out < b.num_qubits):
        raise ValueError("out must be a fresh qubit in range: refusing")
    return _exec_plan(b, _plan_subtree(list(table), list(addr), out,
                                       list(temps), 0, bool(share)))


def build_pruned_qrom(table: list[int], share: bool = True, *,
                      addr: list[int] | None = None, out: int | None = None,
                      temps: list[int] | None = None, nqubits: int | None = None,
                      alloc_temps: bool = True) -> tuple[object, dict]:
    """Variable-spaced QROM loader (paper SI III.3/IV at toy scale, exact).

    Loads one classical bit per address into a zeroed target: out ^= table[a].
    Identical halves emit once with the address bit skipped (share=True, the
    paper's pruning); share=False emits both (naive baseline for the cost
    comparison — same emitter, sharing disabled, never a second formula).
    Proven exhaustive for <= 16 entries (2^k, k <= 4); larger refuses (proof scope).
    Temps: 2 per tree level, allocated up front, freed at the end (unless
    alloc_temps=False, in which case the caller owns their lifecycle —
    composition mode, stated).

    Register placement defaults to the standalone layout; compositions pass
    explicit addr/out/temps/nqubits (validated disjoint and in range).
    Default behavior is byte-identical to before (existing tests pin it).
    """
    from .kickmix import Builder
    k = _check_table(table)
    if addr is None:
        addr = list(range(k))
    if out is None:
        out = k
    if temps is None:
        temps = list(range(k + 1, 3 * k + 1))
    if nqubits is None:
        nqubits = 3 * k + 1
    for name, qs in (("addr", addr), ("temps", temps)):
        if len(qs) != len(set(qs)) or any(not isinstance(q, int) or q < 0 or q >= nqubits
                                          for q in qs):
            raise ValueError(f"{name} must be distinct qubits in [0,{nqubits}): refusing")
    if out in list(addr) + list(temps) or not (0 <= out < nqubits):
        raise ValueError("out must be a fresh qubit in range: refusing")
    if len(addr) != k or len(temps) < 2 * k:
        raise ValueError(f"need {k} addr bits and >= {2 * k} temps: refusing")
    b = Builder(nqubits)
    if alloc_temps:
        for q in temps:
            b.alloc(q)
    n_ccx = emit_pruned_qrom_into(b, table, share, list(addr), out, list(temps))
    if alloc_temps:
        for q in temps:
            b.free(q)
    return b, {"addr": tuple(addr), "out": out, "temps": tuple(temps), "ccx": n_ccx,
               "entries": len(table), "shared": bool(share)}


def pruned_qrom_spec(table: list[int], *, addr: list[int] | None = None,
                     out: int | None = None, temps: list[int] | None = None,
                     nqubits: int | None = None):
    """TaskSpec: zeroed target gets table[address] (single-bit words)."""
    from .kickmix import TaskSpec
    k = _check_table(table)
    if addr is None:
        addr = list(range(k))
    if out is None:
        out = k
    if temps is None:
        temps = list(range(k + 1, 3 * k + 1))
    if nqubits is None:
        nqubits = 3 * k + 1

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        outd = dict(bits)
        a = sum((bits.get(q, 0) & 1) << i for i, q in enumerate(addr))
        outd[out] = bits.get(out, 0) ^ table[a]
        return outd

    return TaskSpec(num_qubits=nqubits, inputs=tuple(addr), outputs=(out,),
                    ancillas=tuple(temps), target_perm=_perm)
