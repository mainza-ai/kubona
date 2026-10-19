"""Divstep branch-decoder family (paper SI II.1, Eqs 56a-d + Cor A8).

The depth-j ambiguous-decision bit is DERIVED here from the generic decoding
rule (Theorem A11 second form: b = bit_{j-1}(p(f'q)^{-1} mod 2^j)) evaluated
over all 2^{2(j-1)} assignments and converted to ANF by Mobius transform —
one source of truth, not a transcription. The derived polynomials reproduce
the paper's Eqs 56a-d term-for-term at depths 2-5 (machine-checked equality
against the equations' text, then the transcription deleted) and its
term counts 79/268/1092 at depths 6-8 (previously anchors, now derived).
The depth distribution (50.1 / 25.0 / 12.5 / 12.4%) stays a paper-measured
anchor — sampling data, never re-derived here.

Variable convention (SI II.1): fi = f'[i], qi = q[i] = f'_v[j+i] for
1 <= i < j, where f' is odd and q the odd part of f'_v / 2^j.
"""

from __future__ import annotations

import itertools

from .divwalk import bit, modinv_odd

#: The paper's 256-bit prime (single home: decoders + sparse-fold import it).
PAPER_P256 = 2**256 - 2**32 - 977

# ---------------------------------------------------------------------------
# GF(2) polynomials as XOR-sums of AND-products: frozenset[frozenset[str]].
# The empty inner frozenset is the constant 1. This is exact Boolean
# algebra (ANF), not a heuristic normalizer.
# ---------------------------------------------------------------------------

Poly = frozenset  # frozenset[frozenset[str]]

_derived_cache: dict[tuple[int, int], Poly] = {}


def decoder_poly_derived(depth: int, p: int = PAPER_P256) -> Poly:
    """ANF of the depth-j rule derived from the generic theorem + prime p.

    b = bit_{j-1}(p (f'q)^{-1} mod 2^j) over f_i = f'[i], q_i = q[i] for
    1 <= i < j (f', q odd). Exhaustive over assignments, Mobius to ANF.
    Depths 2..8 (fail-closed outside: 2^{2(j-1)} assignments go exponential,
    and the paper needs only these).
    """
    if isinstance(depth, bool) or not isinstance(depth, int) or not (2 <= depth <= 8):
        raise ValueError(f"depth must be in [2,8], got {depth!r}: refusing")
    if not isinstance(p, int) or p < 3 or p % 2 == 0:
        raise ValueError(f"p must be an odd prime >= 3, got {p!r}: refusing")
    key = (depth, p)
    if key in _derived_cache:
        return _derived_cache[key]
    j = depth
    n = 2 * (j - 1)
    N = 1 << n
    M = 1 << j
    pm = p % M
    names = [f"f{i + 1}" for i in range(j - 1)] + [f"q{i + 1}" for i in range(j - 1)]
    vals = bytearray(N)
    for mask in range(N):
        fp = 1
        qp = 1
        for i in range(j - 1):
            if (mask >> i) & 1:
                fp |= 1 << (i + 1)
            if (mask >> (j - 1 + i)) & 1:
                qp |= 1 << (i + 1)
        vals[mask] = bit((pm * modinv_odd((fp * qp) % M, M)) % M, j - 1)
    f = list(vals)
    for i in range(n):
        st = 1 << i
        for m in range(N):
            if m & st:
                f[m] ^= f[m ^ st]
    out = frozenset(
        frozenset(names[i] for i in range(n) if (m >> i) & 1)
        for m in range(N) if f[m])
    _derived_cache[key] = out
    return out


def decoder_poly(depth: int) -> Poly:
    """GF(2) polynomial for the depth-j ambiguous-decision bit (paper Eqs 56a-d).

    Derived from the generic theorem at the paper prime (see
    decoder_poly_derived) — verified term-for-term against the equations.
    """
    return decoder_poly_derived(depth)


def term_count(depth: int) -> int:
    """Sum-of-products term count (paper: 3,3,11,26,79,268,1092 — all derived)."""
    return len(decoder_poly(depth))


def eval_decoder(depth: int, bits: dict[str, int], p: int = PAPER_P256) -> int:
    """Evaluate the depth-j rule on a {var: 0/1} assignment (prime p)."""
    poly = decoder_poly_derived(depth, p=p)
    out = 0
    for term in poly:
        prod = 1
        for v in term:
            bv = bits.get(v)
            if bv not in (0, 1):
                raise ValueError(f"bit {v!r} must be 0/1, got {bv!r}: refusing")
            prod &= bv
        out ^= prod
    return out


def depth2_corollary_a8(g1: int, f3: int) -> int:
    """Cor A8 form: b = g'_v[1] XOR f'_v[3] (equivalent presentation of j=2).

    Paper notes: the A14 form costs TWO Toffolis; at depths 3-4 the rule is
    an XOR of q-bits with no products once f' mod 16 is fixed; products of
    q-bits appear only from depth 5 on. Every reverse step carries every
    decoder below jstore (conditioned on a depth flag, >=1T per read term).
    """
    if g1 not in (0, 1) or f3 not in (0, 1):
        raise ValueError("bits must be 0/1: refusing")
    return g1 ^ f3


# ---------------------------------------------------------------------------
# Anchors: TERMS is derived above (len of each derived polynomial — the
# paper's counts, reproduced, never cited); SHARES/HISTORY stay paper-
# measured (sampling data, never re-derived here).
# ---------------------------------------------------------------------------

#: Sum-of-products term counts per excursion depth (derived, paper-matching).
TERMS: dict[int, int] = {2: 3, 3: 3, 4: 11, 5: 26, 6: 79, 7: 268, 8: 1092}

#: Share of ambiguous decisions per depth over sampled walks (paper-measured).
SHARES: dict[str, float] = {"d2": 0.501, "d3": 0.250, "d4": 0.125, "d5plus": 0.124}

#: History-stack sizes for the 533-step schedule at cutoffs (paper-measured).
HISTORY_QUBITS: dict[int, int] = {0: 198, 5: 60, 9: 24}


def stored_share(jstore: int) -> float:
    """Fraction of ambiguous decisions written to the stack at cutoff jstore.

    Exact for jstore <= 5 from the measured depth split. For jstore >= 6
    the split *within* the d5+ tail is unmeasured — refusing rather than
    presenting the 12.4% upper bound as an exact share.
    """
    if not isinstance(jstore, int) or isinstance(jstore, bool) or jstore < 0:
        raise ValueError(f"jstore must be non-negative int, got {jstore!r}: refusing")
    if jstore <= 2:
        return 1.0
    if jstore == 3:
        return 1.0 - SHARES["d2"]
    if jstore == 4:
        return 1.0 - SHARES["d2"] - SHARES["d3"]
    if jstore == 5:
        return SHARES["d5plus"]
    raise ValueError(
        f"stored share unmeasured for jstore={jstore} (d5+ tail split unknown): refusing")


def decoder_terms(jstore: int) -> int:
    """Total sum-of-products terms across decoders for depths 2..jstore-1.

    Every reverse step carries every decoder below the cutoff, so cost adds.
    """
    if not isinstance(jstore, int) or isinstance(jstore, bool) or jstore < 0:
        raise ValueError(f"jstore must be non-negative int, got {jstore!r}: refusing")
    return sum(TERMS[d] for d in range(2, min(jstore, 9)) if d in TERMS)


def reported_history_qubits(jstore: int) -> int:
    """Paper-measured stack size for jstore in {0, 5, 9}. Others refuse.

    No interpolation: a mid-cutoff qubit count is a measurement, not math.
    """
    if jstore not in HISTORY_QUBITS:
        raise ValueError(
            f"history qubits measured only for jstore in {sorted(HISTORY_QUBITS)} "
            f"(got {jstore!r}): refusing interpolation")
    return HISTORY_QUBITS[jstore]


def jstore_tradeoff(jstore: int) -> dict:
    """One-row frontier point: stored share + decoder terms + measured stack.

    Unknown cells read None (unmeasured), never an interpolated guess.
    """
    try:
        share: float | None = stored_share(jstore)
    except ValueError:
        share = None
    return {
        "jstore": jstore,
        "stored_share": share,
        "decoder_terms": decoder_terms(jstore),
        "history_qubits": HISTORY_QUBITS.get(jstore),
    }


def check_term_counts() -> dict[int, int]:
    """Verify derived polynomials reproduce all eight paper counts."""
    return {d: term_count(d) for d in (2, 3, 4, 5, 6, 7, 8)}


def balance(depth: int) -> float:
    """Fraction of assignments evaluating to 1 (a deciding bit should be ~1/2)."""
    poly = decoder_poly(depth)
    varnames = sorted({v for term in poly for v in term}) or ["f1"]
    n = len(varnames)
    ones = 0
    for mask in range(1 << n):
        bits = {v: (mask >> i) & 1 for i, v in enumerate(varnames)}
        ones += eval_decoder(depth, bits)
    return ones / (1 << n)


def _decoder_layout(depth: int) -> tuple[list[str], int, list[int]]:
    """Single home for decoder-circuit layout (builder + spec share it).

    Returns (var names in qubit order, target b id, temp ids). Depth scope
    2-8 (exhaustive-proof range: 4 to 16k inputs) is enforced here, once.
    Depth 9+ refuses: 2^{2(j-1)} inputs exceed exhaustive-proof scope
    (the polynomials still verify symbolically via term counts).
    """
    if isinstance(depth, bool) or not isinstance(depth, int) or not (2 <= depth <= 8):
        raise ValueError(f"circuit scope is depths 2-8 (exhaustive), got {depth!r}: refusing")
    j = depth
    names = [f"f{i + 1}" for i in range(j - 1)] + [f"q{i + 1}" for i in range(j - 1)]
    b = len(names)
    maxextra = 0
    for term in decoder_poly(depth):
        maxextra = max(maxextra, len(term) - 1)
    return names, b, list(range(b + 1, b + 1 + max(0, maxextra)))


def _minimized_decoder_cover(depth: int) -> tuple[list[str], int, list[tuple[int, int]], int]:
    """Single home for the minimized decoder cover (both emitters share it).

    Returns (input names, target b, reduced (value, care) cubes, raw const
    term count). Terms are canonically sorted before merging (frozenset
    iteration order is hash-randomized, so an unsorted input would pin a
    lottery, not a number). The SOP and factored builders consume the
    identical cover — their T-counts compare the emission discipline,
    never different logic.
    """
    from .minimize import reduce_anf
    names, b, _temps = _decoder_layout(depth)
    n_in = len(names)
    qid = {name: i for i, name in enumerate(names)}
    const = 0
    cubes = []
    for term in decoder_poly(depth):
        if len(term) == 0:
            const += 1
            continue
        m = 0
        for v in term:
            m |= 1 << qid[v]
        cubes.append((m, m))
    cubes = sorted(reduce_anf(sorted(cubes), n_in))
    return names, b, cubes, const


def build_decoder_circuit_minimized(depth: int) -> tuple[object, dict]:
    """Branch-decoder rule, ANF-reduced (exact structural merging).

    Same rule, same spec, fewer cubes: the ANF terms are merged by
    adjacent-combination (same-care single-bit) and cross-care absorption
    — exact XOR identities, never worse than the ANF (from-scratch SOP
    search loses badly on these XOR-dense rules; see slice-42 log).
    Terms are canonically sorted before merging (frozenset iteration
    order is hash-randomized, so an unsorted input would pin a lottery,
    not a number). Proven exhaustive downstream under the same spec as
    the naive circuit.
    """
    from .kickmix import Builder, emit_xor_cover
    names, b, cubes, const = _minimized_decoder_cover(depth)
    n_in = len(names)
    maxextra = 0
    for (_v, care) in cubes:
        maxextra = max(maxextra, bin(care).count("1") - 1)
    temps = list(range(n_in + 1, n_in + 1 + max(0, maxextra)))
    prog = Builder(n_in + 1 + len(temps))
    n_ccx = [0]
    for q in temps:
        prog.alloc(q)
    for _ in range(const % 2):
        prog.x(b)
    for (value, care) in cubes:
        lits = [(i, (value >> i) & 1) for i in range(n_in) if (care >> i) & 1]
        emit_xor_cover(prog, n_ccx, [lits], temps, b)
    for q in temps:
        prog.free(q)
    return prog, {"inputs": tuple(range(n_in)), "b": b, "temps": temps,
                  "ccx": n_ccx[0], "terms": len(cubes) + const,
                  "kind": "minimized-anf"}


def build_decoder_circuit_factored(depth: int) -> tuple[object, dict]:
    """Branch-decoder rule from the factored AND-network (multi-level).

    The identical minimized cover as build_decoder_circuit_minimized
    (same helper, same const parity), but shared AND-subproducts across
    all cubes are computed once (greedy CSE in minimize.factor_and_network)
    instead of recomputing every cube's AND-chain. Same spec, proven
    exhaustive downstream; structural cost 2T per live node. Proven
    against decoder_circuit_spec: the spec sizes the SOP-temp interface
    while every factored temp's free_marker is dirty-checked in
    simulation, so extra temps cannot escape verification. Depth scope
    2-8 via _decoder_layout (9+ refuses: beyond exhaustive proof).
    """
    from .kickmix import Builder
    from .minimize import emit_factored, factor_and_network
    names, b, cubes, const = _minimized_decoder_cover(depth)
    n_in = len(names)
    cover = [cubes + ([(0, 0)] if const % 2 else [])]
    net = factor_and_network(cover, n_in)
    nnodes = len(net["nodes"])
    temps = list(range(n_in + 1, n_in + 1 + nnodes))
    prog = Builder(n_in + 1 + nnodes)
    n_ccx = [0]
    for q in temps:
        prog.alloc(q)
    emit_factored(prog, n_ccx, net, temps, [b])
    for q in temps:
        prog.free(q)
    return prog, {"inputs": tuple(range(n_in)), "b": b, "temps": temps,
                  "ccx": n_ccx[0], "terms": len(cubes) + const,
                  "nodes": nnodes, "moves": net["moves"],
                  "kind": "factored-anf"}


def build_decoder_circuit(depth: int) -> tuple[object, dict]:
    """Branch-decoder rule as a kickmix circuit (proven exhaustive ≤ depth 8).

    Each ANF product term becomes an AND-chain into temps (one temp per
    extra literal, sequential reuse across terms), XORed into the fresh
    target b, chains uncomputed in reverse. Single literals are CNOTs, the
    constant term an X. Inputs preserved (controls only); b accumulates;
    every temp freed (dirty-free raises otherwise). Depths 9+ refuse:
    2^{2(j-1)} inputs exceed exhaustive-proof scope (the polynomials still
    verify symbolically via term counts).
    """
    from .kickmix import Builder, emit_xor_cover
    names, b, temps = _decoder_layout(depth)
    n_in = len(names)
    qid = {name: i for i, name in enumerate(names)}
    const = False
    terms: list[list[int]] = []
    for term in decoder_poly(depth):
        if len(term) == 0:
            const = True
            continue
        terms.append(sorted(qid[v] for v in term))
    prog = Builder(n_in + 1 + len(temps))
    n_ccx = [0]
    for q in temps:
        prog.alloc(q)
    if const:
        prog.x(b)
    for lits in terms:
        emit_xor_cover(prog, n_ccx, [[(q, 1) for q in lits]], temps, b)
    for q in temps:
        prog.free(q)
    return prog, {"inputs": tuple(range(n_in)), "b": b, "temps": temps,
                  "ccx": n_ccx[0], "terms": len(terms) + (1 if const else 0)}


def decoder_circuit_spec(depth: int):
    """TaskSpec: fresh b gets the depth-j rule at the input bits."""
    from .kickmix import TaskSpec
    names, b, temps = _decoder_layout(depth)

    def _perm(bits: dict[int, int]) -> dict[int, int]:
        out = dict(bits)
        out[b] = eval_decoder(depth, {name: bits.get(i, 0) & 1
                                      for i, name in enumerate(names)})
        return out

    n_in = len(names)
    return TaskSpec(num_qubits=n_in + 1 + len(temps), inputs=tuple(range(n_in)),
                    outputs=(b,), ancillas=tuple(temps), target_perm=_perm)
