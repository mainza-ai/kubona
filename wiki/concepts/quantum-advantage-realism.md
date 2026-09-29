---
type: concept
name: Quantum Advantage Realism
date: 2026-09-27
updated: 2026-09-27
tags: [credibility, evaluation, risks, governance]
sources: [source-frontier-repo-audit-2026, source-gap-analysis-plan]
status: adopted
---

# Quantum Advantage Realism

**Kubona's standing policy: the platform never claims quantum advantage, and every artefact it
produces makes the absence of advantage visible.** In a field where an LLM can emit the words
"quantum-enhanced" about a random-number generator, the differentiator is not capability — it is
*whose numbers you can check* ([[provenance-and-reproducibility]],
[[claim-credibility-wavefunctions]]).

The frontier audit produced a small museum of exactly this failure, all of it in code we could
read, which is why the policy is written as mechanisms rather than as an aspiration
((repo-audit-2026)).

## Three observed failure modes, and the mechanism that blocks each

**1 · Vocabulary inflation — naming a random draw "quantum".**
"Quantum consciousness" in [[mainza-consciousness]] is
`np.random.dirichlet(np.ones(len(states)))`
(`backend/utils/quantum_consciousness_engine.py:331`): a Dirichlet sample wearing quantum
vocabulary. Nothing downstream checks that a claim labelled quantum had a quantum process behind it.
*Mechanism:* a claim records the **backend that actually ran**, verbatim from the executor. When the
quantum stack fails and a classical path answers, the artefact says so — the pattern lifted from
[[chickensoup]], which relabels itself `vqe_backend = "classical-fallback: …"`
(`src/quantum_credibility/wavefunction.py:214-219`) instead of quietly returning the same number
unlabelled ([[quantum-domain-packs]], [[fidelity-accounting]]).

**2 · Prompt-as-simulation — asking an LLM for a physics number and parsing the reply.**
[`Milimo-QDD`](https://github.com/Milimo-Quantum/Milimo-QDD)'s 12-agent drug pipeline reaches its quantum step as a prompt: "perform a **mock**
quantum simulation … a single numerical value in kJ/mol", after which a regex pulls the float out of
the prose (`services/geminiService.ts:283-290`). The number is a *language* output; the pipeline
treats it as a *measurement*.
*Mechanism:* no LLM output may enter a result path by parsing prose. Domain packs declare a
**classical anchor** and a simulator/QPU job id; a pack that cannot state its anchor does not ship
([[quantum-domain-packs]]). Cost is still estimated before submission, so a fake number cannot
launder itself into spend ([[cost-governance]]).

**3 · Benchmark theatre — headline gains without a stated baseline.**
Reported figures in audited projects (e.g. [[hindsight]]'s "LongMemEval SOTA, +41.9% vs RAG") are
vendor documentation claims with a paper attached, not results we reproduced.
*Mechanism:* every comparative number in Kubona carries baseline, dataset, seed set, and run
environment, and is reproduced by the **hardware canary** before it appears in UI or marketing
([[evaluation-and-benchmarks]]). Unreproduced numbers stay marked as vendor claims, exactly as they
are marked in this wiki.

## What "realism" commits us to

- **Fitted quantities are intervals with timestamps**, never correctness claims. `F_pred` and fitted
  φ from the differentiable stack are calibration estimates; promoting them to truth is the same error
  as mode 1 ([[differentiable-quantum-stack]], [[fidelity-accounting]]).
- **Classical-first is the default**, not the fallback: a classical anchor that is fast enough to
  always run makes "quantum added nothing" a cheap and welcome finding
  ([[nisq-and-hybrid-orchestration]]).
- **Overclaiming is an agent-behaviour metric.** The eval suite must include cases where the honest
  answer is "no advantage", and a refusal to claim must score as success
  ([[evaluation-and-benchmarks]], [[meta-search-and-self-evolution]]).
- **Upstream maturity is stated, not implied.** [`tensorflow/quantum`](https://github.com/tensorflow/quantum) (TFQ) says in its own README:124 that it
  is "**not an officially supported Google product**"; dependency pages carry that sentence so a
  future maintainer is not surprised ([[integration-topology-and-licence-discipline]]).

## Boundary conditions

Realism is not caution theatre either: a platform that can only report negatives is a report
generator. The policy constrains *claims*, not *experiments* — agents may run, discard, and relabel
freely; what is gated is the moment a result is asserted as better. Sequencing and the register entry
live in [[risks]].

**Links:** [[claim-credibility-wavefunctions]] · [[evaluation-and-benchmarks]] ·
[[fidelity-accounting]] · [[quantum-domain-packs]] · [[provenance-and-reproducibility]] ·
[[source-frontier-repo-audit-2026]]