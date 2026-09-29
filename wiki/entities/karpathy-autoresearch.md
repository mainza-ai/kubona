---
type: entity
name: karpathy/autoresearch
date: 2026-08-16
updated: 2026-09-27
tags: [repository, pattern, llm-training]
sources: [source-gap-analysis-plan]
status: tracked
---

# karpathy/autoresearch

A minimal single-GPU LLM training repo (simplified `nanochat`) — MIT-licensed, with a
three-file split: `prepare.py` (fixed infrastructure), `train.py` (the one file the agent
may mutate), and `program.md` (human-authored instructions). The loop — mutate, run to a
fixed budget, evaluate one metric, keep-or-discard, repeat unattended — is reused as
**orchestration scaffolding** by Kubona (source-gap-analysis-plan).

## The generalization (harness registry)

One `prepare.py`/`train.py` pair ⇒ a registry of harnesses, each implementing a four-part
contract:

```
Harness {
  fixed_setup()          // problem instance, backend, noise model — not agent-editable
  mutable_artifact()     // the one file/object the agent is allowed to change
  metric(result)         // single, comparable, normalized score
  validity_check(result) // is this even a legal/correct output?
}
```

## Kubona harnesses (Phase 4)

- **VQE ansatz search** — mutable: `ansatz.py`; metric: energy error per qubit
- **QAOA search** — mixer/cost structure, layer count, optimizer; metric: approximation ratio
- **Mitigation-strategy search** — mitigation config; metric: distance from noiseless reference
- **Compilation-pass search** — transpiler pass sequence; metric: depth/gate count × error rates
- **Correctness regression** — nothing mutated; pass/fail against known answers; gates releases

## Safety (non-negotiable)

Structural write-scope per harness (sandbox-enforced, not harness-honesty); **simulator-only
by default** (a property of the orchestrator, a global setting); correctness harnesses run
in CI and gate releases ("less trust than optimization harnesses"). Output: a continuously
growing [[provenance-and-reproducibility|provenance-backed]] validated algorithm library —
see [[agent-resource-economy]] for its monetization.

## Kubona Search Mode (§11.4)

The engine's first shipped use-case (Phase 4, simulator-only by default): **autonomous
ansatz-search** — the low-effort MVP for algorithmic optimization that validates the
sandboxed-write-scope pattern before GQE lands as a 4th harness. Its exit criterion: at
least one ansatz beating a hand-designed baseline on a benchmark problem, entirely in
simulation (source-gap-analysis-plan §11.4).

## The meta-level tier (§13) — from optimization to self-evolution

§11's loop above only ever improves *artifacts* (an ansatz, a mitigation config); the
orchestrator, the search strategy, and the instructions stay fixed run after run. That is
autonomous **optimization**, not evolution. §13 adds a second tier that revises **how the
search itself runs** — proposal strategy, `search.md` instruction files, and even the
harness registry (new-harness drafts on the same four-part contract) — making the engine
genuinely self-evolving. See [[meta-search-and-self-evolution]] for the full model: the
two-tier table, the GQE flywheel (§13.3: library → retrain GQE → better proposal strategy →
better candidates), the stricter safety tier (versioned/reversible changes, shadow-mode
staged rollout, capability-level human sign-off beyond the dollar-threshold gate, write
scope capped at search strategy/instructions/harnesses — never compiler/MCP/security code),
and the Phase-4-stretch / Phase-5-auto-apply placement.
