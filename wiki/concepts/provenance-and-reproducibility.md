---
type: concept
name: Provenance & Reproducibility
date: 2026-08-16
tags: [provenance, reproducibility, ledger]
sources: [source-gap-analysis-plan]
status: living
---

# Provenance & Reproducibility

The quantum-computing analog of MLflow/W&B experiment tracking, absent from the
architecture paper and added by the gap analysis (source-gap-analysis-plan). Critical for
research and regulatory contexts (pharma, finance), and for the
[[karpathy-autoresearch]] algorithm library's credibility.

## Content-addressed experiment records

Every job submission writes an immutable record:
**circuit hash → backend → noise model → mitigation technique → raw results → mitigated
results → cost → wall-clock time.** At the science-discovery layer
([[science-discovery]]) the record extends to the full experiment lifecycle: **hypothesis →
protocol → controls → data sources fetched (with license/TOU flags, per
[[science-data-integrations]]) → analysis → structured report reference → citations** — so
the ledger doubles as the experiment log and the citation backbone of every result.

## Queryable by agents as an MCP Resource

The ledger doubles as a reasoning substrate: *"what mitigation technique worked best for
this ansatz class on this backend historically?"* — the platform gets smarter with use
(base for the algorithm library; see [[agent-resource-economy]]).

## Additional roles

- Enables crash-recovery / horizontal scaling of the nominally-stateless MCP servers
  (state reconstructed from the ledger via `job_id`).
- Immutable audit trail of every tool call, credential use, and dollar spent (shared with
  [[security-and-pqc]] and [[cost-governance]]).
- Human front-end: provenance/experiment browser console surface with credential-safe
  evidence export (pattern borrowed from [[qflow-studio]]).

Related: [[cost-governance]] · [[security-and-pqc]] · [[karpathy-autoresearch]] ·
[[agent-ux-control-plane]]