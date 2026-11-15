---
type: concept
name: Feedback Loops & Notifications
date: 2026-11-15
updated: 2026-11-15
tags: [ux, feedback, notifications, attention, governance, audit]
sources: [source-gap-analysis-plan, source-frontier-repo-audit-2026]
status: tracked
---

# Feedback Loops & Notifications

Maintainer audit of every human→system feedback path and every
system→human notification surface in the console and Go core. The test is
not “is there a button?” but **does the decision change future behaviour,
and does the human learn what happened?**

## Verdict

The console has many feedback *controls* and many notification *surfaces*, but
few closed loops:

- **Chat votes are a dead end.** `chat_feedback` is written to the ledger and
  read by nothing else. The UI tooltip says “Feeds the credibility flywheel”,
  but no flywheel consumes it, no GET hydrates past votes, and reloading the
  page forgets them.
- **Claim reviews are advisory records only.** A review is stored and listed,
  but does not change the claim verdict, credibility, obligations, or any
  future prompt. The reviewer identity is the literal `operator` /
  `operator-bearer`, not the caller.
- **Proposal decisions are only partly fed back.** Launch is strongly bound
  (digest/CAS/idempotency); dismissal changes a row but emits no decision
  event, carries no reason, and influences no future proposal.
- **Notifications are fragmented.** There is no unified notification record,
  inbox, read state, or preference model. Toasts, status chips, banners,
  inline alerts, local component toasts, the Needs-you tray, and the Attention
  Director are independent mechanisms with different lifetimes and no shared
  dedupe.

> ⚑ Correction (2026-11-15): the Verdict and inventories below record the
> **pre-implementation audit state**. The P0 contracts and notification
> foundation have since landed; see “Implementation status”. Remaining open
> items are the reason/proposal UIs, the claim-review policy, preferences,
> Attention Director wiring, and the P2 learning/metrics consumer.

## Feedback-loop inventory

| Loop | Trigger | Persisted? | Consumer / effect | State |
|---|---|---|---|---|
| Chat vote 👍/👎 | `POST /api/chat/feedback` | `chat_feedback` ledger event | **None** | Dead end |
| Chat correction | rewrite/regenerate | New messages / stored turn | Re-answers thread | Local loop only; no feedback record/reason |
| Claim review request | “Request review” | `peer_review` ledger event + chat message | Opens a chat turn; no obligation/notification | Half-open |
| Claim review decision | review form | `claim_reviews` table + `peer_review` | Listed in Claim History; **claim verdict unchanged** | Advisory only |
| Proposal launch | launch dialog | Proposal status + job + loop | Real dispatch, approval gate | Closed |
| Proposal dismiss | dismiss | Proposal status | Nothing else; no event/reason | Dead end |
| Metasearch approve/reject/apply | panel | `metasearch_applied` only; status rows | Changes the loop strategy | Mostly closed |
| Workflow approval | approval token | Approval row | Gates workflow execution | Closed; no notification |
| Obligation close | tray/panel | Obligation + ledger events | Closes the queue item | Closed |
| Task status | task board | `task_status_changed` | Tasks projection | Closed |
| Run steering | steer box | `loop_steers` → `loop_steered` + part | Applied at next step | Closed |
| Memory retraction | memory panel | Tombstone + `memory_retracted` | Recall excludes it | Closed; no notification |
| Run allowance revoke/suspend | allowance panel | Allowance status + event | Admission excludes it | Closed |
| Model choice / eval acknowledgment | picker/settings | Settings revision + optional event | Changes routing; opt-in eval gate | Closed |
| Appearance/presence/mute | appearance/status bar | Local settings / session | Changes presentation | Local only |

## Notification-surface inventory

| Surface | Scope | Durability | Dedupe / acknowledgment | Notes |
|---|---|---|---|---|
| Needs-you tray + badge | Open obligations | Server rows; “seen” in `sessionStorage` | Per-item seen signatures, capped 500 | Actionable queue, not a history |
| Page toast | Live-run events, outages, some task/create/delete results | 4s, one slot | Replaced by newer toast; no queue | Not for every terminal event |
| `StatusStrip` | Backend, loops, stream, API chip, data refused | Session | API chip dismissed by signature | Several independent signals |
| `ApiProblemChip` | Non-blocking API/contract refusal | `sessionStorage` dismissal, cap 100 | Signature-based | `role="status"`, not alert |
| `BannerStack` | Expired session only | Session/event | Dismiss once | Reserved for blocking states |
| Inline `role=alert/status` | Form/surface-local errors | Component state | Usually none | 51 alerts, 70 statuses, 6 `aria-live` in source |
| Local component toasts | HuggingFace model hub | 5s, one local slot | New replaces old | Duplicates the global toast pattern |
| Attention Director | Anomalies/summons routed to CheckInStrip | In-memory alert + precision tracker | Duplicate collapse by source+level | Only `SpatialCanvas3D` calls `notify` |
| Write settlement labels | Per surface | Component state | Replaced by next write | Not user-inbox history |

## Gaps and improvements

### P0 — honesty and contract

1. **Remove or honour the chat-vote claim.** Either label votes as recorded
   only, or add a consumer (credibility/eval/ranking) and a GET endpoint so
   historical votes hydrate and can be changed.
2. **Make feedback idempotent and attributable.** Add `request_id`, a typed
   reason code, actor identity from auth, and uniqueness/toggle semantics.
3. **Close claim reviews.** A review should either change claim state under a
   written rule (e.g., `verified → disputed`) or be explicitly labelled
   advisory in the UI. Notify the requester when a review is decided.
4. **Record proposal decisions.** Append `proposal_dismissed` /
   `proposal_launched` events with reason/actor and use dismissal reasons in
   proposal ranking.
5. **Stop false or duplicate notifications.** One source of truth for each
   event; remove per-surface toasts that shadow the global one.

### P1 — unified notification model

6. **Server-owned notification projection.** Add a `notifications` row/table:
   `id, kind, severity, source, subject_kind, subject_id, dedupe_key,
   created_at, read_at, resolved_at, action`. Consumers: obligations,
   proposal decisions, claim review requested/decided, job/loop terminal,
   workflow approval, budget cap, auth expiry, model/provider failure,
   allowance suspension, memory retraction.
7. **Endpoints + streaming.** `GET /api/notifications`, mark-read and
   resolve/act; SSE invalidation domain `notifications`; polling fallback.
8. **Notification Center.** A shell inbox with unread count, filters, history,
   read/resolve, and deep links. The Needs-you tray remains the actionable
   subset; toasts become ephemeral projections of the same store.
9. **Preferences.** Per-kind/severity delivery (inbox/toast/attention),
   quiet-hours/presence, server-synced rather than per-device localStorage.
10. **Accessibility.** One polite live region per notification batch; alerts
    only for blocking states; no duplicate announcements from inline + toast
    + status strip.

### P2 — closed-loop learning

11. **Feedback consumers.** Votes and claim reviews feed the recorded
    decision-eval set and credibility/ranking policy; steering outcomes are
    measured; proposal dismissals become ranking features.
12. **Analytics.** Record feedback accepted/rejected, notification
    delivered/read/acted, time-to-decision, and duplicate-suppression counts.
13. **End-to-end acceptance.** Vote → reload preserves state; review request
    appears in the reviewer inbox; decision notifies requester; proposal
    dismissal is auditable; terminal job/loop produces one notification, not
    a toast race.

## Implementation status (2026-11-15)

Landed:

- **Durable chat votes.** `chat_feedback` table + `GET /api/chat/feedback`
  hydration, upsert/toggle/clear, optional reason, authenticated actor, audit
  event with previous/new vote. Console hydrates votes on session load and
  clicking the active vote clears it. The UI now says “recorded for review;
  not yet used for ranking” instead of claiming a flywheel.
- **Review identity and honesty.** Claim reviews use the caller-derived actor
  (`operator`, `lab_admin:<tenant>`, `local_researcher`) and the UI states that
  reviews are advisory and do not change the recorded verdict.
- **Proposal decisions are auditable.** `proposal_launched` and
  `proposal_dismissed` events (with actor/reason) are in the event contract;
  dismissal accepts a reason.
- **Notification inbox foundation.** Derived server projection
  `GET /api/notifications` from terminal events + session-owned obligations,
  coalescing by subject (a completed run supersedes its stalled warning),
  server-owned read/resolve state (`POST /api/notifications`), SSE
  `notifications` invalidation domain, and a Notification Center panel with a
  shell bell/unread badge and deep links.
- **Tests:** vote ledger upsert/hydration/clear, notification derivation and
  read/resolve, notification-center browser tests, invalidation mapping.

Still open: feedback reason UI, claim-review verdict rule, proposal dismiss UI
reason/ranking, server-synced notification preferences/quiet routing,
Attention Director wiring for all producers, and P2 metrics/learning policy.

## Implementation plan

### Phase 0 (1–2 days): stop the lies
- Rewrite the vote tooltip/acknowledgement to “recorded” until a consumer
  exists.
- Add idempotency/actor/reason fields to `chat/feedback`; add
  `GET /api/sessions/{id}/feedback` and hydrate votes.
- Add proposal decision events with reason; show them in proposal history.
- Mark claim reviews advisory in the UI unless a verdict-change rule is
  adopted; add requester/reviewer notification hooks.

### Phase 1 (3–5 days): one notification store
- Schema + Go projection + `GET/POST /api/notifications` + SSE invalidation.
- Producers for obligations, proposal/claim/workflow/job/loop/budget/auth.
- Notification Center shell surface; migrate global toast and API chip;
  keep inline errors for form-local failures.
- Server-synced read/resolve and preferences; dedupe keys per subject.

### Phase 2 (3–5 days): attention and learning
- Route Attention Director from the notification store; persist precision
  and presence preferences.
- Wire votes/reviews/dismissals into eval/ranking with explicit policy.
- Add end-to-end tests and metrics.

## Acceptance criteria

- A vote survives reload, is idempotent, and either affects a named consumer
  or is labelled as recorded-only.
- A claim review request appears in an inbox; its decision notifies the
  requester and has a documented effect on claim state.
- Every terminal job/loop and proposal decision produces exactly one
  notification record, readable later.
- Read/resolve state is server-owned and shared across devices.
- Blocking states use alert semantics; non-blocking states use one polite
  live-region batch, with no duplicate announcements.
- Notification delivery respects presence/mute/quiet preferences.

Related: [[ui-ux-vision]] · [[agent-ux-control-plane]] ·
[[runtime-honesty-and-commit-policy]] · [[remaining-work]] · [[claim-credibility-wavefunctions]] ·
[[provenance-and-reproducibility]]
