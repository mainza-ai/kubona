"""Literature-derived magic-state factory cost model (diagnostics only).

Unblocks A5 WITHOUT the paper's numbers: the SI only motivates the T-count
proxy via factory throughput (main.tex:1018 — "when there is limited
magic-state-factory throughput ... runtime is bottlenecked by the Toffoli
count") and gives no throughput, routing, or decoder data. This module
therefore wires in *published* factory costs, every number traced below,
labeled literature-derived and NEVER paper-measured.

RANKING POLICY (settled, cf. tq.py POLICY): ranking stays TQ. Everything
here is a measured diagnostic served alongside depth / toffoli_depth /
active_qubits — never a ranking or gating input.

Sources (all numbers below trace to exactly these):
- GF2019: Gidney & Fowler, "Efficient magic state factories with a
  catalyzed |CCZ> to 2|T> transformation", Quantum 3:135 (2019),
  arXiv:1812.01238. CCZ factory: footprint 12d x 6d, one |CCZ> per
  5.5d surface-code cycles; ~1e10 states before failure at p_phys=1e-3
  (distillation-limited; +~20% footprint -> ~1e12 for Shor-scale runs).
- LIT2019: Litinski, "Magic State Distillation: Not As Costly As You
  Think", Quantum 3:205 (2019), arXiv:1905.06903. Space-time cost unit is
  d^3 (qubit x cycles); distillation is ~10% of physical qubits for a
  100-qubit computation at 1e-3/1e-10 — i.e. NOT the dominant cost.
- CCZ2026: "Low Spatial Cost CCZ Magic State Factory", arXiv:2606.24170v1
  (2026). T-factory spatial cost 176d^2 -> 150d^2 at the same 15d time
  (volume 2640d^3 -> 2250d^3, -15%). Used ONLY as a sensitivity bound:
  factory costs move ~15% year-over-year, so point estimates below carry
  that shape of uncertainty. Never a selectable factory (single vintage
  would pretend false precision).
- LIT2023: Litinski, "How to compute a 256-bit elliptic curve private key
  with only 50 million Toffoli gates", arXiv:2306.08585 (2023). Baseline
  (2D-local) architecture cost function n_Q x n_Tof; active-volume
  architecture counts logical blocks with non-local connections instead.
  The n_Q*n_Tof baseline is served here as a diagnostic alongside TQ.

Design rules (production bar):
- No silent physical assumptions: code_distance and cycle_us are REQUIRED
  keyword args. A default d or cycle time would be a silent physics claim.
- Exact where exact: patch counts, state counts, half-cycle ledgers are
  ints. Wall seconds are floats LABELED estimates (a literature model's
  projection, not a measurement).
- Half-cycle unit: 5.5d cycles/CCZ is exact in halves (11d). All cycle
  ledgers are int half-cycles; conversion to seconds divides once, at the
  edge, labeled _est.
"""

from __future__ import annotations

#: Selectable factories. Single shared const — never a second copy.
#: 1 CCZ state per Toffoli-class gate (the paper counts Toffoli + CCZ equally,
#: so states_needed == toffoli exactly, no conversion factor invented).
FACTORIES: dict[str, dict] = {
    "gf2019_ccz": {
        "label": "Gidney-Fowler 2019 CCZ factory",
        "source": "Gidney & Fowler, Quantum 3:135 (2019), arXiv:1812.01238",
        "output": "CCZ",
        "space_patches_per_d2": 72,      # 12d x 6d footprint
        "half_cycles_per_state_per_d": 11,  # 5.5d cycles per |CCZ>
        "states_before_failure": 1e10,   # at p_phys=1e-3, distillation-limited
    },
}


def _require_nonneg_int(name: str, v: object) -> int:
    if isinstance(v, bool) or not isinstance(v, int) or v < 0:
        raise ValueError(f"{name} must be a non-negative int, got {v!r}")
    return v


def _require_positive_number(name: str, v: object) -> float:
    if isinstance(v, bool) or not isinstance(v, (int, float)) or not v > 0:
        raise ValueError(f"{name} must be positive, got {v!r}")
    return float(v)


def factory_report(
    *,
    toffoli: int,
    qubits: int,
    toffoli_depth: int,
    factory: str = "gf2019_ccz",
    code_distance: int,
    cycle_us: float,
) -> dict:
    """Literature-factory diagnostic for a measured (T, Q, depth) triple.

    code_distance and cycle_us are required: no silent physics. Returns
    exact-int ledgers plus labeled float estimates. Never raises on
    literature grounds — only on malformed inputs (fail-closed).
    """
    t = _require_nonneg_int("toffoli", toffoli)
    q = _require_nonneg_int("qubits", qubits)
    _require_nonneg_int("toffoli_depth", toffoli_depth)
    if factory not in FACTORIES:
        raise ValueError(
            f"unknown factory {factory!r}: refusing "
            f"(known: {sorted(FACTORIES)})")
    d = _require_nonneg_int("code_distance", code_distance)
    if d == 0:
        raise ValueError("code_distance must be positive, got 0")
    cycle = _require_positive_number("cycle_us", cycle_us)

    spec = FACTORIES[factory]
    patches = spec["space_patches_per_d2"] * d * d
    half_per_state = spec["half_cycles_per_state_per_d"] * d
    serial_half = t * half_per_state
    serial_seconds_est = (serial_half / 2.0) * cycle * 1e-6
    # Data qubits persist the whole serial run; the factory runs alongside.
    spacetime_qubit_half_cycles = (q + patches) * serial_half
    budget = float(spec["states_before_failure"])
    return {
        "factory": factory,
        "source": spec["source"],
        "output": spec["output"],
        "code_distance": d,
        "cycle_us": cycle,
        "states_needed": t,
        "factory_patches": patches,
        "half_cycles_per_state": half_per_state,
        "serial_half_cycles": serial_half,
        "serial_seconds_est": serial_seconds_est,
        "states_before_failure": budget,
        "within_failure_budget": t <= budget,
        "spacetime_qubit_half_cycles": spacetime_qubit_half_cycles,
        # Litinski-2023 baseline-architecture diagnostic (n_Q x n_Tof),
        # served alongside TQ — never a ranking input.
        "active_volume_baseline": t * q,
        "toffoli_depth": toffoli_depth,
    }
