---
type: concept
name: User & Agent Identity and Profiles
date: 2026-08-16
updated: 2026-10-09
tags: [identity, users, agents, profiles, labs]
sources: [source-gap-analysis-plan, source-architecture-paper]
status: adopted
---

# User & Agent Identity and Profiles

The identity model that makes Kubona a multi-user, multi-agent platform (maintainer
synthesis, borrowing the [[buzz]] pattern). Today's plan has OAuth 2.1 for humans and
per-call identity-aware authorization for the platform agent; this extends it so **every
actor — human, platform agent, or user-created agent — is a first-class identity** with a
profile, scoped access, and a signed audit trail.

## Principles (from Buzz's design)

1. **One identity model for people and processes.** Human and agent identities share the
   same shape (keypair → profile → memberships → audit trail). Scoped by identity, not by
   coarse permission flags.
2. **Agents are members, not bots.** An agent joins a lab/project the way a person does; it
   has its own keys, memberships, budget envelope, and provenance footprint.
3. **Delegated authority, instantly revocable.** An agent inherits its owner's access via a
   delegation attestation (the [[buzz|NIP-OA]] pattern): add a user, all their authorized
   agents gain access; remove the user, all their agents lose it immediately — no per-agent
   permission surgery.
4. **Community-local state.** A lab/team (community) scopes profiles, presence, jobs, DMs,
   memories, memberships, and audit chains by host/URL — a hosted operator may run many labs
   on shared infrastructure without cross-lab leakage.

## Profiles

- **User profile:** identity, lab memberships, roles (operator / researcher / reviewer),
  default budget ceilings, saved workflows, approval prefs, model/tool preferences.
- **Agent profile (persona):** capabilities (which MCP tools are in scope), owner +
  delegation attestation, budget envelope, status, and a public provenance footprint
  (what it has run, success rate) that feeds freshness/trust scores in the
  [[provenance-and-reproducibility]] browser.

## Where it lands in the plan

- Phase 2: identity groundwork — keypair identities for users/agents + owner-delegated
  authorization beneath the existing OAuth/PQC layer ([[security-and-pqc]]).
- Phase 6: full profiles, labs, personas, and the agent registry — see
  [[workflows-and-agent-authoring]] and [[roadmap]].

## Tenancy model (Q-13, closed 2026-10-09; operator ruling: tenant := lab)

A **tenant is a lab**: the billing, credential, and ceiling boundary. The admin is the
lab operator's Ed25519 key (local keypairs first — no OAuth/OIDC until Phase 4
multi-tenant, per ruling; an IdP would punch egress against Q-16).

- `Tenant{id, display_name, admin_pubkey_hex, autonomy_level, spend_ceiling_usd, status}`
  (`platform/core/pkg/tenants`); `tenants` table seeded with `default`; sessions and
  ledger events carry `tenant_id` (backfilled); memory recall, mandate clamps, job
  admission, ledger export, and A2A dispatch all resolve the lab tenant live.
- Admin bootstrap is first-write-wins (ledger-sealed); rotation, ceiling changes, and
  autonomy grants are admin-signed `AdminAction` envelopes (params-bound, 5-minute TTL).
- Vault leases scope to `tenant/<id>` (`SecretLease.tenant_id`; OpenBao namespaces
  per tenant at multi-tenant scale).
- Human membership inside a lab (join/invite/roles) is Phase-4 work; agents are never
  tenants — they carry owner delegation attestations with budget envelopes.

## Tenant suspension (Q-33, closed 2026-10-12; mutator chosen over removal)

`suspended` stays a real status with a real mutator: `POST /api/tenants/status`
(admin-signed `set_status` envelope, bearer + limiter rail) flips `active ↔ suspended`;
the prototype demonstrates the full lifecycle live (suspend → refused dispatch/ceiling
→ reactivate → dispatch proceeds).

- Suspension freezes spend-affecting actions only: dispatch, ceiling, autonomy, and
  key-rotation-by-others refuse with `tenant is suspended`, while the lab admin keeps
  the two recovery actions — **reactivation and key rotation** (a kill-switch that
  bricks recovery after a key compromise is a trap, not a control).
- Enforcement lives in one place, `verifyAdmin` (`platform/core/pkg/tenants/tenants.go`),
  so every mutation inherits it; dispatch checks `requireDispatchAutonomy` at every
  entry (`platform/core/pkg/api/server.go`).
- Tests: `TestTenantSuspendLifecycle` + `TestTenantSuspendedRecoveryPaths`
  (`platform/core/pkg/api/server_new_routes_test.go`).
