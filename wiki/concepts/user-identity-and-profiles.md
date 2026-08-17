---
type: concept
name: User & Agent Identity and Profiles
date: 2026-08-16
tags: [identity, users, agents, profiles, labs]
sources: [source-gap-analysis-plan, source-architecture-paper]
status: living
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
