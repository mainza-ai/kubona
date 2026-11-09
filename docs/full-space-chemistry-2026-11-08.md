# Full-space chemistry: ADAPT-VQE implementation and measured boundary

Date: 2026-11-08. Scope: bridge simulator (`qiskit-aer-simulator`), full
molecular orbital space (no active-space reduction), STO-3G.

## Implemented path

`platform/bridge/adapt.py` implements real adaptive VQE:

- operator pools:
  - `uccsd`: Qiskit Nature singles/doubles mapped through Jordan-Wigner;
  - `ceo`: generalized qubit singles, opposite-spin OVP-CEO `+`/`-`, and
    same-spin QE/OVP-CEO families;
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

## CEO pool boundary

CEO-ADAPT is implemented and selectable (`full_space_pool: "ceo"`; 750
operators for LiH). On the same LiH point it selected 10 operators and
stalled near `4.66 mHa` exact error; it did **not** reach the target in the
measured window. It remains labelled experimental. The validated full-space
default is the UCCSD pool, not a claim that every CEO construction converges
on every molecule.

## BeH2 boundary

BeH2 (14 qubits) is supported as a full-space job, but it is not claimed:
16 operators reached about `4.16 mHa` exact error and 40 operators about
`3.04 mHa`, still above the 1.6 mHa gate. A shot result that happens to land
inside the gate is rejected unless the exact diagnostic also passes. The
honest status is `resource_exhausted`/`contested`, with the operator count and
pool recorded in the result.

## Limits

- The simulator cost grows with the number of selected operators and Pauli
  terms; the bridge enforces a 900 s default `KUBONA_ADAPT_TIME_LIMIT_S`.
- `KUBONA_ADAPT_MAX_OPS` and `KUBONA_ADAPT_MIN_SHOTS` are operator controls;
  defaults are 8×layers capped at 20 and 262144 shots.
- BeH2 chemical accuracy needs a better pool/optimizer schedule, not a label
  change. Until then the result is a measured, contested full-space run.
