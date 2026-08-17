---
type: concept
name: Science Discovery Platform
date: 2026-08-16
tags: [kubona, science, discovery, experiments]
sources: [source-gap-analysis-plan, source-architecture-paper]
status: living
---

# Kubona as an AI-Enhanced Science Discovery Platform

Kubona is defined as both an agent toolset for quantum computing **and** a **core science
discovery platform** — where
users and agents design, run, and archive experiments, and where the platform itself can
conduct experiments autonomously and return results in an organized, systematic form.
Quantum hardware ([[qir-mlir-compilation]], [[provider-capability-schema]]) is one
instrument family; literature, public datasets, and classical compute are the others.

## The discovery loop (what "conducting an experiment" means end-to-end)

| Stage | What happens | Kubona mechanism |
|---|---|---|
| 1. **Ground** | Retrieve literature & prior art; compare against domain data | [[science-data-integrations]] (arXiv, OpenAlex, …) as MCP resources |
| 2. **Hypothesize** | Agent proposes a hypothesis/ansatz/protocol; human or policy approves | Workflow authoring + approval gates ([[workflows-and-agent-authoring]]) |
| 3. **Design** | Protocol, controls, preregistration, cost pre-flight, simulator dry-run | [[cost-governance]]; experiment records in the ledger |
| 4. **Experiment** | Runs on QPU, simulator, or data API; iterative loops | [[nisq-and-hybrid-orchestration]], [[karpathy-autoresearch]] harnesses |
| 5. **Analyze** | Error mitigation, statistical validation, comparison vs. baselines | [[error-mitigation]], [[evaluation-and-benchmarks]] |
| 6. **Report** | Structured, reproducible result artifact with citations & ledger links | Experiment notebook + results studio ([[agent-ux-control-plane]]), [[provenance-and-reproducibility]] |
| 7. **Learn** | Library grows; meta-loop revises search strategy | [[meta-search-and-self-evolution]] flywheel (§13) |

## Self-experimentation

The platform can run whole campaigns on its own: hypothesis sweeps across problem classes,
backend/noise configurations, ansatz families, and mitigation strategies — each run inside
the same safety rail (approval gates, simulator pre-flight, budget ceilings). The
[[karpathy-autoresearch]] engine (§11) is the object-level machinery; the §13 meta-loop is
the reflection tier; both are prerequisites before the platform graduates to proposing
full experiments independently.

## "Organized and systematic" results

Every experiment yields two artifacts, both keyed by the same content address:

- **Machine-readable:** the immutable ledger record (protocol → backend → data → analysis →
  cost → report ref) queryable by agents as an MCP resource.
- **Human-readable:** a structured report (objectives · methods · protocol · results ·
  analysis · conclusions · citations · *how to reproduce*) in the experiment notebook,
  with credential-safe sharing (à la [[qflow-studio]]).

## What this adds to the roadmap

Multi-user labs, user/agent-created workflows and agents, and the result-reporting layer
are scoped as **Phase 6 (Science Discovery & Collaboration)** — see [[roadmap]]; identity
groundwork lands earlier (Phase 2). Related: [[user-identity-and-profiles]] ·
[[workflows-and-agent-authoring]] · [[science-data-integrations]] · [[buzz]] · [[unsloth]]
