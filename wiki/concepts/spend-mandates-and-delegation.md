---
type: concept
name: Spend Mandates & Inter-Agent Delegation
date: 2026-10-03
updated: 2026-10-03
tags: [governance, ap2, a2a, security, delegation, mandates, trust-boundary]
sources: [source-agent-protocol-landscape-2026, source-gap-analysis-plan]
status: adopted
---

# Spend Mandates & Inter-Agent Delegation

The cryptographic governance mechanisms securing autonomous financial transactions and
inter-agent task delegation across the Kubona platform (source-agent-protocol-landscape-2026).

## The Authorization Deficit in Autonomous Agents

Standard agentic tool protocols ([[mcp-and-context-flooding|MCP]]) permit an agent holding an API
bearer token to invoke functions on an MCP server. However, an ambient token cannot prove to a
quantum provider, accounting ledger, or auditor whether a specific expensive hardware job was
explicitly authorized by the human operator, or resulted from an LLM hallucination or prompt
injection attack.

Kubona resolves this via two interlocking standards:

### 1. Spend Mandates via AP2
Borrowing from the [[ap2|Agent Payments Protocol (AP2)]] specification, Kubona models the
[[cost-governance|two-stage approval gate]] as a **cryptographic spend mandate**:
- When estimated circuit execution cost exceeds a tenant's autonomy ceiling (e.g. >$50.00),
  the cost server issues an authorization challenge.
- The human operator signs a structured mandate specifying: `{tenant_id, job_hash, max_budget_usd, expiry_timestamp}`.
- The execution server verifies the signature before submitting the job to the hardware backend.
- The signed mandate is permanently stored in the [[provenance-and-reproducibility|content-addressed audit ledger]],
  providing non-repudiable proof of human authorization.

### 2. Inter-Agent Delegation Boundaries via A2A
When research agents author or delegate tasks to specialized child agents ([[workflows-and-agent-authoring]]),
exposing internal tool credentials violates the principle of least privilege. Under [[a2a|A2A]]:
- Subagents publish capability "Agent Cards" rather than raw tool schemas.
- Tasks are delegated asynchronously using formal state transitions (`submitted` → `working` → `input-required` → `completed`).
- **Autonomy Inheritance:** A child agent does not inherit the parent's spend ceiling; it receives
  an explicitly bounded sub-mandate that cannot exceed the parent's authorized budget.

Related: [[cost-governance]] · [[protocol-strategy]] · [[user-identity-and-profiles]] · [[workflows-and-agent-authoring]]
