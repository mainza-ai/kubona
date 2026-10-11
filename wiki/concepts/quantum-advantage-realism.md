---
type: concept
name: Quantum Advantage Realism
date: 2026-09-27
updated: 2026-10-10
tags: [credibility, evaluation, risks, governance]
sources: [source-frontier-repo-audit-2026, source-gap-analysis-plan]
status: settled
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

## Claim grammar (Q-04, closed 2026-10-10)
The positive vocabulary — what Kubona *may* say, enforced at construction in
`platform/kubona/epistemics/claim_grammar.py` (`ScientificClaim`, fail-closed pydantic):

- **Permitted verbs** (predicate must contain one): measured, reproduced, bounded, refuted,
  converged, estimated, computed, observed, verified, rejected, matched, reached. Reports state
  measurements, never achievements. Banned hype terms (`quantum supremacy`, `unmatched speedup`,
  `miraculous`, `quantum leap`) refuse alongside verb-less predicates.
- **Required qualifiers**: category, hardware provenance (backend verbatim + tier + shots +
  seed), classical anchor name/value, measured value, delta, uncertainty σ with 95% CI,
  falsification criteria (≥1), ledger ref. An advantage claim additionally requires chemical
  accuracy + |Δ| ≤ 1.6 mHa + ≥15-char justification + a credibility field at ρ ≥ 0.90, tier L4/L3.
- **Credibility field**: `{rho, tier, evidence_refs}` — rho IS P(collapsed-state) (0.0 when
  uncollapsed), tier is the continuum level, and the score is never cited without its refs.
  Rendered as `[ρ=0.950 L3 refs=1]` in `formatted_statement`.

## Evidence tiers (enforced 2026-10-10)

Every claim states how its number was earned (`EvidenceTier`, required, no
default — a claim that will not say how it knows refuses at construction):

| Tier | Meaning | Examples in this repo |
|---|---|---|
| **T1 verified** | mechanical, parameter-free check | unitary equality (scheduler), anchor match to tolerance, lint/type gates, proof steps |
| **T2 consistent** | agrees under stated assumptions | model-vs-literature with assumed prefactors, fitted calibration, simulator-vs-theory |
| **T3 sampled** | statistical interval from finite shots/runs/seeds | canary Wilson intervals, seed bands, Monte Carlo estimates |

A T3 number cited without its interval, or a T2 cited without its
assumptions, is a malformed claim — the tier names what must travel with
the number. Rendered as `[T3_SAMPLED]` in `formatted_statement`.

**Links:** [[claim-credibility-wavefunctions]] · [[evaluation-and-benchmarks]] ·
[[fidelity-accounting]] · [[quantum-domain-packs]] · [[provenance-and-reproducibility]] ·
[[source-frontier-repo-audit-2026]]