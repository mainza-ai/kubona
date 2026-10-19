"""Slice 6: divstep decoder family + jstore frontier (paper-anchored)."""

import pytest

from kubona.codesign import (
    balance,
    check_term_counts,
    decoder_terms,
    depth2_corollary_a8,
    eval_decoder,
    jstore_tradeoff,
    reported_history_qubits,
    stored_share,
)


def test_term_counts_reproduce_paper():
    assert check_term_counts() == {2: 3, 3: 3, 4: 11, 5: 26,
                                   6: 79, 7: 268, 8: 1092}


def test_decoders_balanced_deciding_bits():
    for d in (2, 3, 4, 5, 6, 7, 8):
        assert balance(d) == pytest.approx(0.5, abs=1e-9)


def test_depth2_forms_agree_on_wires():
    # 56a: 1^f1^q1 and Cor A8: g1^f3 are distinct presentations; both are
    # balanced single-bit rules over their own wires (no cross-identity claimed).
    assert eval_decoder(2, {"f1": 0, "q1": 0}) == 1
    assert eval_decoder(2, {"f1": 1, "q1": 1}) == 1
    assert depth2_corollary_a8(0, 0) == 0
    assert depth2_corollary_a8(1, 0) == 1
    with pytest.raises(ValueError):
        eval_decoder(2, {"f1": 2, "q1": 0})
    with pytest.raises(ValueError):
        eval_decoder(6, {})


def test_jstore_tradeoff_paper_anchors():
    assert stored_share(0) == 1.0
    assert stored_share(5) == pytest.approx(0.124)
    assert decoder_terms(5) == 3 + 3 + 11
    assert decoder_terms(0) == 0
    assert reported_history_qubits(0) == 198
    assert reported_history_qubits(5) == 60
    assert reported_history_qubits(9) == 24
    row = jstore_tradeoff(5)
    assert row == {"jstore": 5, "stored_share": pytest.approx(0.124),
                   "decoder_terms": 17, "history_qubits": 60}


def test_unmeasured_cells_refuse_not_invent():
    with pytest.raises(ValueError, match="unmeasured"):
        stored_share(7)
    with pytest.raises(ValueError, match="refusing interpolation"):
        reported_history_qubits(4)
    row = jstore_tradeoff(7)
    assert row["stored_share"] is None and row["history_qubits"] is None
    assert row["decoder_terms"] == 3 + 3 + 11 + 26 + 79  # depths 2..6 below cutoff 7
