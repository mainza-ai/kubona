"""Tests for Refuted Genome Prior Library and Hardware Transfer (B.3)."""

from kubona.autoresearch.ansatz_transfer import (
    AnsatzGenome,
    FailureClass,
    GenomeEvaluationRecord,
    RefutedGenomeLibrary,
)


def test_intrinsic_failure_globally_refuses():
    lib = RefutedGenomeLibrary()
    flawed_genome = AnsatzGenome(
        family="hardware_efficient",
        depth=50,
        n_qubits=20,
        entangler="linear",
        symmetry="none",
    )

    # Record intrinsic barren plateau failure on simulator
    record = GenomeEvaluationRecord(
        genome=flawed_genome,
        target_hamiltonian="FermiHubbard-4x5",
        device_id="local-simulator",
        survives=False,
        failure_class=FailureClass.INTRINSIC_BARREN_PLATEAU,
        gradient_variance=1e-12,
    )
    lib.record_refutation(record)

    # Attempt transfer to a high-coherence device (e.g. trapped-ion H1 with T2 = 2,000,000 us)
    admitted, reason = lib.can_transfer_to_device(
        genome=flawed_genome,
        target_device_id="quantinuum-h1",
        device_t2_us=2000000.0,
    )
    assert not admitted
    assert "globally refuted (intrinsic mathematical flaw)" in reason


def test_hardware_coherence_failure_permits_transfer_to_superior_device():
    lib = RefutedGenomeLibrary()
    deep_genome = AnsatzGenome(
        family="uccsd",
        depth=100,
        n_qubits=4,
        entangler="all_to_all",
        symmetry="particle_number",
    )

    # Genome fails on short-coherence device (T2 = 20 us)
    rec_short = GenomeEvaluationRecord(
        genome=deep_genome,
        target_hamiltonian="H2-STO3G",
        device_id="noisy-sc-device",
        survives=False,
        failure_class=FailureClass.HARDWARE_COHERENCE_LIMIT,
        predicted_fidelity=0.45,
    )
    lib.record_refutation(rec_short)

    # Transfer to short-coherence device is refused
    admitted_sc, reason_sc = lib.can_transfer_to_device(
        genome=deep_genome,
        target_device_id="noisy-sc-device",
        device_t2_us=20.0,
        gate_time_ns=200.0,  # 100 * 200 = 20,000ns = 20us -> 100% of T2
    )
    assert not admitted_sc
    assert "exceeds 50% of device T2" in reason_sc

    # Transfer to high-coherence device (T2 = 5,000 us) succeeds because intrinsic properties are sound
    admitted_ion, reason_ion = lib.can_transfer_to_device(
        genome=deep_genome,
        target_device_id="quantinuum-h1",
        device_t2_us=5000.0,
        gate_time_ns=200.0,
    )
    assert admitted_ion
    assert reason_ion is None
