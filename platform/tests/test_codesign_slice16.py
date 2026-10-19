"""Slice 16: F2^12 + BM-70, n-bit freeze machinery, scheduled Lanczos, API depth.

Production bar: paper-scale behavior proven at paper scale (70 rounds over
F2^12, depth-3 scheduled repairs), refusals pinned, no invented constants.
"""

import pytest

from kubona.codesign import (
    F2_12,
    KickmixEvaluator,
    ScheduledWalker,
    apply_frame,
    apply_perm,
    berlekamp_massey,
    build_index_match,
    build_zero_flag_n,
    cnot_frame,
    find_blocks,
    find_blocks_symplectic,
    hadamard_frame,
    index_match_spec,
    locator_roots,
    matvec,
    multi_error_syndromes,
    perm_frame,
    scheduled_find_null,
    zero_flag_spec_n,
)


def test_f2_12_field_axioms():
    assert F2_12.size == 4096 and F2_12.n == 12
    assert F2_12.modulus == 0x1053
    for x in (1, 2, 100, 4095):
        assert F2_12.mul(x, F2_12.inv(x)) == 1
    with pytest.raises(ValueError):
        F2_12.inv(0)


def test_bm_70_single_error_paper_scale():
    syn = multi_error_syndromes(F2_12, 2, [(171, 123)], 70)
    assert len(syn) == 70
    rep = berlekamp_massey(syn, F2_12)
    assert len(rep["sigma"]) - 1 == 1
    assert locator_roots(rep["sigma"], F2_12, 2) == [171]
    assert rep["invariant_holds"] is True
    # adaptive profile: measured flat far below the min(r,35) worst bound
    assert rep["active_widths"][-1] == 2
    assert rep["worst_widths"][-1] == 36
    assert all(a <= w for a, w in zip(rep["active_widths"], rep["worst_widths"]))
    assert rep["stack_needed"] == 0


def test_bm_70_three_errors_decode_and_history():
    errs = [(5, 7), (400, 9), (2701, 11)]
    rep = berlekamp_massey(multi_error_syndromes(F2_12, 2, errs, 70), F2_12)
    assert len(rep["sigma"]) - 1 == 3
    assert sorted(locator_roots(rep["sigma"], F2_12, 2)) == [5, 400, 2701]
    assert rep["active_widths"][-1] <= 6  # typical << worst-case 36
    assert rep["stack_needed"] == rep["n_retain"]
    with pytest.raises(ValueError):
        berlekamp_massey([0] * 81, F2_12)  # round cap refuses


def test_nbit_freeze_and_index_match_proven():
    for n, t in ((2, 1), (3, 3), (4, 5)):
        (bld, regs), spec = build_zero_flag_n(n), zero_flag_spec_n(n)
        assert regs["ccx"] == t == 2 * n - 3
        res = KickmixEvaluator().evaluate(bld, spec, num_tests=1 << n, seed=7)
        assert res["valid"] is True and res["tests"] == 1 << n
    (bld, regs), spec = build_index_match(3, 5), index_match_spec(3, 5)
    res = KickmixEvaluator().evaluate(bld, spec, num_tests=8, seed=7)
    assert res["valid"] is True and res["tests"] == 8
    # a wrong constant on the same circuit is detected, not absorbed
    bad = KickmixEvaluator().evaluate(bld, index_match_spec(3, 4), num_tests=8, seed=7)
    assert bad["valid"] is False
    with pytest.raises(ValueError):
        build_zero_flag_n(7)  # outside exhaustive-proof scope
    with pytest.raises(ValueError):
        build_index_match(3, 8)  # const overflows width


def test_scheduled_lanczos_freeze_carry_depth3_and_reverse():
    # rowsB: the full paper pattern — freeze, depth-3 repair, freeze-carry,
    # depth-2 repair, dependence (decisions pinned exactly).
    rows = [0xD, 0xC, 0x1F, 0xF, 0x34, 0x30]
    sched = {2, 5, 8}
    w = ScheduledWalker(rows, 6, sched)
    assert w.run(1) == "dependent"
    assert w.repairs == 2 and w.maxdepth == 3 and w.frozen_rounds == 3
    assert [d["note"] for d in w.decisions] == [
        "ordinary", "frozen", "repaired", "frozen", "frozen", "repaired", "dependent"]
    assert [d["depth"] for d in w.decisions] == [1, 0, 3, 0, 0, 2, 0]
    rev = w.reverse()
    assert rev["history_bits"] == len(w.decisions) == 7 and rev["recomputed"] is True
    # the history bit is load-bearing: flipping one refuses reversal
    w.decisions[1] = dict(w.decisions[1], advanced=True, depth=1)
    with pytest.raises(ValueError):
        w.reverse()
    # rowsA: same machinery, seed span carries a verified null (first seed)
    rowsA = [0x1D, 0x4, 0x3B, 0x35, 0xD, 0x2C]
    rep = scheduled_find_null(rowsA, 6, [1, 2, 4, 7], sched)
    assert rep["proven"] is True and rep["null"] == 43
    assert rep["maxdepth"] == 3 and rep["history_bits"] == 7 and rep["frozen_rounds"] == 2
    assert matvec(rowsA, rep["null"], 6) == 0


def test_scheduled_lanczos_honest_misses():
    nd = [0xD, 0x2A, 0x9, 0x1F, 0x28, 0x12]
    assert ScheduledWalker(nd, 6, {2, 5, 8}).run(1) == "needs_deeper"
    uf = [0x12, 0xD, 0x36, 0x2, 0x5, 0x4]
    assert ScheduledWalker(uf, 6, set(), 1).run(1) == "unfinished"
    rep = scheduled_find_null(nd, 6, [1], {2, 5, 8})
    assert rep["proven"] is False and rep["null"] is None
    with pytest.raises(ValueError):
        ScheduledWalker(nd, 6, "every-third")  # schedule shape refuses


def test_symplectic_frames_group_what_perms_cannot():
    mats = [perm_frame((1, 0)), hadamard_frame(2, 0), hadamard_frame(2, 1)]
    # perm embedding: symplectic perm acts exactly like the qubit perm
    for x in range(4):
        for z in range(4):
            assert apply_frame((x, z), perm_frame((1, 0))) == apply_perm((x, z), (1, 0))
    # H-frame pairs X with Z (phase-blind masks shown; perms preserve x/z weight)
    h = find_blocks_symplectic([(1, 0), (0, 1)], [hadamard_frame(1, 0)])
    assert h["blocks"][0]["size"] == 2 and h["closed"] is True
    # mixed X/Z orbit: one 4-block under perm+H, two 2-blocks under perms alone
    table = [(0, 1), (0, 2), (1, 0), (2, 0)]
    rep = find_blocks_symplectic(table, mats)
    assert len(rep["blocks"]) == 1
    blk = rep["blocks"][0]
    assert blk["size"] == 4 and blk["addr_bits"] == 2
    for m in blk["members"]:
        assert apply_frame(blk["rep"], blk["witnesses"][f"{m}"]) == m
    narrow = find_blocks(table, [(0, 1), (1, 0)])
    assert sorted(b["size"] for b in narrow["blocks"]) == [2, 2]
    # CNOT frame is a genuine symplectic frame (non-permutation action)
    assert apply_frame((0b01, 0), cnot_frame(2, 0, 1)) == (0b11, 0)
    with pytest.raises(ValueError):
        cnot_frame(2, 1, 1)  # control == target refuses
    with pytest.raises(ValueError):
        find_blocks_symplectic([], mats)


def test_api_reports_depth_triple():
    import os
    import tempfile
    os.environ.setdefault("KUBONA_STATE_DIR", tempfile.mkdtemp(prefix="kubona-s16-"))
    from starlette.testclient import TestClient
    from kubona.console_api import app
    from kubona.codesign import artifact_from_builder, build_routed_1bit_madd
    (bld, _), _spec = build_zero_flag_n(2), None
    client = TestClient(app)
    h = {"Authorization": "Bearer dev-token"}
    r = client.post("/api/codesign/evaluate",
                    json={"spec_kind": "and2", "artifact": artifact_from_builder(bld),
                          "num_tests": 4}, headers=h)
    assert r.status_code == 200
    res = r.json()["result"]
    assert (res["depth"], res["toffoli_depth"], res["active_qubits"]) == (3, 1, 3)
    art = artifact_from_builder(build_routed_1bit_madd()[0])
    # 5-qubit routed MADD needs its own truth table (4 inputs -> x target)
    table = {}
    for b in (0, 1):
        for c in (0, 1):
            for y in (0, 1):
                for z in (0, 1):
                    table[f"{b}{c}{y}{z}"] = {"out": {"4": c & (z if b else y)}}
    r = client.post("/api/codesign/frontier",
                    json={"spec_table": {"num_qubits": 5, "inputs": [0, 1, 2, 3],
                                         "outputs": [4], "table": table},
                          "artifacts": [art]}, headers=h)
    assert r.status_code == 200, r.json()
    det = r.json()["details"][0]
    assert det["valid"] is True
    assert (det["depth"], det["toffoli_depth"], det["active_qubits"]) == (7, 3, 5)
