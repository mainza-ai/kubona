---
type: concept
name: Cost Governance
date: 2026-08-16
updated: 2026-10-10
tags: [cost, budget, governance]
sources: [source-gap-analysis-plan, source-architecture-paper]
status: settled
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

## Autonomy ladder (Q-10, closed 2026-10-09)

Four levels, enforced live at every dispatch entry point (`platform/core/pkg/tenants`,
`requireDispatchAutonomy`, A2A runTask checker — no cached grants):

- **L0 — observe/suggest only.** Dispatch refuses everywhere (jobs, chat launch, A2A).
  Revocation lands here.
- **L1 — simulator-only, auto-approved.** The default for every new lab tenant.
  Simulator-class backends proceed on autonomy alone.
- **L2 — capped QPU.** Metered-hardware jobs additionally require a verified,
  unexpired, budget-covering spend-mandate receipt cited at submit time
  (`mandate_ref`, checked against the ledger inside `SubmitJob`).
- **L3 — program envelope.** Pre-approved budget with burn-rate kill-switch
  (telemetry + abort rail; envelope accounting is Phase-5 x402 work).

Grants and revocations are admin-signed envelopes (`AdminAction`: params-bound hash,
5-minute TTL, Ed25519) sealed as `tenant_autonomy_granted/revoked` ledger events.
What the approver sees at each level (burden-scaled approval packets) is decided
under [[questions]] Q-11 (closed 2026-10-09) — see Approval packet schema below.

## Approval packet schema (Q-11 design, 2026-10-09)

A competent approval needs the decision reconstructible an hour later without opening
five tabs. Every spend approval (console card and MCP elicitation) carries:

`packet_id, agent_id, backend, program_hash, qubits, shots, cost_max_usd,
budget_remaining_usd, cost_basis (price-sheet version + queue fee),
blast_radius, reversibility, evidence {dry_run, classical_anchor, quote_url,
notebook_url}, alternatives {sim_first_savings, fewer_shots_savings},
expires_at, nonce_single_use, requestor_principal,
decision {approve_once | approve_up_to_max | deny + reason}`

Transport rules: the full packet lives on the console card; only the decision
transits MCP, as a form-mode `elicitation/create` (flat schema: decision enum +
cost-ack number + reason) answered via the MRTR retry pattern (`input_required`
→ same-method retry with `inputResponses` + HMAC'd opaque `requestState`:
principal-bound, short TTL, single-use nonce). Denials return a reason into agent
context. Every decision logs `packet_id + decision + principal + time`.

Anti-fatigue rules (a gate, not theater): risk-tiered prompting (L1 simulator
auto-allows, destructive outright-blocked, only the middle prompts), step budgets,
and flat-latency / ~0%-reject telemetry treated as a smell, not health.

## Landed (Q-11 closed 2026-10-09)

- Protocol: MRTR approval retry in the Python gateway (`platform/kubona/mcp/approval.py`,
  `gateway.require_approval/call`): first call returns `input_required` with a form-mode
  `elicitation/create` (flat schema: decision enum + cost-ack + reason) and HMAC'd opaque
  `requestState` (principal-bound, 15-min TTL, single-use nonce); retry verifies and executes
  exactly once; decline/tamper/expiry/replay refuse without executing. All three outcomes
  audit-logged with `packet_id`.
- Surface: `ApprovalPacketCard` (`platform/console/src/components/common/`) renders the full
  schema (cost bound + budget left, blast radius, reversibility, evidence, alternatives,
  expiry), distinct approve-once / approve-up-to-max actions, and reason-mandatory denial —
  wired into the AutonomyCockpit approve flow with packet assembly factored as pure,
  unit-tested `buildApprovalPacketFromCampaign` (`src/lib/approvalPacket.ts`).

## Placement in the plan

Non-optional in **Phase 1** (not deferred); the cost-gov MCP server is one of the four
federated servers (see [[mcp-and-context-flooding]]). Console pairing:
[[agent-ux-control-plane|Cost & approval console]].

Related: [[provenance-and-reproducibility]] · [[security-and-pqc]] · [[x402]]