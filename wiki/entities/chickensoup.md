---
type: entity
name: chickensoup (mainza-ai)
date: 2026-09-27
updated: 2026-09-27
tags: [prior-art, owner-repo, credibility, unlicensed]
sources: [source-frontier-repo-audit-2026]
status: candidate
---

# chickensoup — the credibility maths, already written, unlicensed

**https://github.com/mainza-ai/chickensoup** · maintainer's own repo · HEAD 2026-07-18 ·
no `LICENSE` file, which matters only if a derivative ever ships — while the code stays the
maintainer's, mining it needs no grant
([[integration-topology-and-licence-discipline]]).

Integration class: **Blueprint — insight material from the maintainer's own WIP.** This repo is
incomplete research work, so the value here is *method*: read the maths, reimplement it inside
`platform/` under test, and do not merge a file across the boundary. Its most exportable habit is
not the algorithm at all but the honesty of its degradation.

What is actually in it, verified line by line:

- `ClaimWavefunction` (`src/quantum_credibility/wavefunction.py:159`) and `score_claim` (:163), fed
  by `_source_diversity` (:20), `_engagement_magnitude` (:28) and `_polymarket_prior` (:36) —
  credibility as an amplitude built from independent evidence channels
  ([[claim-credibility-wavefunctions]]).
- On quantum-stack failure it **relabels itself honestly**:
  `vqe_backend = "classical-fallback: …"` (:214-219). This is the pattern Kubona adopts as policy,
  not as a nicety ([[quantum-advantage-realism]]).
- The quantum part is real Qiskit: `EstimatorV2` / `Statevector` with an
  Aer → `primitives` → legacy fallback chain (`src/spacetime_engine/vqe_runner.py:24-38`), plus
  `meyer_wallach` entanglement measure (`src/spacetime_engine/entanglement.py:10`).

Lands on: [[claim-credibility-wavefunctions]], [[epistemic-curvature]], [[fidelity-accounting]].
**Never** cite a `dev-docs/…` path for any of this — the clone is deleted when development ends; the
URL, the SHA and the line numbers above are the record.