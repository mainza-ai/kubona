"""Real variational experiment runners used by the bridge RPC.

This module is intentionally narrower than the research notebooks:

- H2 VQE in the STO-3G 2-qubit reduced space, with a live PySCF FCI anchor.
- H2 potential-energy scan built from the same VQE implementation.

The optimizer uses an exact statevector objective on Qiskit Aer's statevector
path; the reported energy is a *shot-measured* Pauli expectation, with the
exact optimizer value returned beside it and labelled. Unsupported molecules
refuse instead of borrowing H2 numbers.
"""
from __future__ import annotations

import math
import time
from typing import Any

import numpy as np

try:  # Qiskit is part of the bridge image; source checkouts may miss it.
    from qiskit import QuantumCircuit
    from qiskit.quantum_info import Statevector
    from qiskit_aer import AerSimulator
    HAS_QISKIT = True
except ImportError:  # pragma: no cover - bridge image always has it
    HAS_QISKIT = False

try:
    from krylov import build_h2_hamiltonian_matrix
    HAS_PYSCF = True
except ImportError:  # pragma: no cover
    HAS_PYSCF = False

try:
    from qiskit_nature.second_q.drivers import PySCFDriver
    from qiskit_nature.second_q.transformers import ActiveSpaceTransformer
    from qiskit_nature.second_q.mappers import JordanWignerMapper
    from qiskit_nature.second_q.circuit.library import UCCSD, HartreeFock
    HAS_NATURE = True
except ImportError:  # pragma: no cover - bridge image installs qiskit extra
    HAS_NATURE = False


_I = np.eye(2, dtype=complex)
_X = np.array([[0, 1], [1, 0]], dtype=complex)
_Y = np.array([[0, -1j], [1j, 0]], dtype=complex)
_Z = np.array([[1, 0], [0, -1]], dtype=complex)
_PAULI = {"I": _I, "X": _X, "Y": _Y, "Z": _Z}
_TERMS = ("II", "IZ", "ZI", "ZZ", "XX", "YY")


def _pauli_matrix(name: str) -> np.ndarray:
    if len(name) != 2 or any(ch not in _PAULI for ch in name):
        raise ValueError(f"invalid two-qubit Pauli label {name!r}")
    return np.kron(_PAULI[name[0]], _PAULI[name[1]])


def h2_hamiltonian(r: float) -> tuple[np.ndarray, float, float]:
    """Return the 4x4 H2 Hamiltonian, FCI energy, and RHF energy."""
    if not HAS_PYSCF:
        raise RuntimeError("PySCF unavailable: cannot compute the H2 Hamiltonian")
    h4, fci, _nuc = build_h2_hamiltonian_matrix(r=r)
    # The 4x4 matrix is ordered |1a1b>, |1a2b>, |2a1b>, |2a2b>. Qiskit's
    # 2-qubit statevector index q0 + 2*q1 uses the same four slots for this
    # reduced model; the labels are a basis relabelling, not a claim about
    # orbital identity.
    h4 = np.asarray(h4, dtype=complex)
    # RHF is the diagonal HF matrix element in the same reduced basis.
    rhf = float(np.real(h4[0, 0]))
    return h4, float(fci), rhf


def pauli_coefficients(h4: np.ndarray) -> dict[str, float]:
    """Decompose a 4x4 Hermitian matrix over the normalized Pauli basis."""
    out: dict[str, float] = {}
    for term in _TERMS:
        p = _pauli_matrix(term)
        c = float(np.real(np.trace(p @ h4)) / 4.0)
        if abs(c) > 1e-12:
            out[term] = c
    return out


def _ansatz(theta: np.ndarray, layers: int) -> QuantumCircuit:
    qc = QuantumCircuit(2)
    need = 4 * layers
    if len(theta) != need:
        raise ValueError(f"ansatz needs {need} parameters, got {len(theta)}")
    k = 0
    for _ in range(layers):
        qc.ry(float(theta[k]), 0); k += 1
        qc.ry(float(theta[k]), 1); k += 1
        qc.rz(float(theta[k]), 0); k += 1
        qc.rz(float(theta[k]), 1); k += 1
        qc.cx(0, 1)
    return qc


def _energy_exact(theta: np.ndarray, h4: np.ndarray, layers: int) -> float:
    state = Statevector.from_instruction(_ansatz(theta, layers)).data
    return float(np.real(np.vdot(state, h4 @ state)))


def _shot_expectation(
    qc: QuantumCircuit, term: str, shots: int, seed: int, sim: Any
) -> tuple[float, float]:
    """Measure one Pauli string and return (mean, standard error)."""
    if term == "II":
        return 1.0, 0.0
    measured = qc.copy()
    for q, ch in enumerate(term):
        if ch == "X":
            measured.h(q)
        elif ch == "Y":
            measured.sdg(q)
            measured.h(q)
        # Z and I need no basis rotation.
    measured.measure_all()
    counts = sim.run(measured, shots=shots, seed_simulator=seed).result().get_counts()
    total = sum(counts.values())
    if total <= 0:
        raise RuntimeError("measurement returned zero counts")
    # Qiskit prints c[n-1]..c[0]; term[0] is q0, so reverse the bitstring.
    acc = 0
    for bitstring, n in counts.items():
        bits = bitstring.replace(" ", "")[::-1]
        parity = sum(1 for q, ch in enumerate(term) if ch != "I" and bits[q] == "1")
        acc += n * (1 if parity % 2 == 0 else -1)
    mean = acc / total
    std = math.sqrt(max(0.0, 1.0 - mean * mean) / total)
    return float(mean), float(std)


def _shot_energy(
    theta: np.ndarray, coeffs: dict[str, float], layers: int,
    shots: int, seed: int,
) -> dict[str, Any]:
    sim = AerSimulator()
    qc = _ansatz(theta, layers)
    by_term: dict[str, Any] = {}
    energy = 0.0
    variance = 0.0
    for term, c in sorted(coeffs.items()):
        if term == "II":
            mean, std = 1.0, 0.0
        else:
            mean, std = _shot_expectation(qc, term, shots, seed, sim)
        by_term[term] = {"coefficient": c, "expectation": mean, "std_error": std}
        energy += c * mean
        variance += (c * std) ** 2
    return {
        "energy": float(energy),
        "uncertainty": float(math.sqrt(max(0.0, variance))),
        "shots": shots,
        "seed": seed,
        "terms": by_term,
    }


def run_vqe(
    molecule: str = "H2",
    bond_distance: float = 0.7414,
    layers: int = 2,
    shots: int = 16384,
    seed: int = 7,
    max_iterations: int = 80,
    full_space: bool = False,
) -> dict[str, Any]:
    """Run a real H2 VQE with a live anchor and shot-measured final energy."""
    t0 = time.perf_counter()
    if not HAS_QISKIT:
        return {"success": False, "error": "qiskit/aer unavailable: refusing VQE"}
    mol = (molecule or "").upper()
    if mol in _NATURE_ACTIVE:
        return run_nature_vqe(mol, float(bond_distance), shots=shots,
                              seed=seed, max_iterations=max_iterations,
                              full_space=bool(full_space), layers=int(layers))
    if mol != "H2":
        return {
            "success": False,
            "error": f"VQE supports H2, {sorted(_NATURE_ACTIVE)}; refusing {mol!r}",
        }
    if not (0.3 <= float(bond_distance) <= 10.0):
        return {"success": False, "error": f"bond_distance {bond_distance!r} outside [0.3,10.0]: refusing"}
    if layers < 1 or layers > 8:
        return {"success": False, "error": f"layers {layers!r} outside [1,8]: refusing"}
    if shots < 128 or shots > 1_000_000:
        return {"success": False, "error": f"shots {shots!r} outside [128,1000000]: refusing"}
    if max_iterations < 1 or max_iterations > 1000:
        return {"success": False, "error": f"max_iterations {max_iterations!r} outside [1,1000]: refusing"}

    from scipy.optimize import minimize

    r = float(bond_distance)
    h4, fci, rhf = h2_hamiltonian(r)
    coeffs = pauli_coefficients(h4)
    rng = np.random.default_rng(seed)
    x0 = rng.uniform(-0.2, 0.2, size=4 * layers)
    trajectory: list[dict[str, Any]] = []
    best = {"energy": float("inf")}
    evals = 0

    def objective(x: np.ndarray) -> float:
        nonlocal evals
        evals += 1
        e = _energy_exact(x, h4, layers)
        if e < best["energy"]:
            best["energy"] = e
            best["theta"] = [float(v) for v in x]
        if len(trajectory) < 500:
            trajectory.append({"iteration": evals, "energy": e,
                               "theta": [float(v) for v in x]})
        return e

    opt = minimize(
        objective, x0, method="COBYLA",
        options={"maxiter": int(max_iterations), "rhobeg": 0.35, "tol": 1e-9},
    )
    theta = np.asarray(opt.x if opt.x is not None else best.get("theta", x0), dtype=float)
    exact_energy = _energy_exact(theta, h4, layers)
    measured = _shot_energy(theta, coeffs, layers, int(shots), int(seed))
    energy = measured["energy"]
    delta = abs(energy - fci)
    elapsed_ms = (time.perf_counter() - t0) * 1000.0
    return {
        "success": True,
        "kind": "vqe",
        "result_version": "experiment-result-v1",
        "molecule": "H2",
        "bond_distance": r,
        "basis": "sto-3g",
        "method": "VQE hardware-efficient RY/RZ + CX, COBYLA",
        "ansatz": f"hardware-efficient layers={layers}",
        "layers": layers,
        "shots": int(shots),
        "seed": int(seed),
        "optimizer": {"name": "COBYLA", "max_iterations": int(max_iterations),
                      "evaluations": evals, "success": bool(opt.success),
                      "message": str(opt.message)},
        "theta": [float(v) for v in theta],
        "energy": float(energy),
        "energy_exact": float(exact_energy),
        "energy_measured": float(energy),
        "energy_measured_source": "shot-sampled Pauli expectation",
        "uncertainty": float(measured["uncertainty"]),
        "measurements": measured["terms"],
        "fci_exact_energy": float(fci),
        "rhf_mean_field_energy": float(rhf),
        "delta_mha": float(delta * 1000.0),
        "chemical_accuracy": bool(delta <= 1.6e-3),
        "chemical_accuracy_limit_mha": 1.6,
        "converged": bool(opt.success),
        "trajectory": trajectory,
        "pauli_coefficients": coeffs,
        "backend": "qiskit-aer-simulator",
        "provider": "qiskit-aer-simulator",
        "hardware": False,
        "optimizer_backend": "qiskit-aer-simulator",
        "measurement_backend": "qiskit-aer-simulator",
        "measured": True,
        "execution_time_ms": elapsed_ms,
        "error_message": "",
    }


def _run_full_space_vqe(spec: dict[str, Any], atom: str, bond_distance: float,
                         problem: Any, ncas: int, nelecas: int, shots: int,
                         seed: int, max_iterations: int, layers: int,
                         t0: float) -> dict[str, Any]:
    """Full-orbital-space VQE with a compact hardware-efficient ansatz.

    Full UCCSD is combinatorial at 10–14 qubits and was measured to exceed
    30 minutes for LiH before its first optimization pass. EfficientSU2 keeps
    the run tractable while still measuring the complete molecular Hamiltonian;
    the result labels the ansatz and whether the exact optimizer reached the
    full-FCI target. No active-space reduction is applied.
    """
    if not HAS_NATURE or not HAS_QISKIT:
        return {"success": False, "error": "qiskit-nature/pyscf unavailable: refusing molecular VQE"}
    if layers < 1 or layers > 8:
        return {"success": False, "error": f"layers {layers!r} outside [1,8]"}
    from qiskit.circuit.library import EfficientSU2
    from qiskit.quantum_info import Statevector
    from scipy.optimize import minimize

    e_full, _ = _full_fci_and_casci(atom, ncas, nelecas)
    mapper = JordanWignerMapper()
    qop = mapper.map(problem.hamiltonian.second_q_op())
    constants = float(sum(problem.hamiltonian.constants.values()))
    hf = HartreeFock(int(problem.num_spatial_orbitals), problem.num_particles, mapper)
    ansatz = EfficientSU2(qop.num_qubits, reps=int(layers),
                          entanglement="linear", initial_state=hf)
    rng = np.random.default_rng(int(seed))
    x0 = rng.uniform(-0.15, 0.15, size=ansatz.num_parameters)
    trajectory: list[dict[str, Any]] = []

    def objective(x: np.ndarray) -> float:
        sv = Statevector.from_instruction(ansatz.assign_parameters(x))
        e = float(np.real(sv.expectation_value(qop)) + constants)
        if len(trajectory) < 500:
            trajectory.append({"iteration": len(trajectory) + 1, "energy": e,
                               "theta": [float(v) for v in x]})
        return e

    opt = minimize(objective, x0, method="COBYLA",
                   options={"maxiter": int(max_iterations), "rhobeg": 0.25, "tol": 1e-8})
    theta = np.asarray(opt.x, dtype=float)
    exact = float(np.real(Statevector.from_instruction(ansatz.assign_parameters(theta))
                          .expectation_value(qop)) + constants)
    measured = _pauli_measurement_energy(ansatz, theta, qop, int(shots), int(seed))
    e = measured["energy"] + constants
    delta_full = abs(e - e_full) * 1000.0
    return {
        "success": True,
        "kind": "vqe",
        "result_version": "experiment-result-v1",
        "molecule": spec["name"],
        "bond_distance": float(bond_distance),
        "basis": "sto-3g",
        "method": "VQE EfficientSU2 full-space, COBYLA",
        "ansatz": f"EfficientSU2 full-space {qop.num_qubits} qubits, layers={int(layers)}",
        "qubit_count": int(qop.num_qubits),
        "active_space": {"ncas": ncas, "nelecas": nelecas,
                         "spatial_orbitals": int(problem.num_spatial_orbitals),
                         "full_space": True},
        "shots": int(shots),
        "seed": int(seed),
        "optimizer": {"name": "COBYLA", "max_iterations": int(max_iterations),
                      "evaluations": int(opt.nfev), "success": bool(opt.success),
                      "message": str(opt.message)},
        "theta": [float(v) for v in theta],
        "energy": float(e),
        "energy_exact": exact,
        "energy_measured": float(e),
        "energy_measured_source": "shot-sampled Pauli expectation",
        "uncertainty": float(measured["uncertainty"]),
        "measurements": measured["terms"],
        "fci_exact_energy": e_full,
        "active_space_fci_energy": e_full,
        "delta_mha": delta_full,
        "active_space_delta_mha": delta_full,
        "chemical_accuracy": bool(delta_full <= 1.6e-3 and measured["uncertainty"] <= 1.6e-3),
        "active_space_chemical_accuracy": bool(delta_full <= 1.6e-3 and measured["uncertainty"] <= 1.6e-3),
        "chemical_accuracy_limit_mha": 1.6,
        "full_space_status": "within_target" if (delta_full <= 1.6e-3 and measured["uncertainty"] <= 1.6e-3) else "contested",
        "claimable": bool(delta_full <= 1.6e-3 and measured["uncertainty"] <= 1.6e-3),
        "converged": bool(opt.success),
        "trajectory": trajectory,
        "backend": "qiskit-aer-simulator",
        "provider": "qiskit-aer-simulator",
        "hardware": False,
        "optimizer_backend": "qiskit-aer-simulator",
        "measurement_backend": "qiskit-aer-simulator",
        "measured": True,
        "limitations": "Full orbital space; EfficientSU2 is an expressive ansatz, not a UCCSD guarantee. Accuracy is reported against full FCI.",
        "execution_time_ms": (time.perf_counter() - t0) * 1000.0,
        "error_message": "",
    }


#: Active spaces for the molecular VQEs. These are deliberately smaller than
#: the full molecular orbital space; the result reports both the active-space
#: CASCI and the full-molecule FCI so no one mistakes a reduced model for the
#: full molecule.
_NATURE_ACTIVE = {
    "LIH": {"name": "LiH", "atom": "Li 0 0 0; H 0 0 {r}", "ncas": 2, "nelecas": 2},
    "BEH2": {"name": "BeH2", "atom": "Be 0 0 0; H 0 0 {r}; H 0 0 -{r}", "ncas": 3, "nelecas": 4},
}


def _full_fci_and_casci(atom: str, ncas: int, nelecas: int) -> tuple[float, float]:
    """Compute full FCI and active-space CASCI for the same geometry."""
    from pyscf import gto, scf, fci, mcscf
    mol = gto.M(atom=atom, basis="sto-3g", charge=0, spin=0, verbose=0)
    mf = scf.RHF(mol).run()
    e_fci, _ = fci.FCI(mf).kernel()
    cas = mcscf.CASCI(mf, ncas, nelecas).run()
    return float(e_fci), float(cas.e_tot)


def _pauli_measurement_energy(
    ansatz, theta: np.ndarray, qop: Any, shots: int, seed: int,
) -> dict[str, Any]:
    """Shot-measure a Qiskit Nature SparsePauliOp on the ansatz state."""
    qc = ansatz.assign_parameters(theta)
    from qiskit import transpile
    # Qiskit Nature UCCSD carries Pauli-evolution pseudo-ops; Aer needs a
    # concrete basis before measurement. Transpilation is part of execution,
    # not a substitute for it.
    qc = transpile(qc, basis_gates=["u1", "u2", "u3", "cx"],
                   optimization_level=1, seed_transpiler=seed)
    sim = AerSimulator()
    total = 0.0
    variance = 0.0
    terms: dict[str, Any] = {}
    for pauli, coeff in zip(qop.paulis, qop.coeffs):
        label = pauli.to_label()
        c = float(np.real(coeff))
        if abs(c) < 1e-14:
            continue
        if set(label) <= {"I"}:
            mean, std = 1.0, 0.0
        else:
            measured = qc.copy()
            # Qiskit Pauli labels are little-endian (qubit 0 is the LAST
            # character); rotate against the reversed label.
            for q, ch in enumerate(label[::-1]):
                if ch == "X":
                    measured.h(q)
                elif ch == "Y":
                    measured.sdg(q)
                    measured.h(q)
            measured.measure_all()
            counts = sim.run(measured, shots=shots, seed_simulator=seed).result().get_counts()
            n = sum(counts.values())
            acc = 0
            for bitstring, count in counts.items():
                bits = bitstring.replace(" ", "")[::-1]
                parity = sum(1 for q, ch in enumerate(label[::-1]) if ch != "I" and bits[q] == "1")
                acc += count * (1 if parity % 2 == 0 else -1)
            mean = acc / max(1, n)
            std = math.sqrt(max(0.0, 1.0 - mean * mean) / max(1, n))
        terms[label] = {"coefficient": c, "expectation": float(mean), "std_error": float(std)}
        total += c * mean
        variance += (c * std) ** 2
    return {"energy": float(total), "uncertainty": float(math.sqrt(max(0.0, variance))),
            "terms": terms, "shots": shots, "seed": seed}


def run_nature_vqe(
    molecule: str,
    bond_distance: float,
    shots: int = 8192,
    seed: int = 7,
    max_iterations: int = 80,
    full_space: bool = False,
    layers: int = 2,
) -> dict[str, Any]:
    """Active-space molecular VQE via Qiskit Nature + PySCF.

    The reported energy is shot-measured. The result explicitly distinguishes
    the active-space CASCI target from the full-molecule FCI anchor: a reduced
    active space is not the full molecule, and the claim must be judged against
    the anchor it actually compared with.
    """
    mol = (molecule or "").upper()
    spec = _NATURE_ACTIVE.get(mol)
    if spec is None:
        return {"success": False, "error": f"molecular VQE supports {sorted(_NATURE_ACTIVE)}"}
    if not HAS_NATURE or not HAS_QISKIT:
        return {"success": False, "error": "qiskit-nature/pyscf unavailable: refusing molecular VQE"}
    if not (0.3 <= float(bond_distance) <= 10.0):
        return {"success": False, "error": f"bond_distance {bond_distance!r} outside [0.3,10.0]"}
    t0 = time.perf_counter()
    atom = spec["atom"].format(r=float(bond_distance))
    problem = PySCFDriver(atom=atom, basis="sto-3g", charge=0, spin=0).run()
    ncas = int(problem.num_spatial_orbitals)
    particles = problem.num_particles
    nelecas = int(sum(particles) if isinstance(particles, (tuple, list)) else particles)
    requested_full = bool(full_space)
    if requested_full:
        return _run_full_space_vqe(spec, atom, float(bond_distance), problem,
                                   ncas, nelecas, shots, seed, max_iterations, layers, t0)
    ncas, nelecas = int(spec["ncas"]), int(spec["nelecas"])
    e_full, e_casci = _full_fci_and_casci(atom, ncas, nelecas)
    reduced = ActiveSpaceTransformer(
        num_electrons=nelecas, num_spatial_orbitals=ncas,
    ).transform(problem)
    mapper = JordanWignerMapper()
    qop = mapper.map(reduced.hamiltonian.second_q_op())
    constants = float(sum(reduced.hamiltonian.constants.values()))
    hf = HartreeFock(reduced.num_spatial_orbitals, reduced.num_particles, mapper)
    ansatz = UCCSD(reduced.num_spatial_orbitals, reduced.num_particles, mapper,
                   initial_state=hf)
    rng = np.random.default_rng(seed)
    x0 = rng.uniform(-0.15, 0.15, size=ansatz.num_parameters)
    trajectory: list[dict[str, Any]] = []

    def objective(x: np.ndarray) -> float:
        sv = Statevector.from_instruction(ansatz.assign_parameters(x))
        e = float(np.real(sv.expectation_value(qop)) + constants)
        if len(trajectory) < 500:
            trajectory.append({"iteration": len(trajectory) + 1, "energy": e,
                               "theta": [float(v) for v in x]})
        return e

    from scipy.optimize import minimize
    opt = minimize(objective, x0, method="COBYLA",
                   options={"maxiter": int(max_iterations), "rhobeg": 0.3, "tol": 1e-8})
    theta = np.asarray(opt.x, dtype=float)
    exact = float(np.real(Statevector.from_instruction(ansatz.assign_parameters(theta))
                          .expectation_value(qop)) + constants)
    measured = _pauli_measurement_energy(ansatz, theta, qop, int(shots), int(seed))
    # The Pauli expectation excludes the active-space constant; add it once
    # (core/nuclear contribution) before comparing with FCI.
    e = measured["energy"] + constants
    measured["constant_energy"] = constants
    # The shot measurement is the result; the exact value is retained as a
    # labelled optimizer diagnostic, never silently substituted.
    delta_full = abs(e - e_full) * 1000.0
    delta_active = abs(e - e_casci) * 1000.0
    return {
        "success": True,
        "kind": "vqe",
        "result_version": "experiment-result-v1",
        "molecule": spec["name"],
        "bond_distance": float(bond_distance),
        "basis": "sto-3g",
        "method": "VQE UCCSD full-space, COBYLA" if requested_full else "VQE UCCSD active-space, COBYLA",
        "ansatz": (f"UCCSD full-space {reduced.num_spatial_orbitals} orbitals"
                   if requested_full else f"UCCSD active-space {reduced.num_spatial_orbitals} orbitals"),
        "qubit_count": int(qop.num_qubits),
        "active_space": {"ncas": ncas, "nelecas": nelecas,
                         "spatial_orbitals": int(reduced.num_spatial_orbitals),
                         "full_space": requested_full},
        "shots": int(shots),
        "seed": int(seed),
        "optimizer": {"name": "COBYLA", "max_iterations": int(max_iterations),
                      "evaluations": int(opt.nfev), "success": bool(opt.success),
                      "message": str(opt.message)},
        "theta": [float(v) for v in theta],
        "energy": float(e),
        "energy_exact": exact,
        "energy_measured": float(e),
        "energy_measured_source": "shot-sampled Pauli expectation",
        "uncertainty": float(measured["uncertainty"]),
        "measurements": measured["terms"],
        "fci_exact_energy": e_full,
        "active_space_fci_energy": e_casci,
        "delta_mha": delta_full,
        "active_space_delta_mha": delta_active,
        "chemical_accuracy": bool(delta_full <= 1.6e-3 and measured["uncertainty"] <= 1.6e-3),
        "active_space_chemical_accuracy": bool(delta_active <= 1.6e-3 and measured["uncertainty"] <= 1.6e-3),
        "chemical_accuracy_limit_mha": 1.6,
        "claimable": bool(delta_full <= 1.6e-3 and measured["uncertainty"] <= 1.6e-3),
        "converged": bool(opt.success),
        "trajectory": trajectory,
        "backend": "qiskit-aer-simulator",
        "provider": "qiskit-aer-simulator",
        "hardware": False,
        "optimizer_backend": "qiskit-aer-simulator",
        "measurement_backend": "qiskit-aer-simulator",
        "measured": True,
        "limitations": ("Full orbital space; full-molecule FCI and CASCI coincide."
                        if requested_full else
                        "Active-space model; full-molecule FCI is reported separately and is the physical comparison."),
        "execution_time_ms": (time.perf_counter() - t0) * 1000.0,
        "error_message": "",
    }


def run_qaoa(
    layers: int = 2,
    shots: int = 4096,
    seed: int = 7,
    max_iterations: int = 80,
    graph_seed: int = 3,
) -> dict[str, Any]:
    """Run QAOA MaxCut on a fixed 8-node 3-regular graph."""
    if not HAS_QISKIT:
        return {"success": False, "error": "qiskit/aer unavailable: refusing QAOA"}
    import itertools
    import networkx as nx
    from qiskit import QuantumCircuit
    from qiskit.quantum_info import Statevector
    from scipy.optimize import minimize

    if layers < 1 or layers > 8:
        return {"success": False, "error": f"layers {layers!r} outside [1,8]"}
    g = nx.random_regular_graph(3, 8, seed=int(graph_seed))
    edges = sorted(tuple(sorted(e)) for e in g.edges())
    n = g.number_of_nodes()
    # Exact MaxCut, computed here (not hardcoded).
    best = -1
    mask_best = 0
    for mask in range(1 << n):
        cut = sum(1 for a, b in edges if ((mask >> a) & 1) != ((mask >> b) & 1))
        if cut > best:
            best, mask_best = cut, mask

    def qaoa(params: np.ndarray) -> QuantumCircuit:
        qc = QuantumCircuit(n)
        qc.h(range(n))
        for layer in range(int(layers)):
            gamma, beta = float(params[2 * layer]), float(params[2 * layer + 1])
            for a, b in edges:
                qc.cx(a, b)
                qc.rz(2.0 * gamma, b)
                qc.cx(a, b)
            qc.rx(2.0 * beta, range(n))
        return qc

    def cut_value(params: np.ndarray) -> float:
        sv = Statevector.from_instruction(qaoa(params)).data
        probs = np.abs(sv) ** 2
        val = 0.0
        for idx, p in enumerate(probs):
            if p <= 0:
                continue
            val += p * sum(1 for a, b in edges if ((idx >> a) & 1) != ((idx >> b) & 1))
        return float(val)

    rng = np.random.default_rng(seed)
    x0 = rng.uniform(0.0, np.pi, size=2 * int(layers))
    opt = minimize(lambda x: -cut_value(x), x0, method="COBYLA",
                   options={"maxiter": int(max_iterations), "rhobeg": 0.2, "tol": 1e-8})
    theta = np.asarray(opt.x, dtype=float)
    value = cut_value(theta)
    qc = qaoa(theta)
    qc.measure_all()
    counts = AerSimulator().run(qc, shots=int(shots), seed_simulator=int(seed)).result().get_counts()
    try:
        from qiskit import qasm2
        qaoa_qasm = qasm2.dumps(qc)
    except Exception:
        qaoa_qasm = ""
    total = sum(counts.values())
    shot_cut = sum(count * sum(1 for a, b in edges if ((int(bs.replace(" ", "")[::-1], 2) >> a) & 1) != ((int(bs.replace(" ", "")[::-1], 2) >> b) & 1))
                  for bs, count in counts.items()) / max(1, total)
    return {
        "success": True, "kind": "qaoa", "result_version": "experiment-result-v1", "molecule": "MaxCut",
        "graph": {"nodes": n, "edges": edges, "regular_degree": 3},
        "circuit_qasm": qaoa_qasm,
        "layers": int(layers), "shots": int(shots), "seed": int(seed),
        "theta": [float(v) for v in theta],
        "energy": float(shot_cut), "energy_exact": float(value),
        "energy_measured": float(shot_cut),
        "optimal_cut": int(best), "optimal_assignment": int(mask_best),
        "approximation_ratio": float(shot_cut / best) if best > 0 else None,
        "uncertainty": None, "measured": True,
        "backend": "qiskit-aer-simulator",
        "provider": "qiskit-aer-simulator",
        "hardware": False,
        "optimizer_backend": "qiskit-aer-simulator",
        "measurement_backend": "qiskit-aer-simulator",
        "optimizer": {"name": "COBYLA", "success": bool(opt.success),
                      "evaluations": int(opt.nfev), "message": str(opt.message)},
        "converged": bool(opt.success),
        "execution_time_ms": 0.0,
        "error_message": "",
    }


def run_pes(
    molecule: str = "H2",
    r_min: float = 0.5,
    r_max: float = 2.0,
    steps: int = 5,
    layers: int = 2,
    shots: int = 4096,
    seed: int = 7,
    max_iterations: int = 60,
) -> dict[str, Any]:
    """Run a real H2 VQE potential-energy scan over a caller-stated grid."""
    if not HAS_QISKIT:
        return {"success": False, "error": "qiskit/aer unavailable: refusing PES"}
    if (molecule or "").upper() != "H2":
        return {"success": False, "error": "PES currently supports H2 only; refusing another molecule"}
    try:
        rmin, rmax, n = float(r_min), float(r_max), int(steps)
    except (TypeError, ValueError):
        return {"success": False, "error": "r_min/r_max/steps must be numeric"}
    if not (0.3 <= rmin < rmax <= 10.0):
        return {"success": False, "error": "require 0.3 <= r_min < r_max <= 10.0: refusing"}
    if n < 2 or n > 21:
        return {"success": False, "error": "steps must be in [2,21]: refusing"}
    t0 = time.perf_counter()
    points = []
    for i in range(n):
        r = rmin + (rmax - rmin) * i / (n - 1)
        res = run_vqe(molecule="H2", bond_distance=r, layers=layers,
                      shots=shots, seed=seed + i, max_iterations=max_iterations)
        if not res.get("success"):
            return {"success": False, "error": f"PES point {r:.4f} failed: {res.get('error')}",
                    "failed_point": r, "completed_points": points}
        points.append({
            "bond_distance": r,
            "energy": res["energy"],
            "energy_exact": res["energy_exact"],
            "uncertainty": res["uncertainty"],
            "fci_exact_energy": res["fci_exact_energy"],
            "rhf_mean_field_energy": res["rhf_mean_field_energy"],
            "delta_mha": res["delta_mha"],
            "chemical_accuracy": res["chemical_accuracy"],
            "theta": res["theta"],
        })
    return {
        "success": True,
        "kind": "pes",
        "result_version": "experiment-result-v1",
        "molecule": "H2",
        "basis": "sto-3g",
        "r_min": rmin,
        "r_max": rmax,
        "steps": n,
        "points": points,
        "backend": "qiskit-aer-simulator",
        "measured": True,
        "execution_time_ms": (time.perf_counter() - t0) * 1000.0,
        "error_message": "",
    }
