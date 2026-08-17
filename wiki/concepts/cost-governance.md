---
type: concept
name: Cost Governance
date: 2026-08-16
tags: [cost, budget, governance]
sources: [source-gap-analysis-plan, source-architecture-paper]
status: living
---

# Cost Governance

The mechanism that keeps agent-driven hardware spend controlled. A single
mis-configured [[vqa-vqe|VQE]] run with the wrong shot count can burn thousands of
dollars on trapped-ion hardware in minutes — so governance is a **functional requirement**,
not a nice-to-have (source-gap-analysis-plan).

## The four mechanisms

1. **Mandatory simulator pre-flight** — no job reaches a paid queue without a successful
   local/GPU simulator dry-run and a cost estimate from actual shot count × pricing model.
2. **Per-agent/session budget ceiling** — set by the human operator in the console; enforced
   at the MCP gateway (not just logged).
3. **Two-stage approval gate** for anything above a configurable threshold — human approval
   in the loop for high-cost spend (pattern validated in Mainza's Finance Claw).
4. **Real-time burn-rate telemetry** surfaced back to the agent as an MCP resource, so it can
   self-throttle (cut shots, switch backends) before hitting the ceiling.

## The payment rail underneath

Below the approval threshold, sub-cent-to-sub-dollar transactions (simulator-tier runs,
algorithm-library lookups) run through [[x402]] instead of a human queue — additive, not a
replacement for the hard rail above. The audit ledger doubles as the payment-proof ledger.

## Placement in the plan

Non-optional in **Phase 1** (not deferred); the cost-gov MCP server is one of the four
federated servers (see [[mcp-and-context-flooding]]). Console pairing:
[[agent-ux-control-plane|Cost & approval console]].

Related: [[provenance-and-reproducibility]] · [[security-and-pqc]] · [[x402]]