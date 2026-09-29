---
name: Epistemic Curvature and Research Stagnation
type: concept
tags: [frontier, epistemics, observability, stagnation, metrics]
date: 2026-09-27
updated: 2026-09-27
status: open-question
sources: [source-frontier-repo-audit-2026]
---

# Epistemic Curvature and Research Stagnation

**Definition.** A family of cheap, computable diagnostics over an agent collective's
*evidence graph* that answer one question: **is this research still happening, or has it
started circling?** Kubona measures physics drift with telemetry; it has no instrument for
*thought* drift. This page proposes the instrument, and states plainly how much of it is
metaphor.

## Verified primitives it stands on

From `chickensoup/src/quantum_credibility/` and `src/spacetime_engine/` (344 + 185 + 135 +
198 + 73 LOC; (repo-audit-2026)):

- **`claims_to_vector` / `canon_page_to_vector`** (`vectorizer.py:26,119`) — embed live
  evidence and the *canonical* page for the same topic, plus `hash_vector` (`:196`) so the
  embedding is content-addressable (a Kubona requirement: a diagnostic you cannot re-derive
  is not a diagnostic).
- **`_compute_divergence_risk(canon_vec, live_vec, geometry)`** (`divergence_engine.py:23`)
  — how far live evidence has moved from what the system currently asserts, *modulated by
  geometry*.
- **`vector_to_field_geometry` → `FieldGeometryTensor`** (`vectorizer.py:160`, `tensor.py`) —
  a 42-line tensor that bends the distance metric between two vectors.
- **`compute_entanglement_correlation(entity_a, entity_b, evidence)`**
  (`entanglement_corr.py:35`) — clusters evidence by `cluster_id`, extracts entity
  co-mentions (`:21`), and scores the pair with the real Meyer–Wallach measure
  (`entanglement.py:10`), thresholded at 0.3 (`:72-73`).
- **`staleness_queue.py`, `idle_sentinel.py`, `progress_tracker.py`, `observability.py`** —
  the scheduling half: what is due for re-checking, what has gone quiet, what is advancing.

## What Kubona computes

| Diagnostic | Formula shape | Alarm condition |
|---|---|---|
| **Evidence distance** `d(E_canon, E_live)` | cosine distance between canonical and live vectors | high + rising → the literature has moved and the claim is stale → re-open the [[claim-credibility-wavefunctions]] record |
| **Concentration** `κ` | inverse participation ratio of hypothesis mass over the topic graph | `κ → 1` → the collective has one favourite hypothesis → **diversify or stop** |
| **Co-mention coupling** | co-occurrence mutual information, reported beside `meyer_wallach` when a genuine superposed state exists | coupling high while evidence sources are few → echo chamber, not corroboration |
| **Refusal ratio** | refusals / assertions over a window | near 0 → the epistemic gate is being routed around; near 1 → the research programme is empty |

`κ` and the refusal ratio are the honest, defensible half. They need no geometry at all.

## Where the metaphor must stop

The `FieldGeometryTensor` route is where this design becomes weakest, not strongest: a
4×4 "curvature" tensor applied to a cosine distance is a monotone reweighting of one scalar,
and it is not differential geometry in any testable sense. Kubona therefore (a) keeps
`κ`, distance, coupling, and refusal ratio as first-class named metrics; (b) treats any
curvature-like transform as an *optional reweighting* that must be ablated against the flat
metric before it may influence a decision; (c) forbids curvature *language* in generated
reports unless the ablation showed it mattered. The failure mode this guards against is the
one documented in the same family: a random draw named "consciousness"
([[mainza-consciousness]]) — see (repo-audit-2026) §"Not adopted". Naming a number after
physics is how a system loses the right to be believed about physics.

## Landing in the platform

- `platform/kubona/observability/epistemics.py`: pure functions, golden-tested on a synthetic
  evidence corpus (a diverging corpus, an echo corpus, a stale corpus).
- Emitted as ledger records on the same append-only store as runs, so a stagnation alarm is
  itself provenance ([[provenance-and-reproducibility]]).
- Consumed by the Ring 0 watchdog: `κ > 0.85 ∧ no new evidence in window → re-scope` (never
  "continue, but say something new").
- Surfaced as one sparkline per campaign in the console ([[agent-ux-control-plane]]).
- Scored by [[evaluation-and-benchmarks]] as a *calibration* target: does a high-κ campaign
  predict a later refutation? If it does not, the metric is deleted. That is the falsifier.

See also [[agentic-memory-and-observability]], [[multi-agent-research-collective]],
[[meta-search-and-self-evolution]].
