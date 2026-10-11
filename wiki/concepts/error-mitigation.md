---
type: concept
name: Quantum Error Mitigation
date: 2026-08-16
updated: 2026-09-27
tags: [error-mitigation, noise]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: adopted
---

# Quantum Error Mitigation (QEM)

Software techniques to extract accurate signals from NISQ noise without full error
correction (source-architecture-paper). Delivered via [[mitiq]] as a compiler-level wrapper.

| Technique | Mechanism | Operational cost |
|---|---|---|
| **ZNE** | Unitary folding amplifies noise; measure at scaled noise levels; extrapolate to zero-noise (e.g. Richardson) | Multiple depth multipliers → more QPU time + extrapolation variance |
| **PEC** | Express ideal unitary as quasi-probability combo of known noisy ops; sample + classically re-weight | Exponentially more shots for statistical precision |
| **CDR** | Train a regression on near-Clifford (classically simulable) circuits mapping noisy→exact output; apply to non-Clifford cases | Upfront classical simulation + training |

## Kubona angle

Mitigation choice is a **per-backend optimization problem**, not a default — which is why the
[[karpathy-autoresearch]] engine runs a mitigation-strategy harness
(mutable = ZNE fold factors / PEC budget / CDR set size; metric = distance from a
classically-computable noiseless reference). ZNE ships first (Phase 1) as the best
value-to-effort ratio. Budget overruns from mitigation shot amplification are caught by
[[cost-governance]].

## Re-fittable records (built 2026-10-11)

`ZNEResult` archives per-scale values with shots + seed provenance, and
`refit()` re-estimates from the *same* shots under Richardson / linear /
exponential — no new execution. Estimator spread is reported, not hidden;
unknown extrapolators refuse, and exponential refuses signed data (log-fit
needs positive values) rather than inventing a number. Same shots,
multiple estimators, zero new spend.

## Deferred with reason: syndrome pipelines

Syndrome archiving (records, decoders, postprocessing estimators) waits for
a real QEC run to consume it — building the pipeline with no producer
would be speculative scaffolding. The re-fit discipline above is the
down payment: when syndromes arrive, they land in records shaped like this.

Related: [[mitiq]] · [[barren-plateau]] · [[circuit-cutting]] · [[nisq-and-hybrid-orchestration]]