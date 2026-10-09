---
type: concept
name: Spend Mandates & Inter-Agent Delegation
date: 2026-10-03
updated: 2026-10-09
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
- In the production console (`platform/console/components/WebCryptoKeyring.tsx`), the browser maintains a secure Ed25519 keypair in browser storage via W3C WebCrypto API. Mandates are signed client-side over a SHA-256 canonical hash string.
- The Go execution orchestrator (`platform/core/pkg/api/server.go:/api/mandates/verify`) cryptographically validates the signature via Go `crypto/ed25519.Verify` before dispatching to any provider queue.
- The signed mandate is permanently stored in the [[provenance-and-reproducibility|content-addressed audit ledger]],
  providing non-repudiable proof of human authorization.

### 2. Inter-Agent Delegation Boundaries via A2A
When research agents author or delegate tasks to specialized child agents ([[workflows-and-agent-authoring]]),
exposing internal tool credentials violates the principle of least privilege. Under [[a2a|A2A]]:
- Subagents publish capability "Agent Cards" rather than raw tool schemas.
- Tasks are delegated asynchronously using formal state transitions (`submitted` → `working` → `input-required` → `completed`).
- **Autonomy Inheritance:** A child agent does not inherit the parent's spend ceiling; it receives
  an explicitly bounded sub-mandate that cannot exceed the parent's authorized budget.

## Delegation chain rules (Q-12, closed 2026-10-09)

Budget was already bounded (above); the remaining semantics are now decided and implemented
(`platform/core/pkg/agents/a2a.go`, `platform/core/pkg/jobs/jobs.go`):

1. **Scope narrowing only.** A child's scopes must be a subset of its parent's scopes
   (server-resolved from the stored parent task, never client-claimed). Widening refuses.
   Root delegations start with the collective's advertised capabilities.
2. **Depth cap 2.** Root = depth 0, child = 1, grandchild = 2; deeper chains refuse.
   Chains, not just edges, are bounded.
3. **Revocation propagates.** `RevokeTask` settles queued/running tasks as revoked (completed
   history is immutable); children of revoked/failed parents cannot be created; in-flight runs
   fence on the flag. Tenant autonomy revocation (`POST /api/tenants/revoke`) additionally
   aborts every unsettled job in the tenant's sessions (`CancelTenantJobs`) and L0 tenants
   refuse all new dispatch (checked live in `runTask`, `handleJobs`, `handleChatLaunch` —
   no cached grants).
4. **Tenant attribution.** Delegated execution runs in a per-tenant `a2a-delegation-<tenant>`
   session derived from the parent mandate's tenant, so spend and autonomy resolve against
   the delegating lab — never a shared pseudo-session.

Related: [[cost-governance]] · [[protocol-strategy]] · [[user-identity-and-profiles]] · [[workflows-and-agent-authoring]]
