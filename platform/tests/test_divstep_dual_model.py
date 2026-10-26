"""Tests for Divstep Register Allocation Dual-Model (B.4)."""

import pytest
from kubona.codesign.packing import plan_dual_model_packing, walk_widths
from kubona.codesign.divsteps import PAPER_P256


def test_divstep_dual_model_safe_and_paper():
    # Small toy walk
    w = 123456789
    p = 2**31 - 1  # Mersenne 31
    trace = walk_widths(p, w)

    # 1. safe_288 mode guarantees fit
    safe_rep = plan_dual_model_packing(trace, mode="safe_288")
    assert safe_rep["mode"] == "safe_288"
    assert safe_rep["fits"] is True
    assert safe_rep["zero_overflow_guarantee"] is True

    # 2. paper_256 mode
    paper_rep = plan_dual_model_packing(trace, mode="paper_256")
    assert paper_rep["mode"] == "paper_256"
    assert "tail_overflow_tripped" in paper_rep

    # 3. Synthetic excursion exceeding 256 bits
    deep_excursion_trace = [{"wg": 140, "wf": 140, "wfv": 130, "wgv": 130}]  # 140 + 130 = 270 bits
    deep_safe = plan_dual_model_packing(deep_excursion_trace, mode="safe_288")
    assert deep_safe["fits"] is True  # 270 <= 288

    deep_paper = plan_dual_model_packing(deep_excursion_trace, mode="paper_256")
    assert deep_paper["fits"] is False
    assert deep_paper["tail_overflow_tripped"] is True
    assert deep_paper["violator"]["step"] == 0
