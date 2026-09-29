---
name: Claim Credibility Wavefunctions
type: concept
tags: [frontier, epistemics, provenance, literature, assertion-gating]
date: 2026-09-27
updated: 2026-09-27
status: open-question
sources: [source-frontier-repo-audit-2026]
---

# Claim Credibility Wavefunctions

**Definition.** Every literature-derived statement Kubona may repeat is stored as a *claim
record* carrying a three-level state over `{corroborated, contested, unverified}`. A claim
may be **asserted** in an artefact only when its state has *collapsed* — and collapse
requires both a peaked state **and** a minimum evidence strength. Un-collapsed claims are
still usable, but only as explicitly uncertain probes.

This is Ring 1 of [[five-ring-research-loop]] and it is the mechanism that lets Kubona be
unapologetically speculative in *generation* while being unconditionally strict in
*assertion*.

## What exists (verified, (repo-audit-2026))

`chickensoup/src/quantum_credibility/wavefunction.py` (344 LOC) already implements this as a
working decision procedure:

| Piece | Location | Detail |
|---|---|---|
| Basis states | `:17` | `STATE_LABELS = ["corroborated", "contested", "unverified"]` |
| Collapse threshold | `:16` | `COLLAPSE_THRESHOLD = 0.75` |
| Evidence signals | `:20-:96` | source diversity (`platforms/5`, capped), engagement magnitude (`log1p`, capped at 10 k), market prior, contradiction signal, social traction with half-life decay |
| Amplitude build | `:98-:156` | unnormalised amplitudes per basis state, floored at `0.01` so no state is ever exactly zero |
| Quantum routing | `:201` | `score_claim_state(amplitudes)` → real Qiskit path |
| Honest degradation | `:213-220` | on failure: Born-rule fallback and `vqe_backend = "classical-fallback: {exc}"` **returned in the result** |
| Epistemic confidence | `:235-:246` | weighted in `p_corr`, `1-p_unver`, `1-p_cont`, diversity, prior; contested states pulled toward 0.5 |
| **Evidence-strength gate** | `:253-269` | a peaked state is *un-collapsed* unless `diversity > 0.3` or `n_evidence ≥ 3` or a market prior exists or `engagement > 0.4` |
| Observability | `:305` | Prometheus `wavefunction_state_total{state, collapsed}` |

The quantum part is genuine but small: `build_claim_state_circuit`
(`src/spacetime_engine/vqe_runner.py:48-112`) encodes three amplitudes on 2 qubits with
hand-rolled `ry`/`x`/`cx`, and `run_vqe_estimation` (`:119-215`) walks
`AerEstimatorV2 → Statevector → Aer-statevector → numpy-fallback`, always reporting the
answering `backend`. `entanglement.py:10` computes a real Meyer–Wallach
`Q = 1 - (1/n)Σ Tr(ρ_k²)`; `:50` ships a *labelled* classical proxy (normalised Shannon
entropy) for when no statevector exists. The **contradiction signal is a stub**
(`wavefunction.py:43-53`, logged at `:193`).

## The provenance rule (why this matters for Kubona)

Three repos in this family say "quantum" and mean three different things: a real circuit
whose backend is recorded (chickensoup), real VQE/QAOA through Qiskit/qutip
(`milimoquantum/backend/app/quantum/vqe_executor.py:597,687,783`), and `np.random.dirichlet`
labelled consciousness ([[mainza-consciousness]],
`backend/utils/quantum_consciousness_engine.py:331`)).
Therefore:

> Every number Kubona emits carries `quantum_provenance: {circuit_sha, backend,
> shots|statevector, fallback_reason|null}`. A "quantum" number without that field is a
> validation error, not a warning.

chickensoup gets this right and never claims more; Kubona makes it structural.

## Build in Kubona

1. **Ledger object.** `claim` record type in the provenance ledger (same store as
   [[provenance-and-reproducibility]], never a side DB) with the amplitude triple, the
   computed signals, the collapse verdict, and the *evidence set as ledger references* so a
   claim can be recomputed after an ingest.
2. **MCP tools.** `claim_score(claim, evidence[])`, `claim_assert(claim_id)` (fails closed
   when uncollapsed), `claim_query(...)` on the existing [[provenance-and-reproducibility]] server surface
   (`kubona/mcp/servers/provenance.py` → `write_record`/`query_records`).
3. **Two improvements over the original.** (a) Replace the hand-rolled 2-qubit prep with
   Qiskit `StatePreparation` over `ceil(log2 k)` qubits so k-way claims are exact; (b) close
   the stub: `_contradiction_signal` becomes a real detector that samples *opposing* sources
   through [[science-data-integrations]] rather than the stddev of betting odds.
4. **Policy, not constants.** Thresholds (`0.75`, `0.3`, `≥3`) live in a versioned policy
   module with co-located tests — `gods-eye-view` keeps one `policy.js` per data layer
   (`src/layers/{traffic,vessels,launches,flights,radio,localAdsb}/policy.js`) and enforces the
   seam with `scripts/check-package-boundaries.mjs` + `docs/CODE-BOUNDARIES.md`. Kubona mirrors
   that: a `claim_policy.py` plus a boundary check in CI, with the policy file hashed into
   every claim record — otherwise "collapsed" is not reproducible.
5. **Assertion enforcement at write time.** Report/artefact generation calls
   `claim_assert`; an uncollapsed claim renders as `hypothesis (uncollapsed: diversity=0.2)`
   — visible, not hidden.

## Falsifiability and honesty

The 3-level amplitude model is a *decision procedure*, not a physical claim about
information. It is falsifiable in the only way that matters: given a labelled evidence set
(golden corpus), does the gate **reject** single-source peaks and **accept**
multi-source agreement? Those are unit tests, not vibes. Nothing here licenses "quantum
cognition" language; the mechanism earns its keep by refusing outputs, and any report that
uses a collapsed claim still cites the underlying evidence records, not the score.

See also [[epistemic-curvature]] (stagnation), [[agentic-memory-and-observability]]
(memory of claims), [[science-discovery]] (claim → hypothesis → run).
