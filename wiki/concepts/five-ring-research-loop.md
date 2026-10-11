---
name: The Five-Ring Research Loop
type: concept
tags: [frontier, autonomy, closed-loop, orchestration, architecture]
date: 2026-09-27
updated: 2026-10-10
status: adopted
sources: [source-frontier-repo-audit-2026, source-architecture-paper, source-gap-analysis-plan]
---

# The Five-Ring Research Loop

**Definition.** The frontier architecture: five *nested* closed loops, each with its own
metric, budget, and hard stop. Kubona closes Ring 2 today; the frontier is Rings 0, 1, 3, 4.
The discipline binding them: **no ring closes on an LLM's confidence** — each closes on a
measured quantity.

## Canonical ruling (Q-08, closed 2026-10-10): three models, three axes

The three loop descriptions are not competitors — they answer different questions:

| Model | Answers | Status |
|---|---|---|
| **Five rings** (this page) | *Architecture*: the nested control loops and their stop conditions | **canonical** |
| **Discovery 7-stage** ([[science-discovery]]) | *Lifecycle*: one campaign traversal, Ground → Learn, passing through Rings 1→2→3 | a pass through the rings, not a rival loop |
| **§11 object / §13 meta** ([[meta-search-and-self-evolution]]) | *Mechanism*: the §11 object loop is Ring 2's engine; the §13 meta-loop is Ring 4's engine | the two engines inside the architecture |

Mapping: Ground/Hypothesize (stages 1–2) run inside Ring 1 (only collapsed claims enter
Design); Design/Experiment/Analyze (3–5) run inside Rings 2–3 (ledger-bound runs, cost +
fidelity gates); Report (6) asserts only collapsed claims; Learn (7) feeds Ring 4 reflection.
No new loop may be introduced without stating which ring(s) it traverses — that is how
"three loops for one system" stops becoming an API problem.

Why five and not one: these systems fail when exactly one loop is closed. Execution without
epistemics yields confident noise (Milimo-QDD's LLM-generated binding energies,
(repo-audit-2026)); literature without execution yields an expensive reading list. Nesting
lets each layer's *stop condition* be owned by the layer beneath it.

## The rings

| Ring | Loops over | Closes on | Invariant | Status |
|---|---|---|---|---|
| **0 · Reflex** | the running job | telemetry: divergence, timeout, refusal, error signature | a non-LLM watchdog may **pause, re-scope, or abort-to-snapshot** mid-flight | absent |
| **1 · Epistemic** | what may be claimed | wavefunction collapse under an evidence-strength gate | no assertion without a collapsed claim record | absent |
| **2 · Instrumental** | the experiment | measured metric of a [[vqa-vqe]]/sampling run | every result bound to a ledger record | **exists** |
| **3 · Methodological** | how the experiment is built | predicted-then-measured fidelity and cost | a job may not spend money it cannot be shown will produce physics | partial |
| **4 · Reflexive** | strategy and model | task-metric slope; held-out benchmark delta | self-modification only through the same harness as any candidate | partial |

**Ring 0 — Reflex.** A workflow that diverges currently runs to budget. The watchdog is
deliberately *not* an LLM; it holds three powers: `pause`, `re-scope`, `abort-to-snapshot`.
Spine: `gods-eye-view` keeps one `policy.js` per data layer
(`src/layers/{traffic,vessels,launches,flights,radio,localAdsb}/policy.js`) and makes the seam
machine-checked (`scripts/check-import-directions.mjs`, `scripts/check-package-boundaries.mjs`,
`docs/CODE-BOUNDARIES.md`); its ingest edge `server/providers/local-receivers.js` admits only
env-listed user-owned endpoints, gates them with `isLocalIpv4` (:146, :295) and lets the feed's
own clock decide staleness (:376) — a real-time ingest surface that cannot be aimed at the
network. `chickensoup` supplies the coordination object:
`ReconciliationGate.acquire/release/is_busy/refresh_ttl` plus a cooperative stop flag and
stale-gate clear (`src/reconciliation_gate.py:25-102`) — a TTL'd single-writer gate, so two
agents never mutate one campaign. Build: `platform/kubona/observability/`; telemetry goes to
the existing ledger, not a parallel store.

**Ring 1 — Epistemic.** Every hypothesis enters as a claim record carrying a wavefunction
over `{corroborated, contested, unverified}`; only a *collapsed* claim may be asserted in an
artifact, while uncollapsed ones may still be used as probes. One line governs the ring:
**an agent may propose anything and assert only what has collapsed.** Mechanism in
[[claim-credibility-wavefunctions]]; stagnation detector in [[epistemic-curvature]].

**Ring 2 — Instrumental.** `kubona/mcp/servers/{execution,compiler,control,cost,
provenance}.py` expose `transpile_circuit`, `lower_circuit`, `write_record`,
`query_records`; providers register through [[provider-capability-schema]]. Nothing new is
required here.

**Ring 3 — Methodological.** [[cost-governance]] already gates *money*
(estimate → pre-flight → settlement). Add the **fidelity gate** beside it: predict result
fidelity from device noise, and verify the prediction with a cheap Clifford probe before
real QPU time is spent → [[fidelity-accounting]]. Two search harnesses live here:
[[autonomous-ansatz-optimization]] and [[agentic-qir-compilation]].

**Ring 4 — Reflexive.** `platform/kubona/autoresearch/base.py` is already the right shape
and stays untouched: `Harness.fixed_setup / mutable_artifact / metric / validity_check /
run` (:51-:67), `Orchestrator(budget_steps=40, seed=7)` propose→refine→trial→best
(:72-:159). Rings 1, 3, 4 all become harnesses **of this same class** — the whole
integration plan in one sentence. GQE is one such harness
([[meta-search-and-self-evolution]]); its training substrate is in
[[differentiable-quantum-stack]]. Population-coordinated replica search
(`autoresearch/population.py`, built 2026-10-10) is the newest primitive
here: seeded replica populations sharing discoveries under identical
sample budgets over brute-force-anchored QUBOs (n≤14), reporting gaps to
the true optimum per seed. The suite locks mechanics (determinism,
exact sample accounting, equal arms, gaps ≥ 0) and deliberately never
superiority — coordination winning is landscape-dependent, and a test
asserting it would be fitted, not verifying.

## Cross-ring rules

1. **One budget rail.** All rings draw on the same [[agent-resource-economy]] envelope;
   Ring 4 cannot fund Ring 3 by inventing credits.
2. **Convergence, not confidence.** Termination is metric slope or exhausted budget.
   chickensoup's `COLLAPSE_THRESHOLD = 0.75` *plus* an evidence-strength gate it cannot
   satisfy alone (`wavefunction.py:16,253-269`) is the template: threshold **and** a second
   condition.
3. **Disconfirmation is mandatory.** An artefact labelled *discovery* must survive a harness
   whose `metric()` rewards finding a flaw (`tribunal_agent.py:151`). Refuted hypotheses are
   kept as negative results — a library that stores only wins is marketing, not science
   ([[science-discovery]]).
4. **Refusal is a legal output.** `insufficient_evidence` with a reason; scored so
   abstaining correctly beats answering plausibly ([[evaluation-and-benchmarks]]).
5. **No side channels.** Every ring emits ledger records and is inspectable through the
   same surface ([[agentic-memory-and-observability]]).

## Gate tests and non-goals

Ring 0: a diverging synthetic run is aborted at the configured slope and resumed from
snapshot. Ring 1: a peaked-but-single-source claim **must not** collapse while a
5-platform claim must. Ring 2: existing P0 gate, unchanged. Ring 3: predicted-vs-measured
fidelity within tolerance on a probe, job blocked when the floor is unmet. Ring 4: existing
autoresearch tests plus a held-out delta reported as a range.

Non-goals: no quantum-advantage claim (`wiki/concepts/risks.md`); no consciousness or
"quantum brain" ([[mainza-consciousness]]); no autonomous real-QPU access without a human
grant; **no ring may edit its own stop condition**.
