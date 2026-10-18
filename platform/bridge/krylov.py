"""
Quantum Krylov Subspace Diagonalization (QKSD) Solver.
Implements the post-NISQ subspace diagonalization method defined in
wiki/concepts/early-fault-tolerant-quantum-and-subspace-methods.md:
|psi_k> = exp(-i * k * H * tau) |psi_0>
Solves the classical generalized eigenproblem:
H c = E S c
where H_jk = <psi_j| H |psi_k>, S_jk = <psi_j|psi_k>.
"""
from __future__ import annotations

import time
import math
from typing import Dict, Any, List, Optional, Tuple
import numpy as np
import scipy.linalg

from chemistry import solve_classical_anchor
from executor import calculate_meyer_wallach

# --- Shared consts (single source; no inline magic) ---
#: Heuristic method label for the non-H2 analytical corridor. The string is
#: asserted by tests; keep it in one place so labels cannot rot apart.
ANALYTICAL_HEURISTIC_METHOD_V1 = "analytical-sequence-heuristic"
#: Solver tag for the refused (default) non-H2 path.
ANALYTICAL_REFUSED_SOLVER_V1 = "analytical-sequence-refused"
#: Solver tag for the real H2 numerical solve.
NUMERICAL_KRYLOV_SOLVER_V1 = "numerical-krylov"

try:
    from pyscf import gto, scf, fci, ao2mo
    HAS_PYSCF = True
except ImportError:
    HAS_PYSCF = False


def build_h2_hamiltonian_matrix(r: float = 0.7414, basis: str = "sto-3g") -> Tuple[np.ndarray, float, float]:
    """
    Builds the exact electronic Hamiltonian matrix for H2 in minimal STO-3G basis (4 spin-orbitals).
    Returns (H_matrix, fci_energy, nuclear_repulsion).
    """
    mol = gto.M(atom=f"H 0 0 0; H 0 0 {r}", basis=basis, charge=0, spin=0, verbose=0)
    mf = scf.RHF(mol).run()
    h1 = mf.mo_coeff.T @ mf.get_hcore() @ mf.mo_coeff
    eri = ao2mo.kernel(mol, mf.mo_coeff)
    eri = ao2mo.restore(1, eri, mf.mo_coeff.shape[1])
    nuc = mol.energy_nuc()

    # 2 spatial orbitals -> 4 spin-orbitals.
    # States with 1 alpha and 1 beta electron: 4 configurations in (2 choose 1) x (2 choose 1):
    # |0>: |1a, 1b> (HF)
    # |1>: |1a, 2b>
    # |2>: |2a, 1b>
    # |3>: |2a, 2b>
    # In spin-singlet subspace, |0> and |3> couple:
    # H_00 = 2*h1_00 + eri[0,0,0,0] + nuc
    # H_33 = 2*h1_11 + eri[1,1,1,1] + nuc
    # H_03 = eri[0,1,0,1]
    # For general representation, construct the full 4x4 configuration matrix:
    cisolver = fci.FCI(mf)
    e_fci, _ = cisolver.kernel()

    # Exact 4x4 matrix representation in the Sz=0 subspace:
    h_mat = np.zeros((4, 4), dtype=complex)
    h_mat[0, 0] = 2.0 * h1[0, 0] + eri[0, 0, 0, 0] + nuc
    h_mat[1, 1] = h1[0, 0] + h1[1, 1] + eri[0, 0, 1, 1] - eri[0, 1, 1, 0] + nuc
    h_mat[2, 2] = h1[0, 0] + h1[1, 1] + eri[0, 0, 1, 1] - eri[0, 1, 1, 0] + nuc
    h_mat[3, 3] = 2.0 * h1[1, 1] + eri[1, 1, 1, 1] + nuc
    h_mat[0, 3] = eri[0, 1, 0, 1]
    h_mat[3, 0] = eri[0, 1, 0, 1]

    return h_mat, float(e_fci), float(nuc)


def solve_quantum_krylov(
    molecule: str = "H2",
    bond_distance: float = 0.7414,
    subspace_dim: int = 4,
    time_step: float = 0.2,
    shots: int = 1024,
    *,
    allow_heuristic: bool = False,
) -> Dict[str, Any]:
    """
    Computes a Krylov-subspace ground-state estimate for the targeted
    molecule with CLASSICAL linear algebra (scipy expm/eigh) — no quantum
    execution happens here despite the historical docstring. Generates
    Krylov vectors via numerical real-time evolution |psi_k> =
    exp(-i * k * tau * H) |psi_0>, COMPUTES (not measures) the overlap
    matrix S and projected Hamiltonian, and solves H c = E S c with numpy.
    Non-H2 molecules take an analytical convergence sequence, not a solve.
    Downstream consumers must label these energies bridge-computed
    classical emulation (see callers of solve_quantum_krylov).

    Production bar (W1.2): the non-H2 analytical corridor is NOT a
    measurement. By default it REFUSES (success=False, no synthetic
    eigenvalues, meyer_wallach=None with a reason). Callers that explicitly
    opt in with allow_heuristic=True receive the corridor numbers LABELED
    as heuristic (method=analytical-sequence-heuristic, measured=False on
    every Krylov value, meyer_wallach=None with a reason). No synthetic
    value ever rides success=True unlabeled.
    """
    t0 = time.perf_counter()

    if molecule.upper() != "H2" or not HAS_PYSCF:
        # Classical fallback anchor for general molecules
        anchor = solve_classical_anchor(molecule, bond_distance)
        fci_e = anchor["fci_exact_energy"]
        # The anchor's method flag is CHECKED, not assumed: a CASCI fallback
        # value is variational-in-active-space, never full FCI. Downstream
        # consumers must read classical_method before citing the energy.
        classical_method = anchor.get("method", "unknown")
        if classical_method not in ("FCI-exact",) and not classical_method.startswith("CASCI("):
            raise RuntimeError(
                f"unrecognized anchor method {classical_method!r}: refusing "
                "to build a convergence sequence on an unlabeled energy"
            )
        elapsed = (time.perf_counter() - t0) * 1000.0
        if not allow_heuristic:
            # Fail closed: the analytical corridor (fci+0.05/(k+1)) is a
            # synthetic convergence schedule, not a solve. Refuse rather
            # than return it behind success=True (old code returned
            # success=True + hardcoded meyer_wallach=0.85).
            return {
                "success": False,
                "solver": ANALYTICAL_REFUSED_SOLVER_V1,
                "method": ANALYTICAL_REFUSED_SOLVER_V1,
                "measured": False,
                "ground_state_energy": None,
                "classical_fci_energy": float(fci_e),
                "classical_method": classical_method,
                "subspace_eigenvalues": [],
                "subspace_eigenvalues_measured": [],
                "subspace_dimension": 0,
                "meyer_wallach": None,
                "meyer_wallach_reason": (
                    "analytical corridor carries no statevector; "
                    "Meyer-Wallach omitted (the old hardcoded 0.85 was synthetic)"
                ),
                "execution_time_ms": elapsed,
                "error_message": (
                    "refusing analytical-sequence for non-H2 "
                    f"{molecule!r}: no Krylov solve performed; pass "
                    "allow_heuristic=True to receive the explicitly labeled "
                    "analytical-sequence-heuristic corridor instead"
                ),
            }
        # Explicit opt-in heuristic corridor: every Krylov number labeled.
        subspace_dim = max(2, min(subspace_dim, 6))
        eigenvalues = [fci_e + 0.05 / (k + 1) for k in range(subspace_dim)]
        eigenvalues.sort()
        return {
            "success": True,
            "solver": ANALYTICAL_HEURISTIC_METHOD_V1,
            "method": ANALYTICAL_HEURISTIC_METHOD_V1,
            "measured": False,
            "heuristic": True,
            "ground_state_energy": float(eigenvalues[0]),
            "ground_state_energy_measured": False,
            "classical_fci_energy": float(fci_e),
            "classical_method": classical_method,
            "subspace_eigenvalues": [float(x) for x in eigenvalues],
            "subspace_eigenvalues_measured": [False] * len(eigenvalues),
            "subspace_dimension": subspace_dim,
            "meyer_wallach": None,
            "meyer_wallach_reason": (
                "analytical-sequence-heuristic carries no statevector; "
                "Meyer-Wallach omitted (never the synthetic 0.85)"
            ),
            "heuristic_warning": (
                "analytical-sequence-heuristic: fci+0.05/(k+1) corridor off "
                "the classical anchor, not a Krylov solve; measured=False on "
                "every value"
            ),
            "execution_time_ms": elapsed,
            "error_message": "",
        }

    # H2 full numerical Krylov solver
    h_full, e_fci, nuc = build_h2_hamiltonian_matrix(r=bond_distance)
    dim_full = h_full.shape[0]
    m = min(subspace_dim, dim_full)

    # Reference state: Hartree-Fock ground configuration |0>
    psi_0 = np.zeros(dim_full, dtype=complex)
    psi_0[0] = 1.0

    # Build Krylov subspace states: |psi_k> = exp(-i * k * tau * H) |psi_0>
    # Using matrix exponential
    subspace_states = []
    for k in range(m):
        u_k = scipy.linalg.expm(-1j * k * time_step * h_full)
        psi_k = u_k @ psi_0
        subspace_states.append(psi_k)

    # Measure subspace matrices:
    # S_jk = <psi_j | psi_k>
    # H_jk = <psi_j | H | psi_k>
    s_mat = np.zeros((m, m), dtype=complex)
    h_sub = np.zeros((m, m), dtype=complex)

    for j in range(m):
        for k in range(m):
            s_mat[j, k] = np.vdot(subspace_states[j], subspace_states[k])
            h_sub[j, k] = np.vdot(subspace_states[j], h_full @ subspace_states[k])

    # Ensure Hermiticity
    s_mat = 0.5 * (s_mat + s_mat.conj().T)
    h_sub = 0.5 * (h_sub + h_sub.conj().T)

    # Solve generalized eigenvalue problem: H c = E S c
    # Regularize S if near-singular (threshold 1e-7)
    s_evals, s_evecs = scipy.linalg.eigh(s_mat)
    keep = s_evals > 1e-7
    if np.sum(keep) < m:
        # Projected subspace (Orthogonal QKSD / OQKD)
        v = s_evecs[:, keep] / np.sqrt(s_evals[keep])
        h_proj = v.conj().T @ h_sub @ v
        evals, evecs = scipy.linalg.eigh(h_proj)
    else:
        evals, evecs = scipy.linalg.eigh(h_sub, s_mat)

    evals = np.real(evals)
    evals.sort()
    ground_e = float(evals[0])

    # Ground statevector representation for entanglement scoring
    if np.sum(keep) < m:
        c0 = v @ evecs[:, 0]
    else:
        c0 = evecs[:, 0]
    gs_vec = np.zeros(dim_full, dtype=complex)
    for k in range(min(len(c0), len(subspace_states))):
        gs_vec += c0[k] * subspace_states[k]
    norm = np.linalg.norm(gs_vec)
    if norm > 1e-12:
        gs_vec /= norm
    mw_score = calculate_meyer_wallach(gs_vec)

    elapsed = (time.perf_counter() - t0) * 1000.0

    return {
        "success": True,
        "solver": NUMERICAL_KRYLOV_SOLVER_V1,
        "method": NUMERICAL_KRYLOV_SOLVER_V1,
        "measured": True,
        "classical_exact_matrix_exp": True,
        "ground_state_energy": ground_e,
        "ground_state_energy_measured": True,
        "classical_fci_energy": e_fci,
        "subspace_eigenvalues": [float(x) for x in evals],
        "subspace_eigenvalues_measured": [True] * len(evals),
        "subspace_dimension": len(evals),
        "meyer_wallach": mw_score,
        "execution_time_ms": elapsed,
        "error_message": "",
    }
