"""Shot-measured spin-chain experiment on the bridge compute plane.

The exact finite-size diagonalization is the classical anchor. The ansatz
optimizer is local statevector optimization, and the reported energy is a
shot-sampled Pauli expectation on Aer. No thermodynamic-limit or quantum
advantage claim is made.
"""

from __future__ import annotations

import math
import time
from typing import Any

import numpy as np

try:
    from qiskit import QuantumCircuit, qasm2
    from qiskit.quantum_info import Statevector
    from qiskit_aer import AerSimulator
    HAS_QISKIT = True
except ImportError:  # pragma: no cover - bridge image always has it
    HAS_QISKIT = False

from kubona.science.lattice import SpinChainModel


def _ansatz(n_sites: int, layers: int, theta: np.ndarray) -> Any:
    need = 2 * n_sites * int(layers)
    if len(theta) != need:
        raise ValueError(f"ansatz needs {need} parameters, got {len(theta)}")
    qc = QuantumCircuit(int(n_sites))
    k = 0
    for _ in range(int(layers)):
        for q in range(int(n_sites)):
            qc.ry(float(theta[k]), q)
            k += 1
            qc.rz(float(theta[k]), q)
            k += 1
        for q in range(int(n_sites)):
            qc.cx(q, (q + 1) % int(n_sites))
    return qc


def _measurement_circuit(qc: Any, label: str) -> Any:
    measured = qc.copy()
    for q, char in enumerate(label):
        if char == "X":
            measured.h(q)
        elif char == "Y":
            measured.sdg(q)
            measured.h(q)
    measured.measure_all()
    return measured


def _shot_expectation(counts: dict[str, int], label: str) -> tuple[float, float]:
    total = sum(int(v) for v in counts.values())
    if total <= 0:
        raise RuntimeError(f"measurement for {label} returned zero counts")
    acc = 0
    for bitstring, count in counts.items():
        bits = str(bitstring).replace(" ", "")[::-1]
        if len(bits) < len(label):
            raise RuntimeError(f"counts bitstring {bitstring!r} is shorter than {label!r}")
        parity = sum(1 for q, char in enumerate(label) if char != "I" and bits[q] == "1")
        acc += int(count) * (1 if parity % 2 == 0 else -1)
    mean = acc / total
    std = math.sqrt(max(0.0, 1.0 - mean * mean) / total)
    return float(mean), float(std)


def _measured_terms(qc: Any, terms: list[tuple[str, float]], shots: int,
                    seed: int) -> dict[str, Any]:
    sim = AerSimulator()
    out: dict[str, Any] = {}
    energy = 0.0
    variance = 0.0
    for index, (label, coeff) in enumerate(terms):
        if set(label) <= {"I"}:
            mean, std = 1.0, 0.0
        else:
            measured = _measurement_circuit(qc, label)
            counts = sim.run(measured, shots=int(shots), seed_simulator=int(seed) + index).result().get_counts()
            mean, std = _shot_expectation(counts, label)
        out[label] = {"coefficient": float(coeff), "expectation": mean, "std_error": std}
        energy += float(coeff) * mean
        variance += (float(coeff) * std) ** 2
    return {"energy": float(energy), "uncertainty": float(math.sqrt(max(0.0, variance))),
            "terms": out}


def _measured_magnetization(qc: Any, n_sites: int, shots: int, seed: int) -> list[float]:
    sim = AerSimulator()
    measured = qc.copy()
    measured.measure_all()
    counts = sim.run(measured, shots=int(shots), seed_simulator=int(seed)).result().get_counts()
    total = sum(int(v) for v in counts.values())
    if total <= 0:
        raise RuntimeError("Z-basis measurement returned zero counts")
    values = []
    for q in range(n_sites):
        acc = 0
        for bitstring, count in counts.items():
            bits = str(bitstring).replace(" ", "")[::-1]
            acc += int(count) * (1 if bits[q] == "0" else -1)
        values.append(float(acc) / float(total))
    return values


def run_spin_chain(
    model: str = "tfim",
    n_sites: int = 4,
    coupling: float = 1.0,
    field: float = 1.0,
    anisotropy: float = 1.0,
    periodic: bool = True,
    layers: int = 2,
    shots: int = 8192,
    seed: int = 7,
    max_iterations: int = 160,
    target_error: float = 1e-3,
    state_mode: str = "vqe",
) -> dict[str, Any]:
    if not HAS_QISKIT:
        return {"success": False, "error": "qiskit/aer unavailable: refusing spin-chain experiment"}
    if int(layers) < 1 or int(layers) > 6:
        return {"success": False, "error": f"layers {layers!r} outside [1,6]"}
    if int(shots) < 128 or int(shots) > 1_000_000:
        return {"success": False, "error": f"shots {shots!r} outside [128,1000000]"}
    if int(max_iterations) < 1 or int(max_iterations) > 1000:
        return {"success": False, "error": f"max_iterations {max_iterations!r} outside [1,1000]"}
    mode = str(state_mode or "vqe").strip().lower()
    if mode not in ("vqe", "exact"):
        return {"success": False, "error": f"state_mode {state_mode!r} must be vqe|exact"}
    if not math.isfinite(float(target_error)) or not (0.0 < float(target_error) <= 1.0):
        return {"success": False, "error": "target_error must be finite in (0,1]"}
    try:
        lattice = SpinChainModel(
            model=model, n_sites=int(n_sites), coupling=float(coupling),
            field=float(field), anisotropy=float(anisotropy), periodic=bool(periodic),
        )
    except ValueError as exc:
        return {"success": False, "error": str(exc)}

    t0 = time.perf_counter()
    h = lattice.hamiltonian()
    exact_energy, exact_vec, eigenvalues = lattice.ground_state()
    observables = lattice.observables(exact_vec)
    terms = lattice.pauli_terms()
    if mode == "exact":
        # A circuit-level state-preparation benchmark: initialize the exact
        # finite-size ground state and measure it. This is not VQE and makes
        # no variational-discovery claim; it is useful for shot-level
        # observable readout against a known anchor.
        qc = QuantumCircuit(lattice.n_sites)
        qc.initialize(exact_vec, list(range(lattice.n_sites)))
        theta = np.zeros(0, dtype=float)
        exact_opt = float(exact_energy)
        opt = None
    else:
        rng = np.random.default_rng(int(seed))
        x0 = rng.uniform(-0.5, 0.5, size=2 * lattice.n_sites * int(layers))

        def objective(x: np.ndarray) -> float:
            state = Statevector.from_instruction(_ansatz(lattice.n_sites, int(layers), np.asarray(x))).data
            return float(np.real(np.vdot(state, h.dot(state))))

        from scipy.optimize import minimize
        opt = minimize(
            objective, x0, method="COBYLA",
            options={"maxiter": int(max_iterations), "rhobeg": 0.25, "tol": 1e-8},
        )
        theta = np.asarray(opt.x if opt.x is not None else x0, dtype=float)
        qc = _ansatz(lattice.n_sites, int(layers), theta)
        exact_opt = float(np.real(np.vdot(
            Statevector.from_instruction(qc).data,
            h.dot(Statevector.from_instruction(qc).data),
        )))
    measured = _measured_terms(qc, terms, int(shots), int(seed))
    measured_mag = _measured_magnetization(qc, lattice.n_sites, int(shots), int(seed) + 1)
    energy = float(measured["energy"])
    uncertainty = float(measured["uncertainty"])
    delta = abs(energy - exact_energy)
    below_exact = bool(energy < exact_energy - 3.0 * uncertainty)
    claimable = bool(
        delta <= float(target_error)
        and uncertainty <= float(target_error)
        and not below_exact
        and mode == "vqe"
    )
    return {
        "success": True,
        "kind": "spin_chain",
        "result_version": "experiment-result-v1",
        "model": lattice.model,
        "n_sites": lattice.n_sites,
        "coupling": lattice.coupling,
        "field": lattice.field,
        "anisotropy": lattice.anisotropy,
        "periodic": lattice.periodic,
        "qubit_count": lattice.n_sites,
        "layers": int(layers),
        "shots": int(shots),
        "seed": int(seed),
        "target_error": float(target_error),
        "method": ("exact finite-size state preparation + shot readout"
                   if mode == "exact" else "VQE hardware-efficient ansatz"),
        "state_mode": mode,
        "ansatz": ("exact ground-state preparation"
                   if mode == "exact" else "hardware-efficient RY/RZ + ring CX"),
        "theta": [float(v) for v in theta],
        "ansatz_qasm": qasm2.dumps(qc),
        "energy": energy,
        "energy_exact": exact_energy,
        "energy_measured": energy,
        "energy_measured_source": "shot-sampled Pauli expectation",
        "energy_optimizer_exact": exact_opt,
        "uncertainty": uncertainty,
        "ci95": float(1.96 * uncertainty),
        "delta": float(delta),
        "below_exact": below_exact,
        "claimable": claimable,
        "gap": observables.gap,
        "eigenvalues": eigenvalues,
        "ground_state_reached": True if mode == "exact" else claimable,
        "observable_state": (
            "exact ground state prepared on the circuit and shot-measured"
            if mode == "exact" else
            "optimized VQE ansatz state; exact_* fields are the finite-size "
            "ground-state anchor, measured_* fields are not ground-state "
            "observables unless ground_state_reached is true"
        ),
        "exact_magnetization_z": observables.magnetization_z,
        "measured_magnetization_z": measured_mag,
        "exact_correlation_zz": observables.correlation_zz,
        "entanglement_entropy_half": observables.entanglement_entropy_half,
        "measurements": measured["terms"],
        "pauli_term_count": len(terms),
        "pauli_coefficients": {label: float(coeff) for label, coeff in terms},
        "optimizer": ({
            "name": "none", "success": True, "evaluations": 0,
            "message": "exact state preparation", "max_iterations": 0,
        } if mode == "exact" else {
            "name": "COBYLA", "success": bool(opt.success),
            "evaluations": int(opt.nfev), "message": str(opt.message),
            "max_iterations": int(max_iterations),
        }),
        "converged": True if mode == "exact" else bool(opt.success),
        "backend": "qiskit-aer-simulator",
        "provider": "qiskit-aer-simulator",
        "hardware": False,
        "measured": True,
        "simulated_optimizer": False if mode == "exact" else True,
        "optimizer_backend": ("exact-diagonalization" if mode == "exact" else "qiskit-aer-simulator"),
        "measurement_backend": "qiskit-aer-simulator",
        "limitations": (
            "Finite-size exact diagonalization anchor; the exact-state mode "
            "prepares that anchor on the circuit and measures it, while the "
            "vqe mode is local statevector optimization. Reported energy is "
            "shot-sampled. No thermodynamic-limit or quantum-advantage claim."
        ),
        "execution_time_ms": (time.perf_counter() - t0) * 1000.0,
        "error_message": "",
    }


def run_spin_chain_hardware(
    qasm: str,
    pauli_coefficients: dict[str, float],
    n_sites: int,
    energy_exact: float,
    model: str = "tfim",
    coupling: float = 1.0,
    field: float = 0.0,
    anisotropy: float = 1.0,
    periodic: bool = True,
    layers: int = 2,
    shots: int = 4096,
    seed: int = 7,
    target_error: float = 1e-3,
    backend: str | None = None,
    state_mode: str = "vqe",
    source_job_id: str = "",
) -> dict[str, Any]:
    """Execute a locally optimized spin-chain ansatz on a governed QPU batch.

    Every Pauli term is measured from hardware counts. The optimizer remains
    local; `simulated_optimizer:true` and `measured:true` are both explicit.
    One provider batch is one vendor job, one usage reading, and one settlement.
    """
    if not HAS_QISKIT:
        return {"success": False, "error": "qiskit unavailable: refusing spin-chain hardware"}
    if not str(qasm or "").strip():
        return {"success": False, "error": "spin-chain hardware requires ansatz qasm"}
    coeffs = {str(label): float(coeff) for label, coeff in (pauli_coefficients or {}).items()}
    if not coeffs:
        return {"success": False, "error": "spin-chain hardware requires pauli_coefficients"}
    n = int(n_sites)
    if n < 2 or n > 14:
        return {"success": False, "error": f"n_sites {n_sites!r} outside [2,14]"}
    if any(len(label) != n or set(label) - set("IXYZ") for label in coeffs):
        return {"success": False, "error": "every Pauli label must be a length-n string over IXYZ"}
    if int(shots) < 128 or int(shots) > 1_000_000:
        return {"success": False, "error": f"shots {shots!r} outside [128,1000000]"}
    if not math.isfinite(float(energy_exact)):
        return {"success": False, "error": "energy_exact must be finite"}
    if not math.isfinite(float(target_error)) or not (0.0 < float(target_error) <= 1.0):
        return {"success": False, "error": "target_error must be finite in (0,1]"}
    mode = str(state_mode or "vqe").strip().lower()
    if mode != "vqe":
        return {"success": False, "error": "spin-chain hardware accepts state_mode='vqe' only; exact-state QASM contains reset, which QASM2 cannot carry"}

    try:
        from experiments import _hardware_batch, _measurement_qasm, _hardware_counts_to_energy
        from qiskit import qasm2
        qc = qasm2.loads(str(qasm))
    except Exception as exc:  # noqa: BLE001
        return {"success": False, "error": f"ansatz qasm is not executable: {exc}"}
    if qc.num_qubits != n:
        return {"success": False, "error": f"ansatz has {qc.num_qubits} qubits; n_sites is {n}"}
    if getattr(qc, "num_clbits", 0) > 0 or any(
        getattr(inst.operation, "name", "") == "measure" for inst in qc.data
    ):
        return {"success": False, "error": "spin-chain hardware requires a unitary ansatz without measurement"}

    qasms: list[str] = []
    labels: list[str] = []
    for label in sorted(coeffs):
        qasms.append(_measurement_qasm(qc, label))
        labels.append(label)
    # One extra Z-basis circuit for the measured magnetization vector.
    z_circuit = qc.copy()
    z_circuit.measure_all()
    qasms.append(qasm2.dumps(z_circuit))
    try:
        batch = _hardware_batch(qasms, int(shots), int(seed), backend=backend)
    except Exception as exc:  # noqa: BLE001
        return {"success": False, "error": f"hardware batch refused: {exc}"}
    counts_list = batch.get("counts")
    if not isinstance(counts_list, list) or len(counts_list) != len(qasms):
        return {"success": False, "error": "hardware batch returned no per-term counts"}
    measured = _hardware_counts_to_energy(counts_list[:-1], coeffs)
    z_counts = counts_list[-1]
    z_total = sum(int(v) for v in z_counts.values())
    if z_total <= 0:
        return {"success": False, "error": "hardware Z-basis measurement returned zero counts"}
    magnetization = []
    for q in range(n):
        acc = 0
        for bitstring, count in z_counts.items():
            bits = str(bitstring).replace(" ", "")[::-1]
            if len(bits) < n:
                return {"success": False, "error": f"counts bitstring {bitstring!r} is shorter than {n}"}
            acc += int(count) * (1 if bits[q] == "0" else -1)
        magnetization.append(float(acc) / float(z_total))
    energy = float(measured["energy"])
    uncertainty = float(measured["uncertainty"])
    delta = abs(energy - float(energy_exact))
    below_exact = bool(energy < float(energy_exact) - 3.0 * uncertainty)
    claimable = bool(
        delta <= float(target_error)
        and uncertainty <= float(target_error)
        and not below_exact
    )
    return {
        "success": True,
        "kind": "spin_chain_hardware",
        "result_version": "experiment-result-v1",
        "model": str(model),
        "n_sites": n,
        "coupling": float(coupling),
        "field": float(field),
        "anisotropy": float(anisotropy),
        "periodic": bool(periodic),
        "layers": int(layers),
        "shots": int(shots),
        "seed": int(seed),
        "target_error": float(target_error),
        "state_mode": mode,
        "source_job_id": str(source_job_id),
        "theta": [],
        "pauli_coefficients": coeffs,
        "energy": energy,
        "energy_exact": float(energy_exact),
        "energy_measured": energy,
        "energy_measured_source": "QPU measurement-basis batch",
        "uncertainty": uncertainty,
        "delta": delta,
        "below_exact": below_exact,
        "measured_magnetization_z": magnetization,
        "claimable": claimable,
        "ground_state_reached": bool(claimable or mode == "exact"),
        "hardware": True,
        "measured": True,
        "simulated_optimizer": True,
        "optimizer_backend": "local-source-job",
        "measurement_backend": "ibm-qiskit",
        "provider": "ibm-qiskit",
        "backend": str(batch.get("backend") or "ibm-qiskit"),
        "hardware_job_id": str(batch.get("hardware_job_id") or batch.get("job_id") or ""),
        "layout": batch.get("layout"),
        "calibration": batch.get("calibration"),
        "usage_seconds": batch.get("usage_seconds"),
        "actual_usd": batch.get("actual_usd"),
        "execution_time_ms": 0.0,
        "limitations": (
            "Finite-size exact anchor; ansatz optimized locally; every reported "
            "term is from QPU counts. No thermodynamic-limit or advantage claim."
        ),
        "error_message": "",
    }
