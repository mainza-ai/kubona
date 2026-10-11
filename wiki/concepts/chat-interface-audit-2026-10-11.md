---
type: concept
name: Chat Interface Audit 2026-10-11
date: 2026-10-11
updated: 2026-10-11
tags: [audit, chat, copilot, saas, ux]
sources: [source-gap-analysis-plan]
status: tracked
---

# Chat Interface Audit (2026-10-11) — entry point, below standard

**Method:** full `CopilotDrawer` read; backend message ops read (`Add/Get`
only — no delete/edit/clear); SaaS patterns surveyed (IBM Bob: refs, copy,
expand/collapse, rollback, ⌘F search; Ably: edit + regenerate + branching;
enterprise history: search/sort/bulk/export). Report only — no code changed.

## What exists (working)

Streaming bubbles with caret, tribunal cards with collapsible turns, launch
cards, proposal pills, canvas dispatch (4 widgets), file attach, suggestion
chips, synthesis copy, target/Δ ribbon, served-by line, error states,
empty-state prompts.

## P0 gaps (table stakes — one already user-reported)

1. **No clear chat.** Backend has no message delete; needs `DELETE
   chat_messages WHERE session_id` + ledger event + confirm UI.
2. **No stop generation.** Streaming runs minutes with no abort. Client
   `AbortController` first (server run continues — acceptable v1, marked);
   backend cancellation later.
3. **No retry/regenerate.** Re-sending duplicates the user turn. Needs a
   regenerate path that drops the last assistant message and re-runs.
4. **No edit-and-resend.** Standard branching (Ably pattern) needs
   truncate/replace ops; absent backend-wide.
5. **Copy only on synthesis.** Every message + code block needs copy.
6. **No in-chat search.** ⌘F-scoped find with highlight + navigation.
7. **No per-turn metadata.** Served-by exists; add duration/tokens/cost.
8. **No feedback.** 👍/👎 → ledger event (feeds the GQE flywheel).
9. **No export.** Markdown transcript download, credential-safe.
10. **No slash commands.** `/tribunal /launch /clear /budget /anchor` —
    cheapest high-leverage add.
11. **No mode selector.** Chat vs tribunal-intent vs launch-direct.
12. **Attachments invisible.** Attach works; no context pill showing what rode
    along (Bob pattern).

## Kubona-vision gaps (chat ↔ canvas ↔ ledger)

- Dispatch pills only on tribunal synthesis → contextual pills on every
  assistant message (numbers → chart, errors → critic path).
- Launched jobs never report back into chat (proposal card goes silent after
  dispatch; poll job state inline).
- No branch/fork affordance from chat into the flight recorder.
- Tribunal turns don't ignite canvas probes (run-state maps prompt keywords
  only).
- Claim numbers in chat don't cite ledger records (click-to-evidence).
- No canvas stop control during runs (HUD only shows status).

## Backend ops required (Batch 2)

Clear-history delete, regenerate re-run, truncate-from for edit-resend,
👍/👎 ledger events, per-message token usage persistence (usage already
returned by providers, currently discarded).

## Proposed build order (awaiting approval)

- **Batch 1 (frontend only):** stop button, copy everywhere, in-chat search,
  metadata line, export, slash commands, contextual follow-ups.
- **Batch 2 (backend):** clear endpoint, regenerate, feedback events, usage
  persistence.
- **Batch 3 (integration):** edit-resend branching, turn→probe ignition,
  inline job tracking, claim citations.

Related: [[ui-ux-vision]] · [[ui-honesty-audit-2026-10-11]] ·
[[agent-ux-control-plane]] · [[evaluation-and-benchmarks]]
