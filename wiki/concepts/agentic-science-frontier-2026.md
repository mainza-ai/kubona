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

- **AG-1a closed:** `platform/bench_agentic` runner/tasks/tests; disposable
  stack baseline 2/3 pass@1 (H2 VQE 4.5 mHa; anchor exact; TFIM 1.227 Ha off).
- **AG-1b closed:** nightly/dispatch benchmark workflow with `--allow-failures`
  and a retained JSON artifact.
- **AG-2a closed:** `pkg/hypotheses` + ledger table + session API; 10 ranked
  falsifiable hypotheses from the H2 campaign, literature refs, shortlist.
- **AG-2b closed:** strict-JSON LLM proposer (diagnostics floor, honest
  fallback), deterministic Elo tournament + derived meta-review,
  `lexical_containment_v1` measured novelty (schema 7), evidence-gated collapse
  to a claim, shortlist-before-plan gate.
- **AG-3a closed:** typed plans (schema 6), approval, execution through the
  job rail, observable-vs-acceptance scoring.
- **AG-4a closed:** `/research-graph` ledger projection.
- **AG-6a closed:** RO-Crate 1.1 metadata in `manuscript-export`.
- **AG-8a closed:** hypothesis/plan status gauges.
- **Open:** AG-3b automatic spec synthesis; AG-4b compaction/resume; AG-5
  verification; AG-6b PROV; AG-7 autonomy/safety; AG-8b dashboards; AG-9
  registry.
- Schema 7.

## Cross-references

- [[five-ring-research-loop]] — where these capabilities sit in the ring model.
- [[autonomous-mathematical-discovery]] — the existing thinking-compute loop.
- [[evaluation-and-benchmarks]] — current eval surface.
- [[provenance-and-reproducibility]] — ledger and receipts.
- [[remaining-work]] — execution queue.
