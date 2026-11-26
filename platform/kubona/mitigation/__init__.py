"""Frontier Quantum Error Mitigation suite for Kubona.

Includes:
  - Compact symmetry-preserving ansätze & fermionic Z2 tapering (``ansatz``)
  - QWC Pauli grouping, exact joint covariance & Neyman shots (``qwc``)
  - Exact Bonet-Monroig Z2 symmetry verification (``symmetry``)
  - Compile-Once ISA Virtual-RZ CDR, Per-Group CDR & Variable-Noise CDR (``cdr``)
  - General N-qubit Transversal Virtual Distillation (``virtual_distillation``)
  - General N-qubit Echo Verification / Dual-State Purification (``echo_verification``)
  - Scalable Matrix-Free M3 Readout Mitigation for n=1..32 qubits (``readout_m3``)
  - IBM Runtime NoiseLearnerV3 Sparse Pauli-Lindblad PEA & PEC (``spl_noise``)
  - Sample-based Quantum Diagonalization with S-CORE (``sqd``)
  - Quantum Subspace Expansion (``qse``)
  - Fermionic 1-RDM McWeeny & 2-RDM N-Representability Purification (``rdm_purification``)
  - Pre-Dispatch Statistical Power Gate & Autonomous Mitigation Advisor (``power_gate``)
  - Zero-Noise Extrapolation (``zne``)
"""

from .ansatz import (
    build_h2_givens_ansatz,
    build_lucj_ansatz,
    taper_fermionic_hamiltonian,
)
from .cdr import (
    allocate_cdr_training_shots,
    build_assignment_matrix,
    build_readout_cal_circuits,
    compile_isa_rz_template,
    evaluate_cdr_batch,
    fit_cdr_odr,
    fit_per_group_cdr,
    fit_vncdr,
    generate_cdr_training_circuits,
    generate_isa_rz_cdr_training_circuits,
    isa_circuit_to_qasm2,
    lock_cdr_native_slots,
)
from .echo_verification import (
    build_echo_verification_circuits,
    evaluate_echo_verification_counts,
)
from .power_gate import (
    advise_mitigation,
    assess_statistical_power,
)
from .qse import (
    build_qse_plan,
    default_qse_operators,
    solve_qse_from_expectations,
)
from .qwc import (
    allocate_neyman_shots,
    estimate_circuit_qwc_variances,
    estimate_energy_from_qwc_counts,
    group_pauli_terms,
    measurement_circuit_for_basis,
)
from .rdm_purification import (
    mcweeny_purify_1rdm,
    purify_2rdm_n_representable,
)
from .readout_m3 import (
    build_m3_cal_circuits,
    calibrate_m3,
    mitigate_bitstring_observable_m3,
    mitigate_counts_m3,
)
from .spl_noise import run_estimator_spl_mitigation
from .sqd import run_sqd_from_counts
from .symmetry import (
    detect_z_symmetries,
    postselect_z_counts,
    symmetry_verified_energy,
)
from .virtual_distillation import (
    build_vd_circuits,
    evaluate_vd_counts,
)
from .zne import run_zne

__all__ = [
    "advise_mitigation",
    "allocate_cdr_training_shots",
    "allocate_neyman_shots",
    "assess_statistical_power",
    "build_assignment_matrix",
    "build_echo_verification_circuits",
    "build_h2_givens_ansatz",
    "build_lucj_ansatz",
    "build_m3_cal_circuits",
    "build_qse_plan",
    "build_readout_cal_circuits",
    "build_vd_circuits",
    "calibrate_m3",
    "compile_isa_rz_template",
    "default_qse_operators",
    "detect_z_symmetries",
    "estimate_circuit_qwc_variances",
    "estimate_energy_from_qwc_counts",
    "evaluate_cdr_batch",
    "evaluate_echo_verification_counts",
    "evaluate_vd_counts",
    "fit_cdr_odr",
    "fit_per_group_cdr",
    "fit_vncdr",
    "generate_cdr_training_circuits",
    "generate_isa_rz_cdr_training_circuits",
    "group_pauli_terms",
    "isa_circuit_to_qasm2",
    "lock_cdr_native_slots",
    "mcweeny_purify_1rdm",
    "measurement_circuit_for_basis",
    "mitigate_bitstring_observable_m3",
    "mitigate_counts_m3",
    "postselect_z_counts",
    "purify_2rdm_n_representable",
    "run_estimator_spl_mitigation",
    "run_sqd_from_counts",
    "run_zne",
    "solve_qse_from_expectations",
    "symmetry_verified_energy",
    "taper_fermionic_hamiltonian",
]
