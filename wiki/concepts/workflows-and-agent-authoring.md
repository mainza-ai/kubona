---
type: concept
name: Workflows & Agent Authoring
date: 2026-08-16
updated: 2026-09-27
tags: [workflows, agents, authoring, science]
sources: [source-gap-analysis-plan]
status: adopted
---

# Workflows & Agent Authoring (user-created *and* agent-created)

Maintainer synthesis. Today's plan hard-codes a handful of pathways (a VQE loop, an ansatz
search). This page opens the platform to **user-created and agent-created workflows
(and agents)** for anything — including scientific discovery and new research — so that
Kubona compounds what its users build, not just what its maintainers shipped.

## Workflows

A workflow is a **versioned, YAML-declared sequence of steps**, each step calling an MCP
tool (transpile, estimate, submit, analyze, fetch-literature, …), with:

- **Triggers:** manual, schedule, event (new result / new data / message), or webhook
  (Buzz pattern: message / reaction / schedule / webhook triggers, [[buzz]]).
- **Gates between steps:** cost approval ([[cost-governance]]) or capability approval
  (harness change, workflow change — the §13.4 tier, [[meta-search-and-self-evolution]]).
- **Dry-run mode:** every workflow runs through the simulator-tier first; nothing touches a
  paid queue without pre-flight.
> ⚑ Landed in part (2026-10-12): trigger values are enforced at parse
> (`manual|schedule|event`, unknown refused); `schedule`/`event` never fire
> — `run()` refuses them without `force=True` (no silent manual execution),
> with the opaque `schedule` mapping stored and ledger-stamped;
> `dry_run=True` validates scopes, templates all steps and records a
> `workflow_dry_run` without touching any tool. Proven in
> `platform/tests/test_phase6.py` (13 green, full-stack through the real
> gateway). Still open: a real scheduler to fire schedule/event triggers,
> per-step approval gates, and `budget_ceiling_usd` enforcement (field
> exists, no cost-extraction contract yet).
- **Library + reuse:** workflows are indexed artifacts (compound asset), reusable as
  templates by other users and agents, each traceable to its creator identity.

## Agents (user-created and agent-created)

- **User-created agent = persona + workflow + tool scope + budget envelope** — declared in
  one manifest, registered in the agent registry, subject to the identity rules in
  [[user-identity-and-profiles]].
- **Agent-created agents:** a workflow or agent that constructs another agent (e.g. spawn a
  specialist reviewer for a new experiment class). Gated like the §13 meta-loop: proposed,
  versioned, shadow-tested, and human-approved before it can act independently — an agent
  cannot grant powers it does not itself hold (no privilege escalation through creation).

## Safety rail (reuses §13.4, deliberately)

Every workflow/agent change is **versioned and reversible**; new workflows and agents run
in **shadow/limited mode** before wider scope; anything touching budgets, tool scope, or
the agent registry requires **human sign-off** beyond (or in addition to) the dollar gate;
and no workflow may grant itself more than its creator's delegated authority
([[user-identity-and-profiles]] NIP-OA pattern). Agent-created artifacts show their
provenance lineage, so trust is inspectable, not assumed.

## Where it lives in the platform

- Authoring surface in the control plane ([[agent-ux-control-plane]]) + workflow/agent
  registry as MCP resources so any agent can discover and invoke them.
- Phase 6 scope ([[roadmap]]), with identity groundwork in Phase 2.
