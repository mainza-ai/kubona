"""Circuit-cutting tests: the wire cut must reproduce the uncut expectation."""

from kubona.circuits import Circuit, Gate
from kubona.cutting import cut_expectation
from kubona.providers import get_provider

SIM = get_provider("local-simulator")


def _backend(c, shots):
    return SIM.submit(c, shots, seed=7)


def _uncut_z1(gates, shots=120000):
    return _backend(Circuit("u", 2, gates), shots)["counts"]


def _z1(counts):
    total = sum(counts.values()) or 1
    return sum(v * (1.0 if k[1] == "0" else -1.0) for k, v in counts.items()) / total


def test_z_cut_matches_uncut_asym():
    prep = [Gate("ry", (0,), (0.6,))]
    cut = cut_expectation(Circuit("c", 2, prep + [Gate("cx", (0, 1))]), 1, 1, _backend, shots=120000)
    uncut = _z1(_uncut_z1(prep + [Gate("cx", (0, 1))]))
    assert abs(cut["expected"] - uncut) < 0.02, (cut["expected"], uncut)


def test_z_cut_with_post_cut_target_gate():
    prep = [Gate("ry", (0,), (0.6,))]
    after = [Gate("ry", (1,), (0.8,))]
    cut = cut_expectation(Circuit("c", 2, prep + [Gate("cx", (0, 1))] + after), 1, 1, _backend, shots=120000)
    uncut = _z1(_uncut_z1(prep + [Gate("cx", (0, 1))] + after))
    assert abs(cut["expected"] - uncut) < 0.02, (cut["expected"], uncut)


def test_z_cut_bell_is_zero():
    cut = cut_expectation(Circuit("c", 2, [Gate("h", (0,)), Gate("cx", (0, 1))]), 1, 1, _backend, shots=120000)
    assert abs(cut["expected"]) < 0.02


def test_fragments_are_smaller():
    """The point of cutting: each fragment involves one active qubit."""
    plan = __import__("kubona.cutting", fromlist=["plan_cut"]).plan_cut(
        Circuit("c", 2, [Gate("h", (0,)), Gate("cx", (0, 1))]), 1)
    assert plan.subcircuit_widths() == (1, 1)


def test_requires_cx_at_index():
    import pytest
    from kubona.cutting.wire import plan_cut
    with pytest.raises(ValueError):
        plan_cut(Circuit("c", 2, [Gate("h", (0,)), Gate("cx", (0, 1))]), 0)  # gate 0 is h
