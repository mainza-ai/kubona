# Agentic scientific workflows — 2026 frontier vs Kubona

**Date:** 2026-11-18
**Scope:** end-to-end, AI-agent-led scientific workflows — ideation through
execution, analysis, manuscript, review, and deposit — and what Kubona is
missing to claim that capability honestly.
**Method:** web research on primary sources published 2025–2026 (Nature,
arXiv, lab/project announcements), then a code-grounded read of Kubona's
actual agentic surfaces. No claim below is based on a vendor summary alone
where a primary paper exists.

> **Register.** Kubona is a quantum-computing and quantum/chemistry science
> platform, not a general wet-lab system. Comparisons to biology agents are
> about *workflow capability*, not domain parity. Wet-lab robotics is treated
> as out of scope unless Kubona explicitly expands into it.

---

## 1. What the frontier actually established (2025–2026)

| # | Finding | Evidence |
|---|---|---|
| F1 | **End-to-end paper generation is real but narrow.** The AI Scientist generated a fully AI-written paper that passed blind peer review at an ICLR 2025 workshop (scores 6/7/6, mean 6.33); its Automated Reviewer reached 69% balanced accuracy against human decisions. Authors still report naive ideas, weak methodological rigor, and citation errors. | Lu et al., *Nature* 651 (2026), `s41586-026-10265-5`; Sakana AI (2026-03-26) |
| F2 | **Hypothesis generation is a search problem, not a prompt.** Co-Scientist runs generation, reflection, ranking (Elo tournament), evolution, proximity, and meta-review agents with a supervisor and persistent context, scaling test-time compute; it was validated in three biomedical settings with wet-lab follow-up. | Gottweis et al., *Nature* 655 (2026), `s41586-026-10644-y` |
| F3 | **Lab-in-the-loop closes the loop.** GPT-5 + Ginkgo's cloud lab ran six rounds / 36,000 reactions / 580 plates and cut cell-free protein synthesis cost 40%; Co-Scientist's 2026 extension planned experiments, drove a CVD reactor, and wrote manuscripts; A-Lab GPSS ran 352 air-sensitive samples; AutoDNA orchestrated 20+ instruments over 11,000 steps. | OpenAI (2026-02-05); Schmidgall et al. arXiv `2608.26701`; A-Lab GPSS arXiv `2604.11957`; AutoDNA *Nature* `s41586-026-11228-6` |
| F4 | **Literature-grounded analysis agents work.** Robin (FutureHouse) combined PaperQA2 search agents with data-analysis agents, read 551 papers in 30 min, and produced experimentally validated dAMD candidates (ripasudil, KL001). Its own limits: protocols are outlines, not executable; analysis needs expert prompts. | *Nature* `s41586-026-10652-y` (2026-05-19) |
| F5 | **Long-horizon state matters.** Kosmos uses structured world models to stay coherent over ~1,500 papers and ~42,000 lines of analysis code; it reports 79.4% conclusion accuracy (self-estimated) and full traceability to code/literature. | Edison Scientific (2025-11-05) |
| F6 | **Teams beat a single agent on ideation.** Virtual Lab's AI PI + specialist agents + critic designed 92 nanobodies, two with improved binding; Agent Laboratory showed human feedback at stage boundaries improves quality and cut cost 84%; AgentRxiv showed shared reports improve subsequent agent laboratories (11.4% relative on MATH-500). | Swanson et al., *Nature* 646 (2025); Schmidgall et al. arXiv `2501.04227`; arXiv `2503.18102` |
| F7 | **Benchmarks say autonomy is not solved.** ScienceAgentBench: best agent 32.4% of 102 tasks (42.2% with o1-preview at >10× cost). CORE-Bench hard: strong scaffolds reach ~78% after fixes, but early agents ~21%. ResearchClawBench: best autonomous agent 21.5/100, frontier mean 24.6; failures concentrate in *protocol mismatch, evidence mismatch, missing scientific core* — not report formatting. SCI-AGENT-EXAM: mean 80.9 but only 26% of runs exceed 0.95. SciAgentArena: agents struggle with genuine novelty and self-directed exploration. | Chen et al. arXiv `2410.05080`; Siegel et al. CORE-Bench; ResearchClawBench arXiv `2606.07591`; SCI-AGENT-EXAM arXiv `2610.32441`; SciAgentArena arXiv `2606.12736` |
| F8 | **Provenance is becoming an interoperability contract.** Workflow Run RO-Crate + W3C PROV, PROV-AGENT (prompts/responses/decisions), and PROV-AGENT-LOOP (objective→plan→act→evaluate loops) are the emerging way to make agent actions auditable; aiprov adds human-only verification rungs and per-claim attribution. | Leo et al., *PLOS ONE* (2024); PROV-AGENT IEEE 11181558; PROV-AGENT-LOOP SC-W 2026; arXiv `2607.25637` |

**The cross-cutting result:** the bottleneck is no longer writing or code
execution. It is **scientific judgment under uncertainty** — choosing the
right protocol, evidence chain, and mechanism, and refusing to overclaim.
ResearchClawBench's error taxonomy is the clearest statement of this.

---

## 2. Kubona's actual position (code-grounded)

### 2.1 What is genuinely strong

| Capability | Evidence | Frontier comparison |
|---|---|---|
| Ledger-bound execution | `pkg/ledger`, signed result receipts, `experiment-export`, chain verify | Stronger provenance than most research agents |
| Spend/cost governance | `pkg/mandates`, preflight/settlement, loop budget pauses | Rare among science agents |
| Real quantum execution | QPU/simulator bridge, calibration, ZNE/mitigation, measured results | Domain advantage |
| MCP/A2A/AP2 surfaces | 6 servers / 38 tools; `pkg/agents/a2a.go`; tenant grants | Strong agent interoperability |
| Multi-agent critique | `pkg/agents/collective.go:403` tribunal (scribe, ansatz scout, device auditor, disconfirmation officer, human reviewer) | Comparable in spirit to Virtual Lab critic, narrower in roles |
| Loop engine | `pkg/autoresearch/engine.go`: pause/resume/sleep, budget breach, branch manager, stagnation | Solid runtime; no hypothesis tree |
| Publication pipeline | deterministic review + signed attestation + reproducible PDF + deposit | Stronger integrity than AI Scientist's self-review |

### 2.2 What is partial

| Capability | Current state | Gap |
|---|---|---|
| Hypothesis generation | Tribunal prompt; Python `DiscoveryCampaign` suggests one float; Go discovery protocol runs a fixed step list (`pkg/api/discovery_protocol.go:304`) | No hypothesis portfolio, dedupe, ranking, or evolution |
| Literature grounding | OpenAlex lookup (`/api/literature`), citation persistence | No synthesis, novelty measurement, or citation verification |
| Experiment design | Harnesses tune ansatz/QAOA/mitigation/compilation (`pkg/autoresearch/harness.go`) | No protocol synthesis or pre-registered acceptance criteria |
| Research state | Campaign JSON + Hindsight memory + ledger | No long-horizon structured graph / context compaction at Kosmos scale |
| Human oversight | Spend approvals, obligations, tribunal reviewer | No per-stage autonomy levels or stage gates |
| Evaluation | `pkg/evals` scores recorded LLM decisions | No end-to-end science benchmark; no pass@k/cost/stability |

### 2.3 What is absent

- Hypothesis tournament with literature-grounded novelty and falsifiability.
- End-to-end benchmark harness (ScienceAgentBench / CORE-Bench /
  ResearchClawBench-style) for Kubona's own domains.
- Protocol/experiment planner with a typed plan and stop rules.
- Claim-level verification of prose against execution logs/code.
- RO-Crate/PROV export and CRediT-style agent attribution.
- Safety/dual-use screening of research directions.
- Autonomous/co-pilot stage modes with explicit checkpoints.
- Cross-lab cumulative knowledge exchange (AgentRxiv-like).

---

## 3. Gap register

### P0 — blocks the "agentic lead" claim

| ID | Gap | Why it matters | Evidence |
|---|---|---|---|
| **AG-P0-1** | No end-to-end evaluation of autonomy | Without a task suite, "autonomous" is unmeasured; the frontier shows even strong agents score ~21–32% on re-discovery. | ResearchClawBench; ScienceAgentBench |
| **AG-P0-2** | No hypothesis engine | Kubona executes experiments but does not lead ideation; SOTA treats hypotheses as a ranked, evolving population. | Co-Scientist; AI Scientist |
| **AG-P0-3** | No structured research state / long-horizon memory | Campaign JSON cannot hold 1,500 papers / 42k lines of analysis; restart loses in-memory engines. | Kosmos; `engine.go:77-79` |
| **AG-P0-4** | No protocol planner with acceptance criteria | Harnesses optimize parameters, not scientific protocols; benchmark failures concentrate here. | ResearchClawBench error analysis |
| **AG-P0-5** | No claim-level verification of generated prose | Numeric/citation claims are not checked against execution; SOTA reports 46% fabrication without verification. | Co-Scientist extension (4% vs 46%); AI Scientist limitations |

### P1 — required for a trustworthy autonomous workflow

| ID | Gap | Recommendation |
|---|---|---|
| AG-P1-1 | Literature is lookup-only | Add literature synthesis + novelty distance + citation verification |
| AG-P1-2 | Tribunal is one debate, not a tournament | Add generation/reflection/ranking/evolution/meta-review roles |
| AG-P1-3 | No autonomy levels / stage gates | Add suggest / approve / act levels and human checkpoints per stage |
| AG-P1-4 | No failure taxonomy or economics | Instrument protocol/evidence/core failures, cost per accepted claim, human minutes |
| AG-P1-5 | No provenance interoperability | Export RO-Crate/PROV with agent decisions and per-claim attribution |
| AG-P1-6 | No dual-use/safety screen | Domain risk classifier + protocol screening + human signoff |
| AG-P1-7 | No cumulative lab knowledge | Shared hypothesis/negative-result registry across tenants (AgentRxiv-like) |

### P2 — polish and scale

- Reviewer calibration against human decisions.
- Multimodal ingestion (spectra, figures) and figure synthesis.
- Data/ELN/LIMS integrations and instrument protocol IR.
- Venue/DOI workflows beyond staged deposit.
- Bilingual manuscript completion.

---

## 4. Recommended plan (AG-series)

Ordered so each slice produces a measurable capability, not a demo.

### AG-1 — End-to-end benchmark harness (P0)
Build `platform/bench/agentic/` with 10–20 Kubona-domain re-discovery tasks:
H2/LiH/BeH2/H2O VQE, spin-chain gaps, DMRG thermodynamic limit, reaction
energies, mitigation, and one publication-from-campaign task. Each task has a
task statement, data/seed, hidden target, and a rubric; run against an isolated
stack. Metrics: task score, pass@k, cost, wall time, human interventions,
failure label.

**Acceptance:** the suite runs in CI/nightly; a baseline score is published
with the exact commit; adding a feature cannot silently lower it.

### AG-2 — Hypothesis engine (P0)
A `hypotheses` package/table with: generated hypothesis, literature refs,
falsifier, novelty status, Elo/score, provenance. Agents: Generator,
Proximity/dedupe, Reflection, Ranking, Evolution, Meta-review. Literature via
OpenAlex/Crossref; novelty is measured (distance/prior art) or explicitly
unmeasured. Hypotheses are claims; only collapsed hypotheses enter planning.

**Acceptance:** ≥10 hypotheses per goal, deduped; a human shortlist; every
hypothesis has a falsifier; novelty either measured or marked unmeasured.

### AG-3 — Protocol planner (P0)
Typed `ExperimentPlan`: hypothesis id, method, observable, acceptance metric,
budget, stop rule, falsifier, executor (bridge job/harness/QPU). Reuse the
discovery-protocol step runner; require a plan before execution. Human
approval policy by risk and cost.

**Acceptance:** a plan can be generated, approved, executed, and scored; an
experiment without an acceptance metric refuses.

### AG-4 — Structured research state (P0)
Persist a research graph (goal → hypothesis → plan → run → evidence →
decision → negative result) with token-budgeted summaries and auto-resume
after restart. Reuse campaign state, Hindsight, ledger events. Bound context
like Kosmos/ResearchHarness compaction.

**Acceptance:** a restart resumes a sleeping loop; the graph reconstructs a
campaign's full rationale; no context exceeds the configured budget.

### AG-5 — Claim and citation verification (P0/P1)
Extract numeric and citation claims from manuscripts; verify numbers against
result JSON/logs and citations against DOI/OpenAlex; block signoff on
unsupported claims. Calibrate the reviewer against a small human-labeled set.

**Acceptance:** a fabricated number or dead citation blocks publication; the
reviewer's agreement with human labels is reported.

### AG-6 — Provenance interoperability (P1)
Export RO-Crate/PROV (agent decisions, prompts/responses, tool calls, claims,
verification rungs) alongside the manuscript. Add CRediT-style attribution for
human and agent contributors.

**Acceptance:** an external PROV/RO-Crate consumer can reconstruct who did
what, with which inputs, and which claims were verified.

### AG-7 — Autonomy levels and safety (P1)
Stage-level modes: suggest (human executes), approve (agent executes after
approval), act (bounded autonomous). Risk classifier for domain/protocol;
human signoff for flagged directions; kill switch and budget caps.

**Acceptance:** a flagged protocol cannot run without signoff; every stage
records its autonomy level.

### AG-8 — Observability and economics (P1)
Stage success/failure taxonomy, cost per accepted claim, human minutes,
queue/loop health, and a dashboard.

**Acceptance:** an operator can answer "what did the agent do, what did it
cost, and where did it fail" from metrics alone.

### AG-9 — Cumulative lab knowledge (P2)
A shared, provenance-bound registry of hypotheses, negative results, and
reports across tenants (AgentRxiv-like), with A2A/tenant grants controlling
access.

**Acceptance:** a second campaign retrieves a prior negative result and does
not repeat it; access is tenant-scoped.

---

## 5. What to reuse (do not rebuild)

- `pkg/autoresearch` harness interface + LoopEngine (pause/resume/budget).
- `pkg/agents/collective.go` tribunal + `pkg/epistemics` claims.
- `pkg/agents/campaign_synth.go` + `replan.go` campaign projections.
- `pkg/memory` Hindsight (retain/recall/semantic).
- `pkg/api/discovery_protocol.go` step runner.
- `pkg/mandates` + obligations for approvals.
- `pkg/evals` fixture pattern (extend to end-to-end tasks).
- Publication pipeline (manuscripts, signed comparator attestation, deposit).
- MCP catalog + A2A for tool/agent interop.

## 6. Non-goals / honest boundaries

- No wet-lab robotics; Kubona is dry-lab (simulator + QPU + classical).
- No claim of discovery-level autonomy until AG-1 shows it on Kubona tasks.
- No LLM-only reviewer as the publication gate; the deterministic rule engine
  and signed evidence remain authoritative.
- No cross-domain parity claim with biomedical agents.

## 8. Implementation (2026-11-18)

### AG-1a — benchmark harness and baseline

- `platform/bench_agentic/runner.py`: drives the real HTTP API, creates a
  session per task, submits the experiment, polls the job, scores the measured
  field against a hidden target, and reports pass@k, cost, wall time, and
  failure labels.
- `platform/bench_agentic/tasks.json`: H2 VQE, H2 classical-anchor
  verification, and TFIM spin-chain self-consistency.
- Unit tests: `platform/tests/test_agentic_bench.py` (scoring, target
  resolution, dedupe/contract checks; no network).

Baseline on a disposable stack (schema 5, 2026-11-18):

| Task | Result | Measured | Target | Delta | Failure |
|---|---|---|---|---|---|
| `h2_vqe_ground_state` | pass | -1.132769 Ha | -1.137270 Ha | 4.50 mHa | — (chemical accuracy not met) |
| `h2_classical_anchor` | pass | -1.137270 Ha | -1.137270 Ha | 0.0 | — |
| `tfim_spin_chain_exact` | **fail** | -9.024414 Ha | -10.251662 Ha | 1.227 Ha | tolerance |

**Baseline: 2/3 pass@1, $0.00, 16.7 s total.** The TFIM failure is a real
accuracy gap (shot-based 2-layer VQE vs the exact anchor), not a harness bug —
exactly the class of failure ResearchClawBench reports. AG-1b (nightly CI job
with a published baseline artifact) remains open.

### AG-2a — hypothesis engine

- `pkg/hypotheses`: deterministic, evidence-grounded generation from the
  campaign state (discrepancy, hardware, claimability, missing baseline,
  prior art, negative controls), with de-duplication and score ranking.
- Ledger `hypotheses` table (schema 5), session-scoped API:
  `POST/GET /api/sessions/{sid}/hypotheses`, `PATCH …/{id}` for
  shortlist/reject/collapse; literature refs attach from the bridge when
  available; novelty is explicitly `unmeasured`.
- Tests: generator rules/dedupe/ranking; API generation, shortlist,
  cross-session isolation.

Runtime on the H2 benchmark campaign: **10 hypotheses**, direct experiments
ranked above prior-art checks, every candidate has a falsifier, shortlist
transition returns 200. AG-2b (LLM proposer, tournament evolution, meta-review,
measured novelty) remains open; the API and storage are the substrate for it.

### Still open

AG-1b, AG-2b, AG-3 (protocol planner), AG-4 (structured research state),
AG-5 (claim/citation verification), AG-6 (RO-Crate/PROV export), AG-7
(autonomy/safety), AG-8 (telemetry), AG-9 (cumulative lab knowledge).

## 7. Source notes and limitations

- Benchmark numbers are not directly comparable across suites; they measure
  different task slices (code, reproduction, re-discovery, workflow).
- Several 2026 systems are company-reported; where possible the primary paper
  or arXiv is cited. Self-estimated accuracy (Kosmos 79.4%) is labeled as such.
- The Lean comparator attestation boundary is implemented; the live Lean
  sandbox image is still operator-built, so no real kernel proof has been run
  in this environment.
- Web research date: 2026-11-18. Re-check the source list before citing these
  numbers in a publication.
