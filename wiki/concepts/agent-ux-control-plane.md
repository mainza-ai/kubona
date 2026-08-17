---
type: concept
name: Agent UX & the Control Plane
date: 2026-08-16
tags: [ux, control-plane, hitl]
sources: [source-gap-analysis-plan]
status: living
---

# Agent UX & the Kubona Control Plane

Kubona's UI sits at the intersection of two disciplines that rarely meet
(source-gap-analysis-plan):

1. **Quantum circuit visualization** — a design space where even 2026 studies call
   platform-agnostic rendering an **open problem**, because visualization is tightly coupled
   to the originating SDK. Kubona inherits this by being multi-SDK from day one. Prior-art
   reference points: [[ibm-quantum-composer]] (bidirectional code↔diagram editor) and
   [[qflow-studio]] (credential-safe evidence sharing).
2. **Agent UX** — an accountability layer: surface *what the system is doing, why it chose an
   action, allow override at any point, recover gracefully from errors.*

No existing tool (quantum or agentic) covers both.

## The eight console surfaces

| Surface | Purpose |
|---|---|
| **Circuit visualizer** | Renders any post-QIR circuit (SDK-independent); bidirectional code↔diagram sync; Q-sphere/statevector/histogram on local sim |
| **Cost & approval console** | Human side of the [[cost-governance]] gate: burn-rate, pending-approval queue, budget ceilings, spend history |
| **Live session / loop inspector** | Watch/interrupt a multi-hour VQE loop: replay/scrub, parameter trace, pause/override/kill, barren-plateau & WBP alerts inline |
| **Provenance / experiment browser** | Search ledger by circuit-hash/backend/mitigation; side-by-side comparison; credential-safe evidence export (à la [[qflow-studio]]) |
| **Provider / fleet status** | Queue depth, fidelity trend, canary-CI pass/fail, jurisdiction/export-control flags |
| **Plugin/provider registration console** (Phase 4+) | Register & test new provider integrations against the capability contract |
| **Experiment notebook & results studio** (Phase 6) | Structured reports — objectives/methods/protocol/results/analysis/conclusions/citations + *how to reproduce*; every section links ledger records; frame-level annotation of figures (à la [[buzz]]) |
| **Lab, workflows & agents view** (Phase 6) | Author/approve [[workflows-and-agent-authoring|workflows & agents]], agent registry, delegation & revocation, lab/team membership ([[user-identity-and-profiles]]) |

## Interaction principles

- **Progressive disclosure** — deep circuits default to summarized stats, drill-down on demand.
- **Confidence/uncertainty viz** — distinguish raw vs. error-mitigated values; show technique
  and its variance cost inline.
- **Status & activity** — the loop inspector always answers "what is the agent doing now,
  and why" in one glance.
- **Error handling & recovery** — failed jobs suggest an actionable next step (switch
  backend, request budget, fall back to simulator).

## Two hard rules

- **Read/approve only** — the console is oversight on top of agent actions, never a second
  path to submit jobs (avoids reopening the credential/spend risk).
- **Fixed, audited human surfaces** for approval/circuit/provenance — generative /
  agent-materialized UI (MCP Apps, A2UI) is a v2+ direction for ad-hoc visualizations only,
  given the security sensitivity.

Related: [[qflow-studio]] · [[cost-governance]] ·
[[provenance-and-reproducibility]] · [[qir-mlir-compilation]]