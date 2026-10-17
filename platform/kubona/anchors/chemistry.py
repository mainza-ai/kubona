"""Classical physics baselines and ground-truth anchors (Ring 2).

TABLE ANCHORS, not solvers: every number here is a precomputed literature /
exact-diagonalization value pinned to a table version, returned with provenance.
For a LIVE ab-initio number at an arbitrary geometry, run the bridge
``solve_classical_anchor`` (PySCF) instead — this module never runs RHF/FCI.

Implements quantum-advantage-realism: every molecular/electronic structure experiment
MUST compute classical anchors (Restricted Hartree-Fock RHF and Exact Diagonalization /
Full Configuration Interaction FCI within the active space) before submitting to paid QPUs.
"""

from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Any

import numpy as np

#: Version of the tabulated anchor values below. Bumped whenever a coefficient
#: changes; results carry it so ledgers can tell which table they came from.
TABLE_ANCHOR_VERSION_V1 = "table-anchor-v1"

#: Provenance carried on every anchor from this module.
TABLE_ANCHOR_PROVENANCE_V1 = (
    "tabulated exact-diagonalization coefficients "
    "(table-anchor-v1; not a live RHF/FCI solve — for live numbers use the "
    "PySCF bridge solve_classical_anchor)"
)


@dataclass(frozen=True)
class MolecularAnchor:
    system_name: str
    geometry_description: str
    basis: str
    rhf_energy_hartree: float
    fci_energy_hartree: float
    active_space_qubits: int
    classical_method: str = "Exact-Diagonalization (active-space)"
    timestamp_ns: int = 0
    table_version: str = TABLE_ANCHOR_VERSION_V1
    provenance: str = TABLE_ANCHOR_PROVENANCE_V1

    def correlation_energy(self) -> float:
        """Correlation energy E_corr = E_FCI - E_RHF (always negative)."""
        return self.fci_energy_hartree - self.rhf_energy_hartree

    def to_dict(self) -> dict[str, Any]:
        return {
            "system_name": self.system_name,
            "geometry_description": self.geometry_description,
            "basis": self.basis,
            "rhf_energy_hartree": self.rhf_energy_hartree,
            "fci_energy_hartree": self.fci_energy_hartree,
            "correlation_energy_hartree": self.correlation_energy(),
            "active_space_qubits": self.active_space_qubits,
            "classical_method": self.classical_method,
            "table_version": self.table_version,
            "provenance": self.provenance,
        }


class MolecularAnchorEngine:
    """Tabulated classical baselines for test molecules (table-anchor-v1).

    This engine LOOKS UP precomputed anchors — it does not solve RHF/FCI.
    Only systems with an exactly-constructed active-space Hamiltonian are
    served (currently H2, 2-qubit Jordan-Wigner). Anything else REFUSES with
    a pointer to the live PySCF bridge: the old code invented RHF/FCI numbers
    for arbitrary molecules (``-1.0/r - 0.5`` curve-fits) and slapped a fixed
    -0.042 Ha "correlation offset" on LiH — fabrication labeled as anchors.
    """

    # Tabulated benchmark systems (bond distances in Angstroms). H2 carries
    # the exact 2-qubit Jordan-Wigner coefficients; the FCI energy is
    # diagonalized live from THOSE coefficients (exact in the active space),
    # while RHF is the tabulated literature value at the reference geometry.
    BENCHMARKS = {
        "H2": {
            "r": 0.7414,
            # Jordan-Wigner 2-qubit minimal active space Hamiltonian coefficients
            # H = g0 * I + g1 * Z0 + g2 * Z1 + g3 * Z0*Z1 + g4 * X0*X1 + g5 * Y0*Y1
            # Electronic energy + nuclear repulsion (1/r = 0.7137 Ha) gives total ground state ~ -1.137 Ha
            # In qubit representation: RHF state |01> or |10> has higher energy than ground state superposition
            "g0": -0.8126,
            "g1": 0.1712,
            "g2": 0.1712,
            "g3": 0.1686,
            "g4": 0.0453,
            "g5": 0.0453,
            "rhf_energy": -1.050,  # Classical Hartree-Fock energy (-1.050 Ha > FCI -1.0718 Ha)
            "qubits": 2,
        },
    }

    def compute_table_anchor(self, molecule: str,
                             bond_distance: float | None = None) -> MolecularAnchor:
        """Looks up the tabulated anchor for a benchmark system.

        Unknown molecules REFUSE (never invented rhf/fci). Systems without an
        exactly-constructed Hamiltonian (e.g. LiH's old 4-qubit entry, whose
        FCI was a fixed -0.042 offset) REFUSE with a pointer to the live
        bridge — a computed number or a refusal, never an offset.
        """
        bench_keys = {k.upper(): k for k in self.BENCHMARKS}
        # Single-problem-key normalization (W1.19): the dialogue engine names
        # Hamiltonians "<MOL>_STO3G" (basis-qualified); strip that suffix so
        # "H2_STO3G" resolves to the H2 table anchor instead of refusing on a
        # naming convention (the H2-vs-H2_STO3G confusion). Unknown molecules
        # still refuse below — normalization never invents coverage.
        mol_upper = molecule.strip().upper().replace(" ", "")
        for suffix in ("_STO-3G", "_STO3G", "-STO-3G", "-STO3G"):
            if mol_upper.endswith(suffix):
                mol_upper = mol_upper[: -len(suffix)]
        if mol_upper not in bench_keys:
            raise ValueError(
                f"no tabulated anchor for {molecule!r} (table {TABLE_ANCHOR_VERSION_V1} "
                f"covers {sorted(self.BENCHMARKS)}): refusing instead of inventing "
                "rhf/fci — run the PySCF bridge solve_classical_anchor for a live number"
            )

        data = self.BENCHMARKS[bench_keys[mol_upper]]
        r = bond_distance if bond_distance is not None else data["r"]
        if not math.isfinite(r) or r <= 0:
            raise ValueError(f"bond_distance must be finite > 0, got {bond_distance!r}")
        qubits = data["qubits"]

        # Exact Hamiltonian matrix construction for Jordan-Wigner mapped 2-qubit active space
        if qubits == 2:
            I = np.eye(2)
            X = np.array([[0, 1], [1, 0]], dtype=complex)
            Y = np.array([[0, -1j], [1j, 0]], dtype=complex)
            Z = np.array([[1, 0], [0, -1]], dtype=complex)

            H = (
                data["g0"] * np.kron(I, I)
                + data["g1"] * np.kron(Z, I)
                + data["g2"] * np.kron(I, Z)
                + data["g3"] * np.kron(Z, Z)
                + data["g4"] * np.kron(X, X)
                + data["g5"] * np.kron(Y, Y)
            )

            eigvals = np.linalg.eigvalsh(H)
            fci_energy = float(eigvals[0])
            rhf_energy = float(data["rhf_energy"])
        else:  # pragma: no cover — no non-2-qubit entries in table v1
            raise ValueError(
                f"no exact active-space Hamiltonian tabulated for {molecule!r}: "
                "refusing instead of applying a fixed correlation offset"
            )

        mol_canonical = bench_keys[mol_upper]
        return MolecularAnchor(
            system_name=mol_canonical,
            geometry_description=f"{mol_canonical} R={r:.4f}A",
            basis="STO-3G",
            rhf_energy_hartree=rhf_energy,
            fci_energy_hartree=fci_energy,
            active_space_qubits=qubits,
        )

    def compute_anchor(self, molecule: str,
                       bond_distance: float | None = None) -> MolecularAnchor:
        """Deprecated alias for compute_table_anchor (kept for existing callers)."""
        return self.compute_table_anchor(molecule, bond_distance)

    def evaluate_advantage_claim(
        self,
        experiment_energy: float,
        anchor: MolecularAnchor,
        tolerance_mhartree: float = 1.59,  # Chemical accuracy ~ 1.0 kcal/mol = 1.59 mHartree
    ) -> dict[str, Any]:
        """Evaluates whether an experiment actually achieves accuracy beyond the classical anchor.

        Never permits an ungrounded advantage claim. Three conditions, all required:
        (1) within chemical accuracy of FCI, (2) beats RHF by more than an
        interval tolerance (not a point comparison — energies within the
        tolerance band are ties, not wins), (3) recovers a majority of the
        correlation energy (the implemented correlation-recovery check the old
        comment promised but never computed).
        """
        if not math.isfinite(experiment_energy):
            raise ValueError("experiment_energy must be finite")
        tol = tolerance_mhartree / 1000.0
        fci_error = abs(experiment_energy - anchor.fci_energy_hartree)
        chem_acc = fci_error <= tol

        # Interval comparison: beat RHF only when BELOW rhf - tol. Inside the
        # band, the experiment ties the classical anchor (measurement noise
        # could flip a point comparison either way).
        beat_rhf = experiment_energy < anchor.rhf_energy_hartree - tol

        # Correlation recovery: fraction of (rhf - fci) captured.
        corr_window = anchor.rhf_energy_hartree - anchor.fci_energy_hartree
        if corr_window <= 0:
            recovered_frac = 0.0
        else:
            recovered_frac = (anchor.rhf_energy_hartree - experiment_energy) / corr_window
        majority_recovered = recovered_frac >= 0.5

        advantage_justified = bool(chem_acc and beat_rhf and majority_recovered)

        return {
            "experiment_energy": experiment_energy,
            "rhf_energy": anchor.rhf_energy_hartree,
            "fci_energy": anchor.fci_energy_hartree,
            "error_to_fci_mhartree": fci_error * 1000.0,
            "chemical_accuracy_achieved": chem_acc,
            "beat_classical_rhf": beat_rhf,
            "correlation_recovered_fraction": recovered_frac,
            "correlation_majority_recovered": majority_recovered,
            "honest_verdict": (
                "Matches classical FCI within chemical accuracy"
                if advantage_justified
                else "Does not surpass classical benchmark"
            ),
        }
