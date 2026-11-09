# Full-space chemistry: ADAPT-VQE implementation and measured boundary

Date: 2026-11-08. Scope: bridge simulator (`qiskit-aer-simulator`), full
molecular orbital space (no active-space reduction), STO-3G.

## Implemented path

`platform/bridge/adapt.py` implements real adaptive VQE:

- operator pools:
  - `uccsd`: Qiskit Nature singles/doubles mapped through Jordan-Wigner;
  - `ceo`: generalized qubit singles, opposite-spin OVP-CEO `+`/`-`, and
    same-spin QE/OVP-CEO families, built on the **block** spin-orbital layout
    (alpha 0..nso-1, beta nso..2nso-1);
- exact energy gradients at the current state;
- explicit operator selection, full-parameter COBYLA optimization, and
  monotonic-energy protection;
- caps for operators, iterations, wall time, and gradient norm;
- final energy measured by shot sampling in qubit-wise-commuting Pauli groups
  with per-term standard error;
- `chemical_accuracy`/`claimable` require all of: measured delta <= 1.6 mHa,
  shot uncertainty <= 1.6 mHa, and exact statevector delta <= 1.6 mHa.

The last condition prevents a noisy shot fluctuation from turning a
non-converged variational state into a chemical-accuracy claim. The exact
statevector value remains a labelled optimizer diagnostic; it is never
substituted for `energy_measured`.

## Measured LiH result

Atom: `Li 0 0 0; H 0 0 1.5`, 12 qubits, UCCSD ADAPT pool, 16 selected
operators, 262144 shots, seed 7.

| quantity | value |
|---|---|
| exact energy | `-7.8821495569 Ha` (diagnostic) |
| shot-measured energy | `-7.8816566228 Ha` (derived from FCI + measured delta) |
| FCI anchor | `-7.882362286798725 Ha` |
| measured delta | `0.7057 mHa` |
| shot uncertainty | `0.5130 mHa` |
| exact delta | `0.2127 mHa` |
| status | `within_target` |
| wall time | ~100 s |

The exact energy diagnostic for this run is `-7.8821495569 Ha` (FCI +
0.0002127 Ha). The earlier 12-operator run reached `1.0040 mHa` measured /
`0.4279 mHa` exact; the 16-operator run is the current validated result.

This is a simulator result, not hardware and not a quantum-advantage claim.
The hardware H2/QAOA evidence lives in
`docs/production-unblock-plan-2026-11-08.md`.

## CEO pool boundary — corrected 2026-11-09

The 2026-11-08 CEO result was invalid: `_ceo_pool` assumed interleaved
even/odd spin orbitals, but the pinned Qiskit Nature Jordan-Wigner layout is
block-ordered (alpha block, then beta block). The pool therefore omitted the
opposite-spin excitation family that carries the leading correlation and
stalled near `4.66 mHa` exact error.

The layout is now derived from `num_spatial_orbitals` and asserted against
the HF determinant's block occupation. On the same LiH point, CEO-ADAPT
selected 20 operators and reached:

| quantity | value |
|---|---|
| exact energy | `-7.8823223741 Ha` (diagnostic) |
| exact delta | `0.0399 mHa` |
| measured delta | `0.4499 mHa` |
| shot uncertainty | `0.5219 mHa` |
| shots | `262144` |
| status | `within_target` |
| wall time | ~63 s |

CEO is now the default full-space pool (`full_space_pool:"ceo"`); the UCCSD
pool remains selectable as the validated reference. Both are measured, and
the claim gate still requires measured delta, uncertainty, and exact delta to
pass. This does not make every CEO construction converge on every molecule;
the pool and selected operators are reported with the result.

## BeH2 boundary

BeH2 (14 qubits, 7 spatial orbitals) is supported as a full-space job. After
the 2026-11-09 spin-block fix, the CEO pool selected 40 operators and reached:

| quantity | value |
|---|---|
| exact delta | `0.3933 mHa` |
| measured delta | `0.0146 mHa` |
| shot uncertainty | `0.7924 mHa` |
| shots | `262144` |
| status | `within_target` |
| wall time | ~299 s |

The earlier `3.23 mHa` BeH2 result came from the wrong spin layout and is
superseded. A shot result that happens to land inside the gate is still
rejected unless the exact diagnostic also passes; the operator count and pool
are recorded with every result.

## Limits

- The simulator cost grows with the number of selected operators and Pauli
  terms; the bridge enforces a 900 s default `KUBONA_ADAPT_TIME_LIMIT_S`.
- `KUBONA_ADAPT_MAX_OPS` and `KUBONA_ADAPT_MIN_SHOTS` are operator controls;
  the default operator cap is 40 (capped at 64) and the default measurement
  floor is 262144 shots.
- BeH2 chemical accuracy is measured on the corrected CEO pool at 40
  operators. Resource/time budgets remain enforced; a run that misses the gate
  stays contested.
