"""Verifier hardening: structure first, numbers second.

Locks: mirror detection on true U...U†, parametric inverses, narrow-cut
flags on separable/chain geometries, score-only refusal, clean-circuit
pass-through, unmeasured-means-flag (never pass).
"""

from kubona.circuits import Circuit, Gate
from kubona.verification import intake_checklist, min_entanglement_cut, mirror_fraction


def _mirror_bell():
    # H,CX,CX,H: second half inverts the first -> perfect mirror.
    return Circuit(name="mirror", num_qubits=2, gates=[
        Gate("h", (0,)), Gate("cx", (0, 1)),
        Gate("cx", (0, 1)), Gate("h", (0,))])


def _plain_bell():
    return Circuit(name="bell", num_qubits=2, gates=[
        Gate("h", (0,)), Gate("cx", (0, 1))])


def _two_pairs():
    return Circuit(name="pairs", num_qubits=4, gates=[
        Gate("h", (0,)), Gate("cx", (0, 1)),
        Gate("h", (2,)), Gate("cx", (2, 3))])


def _crossed():
    # Every qubit entangled across the middle: wide cut (min 2).
    return Circuit(name="crossed", num_qubits=4, gates=[
        Gate("h", (0,)), Gate("h", (1,)), Gate("h", (2,)), Gate("h", (3,)),
        Gate("cx", (0, 2)), Gate("cx", (1, 3)), Gate("cx", (0, 3)),
        Gate("cx", (0, 1)), Gate("cx", (2, 3))])


def test_mirror_detects_true_uncomputation():
    r = mirror_fraction(_mirror_bell())
    assert r["measured"] is True and r["mirror_fraction"] == 1.0


def test_mirror_parametric_inverses():
    c = Circuit(name="t", num_qubits=1, gates=[
        Gate("rx", (0,), (0.7,)), Gate("h", (0,)),
        Gate("h", (0,)), Gate("rx", (0,), (-0.7,))])
    r = mirror_fraction(c)
    assert r["mirror_fraction"] == 1.0


def test_mirror_short_circuit_unmeasured():
    r = mirror_fraction(_plain_bell())
    assert r["measured"] is False and r["mirror_fraction"] is None


def test_cut_separable_is_zero():
    r = min_entanglement_cut(_two_pairs())
    assert r["measured"] is True and r["min_cut"] == 0


def test_cut_wide_geometry_passes():
    r = min_entanglement_cut(_crossed())
    assert r["min_cut"] >= 2, r


def test_cut_too_wide_unmeasured():
    c = Circuit(name="wide", num_qubits=13, gates=[Gate("h", (0,))])
    r = min_entanglement_cut(c)
    assert r["measured"] is False and r["min_cut"] is None


def test_score_only_claims_refuse():
    out = intake_checklist(_plain_bell(), "score_only", fidelity=None)
    assert out["verdict"] == "REFUSE"
    out = intake_checklist(_plain_bell(), "mystery")
    assert out["verdict"] == "REFUSE"


def test_mirrored_claim_flags_for_width_analysis():
    out = intake_checklist(_mirror_bell(), "exact_amplitude")
    assert out["verdict"] == "FLAG"
    assert any("mirror" in reason for reason in out["reasons"])


def test_separable_claim_flags_geometry():
    out = intake_checklist(_two_pairs(), "bounded_fidelity_sampling", fidelity=0.9)
    assert out["verdict"] == "FLAG"
    assert any("cut" in reason or "geometry" in reason or "unmeasured" in reason
               for reason in out["reasons"])


def test_unmeasured_means_flag_never_pass():
    c = Circuit(name="wide", num_qubits=13, gates=[Gate("h", (0,))])
    out = intake_checklist(c, "exact_amplitude")
    assert out["verdict"] == "FLAG"
