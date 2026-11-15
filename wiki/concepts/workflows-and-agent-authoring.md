---
type: concept
name: Workflows & Agent Authoring
date: 2026-08-16
updated: 2026-11-14
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

## Runtime skills (adopted 2026-11-14)

A skill is a **`SKILL.md` instruction file**, not a workflow and not a capability grant.
The copilot prompt carries only each skill's name and ≤200-character description; the body
is fetched on demand through the read-only `load_skill` tool. An advisory `tools:` list
names the observations the instructions expect, but the registry allow-list still decides
what may run — a skill cannot widen its own authority.

The loader is closed and fail-visible: 16 KiB per file, 64 skills total, lowercase
`[a-z0-9-]` names, one-line descriptions, no symlinks, and a fixed `SkillFault` vocabulary
(`skill_name_invalid`, `skill_description_invalid`, `skill_frontmatter_invalid`,
`skill_tool_invalid`, `skill_role_invalid`, `skill_too_large`, `skill_too_many`,
`skill_duplicate`, `skill_symlink`, `skill_unreadable`, `skill_missing`, `skill_unknown`).
A rejected file never disappears silently: `GET /api/skills` returns the body-free
catalogue and the faults, and Settings → Runtime skills renders both. User skills live in
`~/.kubona/skills/<name>/SKILL.md` (overridable with `KUBONA_SKILLS_DIR`); the six seeds
(`pes-scan`, `vqe-campaign`, `classical-anchor-check`, `hardware-readiness`,
`literature-review`, `mitigation-choice`) are embedded in the binary.

The split keeps the cacheable system prefix small and stable while still letting an
operator add local method without a rebuild. It is the same shape Zephium uses for its
work-lead skills: only the advertised surface is cached, and the body is a read.
