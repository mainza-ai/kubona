---
type: concept
name: Meta-Search & True Self-Evolution
date: 2026-08-16
updated: 2026-09-27
tags: [kubona, autoresearch, self-evolution]
sources: [source-gap-analysis-plan]
status: adopted
---

# Meta-Search & Self-Evolution (§13)

§13 of the gap-analysis plan: the second tier that makes the [[karpathy-autoresearch]]
engine *self-evolving* rather than merely self-tuning. §11's object-level loop only improves
**artifacts** — a circuit, a mitigation config, a pass sequence; the orchestrator, the
search strategy, and the instruction files stay fixed run after run. That is autonomous
**optimization**, not evolution. The meta-level loop revises **how Kubona searches**, not
just what it searches for (source-gap-analysis-plan §13).

## Two tiers, not one

| | Object-level loop (§11) | Meta-level loop (§13, new) |
|---|---|---|
| What gets mutated | The artifact — `ansatz.py`, a mitigation config, a pass sequence | The search itself — proposal strategy, harness instructions, harness registry |
| Frequency | Every iteration (minutes) | Periodic reflection over accumulated results (weekly / after N runs) |
| Output | A better circuit/config for one problem class | A better *searcher* — compounds across every future run, harness, problem class |
| Risk profile | Bounded — worst case a wasted simulator run | Higher leverage — a bad meta-change degrades search quality system-wide until caught |

## What the meta-loop is allowed to touch (and nothing else)

1. **Mutation/proposal strategy per harness** — e.g. "the ansatz harness has stalled for
   200 iterations, widen the mutation radius"; "seed new mitigation candidates from the
   top-3 historical library entries instead of pure random search."
2. **Harness instruction files (`search.md`)** — human-authored today; the meta-loop can
   propose versioned, diff-reviewable edits from outcome data (e.g. tighter validity
   guidance for a harness that keeps proposing invalid candidates).
3. **The harness registry** — when provenance data reveals a recurring problem class no
   existing harness covers, propose a *new* harness (same four-part contract as §11.2) as a
   draft for human review.

## The flywheel (§13.3) — what "self-evolve" actually means

Provenance-backed algorithm library → training data → periodically retrain the GQE model
(already slated as a Phase-4 harness upgrade) → the improved model becomes the proposal
strategy for ansatz search → better candidates → richer library → repeat. Object-level
search produces data; the meta-loop turns that data into a better searcher; the better
searcher produces better data. **That closed loop — not any single search run — is the real
mechanism of self-improvement.**

## Safety tier (stricter than §11.3)

- **Every meta-change is versioned and reversible** — a revised `search.md`, a new
  proposal-strategy parameter set, or a harness draft is a diffable, rollback-able artifact,
  never an in-place mutation of live orchestrator state.
- **Staged rollout, not global activation** — shadow mode runs old and new strategy side by
  side and compares outcomes before the new one replaces the incumbent.
- **Human sign-off required** for anything touching the harness registry or a proposal
  strategy in production — a *capability-level* approval gate, separate from and in addition
  to the cost-governance dollar-threshold gate (§4.5): a meta-change costs nothing in QPU
  spend and can still be high-risk system-wide.
- **The write scope stops at search strategy, instruction files, and harness definitions** —
  the meta-loop never edits Kubona's compiler, MCP servers, or security layer. Self-evolution
  here means Kubona gets better at *searching*, not open-ended self-rewriting.

## Roadmap placement (§13.5)

- **Phase 4 stretch goal**, gated on the object-level engine shipping first — a meta-loop
  with nothing to reflect on is premature ("functional product first", §9).
- Phase 4 form: a scheduled reflection job that **proposes** (never auto-applies) strategy
  and harness-registry changes for human review.
- **Phase 5+:** auto-application of meta-changes via the shadow-mode staged rollout, only
  after the propose-and-review version has run safely for a meaningful period.
- **Exit criteria:** ≥1 proposed strategy change that, shadow-tested, measurably improved
  convergence speed or candidate quality on an existing harness — proof the flywheel
  compounds rather than adding noise.

Related: [[karpathy-autoresearch]] · [[provenance-and-reproducibility]] ·
[[evaluation-and-benchmarks]] · [[cost-governance]] · [[risks]] · [[roadmap]]
