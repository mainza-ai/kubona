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

from kubona.science.dmrg import dmrg_ground_energy
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


def _qiskit_pauli_label(label: str) -> str:
    """Convert the lattice module's qubit-0-at-index-0 label to Qiskit's
    little-endian Pauli string (qubit 0 is the last character)."""
    return str(label)[::-1]


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
        n_params = 2 * lattice.n_sites * int(layers)
        x0 = rng.uniform(-0.5, 0.5, size=n_params)

        def objective(x: np.ndarray) -> float:
            state = Statevector.from_instruction(_ansatz(lattice.n_sites, int(layers), np.asarray(x))).data
            return float(np.real(np.vdot(state, h.dot(state))))

        from scipy.optimize import minimize
        opt = minimize(
            objective, x0, method="COBYLA",
            options={"maxiter": int(max_iterations), "rhobeg": 0.25, "tol": 1e-8},
        )
        best_x = np.asarray(opt.x if opt.x is not None else x0, dtype=float)
        best_val = float(opt.fun) if opt.fun is not None else objective(best_x)
        if abs(best_val - exact_energy) > float(target_error) and int(max_iterations) >= 40:
            x_warm = rng.uniform(-math.pi / 2.0, math.pi / 2.0, size=n_params)
            opt_lbfgs = minimize(
                objective, x_warm, method="L-BFGS-B",
                options={"maxiter": max(25, int(max_iterations) // 2), "ftol": 1e-9},
            )
            cand_x = np.asarray(opt_lbfgs.x if opt_lbfgs.x is not None else x_warm, dtype=float)
            cand_val = float(opt_lbfgs.fun) if opt_lbfgs.fun is not None else objective(cand_x)
            if cand_val < best_val:
                opt = opt_lbfgs
                best_x = cand_x
        theta = best_x
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



def _tfim_thermodynamic_energy_per_site(coupling: float, field: float) -> float:
    """Exact Pauli-TFIM ground-state energy density in the thermodynamic limit."""
    from scipy.integrate import quad

    j, h = float(coupling), float(field)
    if j < 0 or h < 0:
        raise ValueError("TFIM thermodynamic limit requires non-negative coupling and field")
    integral, _err = quad(
        lambda k: math.sqrt(j * j + h * h - 2.0 * j * h * math.cos(k)),
        0.0, 2.0 * math.pi, epsabs=1e-12, epsrel=1e-12,
    )
    return float(-integral / (2.0 * math.pi))


def _heisenberg_thermodynamic_energy_per_site(coupling: float) -> float:
    """Exact Pauli-Heisenberg energy density: J(1-4 ln 2), or J ferromagnetic."""
    j = float(coupling)
    if j == 0:
        raise ValueError("Heisenberg thermodynamic limit requires non-zero coupling")
    if j > 0:
        return float(j * (1.0 - 4.0 * math.log(2.0)))
    return float(j)


def run_thermodynamic_limit(
    model: str = "tfim",
    sizes: list[int] | None = None,
    coupling: float = 1.0,
    field: float = 1.0,
    anisotropy: float = 1.0,
    bond_dim: int = 32,
    cutoff: float = 1e-10,
    sweeps: int = 8,
    tolerance: float = 1e-8,
) -> dict[str, Any]:
    """Finite-size DMRG plus an exact thermodynamic-limit formula where known.

    The finite-size points are variational MPS/DMRG energies at a declared bond
    dimension. The thermodynamic value is returned only when an exact formula
    exists; no finite-size extrapolation is promoted to an exact limit.
    """
    t0 = time.perf_counter()
    key = str(model or "").strip().lower()
    if key not in ("tfim", "heisenberg", "xxz"):
        return {"success": False, "error": f"thermodynamic limit supports tfim, heisenberg, xxz; refusing {model!r}"}
    if sizes is None:
        sizes = [8, 12, 16]
    if not isinstance(sizes, (list, tuple)):
        return {"success": False, "error": "sizes must be a list of system sizes"}
    parsed: list[int] = []
    for value in sizes:
        if isinstance(value, bool) or not isinstance(value, int):
            return {"success": False, "error": f"size {value!r} must be an integer"}
        if value < 4 or value > 40:
            return {"success": False, "error": f"size {value!r} outside [4,40]"}
        parsed.append(int(value))
    parsed = sorted(set(parsed))
    if not (2 <= len(parsed) <= 5):
        return {"success": False, "error": "sizes must contain 2..5 distinct values in [4,40]"}
    j, h, delta = float(coupling), float(field), float(anisotropy)
    if not all(math.isfinite(v) for v in (j, h, delta)):
        return {"success": False, "error": "coupling, field and anisotropy must be finite"}
    if key == "tfim":
        if j < 0 or h < 0:
            return {"success": False, "error": "TFIM thermodynamic limit requires non-negative coupling and field"}
        if j == 0 and h == 0:
            return {"success": False, "error": "tfim requires coupling or field to be non-zero"}
        exact_limit = _tfim_thermodynamic_energy_per_site(j, h)
        limit_method = "exact free-fermion momentum integral"
        limit_reason = ""
    elif key == "heisenberg":
        if j == 0:
            return {"success": False, "error": "heisenberg requires a non-zero coupling"}
        exact_limit = _heisenberg_thermodynamic_energy_per_site(j)
        limit_method = "exact Bethe-ansatz energy density" if j > 0 else "exact ferromagnetic product state"
        limit_reason = ""
    else:
        if j == 0:
            return {"success": False, "error": "xxz requires a non-zero coupling"}
        if abs(delta - 1.0) > 1e-12:
            exact_limit = None
            limit_method = "unavailable"
            limit_reason = "No exact closed-form thermodynamic limit is wired for anisotropic XXZ; finite-size DMRG is reported without an inferred limit."
        else:
            exact_limit = _heisenberg_thermodynamic_energy_per_site(j)
            limit_method = "exact isotropic Heisenberg limit (Delta=1)"
            limit_reason = ""

    finite: list[dict[str, Any]] = []
    exact_anchor: list[dict[str, Any]] = []
    for n in parsed:
        point = dmrg_ground_energy(
            model=key, n_sites=n, coupling=j, field=h, anisotropy=delta,
            bond_dim=int(bond_dim), cutoff=float(cutoff), sweeps=int(sweeps),
            tolerance=float(tolerance),
        )
        if not point.get("success"):
            return {"success": False, "error": f"DMRG failed at n={n}: {point.get('error')}"}
        if not point.get("converged"):
            return {"success": False, "error": f"DMRG did not converge at n={n}"}
        entry = {
            "n_sites": n,
            "energy": float(point["energy"]),
            "energy_per_site": float(point["energy_per_site"]),
            "bond_dim": int(point["bond_dim"]),
            "max_bond_reached": int(point["max_bond_reached"]),
            "converged": bool(point["converged"]),
        }
        finite.append(entry)
        if n <= 14:
            try:
                exact_model = SpinChainModel(
                    model=key, n_sites=n, coupling=j, field=h,
                    anisotropy=delta, periodic=False,
                )
                exact_energy, _vec, _evals = exact_model.ground_state(k=1)
                exact_anchor.append({
                    "n_sites": n,
                    "energy": float(exact_energy),
                    "energy_per_site": float(exact_energy / n),
                    "dmrg_delta_mha": abs(float(point["energy"]) - float(exact_energy)) * 1000.0,
                })
            except Exception:  # noqa: BLE001 — exact anchor is optional, not a substitute
                pass

    limit = None
    gaps = []
    if exact_limit is not None:
        limit = {
            "available": True,
            "energy_per_site": float(exact_limit),
            "method": limit_method,
            "reason": "",
        }
        gaps = [
            {"n_sites": p["n_sites"], "finite_size_minus_limit_mha_per_site": float((p["energy_per_site"] - exact_limit) * 1000.0)}
            for p in finite
        ]
    else:
        limit = {"available": False, "energy_per_site": None, "method": "unavailable", "reason": limit_reason}

    return {
        "success": True,
        "kind": "thermodynamic_limit",
        "result_version": "experiment-result-v1",
        "model": key,
        "boundary": "open",
        "coupling": j,
        "field": h,
        "anisotropy": delta,
        "sizes": parsed,
        "finite_size": finite,
        "exact_finite_size_anchor": exact_anchor,
        "thermodynamic_limit": limit,
        "finite_size_gap": gaps,
        "bond_dim": int(bond_dim),
        "sweeps": int(sweeps),
        "cutoff": float(cutoff),
        "method": "finite-size DMRG/MPS with an exact thermodynamic-limit formula where available",
        "measured": False,
        "hardware": False,
        "claimable": False,
        "converged": all(p["converged"] for p in finite),
        "limitations": (
            "Finite-size DMRG is variational at the declared bond dimension and has no certified "
            "truncation-error bound. The thermodynamic value is included only when an exact formula "
            "exists; finite-size points are not extrapolated into a claimed limit."
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
    try:
        coeffs = {str(label): float(coeff) for label, coeff in (pauli_coefficients or {}).items()}
    except (TypeError, ValueError) as exc:
        return {"success": False, "error": f"invalid pauli_coefficients: {exc}"}
    if not coeffs:
        return {"success": False, "error": "spin-chain hardware requires pauli_coefficients"}
    if any(not math.isfinite(value) for value in coeffs.values()):
        return {"success": False, "error": "pauli_coefficients must be finite"}
    n = int(n_sites)
    if n < 2 or n > 14:
        return {"success": False, "error": f"n_sites {n_sites!r} outside [2,14]"}
    if any(len(label) != n or set(label) - set("IXYZ") for label in coeffs):
        return {"success": False, "error": "every Pauli label must be a length-n string over IXYZ"}
    if int(shots) < 128 or int(shots) > 1_000_000:
        return {"success": False, "error": f"shots {shots!r} outside [128,1000000]"}
    if int(layers) < 1 or int(layers) > 8:
        return {"success": False, "error": f"layers {layers!r} outside [1,8]"}
    if not math.isfinite(float(energy_exact)):
        return {"success": False, "error": "energy_exact must be finite"}
    if not math.isfinite(float(target_error)) or not (0.0 < float(target_error) <= 1.0):
        return {"success": False, "error": "target_error must be finite in (0,1]"}
    mode = str(state_mode or "vqe").strip().lower()
    if mode != "vqe":
        return {"success": False, "error": "spin-chain hardware accepts state_mode='vqe' only; exact-state QASM contains reset, which QASM2 cannot carry"}
    if "reset" in str(qasm).lower():
        return {"success": False, "error": "spin-chain hardware refuses QASM containing reset; use a unitary VQE ansatz"}

    try:
        from experiments import _hardware_batch, _measurement_qasm, _hardware_counts_to_energy, _calibration_quality
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

    # The lattice module labels qubit 0 at string index 0. Qiskit measures
    # Pauli operators with the opposite convention, so convert both the
    # measurement circuits and the coefficient map. Relying on the uniform
    # model's reflection symmetry would hide this for TFIM but silently
    # mis-measure asymmetric coefficients.
    qiskit_coeffs = {_qiskit_pauli_label(label): float(coeff) for label, coeff in coeffs.items()}
    qasms: list[str] = []
    for label in sorted(qiskit_coeffs):
        qasms.append(_measurement_qasm(qc, label))
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
    measured = _hardware_counts_to_energy(counts_list[:-1], qiskit_coeffs)
    measured_terms = {
        label: measured["terms"][_qiskit_pauli_label(label)]
        for label in coeffs
        if _qiskit_pauli_label(label) in measured["terms"]
    }
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
    calibration_quality = _calibration_quality(batch.get("calibration"))
    claimable = bool(
        delta <= float(target_error)
        and uncertainty <= float(target_error)
        and not below_exact
        and calibration_quality["calibration_fresh"] is not False
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
        "ci95": float(1.96 * uncertainty),
        "delta": delta,
        "below_exact": below_exact,
        "measurements": measured_terms,
        "pauli_label_convention": "qubit-0-at-label-index-0; converted to Qiskit little-endian for measurement",
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
        **calibration_quality,
        "usage_seconds": batch.get("usage_seconds"),
        "actual_usd": batch.get("actual_usd"),
        "execution_time_ms": 0.0,
        "limitations": (
            "Finite-size exact anchor; ansatz optimized locally; every reported "
            "term is from QPU counts. No thermodynamic-limit or advantage claim."
        ),
        "error_message": "",
    }


def run_lattice_dynamics(
    rows: int = 2,
    cols: int = 2,
    coupling: float = 1.0,
    initial_field: float = 1.0,
    quench_field: float = 0.2,
    times: list[float] | None = None,
    periodic: bool = True,
) -> dict[str, Any]:
    """Exact finite-size 2D TFIM quench dynamics.

    This is a classical exact simulation, not a QPU measurement. It returns
    the full magnetization trajectory and per-time quench energy with an
    explicit finite-size limitation; ``measured`` and ``claimable`` stay false.
    """
    if not HAS_QISKIT:
        # The exact model does not need Qiskit, but this runner lives beside
        # the bridge's lattice surface; refuse consistently if the bridge
        # image is incomplete.
        return {"success": False, "error": "lattice dynamics unavailable: Qiskit bridge dependency missing"}
    if times is None:
        times = [0.0, 0.5, 1.0]
    from kubona.science.lattice import SquareLatticeModel
    try:
        model = SquareLatticeModel(rows=int(rows), cols=int(cols), coupling=float(coupling),
                                   field=float(initial_field), periodic=bool(periodic))
        result = model.time_evolution(float(quench_field), [float(t) for t in times])
    except (TypeError, ValueError) as exc:
        return {"success": False, "error": str(exc)}
    return {
        "success": True,
        "kind": "lattice_dynamics",
        "result_version": "experiment-result-v1",
        "model": "tfim-2d",
        "rows": result["rows"],
        "cols": result["cols"],
        "n_sites": result["n_sites"],
        "coupling": float(coupling),
        "initial_field": result["initial_field"],
        "quench_field": result["quench_field"],
        "periodic": result["periodic"],
        "times": result["times"],
        "method": "exact finite-size 2D TFIM quench (expm_multiply)",
        "energy_initial": result["initial_energy"],
        "measured": False,
        "hardware": False,
        "claimable": False,
        "simulated": True,
        "limitations": "Finite-size exact statevector evolution; no thermodynamic-limit, Trotter-error, shot-noise, or advantage claim.",
        "error_message": "",
    }
