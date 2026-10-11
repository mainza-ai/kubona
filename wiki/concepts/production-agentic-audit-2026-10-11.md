---
type: concept
name: Production Agentic Audit 2026-10-11
date: 2026-10-11
updated: 2026-10-11
tags: [audit, agents, architecture, production, tools, state]
sources: [source-gap-analysis-plan]
status: tracked
---

# Production Agentic Audit (2026-10-11) — why the app feels disconnected

**Method:** three parallel deep-dives (backend agent architecture, frontend
state/data-flow, results display paths) with `file:line` evidence, plus live
reproduction of every user-reported symptom. Headline: **there is no agentic
loop.** The LLM is text-only; science state comes from keyword templates; the
two meet only as co-stored rows. Everything the user feels as
"disconnected" traces to this single architectural fact plus a set of smaller
contract/state bugs (fixed where small, specced where structural).

## A. Findings

### A1. No tool use anywhere in the production path (critical)
`llm.ChatCompletionRequest` is `{model, messages, temperature, max_tokens}`
— no `tools`/`tool_choice`, no `tool_calls` type exists (`pkg/llm`).
`RoleManifest.AllowedTools` (`collective.go:52-120`) and the MCP gateway
catalog are dead allow-lists: zero production callers (tests only). The
LLM↔backend contract is prose + a ` ```json_proposal ` markdown convention
parsed by string search. Tribunal = 4 fixed unary calls, unconditional
`ConsensusPassed=true`, memory hardcoded `store=nil` over HTTP.

### A2. Campaign state is keyword templates, never agent output (critical)
`campaign_synth.go`: 13-branch `strings.Contains` table (anything unmatched
→ H2), one real bridge call (anchor floats only), then closed-form theater:
`vqe = fci + 0.00047`, `BP risk = "LOW"` always, fidelity `0.984` always,
spend `$0.00` always, 7 hardcoded trajectory milestones, `α=0.95/β=0.05`
two-state epistemics, generic QASM ignoring ansatz/depth. Campaign row is
last-write-wins single-row UPSERT — every message overwrites, nothing
versions or diffs. Consequence for the canvas: probes render enum-only
values; Critic is permanently CLEARED; center energy only moves across
molecule keywords. The "spinning thing" is honest emptiness wearing motion.

### A3. Results die in the database (critical)
No job-detail view, no `/api/jobs/{id}` route. `result_json` (counts,
statevector) is written, returned in list rows, and read by zero pixels.
FlightRecorder shows counts + template narrative; ResultsStudio shows the
template notebook; FleetRadar queue depths are hardcoded constants.
Two live contract breaks found and fixed same-day: wire `status` is a
numeric enum (frontend compared strings — terminal never detected, `· 3`
shown); failures write `error` but UI read `error_message` (reasons
invisible).

### A4. Frontend state resets + stale paths (major)
`runPhase` in the polling effect deps tears down/recreates all intervals on
every send→idle cycle; events polled globally (cross-session mixing);
campaign fetch-once (stale between messages; regenerate never refreshes
campaign); session-switch leaves stale transcript visible for empty
sessions; tribunal completion can invert transcript order; aborted streams
never refetch (result "appears in history" only after reload); `synthesizeCampaign`
export dead; `Loops Live` counts global loops.

### A5. Hardcoded engine claims (major — operator-reported, confirmed)
Three oMLX/8787 strings in the drawer (now generic + settings-linked);
`Local-first` footer; IBM Heron suggestion chips; `ACTIVE :8787` badge;
`LIVE` badge over illustrative widgets; `100% Sovereign Air-Gap` filter
wearing enforcement language (relabeled).

### A6. Widget geometry (minor, confirmed)
380 px fixed cap, vertical-only shell scroll, `min-w-[500px]` schematic
forces nested x-scroll, 4-metric grid wraps, no title truncation, open stage
obscures right probes with dismiss-only (no drag/minimize).

### A7. Stream double-prompt (minor, found in audit)
`handleChatStream` stores the user turn then appends `req.Content` again —
the model sees the prompt twice. Cosmetic today, token waste always.

## B. Gaps (production checklist)

1. Tool registry + function-calling protocol in `pkg/llm` (request/response/
   streaming-delta `tool_calls`, dispatcher with authz + observation loop).
2. Context injection: campaign row/diff, recent jobs+results, ledger tail,
   fleet/fidelity, spend remaining, Hindsight recall (un-nil it) — with token
   budgeting (today unbounded history).
3. Memory: per-session working memory, cross-session episodic recall
   actually called, outcome write-back (job results → memory).
4. Planner loop: plan → tool → observe → replan with abort/rescope; kill the
   constant-consensus tribunal or demote it to rehearsal.
5. State discipline: versioned campaigns, event-sourced derivation
   (state = fold over events+jobs), conditional writes keyed on justifying
   job/anchor.
6. Validation gates: QASM→QIR compile before dispatch, proposal schema
   validation, pre-checks before storing proposals.
7. Message model: agent turns as first-class roles visible to history
   assembly (today tribunal roles are invisible to the copilot filter);
   events carrying content hashes.
8. Surfaces: job-detail view + route; results bound to job outputs;
   session-scoped events; regenerate campaign refresh; abort refetch.

## C. Implementation plan (ordered, no simplifications banked)

- **Phase 1 — correctness (done same-day):** numeric status labels,
  `error`-key reasons, generic engine strings, tier normalization, refusal
  surfacing, QASM contract, markdown renderer, tooltips, audit fixes #1–5,
  chat batches 1–3. All green.
- **Phase 2 — state coherence (next):** effect-dep fix (stable intervals),
  session-scoped events, regenerate campaign refresh, abort refetch,
  transcript-order fix, empty-session clear, stream single-prompt,
  job-detail route + view, results bound to `result_json`, widget geometry
  (wider stage, truncate, minimize, LIVE-badge honesty).
- **Phase 3 — agentic core (in build 2026-10-11):** `tools` protocol in
  `pkg/llm` ✅ (tools/tool_choice, tool_calls, streaming deltas, DSML
  fallback) + single registry/dispatcher ✅ (6 read-only tools, deny by
  default, schema validation, `tool_called` audit events); context injection
  ✅ with budgeting (`BudgetHistory`: system survives, newest wins, cuts
  announced); tribunal turns observe ✅ (one bounded round per turn,
  nil-safe); local-sim submit tool ✅ (admission-enforced, 4096-shot cap,
  one dispatch per turn, turn roles never write — live-proven: model
  dispatched one Bell run, read it back, reported genuine counts).
  Remaining: Hindsight wired over HTTP; planner loop with replan;
  versioned campaigns; compile/validate gates on launch.
- **Phase 4 — hardening:** proposal schema validation, dissent-capable
  tribunal or removal, memory write-back, load/chaos tests on the loop,
  multi-geometry scan execution (PES scans today launch one circuit).

Related: [[chat-interface-audit-2026-10-11]] ·
[[ui-honesty-audit-2026-10-11]] · [[ui-ux-vision]] ·
[[local-simulation-test-scenarios]] · [[evaluation-and-benchmarks]] ·
[[agent-ux-control-plane]] · [[science-discovery]]
