"""Mathematical Physics Classical Anchors and Analytical Baselines (Ring 2).

Table anchors plus closed-form family evaluations derived from the
`openai/math` program (Family 271 Bloch law/Heisenberg ferromagnet, Family 281
QAOA SK optimality, Family 268 Haldane gap, Family 269 Laughlin gap, Family 266
MUB-6, ...) and two literature qLDPC/neutral-atom reference points.

HONESTY CONTRACT (production bar):
- BENCHMARKS entries are UNATTESTED table constants (source_url + attestation
  fields; lean_verified is False everywhere — no Lean artifact in this repo
  evidences any of them).
- Every compute_family_* evaluation is CLOSED-FORM ARITHMETIC, not a proof:
  each returns evidence_tier + provenance saying so. No "certified True" and
  no "rigorous-mathematical-proof" confidence comes out of arithmetic.
- Out-of-domain inputs REFUSE (no silent clamps); saturating outputs are
  explicitly capped-with-reason, never silently clipped.

Enforces [[quantum-advantage-realism]]: quantum ansatz evaluations
(VQE, QAOA) must benchmark against known exact analytical ground states and bounds.
"""

from __future__ import annotations

import math
from dataclasses import dataclass
from typing import Any, Dict, List, Optional


#: Upstream repository these families are attributed to. Commits are NOT
#: pinned (no SHA evidenced) — hence every entry is attestation "unattested".
OPENAI_MATH_REPO_URL_V1 = "https://github.com/openai/math"

#: Evidence tier stamped on every closed-form family evaluation.
EVIDENCE_TIER_ARITHMETIC_V1 = (
    "T3-arithmetic-heuristic: closed-form evaluation in this function; "
    "NOT a proof, NOT machine-checked"
)
#: Provenance stamped on every closed-form family evaluation.
PROVENANCE_ARITHMETIC_V1 = (
    "closed-form arithmetic in this function; no Lean artifact, no QPU "
    "measurement — a numerical reference, not a theorem"
)

#: Single source for the Laughlin 1/3 V1-interaction gap lower bound
#: (proven >= 1/25 = 0.04 e^2/(eps*l_B)). The table and compute_family_269
#: MUST agree — the old code had 0.106 in the table vs 0.04 here.
LAUGHLIN_V1_GAP_LOWER_BOUND_V1 = 0.04

#: Single source for the 3D spin-1/2 Heisenberg ferromagnet ground-state
#: energy density on the cubic lattice (z=6): E0/(N*J) = -z*J*S^2 / J = -1.5.
#: The old code carried TWO variables (e0_density = -0.75J for S=1/2 via one
#: normalization, canonical_e0 = -1.5J via another) and returned the second —
#: one quantity, one source now.
HEISENBERG_E0_DENSITY_PER_SITE_V1 = -1.5

#: QAOA depth-approach curve shape (v1 heuristic fit, NOT certified).
QAOA_DEPTH_MODEL_LABEL_V1 = (
    "qaoa-depth-heuristic-v1: (1 - 0.26/sqrt(p)) approach curve; "
    "heuristic, not certified"
)

#: Strong-disorder rule-of-thumb threshold (v1 heuristic, NOT a theorem).
STRONG_DISORDER_THRESHOLD_W_V1_HEURISTIC = 16.5

#: Haldane gap value (J units) — table constant, unattested.
HALDANE_GAP_J_V1 = 0.410479

#: SK Parisi energy (per spin) — table constant, unattested.
PARISI_ENERGY_V1 = -0.763166


@dataclass(frozen=True)
class MathPhysicsAnchor:
    system_name: str
    family_id: int
    physical_quantity: str
    analytical_value: float
    unit: str
    proof_paper_slug: str
    bound_type: str  # "exact", "lower_bound", "upper_bound", "gap"
    confidence: str = "analytical-table-unattested"
    lean_verified: bool = False
    notes: str = ""
    source_url: str = ""
    attestation: str = "unattested"

    def to_dict(self) -> Dict[str, Any]:
        return {
            "system_name": self.system_name,
            "family_id": self.family_id,
            "physical_quantity": self.physical_quantity,
            "analytical_value": self.analytical_value,
            "unit": self.unit,
            "proof_paper_slug": self.proof_paper_slug,
            "bound_type": self.bound_type,
            "confidence": self.confidence,
            "lean_verified": self.lean_verified,
            "notes": self.notes,
            "source_url": self.source_url,
            "attestation": self.attestation,
        }


def _table_entry(family_id: int, quantity: str, value: float, unit: str,
                 slug: str, bound: str, notes: str) -> Dict[str, Any]:
    """One unattested table entry: source pinned to repo+family (commit
    unpinned), attestation explicit, lean_verified never claimed."""
    url = f"{OPENAI_MATH_REPO_URL_V1} (family {family_id}; commit unpinned)" \
        if family_id > 0 else ""
    return {
        "family_id": family_id,
        "physical_quantity": quantity,
        "analytical_value": value,
        "unit": unit,
        "proof_paper_slug": slug,
        "bound_type": bound,
        "confidence": "analytical-table-unattested",
        "lean_verified": False,
        "notes": notes,
        "source_url": url,
        "attestation": "unattested",
    }


class MathPhysicsAnchorEngine:
    """Analytical baselines for quantum many-body and optimization models."""

    # Table benchmarks: unattested constants with pinned (repo, family) source
    # and explicit attestation. lean_verified is False for ALL entries: no
    # Lean artifact in this repo evidences any of them.
    BENCHMARKS: Dict[str, Dict[str, Any]] = {
        "Heisenberg_Ferromagnet_3D": _table_entry(
            271, "ground_state_energy_density_J", -1.500000, "J",
            "Bloch-law-and-spontaneous-ferromagnetic-order", "exact",
            "Family 271 Bloch T^(3/2) law reference value (unattested).",
        ),
        "SK_Spin_Glass_Ground_State": _table_entry(
            281, "parisi_ground_state_energy_limit", -0.763166,
            "energy_per_spin", "QAOA-optimality-for-the-SK-model", "exact",
            "Asymptotic Parisi energy, zero-field Gaussian SK (unattested).",
        ),
        "Haldane_Spin_1_Chain": _table_entry(
            268, "haldane_spectral_gap_J", 0.410479, "J",
            "The-spin-one-Haldane-gap", "gap",
            "Haldane gap, pure AFM Heisenberg chain (unattested).",
        ),
        "MUB_Dimension_6_Overlap": _table_entry(
            266, "max_mutually_unbiased_bases_count", 3.0, "bases_count",
            "Exactly-three-mutually-unbiased-bases-in-dimension-six", "exact",
            "N(6) = 3 per Zauner conjecture (unattested).",
        ),
        "Laughlin_Fractional_Hall_Gap": _table_entry(
            269, "laughlin_filling_one_third_spectral_gap",
            LAUGHLIN_V1_GAP_LOWER_BOUND_V1, "e^2/(epsilon*l_B)",
            "The-Laughlin-gap-and-stability-under-scalar-disorder", "lower_bound",
            "Single-sourced with compute_family_269: proven >= 1/25 (unattested).",
        ),
        "QAC0_Parity_Bound": _table_entry(
            274, "qac0_parity_error_lower_bound", 0.333333,
            "error_prob_lower_bound", "Parity-is-not-in-QAC0", "lower_bound",
            "Constant-depth + unbounded Toffoli cannot compute parity (unattested).",
        ),
        "Continuum_Coulomb_QMA_Gap": _table_entry(
            275, "coulomb_ground_energy_promise_gap", 1.000000,
            "Hartree_promise_gap", "QMA-hardness-of-continuum-Coulomb-energy", "gap",
            "Continuum Coulomb ground-energy promise gap (unattested).",
        ),
        "Quartic_Quantum_Query_Exponent": _table_entry(
            284, "randomized_vs_quantum_query_exponent", 4.000000,
            "power_exponent",
            "The-optimal-quartic-separation-between-randomized-and-quantum-queries",
            "exact",
            "R(f) = O((1+Q(f))^4) sharp for total Boolean functions (unattested).",
        ),
        "Bivariate_Bicycle_Gross_Code": _table_entry(
            0, "qldpc_code_rate_k_over_n", 0.083333,
            "logical_per_physical_ratio",
            "High-threshold-and-low-overhead-fault-tolerant-quantum-memory", "exact",
            "Bravyi et al. [[144,12,12]] qLDPC gross code (literature reference, unattested).",
        ),
        "Neutral_Atom_Logical_Qubits_Bluvstein": _table_entry(
            0, "max_encoded_logical_qubits", 48.000000, "logical_qubits",
            "A-logical-quantum-processor-for-fault-tolerant-quantum-computation",
            "exact",
            "Bluvstein et al. 48 logical qubits, neutral-atom arrays (literature reference, unattested).",
        ),
        "Kohn_Sham_Representability_Failure": _table_entry(
            278, "kohn_sham_ensemble_failure_electrons", 3.0,
            "electrons_counterexample",
            "Failure-of-the-Kohn-Sham-ensemble-representation-for-Coulomb-molecules",
            "exact",
            "Non-interacting KS fails for 3-electron equal-charge Coulomb molecules (unattested).",
        ),
        "Area_Law_2D_PEPS_Bound": _table_entry(
            265, "peps_approximation_vector_error_exponent", -1.0,
            "length_power_exponent",
            "A-polynomial-area-law-for-gapped-two-dimensional-systems", "upper_bound",
            "2D gapped area law; poly bond-dimension PEPS vector error <= L^-1 (unattested).",
        ),
        "Clifford_T_Factoring_Uniform": _table_entry(
            279, "exact_factoring_ancilla_count", 0.0, "ancillas",
            "Uniform-exact-quantum-factoring-over-a-fixed-finite-gate-set", "exact",
            "Uniform Clifford+T factoring without ancillas (unattested).",
        ),
        "Generalized_Amplitude_Damping_Capacity": _table_entry(
            276, "unassisted_classical_capacity_bits", 1.0,
            "bits_per_channel_use_ceiling",
            "The-classical-capacity-of-generalized-amplitude-damping-channels",
            "exact",
            "Classical capacity of GAD channels (unattested).",
        ),
        "Bound_Entanglement_No_Key": _table_entry(
            272, "distillable_secret_key_rate", 0.0, "secret_key_bits",
            "Entanglement-without-distillable-secret-key-and-counterexamples",
            "exact",
            "C^10 x C^10 bound entangled state, 0 distillable key (unattested).",
        ),
        "Threshold_Repetition_Entangled_Games": _table_entry(
            277, "parallel_repetition_threshold_exponent", 1.0,
            "exponential_decay_order",
            "Threshold-repetition-for-entangled-games", "upper_bound",
            "Exponential threshold repetition, two-player entangled games (unattested).",
        ),
        "Unitary_Synthesis_Boolean_Oracle": _table_entry(
            283, "diamond_norm_approximation_error", 0.5, "diamond_norm_bound",
            "Polynomial-time-unitary-synthesis-from-a-Boolean-oracle", "upper_bound",
            "Aaronson-Kuperberg unitary synthesis within diamond-norm 1/2 (unattested).",
        ),
        "Anderson_Localization_2D_3D": _table_entry(
            261, "two_dimension_absolutely_continuous_spectrum", 0.0,
            "spectral_measure",
            "Localization-and-delocalization-in-the-Anderson-model", "exact",
            "Pure point spectrum in 2D for all disorder (unattested).",
        ),
        "Maximum_Electronic_Binding_Limit": _table_entry(
            263, "neutral_atom_max_bound_excess_electrons", 1.0,
            "excess_electrons",
            "Proof-of-the-ionization-and-generalized-ionization-conjectures",
            "upper_bound",
            "Neutral atoms bind at most one excess electron (unattested).",
        ),
        "Positive_Temperature_BEC_Depletion": _table_entry(
            267, "bogoliubov_quantum_depletion_fraction", 0.080000,
            "depletion_fraction",
            "Positive-temperature-Bose-Einstein-condensation-and-quantum-depletion",
            "exact",
            "Bogoliubov leading quantum depletion, 3D hard-sphere Bose gas (unattested).",
        ),
        "BFSS_Matrix_Model_Zero_Energy": _table_entry(
            270, "bfss_ground_state_energy_eigenvalue", 0.0, "eigenvalue",
            "Threshold-and-positive-energy-bound-states-of-the-BFSS-matrix-model",
            "exact",
            "Unique zero-energy state, relative SU(N) BFSS, all N >= 2 (unattested).",
        ),
        "Free_Group_Factors_Isomorphism": _table_entry(
            287, "interpolated_fundamental_group_isomorphism", 1.0,
            "isomorphism_indicator",
            "Isomorphism-of-the-free-group-factors", "exact",
            "L(F_2) isomorphic to L(F_3) (unattested).",
        ),
    }

    @classmethod
    def get_anchor(cls, system_name: str) -> Optional[MathPhysicsAnchor]:
        data = cls.BENCHMARKS.get(system_name)
        if not data:
            return None
        return MathPhysicsAnchor(
            system_name=system_name,
            family_id=data["family_id"],
            physical_quantity=data["physical_quantity"],
            analytical_value=data["analytical_value"],
            unit=data["unit"],
            proof_paper_slug=data["proof_paper_slug"],
            bound_type=data["bound_type"],
            confidence=data.get("confidence", "analytical-table-unattested"),
            lean_verified=bool(data.get("lean_verified", False)),
            notes=data.get("notes", ""),
            source_url=data.get("source_url", ""),
            attestation=data.get("attestation", "unattested"),
        )

    @classmethod
    def list_benchmarks(cls) -> List[str]:
        return list(cls.BENCHMARKS.keys())

    @classmethod
    def compare_vqe_result(
        cls, system_name: str, computed_energy: float
    ) -> Dict[str, Any]:
        """Compares a numerical VQE or QAOA calculation against the proof anchor.

        ``variational_principle_satisfied`` is None when the anchor is not an
        energy bound (gap / count / rate quantities have no variational
        principle — the old code returned True there, a meaningless pass).
        """
        anchor = cls.get_anchor(system_name)
        if not anchor:
            raise ValueError(f"Unknown mathematical physics benchmark system: {system_name}")
        if not math.isfinite(computed_energy):
            raise ValueError("computed_energy must be finite")

        diff = computed_energy - anchor.analytical_value
        rel_error = abs(diff) / max(abs(anchor.analytical_value), 1e-12)

        is_variational_valid: Optional[bool] = None
        if anchor.bound_type == "exact" and "energy" in anchor.physical_quantity:
            # Variational principle: computed energy should be >= ground state energy
            # Allow tiny epsilon for numerical tolerance
            is_variational_valid = computed_energy >= (anchor.analytical_value - 1e-5)

        return {
            "system_name": system_name,
            "computed_value": computed_energy,
            "analytical_anchor": anchor.analytical_value,
            "difference": diff,
            "relative_error": rel_error,
            "variational_principle_satisfied": is_variational_valid,
            "anchor_metadata": anchor.to_dict(),
        }

    # =========================================================================
    # CLOSED-FORM FAMILY EVALUATIONS (arithmetic references, NOT proofs)
    # Every method returns evidence_tier + provenance. Out-of-domain inputs
    # refuse; saturating outputs are capped-with-reason, never silently clipped.
    # =========================================================================

    @staticmethod
    def _tier() -> Dict[str, str]:
        return {"evidence_tier": EVIDENCE_TIER_ARITHMETIC_V1,
                "provenance": PROVENANCE_ARITHMETIC_V1}

    @classmethod
    def compute_family_271_heisenberg_ferromagnet(
        cls, temperature: float = 0.0, J: float = 1.0, spin: float = 0.5
    ) -> Dict[str, Any]:
        """Family 271: 3D Heisenberg Ferromagnet Bloch T^(3/2) law and ground state.

        E0/(NJ) = -1.5 J on simple cubic lattice (z=6, S=1/2), single-sourced
        from HEISENBERG_E0_DENSITY_PER_SITE_V1.
        Bloch law: (M(0) - M(T)) / M(0) = a_0 * (k_B T / (2 J S))^(3/2)
        where a_0 = zeta(3/2) / (8 * pi^(3/2) * S)
        """
        if not math.isfinite(temperature) or temperature < 0:
            raise ValueError(f"temperature must be finite >= 0, got {temperature!r}")
        if not math.isfinite(J) or J <= 0:
            raise ValueError(f"J must be finite > 0 (ferromagnetic convention), got {J!r}")
        if not math.isfinite(spin) or spin <= 0:
            raise ValueError(f"spin must be finite > 0, got {spin!r}")
        zeta_3_2 = 2.612375348685
        a_0 = zeta_3_2 / (8.0 * (math.pi ** 1.5) * spin)

        canonical_e0 = HEISENBERG_E0_DENSITY_PER_SITE_V1 * J

        k_B = 1.0  # natural units
        depletion = 0.0
        if temperature > 0:
            scale = (k_B * temperature) / (2.0 * J * spin)
            depletion = a_0 * (scale ** 1.5)

        capped_reason: Optional[str] = None
        if depletion > 1.0:
            # Bloch law validity ends at full depletion: cap EXPLICITLY with
            # reason instead of the old silent min(depletion, 1.0).
            capped_reason = (
                "Bloch T^(3/2) low-T expansion exceeds its validity "
                f"(raw depletion {depletion:.3f} > 1); capped at full depletion"
            )
            depletion = 1.0

        out = {
            "family_id": 271,
            "system_name": "Heisenberg_Ferromagnet_3D",
            "ground_state_energy_density_J": canonical_e0,
            "bloch_prefactor_a0": a_0,
            "temperature": temperature,
            "magnetization_depletion": depletion,
            "depletion_capped_with_reason": capped_reason,
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_281_sk_spin_glass(
        cls, qaoa_p_depth: int = 1
    ) -> Dict[str, Any]:
        """Family 281: Sherrington-Kirkpatrick Spin Glass Parisi energy and QAOA curve.

        The depth-approach curve is QAOA_DEPTH_MODEL_LABEL_V1 (heuristic fit) —
        the old code stamped it "certified True", which arithmetic cannot do.
        """
        if isinstance(qaoa_p_depth, bool) or not isinstance(qaoa_p_depth, (int,)) \
                or qaoa_p_depth < 1:
            raise ValueError(
                f"qaoa_p_depth must be a positive int, got {qaoa_p_depth!r} "
                "(refusing instead of clamping to 1)"
            )
        parisi_energy = -0.7631668211
        # Asymptotic QAOA energy approximation at depth p (HEURISTIC fit).
        p = qaoa_p_depth
        qaoa_approx_energy = parisi_energy * (1.0 - 0.26 / math.sqrt(p))
        approx_ratio = qaoa_approx_energy / parisi_energy

        out = {
            "family_id": 281,
            "system_name": "SK_Spin_Glass_Ground_State",
            "parisi_ground_state_energy": parisi_energy,
            "qaoa_p_depth": p,
            "qaoa_expected_energy": qaoa_approx_energy,
            "asymptotic_approximation_ratio": approx_ratio,
            "qaoa_depth_model": QAOA_DEPTH_MODEL_LABEL_V1,
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_268_haldane_gap(
        cls, J: float = 1.0
    ) -> Dict[str, Any]:
        """Family 268: Haldane Spin-1 AFM Chain Uniform Spectral Gap and String Order."""
        if not math.isfinite(J) or J <= 0:
            raise ValueError(f"J must be finite > 0 (AFM convention), got {J!r}")
        gap_value = HALDANE_GAP_J_V1 * J
        string_order_parameter = 0.374325  # Non-zero topological string order
        out = {
            "family_id": 268,
            "system_name": "Haldane_Spin_1_Chain",
            "haldane_spectral_gap_J": gap_value,
            "topological_string_order_index": -1,
            "string_order_parameter": string_order_parameter,
            "has_uniform_spectral_gap": True,
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_278_kohn_sham_failure(
        cls, num_electrons: int = 3, nuclear_charges: Optional[List[int]] = None
    ) -> Dict[str, Any]:
        """Family 278: Kohn-Sham Ensemble Representation Failure for Coulomb Molecules."""
        if isinstance(num_electrons, bool) or not isinstance(num_electrons, int) \
                or num_electrons < 1:
            raise ValueError(f"num_electrons must be a positive int, got {num_electrons!r}")
        charges = list(nuclear_charges) if nuclear_charges is not None else [1, 1, 1]
        if not charges or any(isinstance(c, bool) or not isinstance(c, int) or c < 1
                              for c in charges):
            raise ValueError("nuclear_charges must be a non-empty list of positive ints")
        is_counterexample = (num_electrons == 3 and len(charges) >= 3 and all(c == charges[0] for c in charges))
        out = {
            "family_id": 278,
            "system_name": "Kohn_Sham_Representability_Failure",
            "num_electrons": num_electrons,
            "nuclear_charges": charges,
            "fails_noninteracting_representability": is_counterexample,
            "function_space": "L^{3/2} + L^\\infty",
            "justification": "Non-interacting single-determinant representation provably fails for 3-electron equal-charge Coulomb system.",
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_275_continuum_coulomb_qma_gap(
        cls, promise_gap_hartree: float = 1.0
    ) -> Dict[str, Any]:
        """Family 275: QMA-Hardness of Continuum Electronic Coulomb Ground State."""
        if not math.isfinite(promise_gap_hartree) or promise_gap_hartree < 0:
            raise ValueError(f"promise_gap_hartree must be finite >= 0, got {promise_gap_hartree!r}")
        is_qma_hard = promise_gap_hartree >= 1.0
        out = {
            "family_id": 275,
            "system_name": "Continuum_Coulomb_QMA_Gap",
            "promise_gap_hartree": promise_gap_hartree,
            "is_qma_hard": is_qma_hard,
            "complexity_class": "QMA-complete",
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_279_clifford_t_factoring(
        cls, bit_length: int = 64
    ) -> Dict[str, Any]:
        """Family 279: Uniform Exact Quantum Factoring over Fixed Finite Gate Set."""
        if isinstance(bit_length, bool) or not isinstance(bit_length, int) or bit_length < 1:
            raise ValueError(f"bit_length must be a positive int, got {bit_length!r}")
        out = {
            "family_id": 279,
            "system_name": "Clifford_T_Factoring_Uniform",
            "bit_length": bit_length,
            "ancilla_qubits": 0,
            "gate_set": "Clifford+T",
            "t_depth_complexity": f"O({bit_length}^3)",
            "exact_success_probability": 1.0,
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_274_qac0_parity_bound(
        cls, circuit_depth: int = 5, num_qubits: int = 100
    ) -> Dict[str, Any]:
        """Family 274: Parity is Not in QAC^0 (Moore's Parity Conjecture)."""
        if isinstance(circuit_depth, bool) or not isinstance(circuit_depth, int) \
                or circuit_depth < 1:
            raise ValueError(f"circuit_depth must be a positive int, got {circuit_depth!r}")
        if isinstance(num_qubits, bool) or not isinstance(num_qubits, int) or num_qubits < 1:
            raise ValueError(f"num_qubits must be a positive int, got {num_qubits!r}")
        min_depth_required = max(1, math.ceil(math.log2(num_qubits) / max(1.0, math.log2(max(2.0, math.log2(num_qubits))))))
        error_lower_bound = 1.0 / 3.0
        out = {
            "family_id": 274,
            "system_name": "QAC0_Parity_Bound",
            "circuit_depth": circuit_depth,
            "num_qubits": num_qubits,
            "computes_parity_reliably": circuit_depth >= min_depth_required,
            "error_prob_lower_bound": error_lower_bound,
            "in_qac0": False,
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_284_quartic_query_separation(
        cls, quantum_queries: int = 10
    ) -> Dict[str, Any]:
        """Family 284: Optimal Quartic Separation between Randomized and Quantum Queries."""
        if isinstance(quantum_queries, bool) or not isinstance(quantum_queries, int) \
                or quantum_queries < 0:
            raise ValueError(
                f"quantum_queries must be a non-negative int, got {quantum_queries!r} "
                "(refusing instead of clamping to 0)"
            )
        q = quantum_queries
        randomized_bound = (1 + q) ** 4
        out = {
            "family_id": 284,
            "system_name": "Quartic_Quantum_Query_Exponent",
            "quantum_queries": q,
            "randomized_query_upper_bound": randomized_bound,
            "optimal_exponent": 4.0,
            "refutes_cubic_relation": True,
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_265_peps_area_law_bound(
        cls, linear_size_L: float = 10.0, bond_dimension: int = 4
    ) -> Dict[str, Any]:
        """Family 265: 2D Gapped Area Law & Polynomial PEPS Vector Error Bound."""
        if not math.isfinite(linear_size_L) or linear_size_L <= 0:
            raise ValueError(
                f"linear_size_L must be finite > 0, got {linear_size_L!r} "
                "(refusing instead of clamping to 1.0)"
            )
        if isinstance(bond_dimension, bool) or not isinstance(bond_dimension, int) \
                or bond_dimension < 1:
            raise ValueError(f"bond_dimension must be a positive int, got {bond_dimension!r}")
        L = linear_size_L
        truncation_error_bound = 1.0 / L
        out = {
            "family_id": 265,
            "system_name": "Area_Law_2D_PEPS_Bound",
            "linear_size_L": L,
            "bond_dimension": bond_dimension,
            "peps_vector_error_bound": truncation_error_bound,
            "area_law_satisfied": True,
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_266_mub_dimension_6(cls) -> Dict[str, Any]:
        """Family 266: Exactly Three Mutually Unbiased Bases in Dimension Six (Zauner's Conjecture)."""
        out = {
            "family_id": 266,
            "system_name": "MUB_Dimension_6_Overlap",
            "dimension": 6,
            "max_mutually_unbiased_bases": 3,
            "fourth_mub_exists": False,
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_269_laughlin_fractional_gap(
        cls, filling_factor: float = 1.0 / 3.0
    ) -> Dict[str, Any]:
        """Family 269: Laughlin Spectral Gap at Filling 1/3 under V1 interaction.

        Single-sourced from LAUGHLIN_V1_GAP_LOWER_BOUND_V1 (same value as the
        table entry — the old code disagreed with its own table).
        """
        if not math.isfinite(filling_factor) or filling_factor <= 0:
            raise ValueError(f"filling_factor must be finite > 0, got {filling_factor!r}")
        v1_gap = LAUGHLIN_V1_GAP_LOWER_BOUND_V1  # >= 1/25 in e^2 / (epsilon * l_B) units
        out = {
            "family_id": 269,
            "system_name": "Laughlin_Fractional_Hall_Gap",
            "filling_factor": filling_factor,
            "v1_interaction_spectral_gap_lower_bound": v1_gap,
            "quasiparticle_charge": 1.0 / 3.0,
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_276_generalized_amplitude_damping(
        cls, gamma: float = 0.1, thermal_N: float = 0.05
    ) -> Dict[str, Any]:
        """Family 276: Classical Capacity of Generalized Amplitude Damping Channels."""
        if not math.isfinite(gamma) or not 0.0 <= gamma <= 1.0:
            raise ValueError(
                f"gamma must be in [0, 1], got {gamma!r} (refusing instead of clamping)"
            )
        if not math.isfinite(thermal_N) or thermal_N < 0:
            raise ValueError(f"thermal_N must be finite >= 0, got {thermal_N!r}")
        # Binary entropy helper (epsilon guard is a LOG-DOMAIN numeric guard,
        # documented here — not a clamp of caller inputs, which are validated above).
        def h2(p: float) -> float:
            p = max(1e-12, min(1.0 - 1e-12, p))
            return -p * math.log2(p) - (1.0 - p) * math.log2(1.0 - p)

        p_eff = 0.5 * (1.0 - math.sqrt(max(0.0, 1.0 - 4.0 * gamma * (1.0 - gamma) * thermal_N * (1.0 - thermal_N))))
        capacity = 1.0 - h2(p_eff)
        t1 = 1.0 / max(1e-9, gamma * (2.0 * thermal_N + 1.0))
        t2 = 2.0 * t1

        out = {
            "family_id": 276,
            "system_name": "Generalized_Amplitude_Damping_Capacity",
            "gamma": gamma,
            "thermal_parameter_N": thermal_N,
            "classical_capacity_bits": round(capacity, 6),
            "relaxation_time_t1": round(t1, 4),
            "dephasing_time_t2": round(t2, 4),
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_272_bound_entanglement_secret_key(
        cls, dimension: int = 10
    ) -> Dict[str, Any]:
        """Family 272: Bound Entanglement on C^10 x C^10 with Zero Distillable Secret Key."""
        if isinstance(dimension, bool) or not isinstance(dimension, int) or dimension != 10:
            raise ValueError(
                f"dimension must be exactly 10 (the construction is C^10 x C^10-specific), "
                f"got {dimension!r}"
            )
        out = {
            "family_id": 272,
            "system_name": "Bound_Entanglement_No_Key",
            "dimension": dimension,
            "distillable_secret_key_rate": 0.0,
            "entanglement_of_formation_positive": True,
            "disproves_christandl_ppt_square": True,
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_277_threshold_repetition_entangled_games(
        cls, value_g: float = 0.8, repetitions: int = 10
    ) -> Dict[str, Any]:
        """Family 277: Exponential Threshold Repetition for Two-Player Entangled Games."""
        if not math.isfinite(value_g) or not 0.0 <= value_g <= 1.0:
            raise ValueError(
                f"value_g must be in [0, 1], got {value_g!r} (refusing instead of clamping)"
            )
        if isinstance(repetitions, bool) or not isinstance(repetitions, int) \
                or repetitions < 1:
            raise ValueError(
                f"repetitions must be a positive int, got {repetitions!r} "
                "(refusing instead of clamping to 1)"
            )
        val = value_g
        k = repetitions
        decay_rate = math.exp(-0.5 * k * (1.0 - val))
        out = {
            "family_id": 277,
            "system_name": "Threshold_Repetition_Entangled_Games",
            "single_round_value": val,
            "repetitions_k": k,
            "entangled_value_upper_bound": round(decay_rate, 6),
            "decay_type": "exponential",
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_283_unitary_synthesis_oracle(
        cls, dim: int = 8, precision_epsilon: float = 0.5
    ) -> Dict[str, Any]:
        """Family 283: Polynomial-Time Unitary Synthesis from Boolean Oracle within Diamond-Norm 1/2."""
        if isinstance(dim, bool) or not isinstance(dim, int) or dim < 2:
            raise ValueError(f"dim must be an int >= 2, got {dim!r}")
        if not math.isfinite(precision_epsilon) or not 0 < precision_epsilon <= 1:
            raise ValueError(f"precision_epsilon must be in (0, 1], got {precision_epsilon!r}")
        out = {
            "family_id": 283,
            "system_name": "Unitary_Synthesis_Boolean_Oracle",
            "dimension": dim,
            "diamond_norm_approximation_bound": precision_epsilon,
            "circuit_complexity": "poly(n)",
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_261_anderson_localization(
        cls, spatial_dimension: int = 2, disorder_strength: float = 1.0
    ) -> Dict[str, Any]:
        """Family 261: Anderson Localization and Delocalization in 2D and 3D.

        The W >= 16.5 strong-disorder rule is STRONG_DISORDER_THRESHOLD_W_V1
        (heuristic, not a theorem) — labeled on the output with its reason.
        """
        if isinstance(spatial_dimension, bool) or spatial_dimension not in (1, 2, 3):
            raise ValueError(
                f"spatial_dimension must be 1, 2 or 3 (proven regimes), "
                f"got {spatial_dimension!r}"
            )
        dim = spatial_dimension
        W = disorder_strength
        if not math.isfinite(W) or W < 0:
            raise ValueError(f"disorder_strength must be finite >= 0, got {W!r}")
        is_pure_point = (dim <= 2) or (W >= STRONG_DISORDER_THRESHOLD_W_V1_HEURISTIC)
        spec_type = "pure_point_localized" if is_pure_point else "absolutely_continuous_metallic"
        out = {
            "family_id": 261,
            "system_name": "Anderson_Localization_2D_3D",
            "spatial_dimension": dim,
            "disorder_strength": W,
            "spectral_type": spec_type,
            "absolutely_continuous_spectrum_measure": 0.0 if is_pure_point else 1.0,
            "disorder_threshold_model": (
                f"strong-disorder-heuristic-v1: 3D metallic-to-localized call uses "
                f"W >= {STRONG_DISORDER_THRESHOLD_W_V1_HEURISTIC} as a rule of thumb "
                "(NOT a theorem; 1D/2D localization is the proven part)"
            ),
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_263_atomic_ionization_cap(
        cls, atomic_number_Z: int = 6
    ) -> Dict[str, Any]:
        """Family 263: Proof of Ionization and Universal Binding Limit for Neutral Atoms."""
        if isinstance(atomic_number_Z, bool) or not isinstance(atomic_number_Z, int) \
                or atomic_number_Z < 1:
            raise ValueError(f"atomic_number_Z must be a positive int, got {atomic_number_Z!r}")
        max_bound = atomic_number_Z + 1
        out = {
            "family_id": 263,
            "system_name": "Maximum_Electronic_Binding_Limit",
            "atomic_number_Z": atomic_number_Z,
            "max_bound_electrons": max_bound,
            "max_excess_electrons": 1,
            "universal_bound_holds": True,
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_267_bec_depletion_fraction(
        cls, gas_parameter_rho_a3: float = 1e-4
    ) -> Dict[str, Any]:
        """Family 267: Bogoliubov Quantum Depletion in 3D Hard-Sphere Bose Gas."""
        if not math.isfinite(gas_parameter_rho_a3) or gas_parameter_rho_a3 < 0:
            raise ValueError(
                f"gas_parameter_rho_a3 must be finite >= 0, got {gas_parameter_rho_a3!r} "
                "(refusing instead of clamping to 0)"
            )
        prefactor = 8.0 / (3.0 * math.sqrt(math.pi))  # ~ 1.5045
        depletion = prefactor * math.sqrt(gas_parameter_rho_a3)
        capped_reason: Optional[str] = None
        if depletion > 1.0:
            # Bogoliubov theory is dilute-gas only: cap EXPLICITLY with reason.
            capped_reason = (
                "Bogoliubov leading-order law exceeds its dilute-gas validity "
                f"(raw depletion {depletion:.3f} > 1); capped at full depletion"
            )
            depletion = 1.0
        out = {
            "family_id": 267,
            "system_name": "Positive_Temperature_BEC_Depletion",
            "gas_parameter_rho_a3": gas_parameter_rho_a3,
            "bogoliubov_quantum_depletion_fraction": round(depletion, 6),
            "depletion_capped_with_reason": capped_reason,
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_270_bfss_matrix_model(
        cls, gauge_group_N: int = 2
    ) -> Dict[str, Any]:
        """Family 270: Threshold and Positive-Energy Bound States of the BFSS Matrix Model."""
        if isinstance(gauge_group_N, bool) or not isinstance(gauge_group_N, int) \
                or gauge_group_N < 2:
            raise ValueError(
                f"gauge_group_N must be an int >= 2 (proven regime), got {gauge_group_N!r}"
            )
        out = {
            "family_id": 270,
            "system_name": "BFSS_Matrix_Model_Zero_Energy",
            "gauge_group": f"SU({gauge_group_N})",
            "ground_state_energy": 0.0,
            "unique_zero_energy_state": True,
            "infinite_positive_energy_states_su2": gauge_group_N == 2,
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_287_free_group_factor_isomorphism(cls) -> Dict[str, Any]:
        """Family 287: Isomorphism of Free Group Factors L(F_2) ~= L(F_3)."""
        out = {
            "family_id": 287,
            "system_name": "Free_Group_Factors_Isomorphism",
            "isomorphic_factors": ["L(F_2)", "L(F_3)"],
            "interpolated_free_group_factors_isomorphic": True,
            "fundamental_group": "R_{>0}",
            "confidence": "heuristic-arithmetic (NOT a proof; closed-form evaluation)",
        }
        out.update(cls._tier())
        return out

    @classmethod
    def compute_family_engine(cls, family_id: int, **kwargs) -> Dict[str, Any]:
        """Unified dispatch for all 20 analytical anchor families."""
        dispatchers = {
            271: cls.compute_family_271_heisenberg_ferromagnet,
            281: cls.compute_family_281_sk_spin_glass,
            268: cls.compute_family_268_haldane_gap,
            278: cls.compute_family_278_kohn_sham_failure,
            275: cls.compute_family_275_continuum_coulomb_qma_gap,
            279: cls.compute_family_279_clifford_t_factoring,
            274: cls.compute_family_274_qac0_parity_bound,
            284: cls.compute_family_284_quartic_query_separation,
            265: cls.compute_family_265_peps_area_law_bound,
            266: lambda **kw: cls.compute_family_266_mub_dimension_6(),
            269: cls.compute_family_269_laughlin_fractional_gap,
            276: cls.compute_family_276_generalized_amplitude_damping,
            272: cls.compute_family_272_bound_entanglement_secret_key,
            277: cls.compute_family_277_threshold_repetition_entangled_games,
            283: cls.compute_family_283_unitary_synthesis_oracle,
            261: cls.compute_family_261_anderson_localization,
            263: cls.compute_family_263_atomic_ionization_cap,
            267: cls.compute_family_267_bec_depletion_fraction,
            270: cls.compute_family_270_bfss_matrix_model,
            287: lambda **kw: cls.compute_family_287_free_group_factor_isomorphism(),
        }
        if family_id not in dispatchers:
            raise ValueError(f"Unknown family ID {family_id}. Must be one of {list(dispatchers.keys())}.")
        return dispatchers[family_id](**kwargs)
