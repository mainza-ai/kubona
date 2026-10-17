"""Circuit-cutting tests: the wire cut must reproduce the uncut expectation."""

from kubona.circuits import Circuit, Gate
from kubona.cutting import cut_expectation
from kubona.providers import get_provider

SIM = get_provider("local-simulator")


def _backend(c, shots, seed=7):
    return SIM.submit(c, shots, seed=seed)


def _uncut_z1(gates, shots=120000):
    return _backend(Circuit("u", 2, gates), shots)["counts"]


def _z1(counts):
    total = sum(counts.values()) or 1
    return sum(v * (1.0 if k[1] == "0" else -1.0) for k, v in counts.items()) / total


def test_z_cut_matches_uncut_asym():
    prep = [Gate("ry", (0,), (0.6,))]
    cut = cut_expectation(Circuit("c", 2, prep + [Gate("cx", (0, 1))]), 1, 1, _backend, shots=120000, seed=7)
    uncut = _z1(_uncut_z1(prep + [Gate("cx", (0, 1))]))
    assert abs(cut["expected"] - uncut) < 0.02, (cut["expected"], uncut)


def test_z_cut_with_post_cut_target_gate():
    prep = [Gate("ry", (0,), (0.6,))]
    after = [Gate("ry", (1,), (0.8,))]
    cut = cut_expectation(Circuit("c", 2, prep + [Gate("cx", (0, 1))] + after), 1, 1, _backend, shots=120000, seed=7)
    uncut = _z1(_uncut_z1(prep + [Gate("cx", (0, 1))] + after))
    assert abs(cut["expected"] - uncut) < 0.02, (cut["expected"], uncut)


def test_z_cut_bell_is_zero():
    cut = cut_expectation(Circuit("c", 2, [Gate("h", (0,)), Gate("cx", (0, 1))]), 1, 1, _backend, shots=120000, seed=7)
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


# ---------------------------------------------------------------------------
# Extended knitting: product-Z and full histogram reconstruction, verified
# exactly against the UNCUT simulator (the production correctness bar).
# ---------------------------------------------------------------------------

def _uncut_expectation(sim, circuit, shots, qubits, seed=7):
    counts = sim.submit(circuit, shots, seed=seed)["counts"]
    total = sum(counts.values())
    val = 0.0
    for k, v in counts.items():
        prod = 1.0
        for q in qubits:
            prod *= 1.0 if int(k[q]) == 0 else -1.0
        val += (v / total) * prod
    return val


def _make_bell_cut_circuit():
    # q[0] (control) entangled with q[1] (target) via CNOT(0,1) at index 1
    from kubona.circuits import Circuit, Gate
    return Circuit(name="bell", num_qubits=2,
                   gates=[Gate("h", (0,)), Gate("cx", (0, 1))])


def test_single_qubit_expectation_matches_uncut(seed=7):
    from kubona.circuits import Circuit, Gate
    from kubona.providers.local_simulator import LocalSimulator
    from kubona.schema import load_provider
    from kubona.cutting.wire import cut_expectation

    # 3-qubit: q0 -> CNOT(0,1) (the single cut), q2 independent (rotated)
    # The cut reaches ONLY the single target qubit q1; single-qubit Z must match.
    circ = Circuit(name="c", num_qubits=3,
                   gates=[Gate("h", (0,)),
                          Gate("cx", (0, 1)),
                          Gate("ry", (2,), (0.6,))])
    sim = LocalSimulator(load_provider("local-simulator"))
    shots = 65536  # enough samples to make the shot-noise tolerance reliable
    r = cut_expectation(circ, 1, 1, sim.submit, shots=shots, seed=seed)
    uncut = _uncut_expectation(sim, circ, shots, [1], seed=seed)  # same seed -> same stochastic draw family
    assert abs(r["expected"] - uncut) < 0.03, (r["expected"], uncut)


def test_histogram_reconstruction_matches_uncut(tmp_path):
    from kubona.providers.local_simulator import LocalSimulator
    from kubona.schema import load_provider
    from kubona.cutting.wire import cut_histogram

    circ = _make_bell_cut_circuit()
    sim = LocalSimulator(load_provider("local-simulator"))
    shots = 65536
    r = cut_histogram(circ, 1, n_target=1, backend=sim.submit, shots=shots, seed=7)
    dist = r["distribution"]
    # Bell -> the target (q1) is 50/50
    assert abs(dist.get("0", 0) - 0.5) < 0.02 and abs(dist.get("1", 0) - 0.5) < 0.02
    # also matches the uncut marginal exactly
    counts = sim.submit(circ, shots, seed=7)["counts"]
    total = sum(counts.values())
    uncut_p1 = sum(v for k, v in counts.items() if k[1] == "1") / total
    assert abs(dist.get("1", 0) - uncut_p1) < 0.03
