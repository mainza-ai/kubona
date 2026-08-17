---
type: entity
name: Buzz (Block Inc.; Mainza fork)
date: 2026-08-16
tags: [workspace, nostr, prior-art, collaboration]
sources: [source-gap-analysis-plan]
status: prior-art-reference
---

# Buzz — "a hive mind communication platform"

Open-source (Apache-2.0, Rust) workspace where humans and AI agents collaborate in shared
rooms, built on a **Nostr relay**: every message, workflow step, approval, and git event
is a signed event in one log — the same identity model and audit trail whether the author
is a person or a process. The repo at `mainza-ai/buzz` (fork of Block, Inc.'s
`block/buzz`) is maintained by the same operator as the Mainza/Milimo codebase referenced
in [[mainza-milimo-foundation]] — flagged as a **design reference, not a dependency**.

## Ideas that transfer directly to Kubona

| Buzz capability | Kubona application |
|---|---|
| **Agents as members, not bots** — own keys, memberships, audit trail; scoped by identity | [[user-identity-and-profiles]]: every actor is a first-class identity |
| **One event log** — messages, workflow steps, approvals, git all signed events | [[provenance-and-reproducibility]] ledger as the single audit substrate |
| **Owner-delegated authority (NIP-OA)** — agent inherits owner access; revoke owner → agents lose access instantly | Delegated authorization under the OAuth/PQC layer ([[security-and-pqc]]) |
| **YAML workflows with triggers** (message/reaction/schedule/webhook) + approval gates | [[workflows-and-agent-authoring]]: user/agent-created science workflows |
| **Feature branch → room** — channel is the record of why code exists | Experiment → project channel: the record of why a result exists |
| **Frame-anchored media comments** | Annotate figures/plots in the experiment notebook |
| **Unified search** across conversation, patch, run, approval | One search index over the ledger + reports |
| **Persona packs** | Agent personas in the registry ([[workflows-and-agent-authoring]]) |
| **Community-local state, self-hosted relay** | Labs/teams as scoped communities; sovereign self-hosting |
| **Web-of-trust reputation (pending)** | Freshness/trust scores for agents & data sources |

## Notes

Buzz is a young project (created 2026-07-26; Apache-2.0; Rust) and shares the
single-source caveat of [[mainza-milimo-foundation]]: verify current state before adopting
patterns. The *patterns* (identity, event log, delegation, workflow triggers) are the
transferable asset — not the code.

Related: [[mainza-milimo-foundation]] · [[user-identity-and-profiles]] ·
[[workflows-and-agent-authoring]] · [[provenance-and-reproducibility]] ·
[[agent-ux-control-plane]] · [[science-discovery]]
