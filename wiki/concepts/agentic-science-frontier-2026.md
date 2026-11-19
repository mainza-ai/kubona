---
type: concept
name: Agentic science frontier 2026
date: 2026-11-18
updated: 2026-11-18
tags: [agentic-science, autonomy, agents, benchmarks, provenance, plan]
sources: [source-agentic-science-frontier-2026, source-gap-analysis-plan]
status: tracked
---

# Agentic science frontier 2026

A research synthesis of end-to-end, AI-agent-led scientific workflows
(2025–2026) compared with Kubona's actual implementation. Full report with
source table and acceptance criteria:
`docs/agentic-science-frontier-2026-11-18.md`. Web sources:
[[source-agentic-science-frontier-2026]].

## What the frontier established

- **Ideation is a search problem.** Co-Scientist runs generation, reflection,
  ranking (Elo tournament), evolution, proximity, and meta-review agents with
  a supervisor and persistent context; test-time compute scales hypothesis
  quality. AI Scientist's end-to-end paper passed blind review at one workshop
  (6/7/6) but still shows naive ideas, weak rigor, and citation errors.
- **Lab-in-the-loop closes the loop.** GPT-5 + Ginkgo ran six rounds over
  36,000 reactions and cut cell-free protein synthesis cost 40%; Co-Scientist's
  extension planned experiments and wrote papers; A-Lab GPSS, AutoLabs,
  AutoDNA, and independent SDL testbeds show real closed-loop science.
- **Benchmarks show autonomy is not solved.** ScienceAgentBench best agent
  ~32%; ResearchClawBench best autonomous agent 21.5/100 with failures in
  protocol, evidence, and scientific core; SCI-AGENT-EXAM high mean but only
  26% of runs above 0.95. The bottleneck is scientific judgment, not writing.
- **Provenance is becoming interoperable.** Workflow Run RO-Crate/W3C PROV,
  PROV-AGENT, and PROV-AGENT-LOOP make agent decisions auditable; aiprov adds
  per-claim attribution and human-only verification rungs.

## Kubona position

Strong: ledger-bound execution, signed receipts, spend mandates, real
QPU/simulator runs, tribunal critique, MCP/A2A surfaces, publication pipeline
with reproducible build and signed comparator attestation.

Missing for "agentic lead": a hypothesis tournament, literature-grounded
novelty/falsifiability, a typed protocol planner, structured long-horizon
research state, end-to-end evaluation, claim/citation verification, stage-level
autonomy modes, safety screening, provenance export, and cumulative lab
knowledge. The runnable loop executes well; it does not yet lead ideation or
prove autonomy on a benchmark.

## Plan (AG-series)

1. **AG-1** end-to-end Kubona-domain re-discovery benchmark (pass@k, cost,
   stability, failure labels).
2. **AG-2** hypothesis engine (generate/dedupe/rank/evolve, novelty measured or
   unmeasured, every hypothesis falsifiable).
3. **AG-3** typed protocol planner with acceptance metric and stop rule.
4. **AG-4** structured research graph + context-budgeted summaries + restart
   resume.
5. **AG-5** claim/citation verification and reviewer calibration.
6. **AG-6** RO-Crate/PROV export + CRediT-style attribution.
7. **AG-7** autonomy levels + dual-use screening + stage gates.
8. **AG-8** stage/cost/failure telemetry.
9. **AG-9** cross-lab cumulative knowledge registry.

## Implementation status (2026-11-18)

- **AG-1a closed:** `platform/bench_agentic` runner + 13-task catalog/tests;
  disposable-stack baseline 11/13 pass@1 (HVA-6 and H2 chemical accuracy pass;
  the 8/6-site hardware-efficient TFIM tasks remain algorithmic failures).
- **AG-1b closed:** nightly/dispatch benchmark workflow with `--allow-failures`
  and a retained JSON artifact.
- **AG-2a closed:** `pkg/hypotheses` + ledger table + session API; 10 ranked
  falsifiable hypotheses from the H2 campaign, literature refs, shortlist.
- **AG-2b closed:** strict-JSON LLM proposer (diagnostics floor, honest
  fallback), deterministic Elo tournament + derived meta-review,
  `lexical_containment_v1` measured novelty (schema 7), evidence-gated collapse
  to a claim, shortlist-before-plan gate. Live Strata success: 4 model
  proposals, 3 merged with diagnostics, ~94 s; the proposer timeout is
  `KUBONA_HYPOTHESIS_LLM_TIMEOUT_SEC` (default 360 s) and the completion is
  bounded at 8192 tokens.
- **AG-3a closed:** typed plans (schema 6), approval, execution through the
  job rail, observable-vs-acceptance scoring.
- **AG-3b closed:** automatic spec synthesis from campaign facts + the
  hypothesis's relative `spec_patch`, with persisted provenance; ungrounded
  methods/targets refuse with `spec_unavailable` (schema 8).
- **AG-4a closed:** `/research-graph` ledger projection.
- **AG-4b closed:** priority-ordered, byte-budgeted summary with a truncation
  report, plus a `since_seq` resume cursor over the ledger chain.
- **AG-5 closed:** recorded-evidence numeric verification and DOI/title
  citation verification; signoff blocks fabricated numbers and dead citations;
  the resolver-unavailable acknowledgment is explicit and sealed; calibration
  reports 12/12 against maintainer labels (schema 9).
- **AG-6a closed:** RO-Crate 1.1 metadata in `manuscript-export`.
- **AG-6b closed:** W3C PROV-JSON (`prov.json`) and CRediT attribution
  (`credit.json`) per manuscript, plus PROV-O nodes in the RO-Crate; agents are
  attributed only when the ledger records them.
- **AG-7 closed:** session stage modes (suggest/approve/act), a v1 risk
  classifier enforced at plan execution, and an operator halt that cancels live
  jobs (schema 10).
- **AG-8a closed:** hypothesis/plan status gauges.
- **AG-8b closed:** `/api/ops/science` stage taxonomy, cost per accepted claim,
  human intervention counts (minutes explicitly unmeasured), queue/loop health,
  plus matching Prometheus gauges.
- **AG-9 closed:** tenant-scoped cumulative lab-knowledge registry
  (negative results, claims, reports) with fingerprint dedupe, a
  prior-negative gate on plan creation, and owner-granted sharing (schema 11).
- **Open:** none of the AG slices; residual items are operator chores and
  re-verification on the pushed commit.
- Schema was 11 at AG-9; the current ledger schema is **15**.

## Agentic mode and oversight (2026-11-18)

- The auto AI agentic workflow is a session mode the user can turn on or off
  (`agentic_enabled` in the session policy; `PUT /api/sessions/{sid}/agentic`).
  Off blocks agent loop start/resume/wake/step and plan execution, pauses
  running loops, and keeps the manual experiment rail available.
- An operator halt sets the switch off and locks it (`agentic_locked`); a
  session caller cannot re-enable it. Only an operator policy write clears the
  lock. Restart rehydration brings loops back paused when the switch is off.
- The Autoresearch panel shows the switch, the current stage mode, and a clear
  Manual/Auto/Locked status; every change is an `agentic_mode_changed` event.
- Oversight slices AG-18…AG-24 are implemented: a composer Manual/Auto/Locked
  chip; a durable approval inbox with approve/reject/defer, validated reviewer
  edits (risk reclassified, previous values sealed for undo), and durable
  rejection feedback; a `notify` rung with `/agentic/digest` receipts and a
  console digest panel; per-tool `allow|ask|block` enforcement where `ask` holds
  the exact call for a one-time approval bound to its canonical argument digest
  (15-minute TTL, spent once); queue/fatigue metrics; `/receipts` undo for mode
  flips, unrun plan decisions, and deferrals; and an operator-owned tenant
  default with policy history/rollback and strict unreadable-policy refusal.
  The risk classifier is a persisted v2 taxonomy (domain: bio, chem,
  radiological, cyber; protocol: human-subjects, animal, environmental; plus
  hardware and budget) and notify-mode executions reach the notification inbox.
  Ledger schema is 15. See `docs/agentic-mode-toggle-2026-11-18.md`.

## Known accuracy gaps (2026-11-18 investigation)

- **TFIM VQE:** readout is correct (exact-state control measures −10.249 ±
  0.022 vs −10.2517 at 20k shots). The gap is single-start optimization
  (2-layer HEA, 8 sites: 1.202 → 0.246 Ha with three restarts) plus
  depth/expressivity (HEA 4 layers 0.062, 6 layers 0.030 Ha). The 6-layer
  Hamiltonian-variational ansatz reaches **0.0027 Ha** (HVA-4 is 0.125 Ha).
  Shot noise is second-order.
- **QAOA:** a simulator shot bit-order bug under-reported the 2-layer ratio
  (0.788 → 0.891 after the fix, single start). Multi-start reaches **0.899 at
  p=2** and **0.948 at p=3**. The p=2 ceiling is ~0.90, so the benchmark uses
  0.85 at p=2 and 0.90 at p=3.
- **H2 chemical accuracy:** the statevector optimizer reaches FCI; the 4.5 mHa
  flag at 4096 shots was shot noise. At 20k shots the ground-state task measures
  **0.256 mHa** and the separate chemical-accuracy task **0.640 mHa** — both
  inside the 1.6 mHa gate.
- **Frontend:** the Inspector header overlapped the “Narrow” preset with the
  active Copilot tab at 360 px; fixed with a wrapping header and a real-browser
  non-overlap test (old layout fails, fix passes).
- Follow-up slices AG-11…AG-16 cover the optimizer, noise-aware scoring, H2
  depth/ansatz, QAOA depth, console layout, and telemetry.

## Cross-references

- [[five-ring-research-loop]] — where these capabilities sit in the ring model.
- [[autonomous-mathematical-discovery]] — the existing thinking-compute loop.
- [[evaluation-and-benchmarks]] — current eval surface.
- [[provenance-and-reproducibility]] — ledger and receipts.
- [[remaining-work]] — execution queue.
