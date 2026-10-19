"""Completeness audit: alloc lifecycle, record enumeration, strict inputs.

Every gap found in the line-by-line audit gets a pinning test here —
no gap is closed by prose alone.
"""

import pytest

from kubona.codesign import Builder, KickmixEvaluator, TaskSpec
from kubona.codesign.harness import artifact_from_builder


def _identity_ancilla_spec():
    return TaskSpec(num_qubits=3, inputs=(0,), outputs=(0,), ancillas=(2,),
                    target_perm=lambda bits: dict(bits))


def test_alloc_compute_uncompute_free_valid():
    spec = _identity_ancilla_spec()
    b = Builder(3)
    b.alloc(2)
    b.cx(0, 2)
    b.cx(0, 2)
    b.free(2)
    res = KickmixEvaluator().evaluate(b, spec, num_tests=2, seed=7)
    assert res["valid"] is True


def test_alloc_leak_refused():
    spec = _identity_ancilla_spec()
    b = Builder(3)
    b.alloc(2)
    b.cx(0, 2)
    b.cx(0, 2)
    # no free, no measure: leaked alloc refuses even though bits are clean
    res = KickmixEvaluator().evaluate(b, spec, num_tests=2, seed=7)
    assert res["valid"] is False and "never freed" in res["reason"]


def test_dirty_free_refused():
    spec = _identity_ancilla_spec()
    b = Builder(3)
    b.alloc(2)
    b.cx(0, 2)  # ancilla now holds input; freeing here is uncompute fraud
    b.free(2)
    res = KickmixEvaluator().evaluate(b, spec, num_tests=4, seed=7)
    assert res["valid"] is False


def test_realloc_after_free_fresh_zero():
    # free resets bookkeeping; re-alloc must see |0>, never stale bits.
    spec = _identity_ancilla_spec()
    b = Builder(3)
    b.alloc(2)
    b.free(2)
    b.alloc(2)
    b.free(2)
    res = KickmixEvaluator().evaluate(b, spec, num_tests=2, seed=7)
    assert res["valid"] is True


def test_hmr_releases_alloc_no_free_needed():
    spec = TaskSpec(num_qubits=3, inputs=(0, 1), outputs=(),
                    target_perm=lambda bits: dict(bits),
                    target_phase=lambda bits: 0)
    b = Builder(3)
    b.alloc(2)
    b.ccx(0, 1, 2)
    r = b.hmr(2)
    b.cz(0, 1, cond=(r,))
    assert b.leaked_allocs() == set()
    res = KickmixEvaluator().evaluate(b, spec, num_tests=8, seed=11)
    assert res["valid"] is True


def test_record_enumeration_deterministic_kill():
    # No-correction MBU must fail on EVERY seed (both records enumerated),
    # not merely with high probability under sampling.
    spec = TaskSpec(num_qubits=3, inputs=(0, 1), outputs=(),
                    target_perm=lambda bits: dict(bits),
                    target_phase=lambda bits: 0)
    for seed in (1, 7, 11, 99):
        b = Builder(3)
        b.ccx(0, 1, 2)
        b.hmr(2)
        res = KickmixEvaluator().evaluate(b, spec, num_tests=8, seed=seed)
        assert res["valid"] is False, f"seed {seed} falsely passed"
        assert res["tests"] == 8  # 4 inputs x 2 records, fully enumerated


def test_artifact_round_trip_with_alloc():
    spec = _identity_ancilla_spec()
    b = Builder(3)
    b.alloc(2)
    b.cx(0, 2)
    b.cx(0, 2)
    b.free(2)
    art = artifact_from_builder(b)
    assert any(t[0] == "alloc_marker" for t in art)
    from kubona.codesign import CircuitCodesignHarness
    h = CircuitCodesignHarness(spec, num_tests=2)
    assert h.run(art)["valid"] is True


def test_double_alloc_and_free_after_consume_refuse():
    b = Builder(3)
    b.alloc(2)
    with pytest.raises(ValueError):
        b.alloc(2)
    c = Builder(3)
    c.alloc(2)
    c.hmr(2)
    with pytest.raises(ValueError):
        c.free(2)


def test_api_strict_ints_refuse_floats():
    import os
    import tempfile
    os.environ.setdefault("KUBONA_STATE_DIR", tempfile.mkdtemp(prefix="kubona-audit-"))
    from starlette.testclient import TestClient
    from kubona.console_api import app
    client = TestClient(app)
    h = {"Authorization": "Bearer dev-token"}
    r = client.post("/api/codesign/evaluate",
                    json={"spec_kind": "and2", "artifact": [], "kappa": 1.5}, headers=h)
    assert r.status_code == 400
    r = client.post("/api/codesign/evaluate",
                    json={"spec_kind": "and2", "artifact": [], "num_tests": 8.5}, headers=h)
    assert r.status_code == 400


def test_nvidia_base_defaults_to_nim_not_localhost():
    from kubona.llm.factory import NIM_DEFAULT_BASE_URL, get_llm
    llm = get_llm({"llm_provider": "nvidia", "llm_model": "test-model",
                   "llm_api_key": "test-key"})
    assert llm.base_url == NIM_DEFAULT_BASE_URL
    assert "127.0.0.1" not in llm.base_url


def test_g0_window_refuses_silent_clip():
    from kubona.codesign import fit_quadratic_integer
    xs = list(range(10))
    ys = [100 * x for x in xs]  # spread 0..900 needs 10 bits, not 2
    with pytest.raises(ValueError, match="widen b0"):
        fit_quadratic_integer(xs, ys, (2, 4, 4))


def test_package_exports_unique_and_importable():
    # Guards the recurring slip of editing __all__ without the import
    # (collection then fails) and duplicate entries (silent).
    import kubona.codesign as pkg
    assert len(pkg.__all__) == len(set(pkg.__all__))
    for name in pkg.__all__:
        assert getattr(pkg, name, None) is not None, name


def test_mbu_forward_reverse_cnot_clear_cycle():
    # SI II.1 "Branch decoders" prose as a kickmix program: forward step
    # copies d->b then disposes b by MBU (phase pending); reverse re-derives
    # b2 from retained d, applies the conditioned correction, uses b2, and
    # clears it with a CNOT from the restored data bit (no history stored).
    from kubona.codesign import Builder, KickmixEvaluator, TaskSpec
    import random
    spec = TaskSpec(3, (0,), (0,),
                    target_perm=lambda bits: dict(bits),
                    target_phase=lambda bits: 0)
    b = Builder(3)
    d, bb, b2 = 0, 1, 2
    b.alloc(bb)
    b.alloc(b2)
    b.cx(d, bb)     # b = copy of d
    r = b.hmr(bb)   # forward dispose by MBU (releases bb; phase pending)
    b.z(d, cond=(r,))  # conditioned correction cancels (-1)^{r*d}
    b.cx(d, b2)     # reverse: re-derive b2 from retained state
    b.cx(d, b2)     # use-then-clear: CNOT from restored data bit
    b.free(b2)      # bb needs no free (measurement released it)
    res = KickmixEvaluator().evaluate(b, spec, num_tests=4, seed=7)
    assert res["valid"] is True and res["tests"] == 4
    # without the correction the pending phase fails (load-bearing proof)
    c = Builder(3)
    c.alloc(bb)
    c.alloc(b2)
    c.cx(d, bb)
    c.hmr(bb)
    c.cx(d, b2)
    c.cx(d, b2)
    c.free(b2)
    bad = KickmixEvaluator().evaluate(c, spec, num_tests=8, seed=7)
    assert bad["valid"] is False


def test_truncated_carry_bound_exact():
    # Paper §II.1 sparse-prime pattern at toy scale: adding constant C with
    # a k-bit carry window escapes with EXACT probability C/2^k (measured
    # == analytic as Fractions, not sampled approximations).
    from fractions import Fraction
    from kubona.codesign import compare_top_wrong, escape_bound, escape_probability
    assert escape_probability(12, 9, 6) == escape_bound(9, 6) == Fraction(9, 64)
    assert escape_probability(10, 1, 9) == Fraction(1, 512)
    # paper shape: (2^32+977)/2^58 ~= 2^-26
    assert abs(float(escape_bound(2**32 + 977, 58)) - 2**-26) < 2**-28
    hits = sum(1 for a in range(64) for b in range(64)
               if compare_top_wrong(a, b, 6, 3))
    assert Fraction(hits, 64 * 64) == Fraction(1, 8)
    with pytest.raises(ValueError):
        escape_bound(200, 3)  # C > 2^k: bound form invalid, refuse


def test_pruned_qrom_combinatorics():
    # SI III.3 pattern at toy scale: cost = distinct subtrees, never more
    # than the naive tree; uniform tables collapse to k+1 nodes.
    from kubona.codesign import qrom
    assert qrom.naive_nodes(1024) == 2047
    assert qrom.distinct_subtrees(["z"] * 8) == 1  # one value: no bit matters
    assert qrom.distinct_subtrees(["a", "b"]) == 3  # no sharing possible
    full = qrom.gaussian_regimes()
    assert len(full) == 1024
    pruned = qrom.distinct_subtrees(full)
    assert pruned < qrom.naive_nodes(1024)
    with pytest.raises(ValueError):
        qrom.distinct_subtrees(["a", "b", "c"])
    with pytest.raises(ValueError):
        qrom.naive_nodes(7)


def test_alias_row_runs_prune_top_bit():
    # Paper SI IV: eight-row A,A,B,B,A,A,B,B has two identical halves, so
    # the top address bit is irrelevant (look up only A,A,B,B). This is the
    # distinct-subtree rule applied to scheduled alias words.
    from kubona.codesign import qrom
    rows = ["A", "A", "B", "B", "A", "A", "B", "B"]
    assert qrom.distinct_subtrees(rows) == qrom.distinct_subtrees(["A", "A", "B", "B"])


def test_readout_vs_qrom_paper_direction():
    # SI V anchor: 216-entry QROM path 478T vs direct readout 76/75T.
    # The model reproduces DIRECTION + order with stated units (its flags
    # ignore shared substructure, so it overestimates readout honestly).
    from kubona.codesign import qrom
    rep = qrom.qrom_vs_direct(216, 7, 51, addr_bits=9)
    assert rep == {"qrom": 483, "direct": 105, "winner": "direct",
                   "units": rep["units"]}
    wide = qrom.qrom_vs_direct(1024, 1024, 8, addr_bits=10)
    assert wide["winner"] == "qrom"  # full range, wide address: lookup wins
    with pytest.raises(ValueError):
        qrom.qrom_vs_direct(216, 300, 51)


def test_all_public_submodule_names_exported():
    # Every public module-level name in codesign/*.py must be importable
    # from kubona.codesign (the qrom_vs_direct miss: used via module attr
    # while absent top-level — inconsistent API surface).
    import inspect
    import pkgutil
    import kubona.codesign as pkg
    import kubona.codesign as root
    missing = []
    for mod in pkgutil.iter_modules(root.__path__):
        m = __import__(f"kubona.codesign.{mod.name}", fromlist=["x"])
        for name, obj in vars(m).items():
            if name.startswith("_") or not callable(obj):
                continue
            if getattr(obj, "__module__", "") != m.__name__:
                continue  # re-exported import, owned elsewhere
            if name not in pkg.__all__:
                missing.append(f"{mod.name}.{name}")
    assert missing == [], missing
