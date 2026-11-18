---
type: source
name: Agentic science frontier 2026 (web research)
date: 2026-11-18
updated: 2026-11-18
tags: [source, agentic-science, autonomy, benchmarks, provenance, self-driving-lab]
origin: "Web research conducted 2026-11-18; no single raw document. Primary sources listed below, with publication dates and the claims each supports."
status: source
---

# Agentic science frontier 2026

Research pass for [[agentic-science-frontier-2026]]. The full synthesis,
Kubona capability map, gap register, and AG-series plan are in
`docs/agentic-science-frontier-2026-11-18.md`.

## Primary sources

| Source | Date | Used for |
|---|---|---|
| Lu et al., “Towards end-to-end automation of AI research”, *Nature* 651 — https://www.nature.com/articles/s41586-026-10265-5 | 2026-03-25 | AI Scientist end-to-end paper, one workshop acceptance (6/7/6), Automated Reviewer 69% balanced accuracy, stated limitations |
| Sakana AI, “The AI Scientist … Now Published in Nature” — https://sakana.ai/ai-scientist-nature | 2026-03-26 | Context, v2 tree search, peer-review experiment |
| Gottweis et al., “Accelerating scientific discovery with Co-Scientist”, *Nature* 655 — https://www.nature.com/articles/s41586-026-10644-y | 2026-05-19 | Generation/reflection/ranking/evolution/proximity/meta-review agents, Elo tournament, persistent context, three wet-lab validations |
| Schmidgall et al., Co-Scientist lab extension — https://arxiv.org/html/2608.26701 | 2026-08-27 | Experiment planning, lab equipment, manuscript generation; 4% vs 46% fabrication with/without verification; safety rejection |
| “A multi-agent system for automating scientific discovery” (Robin), *Nature* — https://www.nature.com/articles/s41586-026-10652-y | 2026-05-19 | Literature + data-analysis agents, 551 papers/30 min, dAMD candidates, stated limits |
| Edison Scientific, “Kosmos” — https://edisonscientific.com/news/announcing-kosmos | 2025-11-05 | Structured world models, 1,500 papers/42k lines, 79.4% self-estimated accuracy, traceability |
| Swanson et al., Virtual Lab, *Nature* 646 — https://www.nature.com/articles/s41586-025-09442-9 | 2025-07-29 | AI PI + specialist agents + critic, 92 nanobodies, two validated |
| Schmidgall et al., Agent Laboratory — https://arxiv.org/abs/2501.04227 | 2025-01-08 (EMNLP 2025) | Human idea → literature/experiment/write-up; co-pilot vs autonomous; 84% cost reduction; automated-review overestimate |
| Schmidgall & Moor, AgentRxiv — https://arxiv.org/abs/2503.18102 | 2025-03-23 | Shared reports between agent labs; 11.4% relative improvement |
| Chen et al., ScienceAgentBench — https://arxiv.org/abs/2410.05080 | 2024-10-07 (v3 2025-03-31) | 102 tasks/44 papers; best agent 32.4%, 42.2% with o1-preview at >10× cost |
| Siegel et al., CORE-Bench; HAL leaderboard — https://hal.cs.princeton.edu/ | 2024/2026 | Computational reproducibility; scaffold-dependent gains; “CORE-Bench solved” note |
| ResearchClawBench — https://arxiv.org/html/2606.07591v2 | 2026-06-10 | 40 tasks/10 domains; best agent 21.5/100; protocol/evidence/core failure taxonomy |
| SCI-AGENT-EXAM — https://arxiv.org/abs/2610.32441 | 2026-10-23 | 120 tasks/11 fields; 80.9 mean but only 26% >0.95 |
| SciAgentArena — https://arxiv.org/abs/2606.12736 | 2026-06-10 | ~200 tasks; weakness in novelty and self-directed exploration |
| OpenAI, “GPT-5 lowers the cost of cell-free protein synthesis” — https://openai.com/index/gpt-5-lowers-protein-synthesis-cost/ | 2026-02-05 | 6 rounds, 36k reactions/580 plates, 40% cost reduction, human oversight |
| A-Lab GPSS — https://www.alphaxiv.org/abs/2604.11957 | 2026-04-13 | 352 air-sensitive samples; abductive/inductive agents; hit-rate increase |
| AutoLabs, *Scientific Reports* — https://www.nature.com/articles/s41598-026-45593-z | 2026-06-25 | Self-correcting protocol generation; F1 > 0.89; reasoning capacity key |
| AutoDNA, *Nature* — https://www.nature.com/articles/s41586-026-11228-6 | 2026-11-04 | 20+ instruments, 11k steps, 3.2× concurrent protocols |
| Physical SDL testbed — https://arxiv.org/abs/2611.11690 | 2026-11-08 | 24 h unattended; agentic optimizer vs BO |
| Workflow Run RO-Crate, *PLOS ONE* — https://journals.plos.org/plosone/article?id=10.1371/journal.pone.0309210 | 2024-09-10 | Workflow provenance packaging aligned to W3C PROV |
| PROV-AGENT — https://ieeexplore.ieee.org/document/11181558 | 2025-09-18 | Agent prompts/responses/decisions in PROV |
| PROV-AGENT-LOOP — https://doi.org/10.1109/SCW72768.2026.00161 | 2026-11-12 | Objective→plan→act→evaluate loop provenance |
| aiprov / F(AI)²R — https://arxiv.org/html/2607.25637v2 | 2026-08-07 | Per-claim attribution, human-only verification rungs |
| Deep Research survey — https://arxiv.org/pdf/2508.12752 | 2025-08-19 | Taxonomy of single/multi-agent research workflows |

## Caveats

- Benchmark scores measure different slices and are not directly comparable.
- Several 2026 systems are company-reported; primary papers/preprints are used
  where available, and self-estimated numbers are labeled as such.
- Kubona is a quantum/chemistry dry-lab platform; wet-lab comparisons are
  workflow analogies, not domain parity.
