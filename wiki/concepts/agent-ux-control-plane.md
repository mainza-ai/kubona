---
type: concept
name: Agent UX & the Control Plane
date: 2026-08-16
updated: 2026-11-14
tags: [ux, control-plane, hitl]
sources: [source-gap-analysis-plan, source-openai-math-corpus]
status: adopted
---

# Agent UX & the Kubona Control Plane

Kubona's UI sits at the intersection of two disciplines that rarely meet
(source-gap-analysis-plan):

1. **Quantum circuit visualization** — a design space where even 2026 studies call
   platform-agnostic rendering an **open problem**, because visualization is tightly coupled
   to the originating SDK. Kubona inherits this by being multi-SDK from day one. Prior-art
   reference points: [[ibm-quantum-composer]] (bidirectional code↔diagram editor) and
   [[qflow-studio]] (credential-safe evidence sharing).
2. **Agent UX** — an accountability layer: surface *what the system is doing, why it chose an
   action, allow override at any point, recover gracefully from errors.*

No existing tool (quantum or agentic) covers both.

> **Full specification:** the complete UI/UX vision — personas, information architecture,
> per-surface detail, visual language, mobile, open design problems, acceptance criteria —
> lives in [[ui-ux-vision]].
>
> **Implementation stack:** The control plane is implemented in **TypeScript + React/Next.js**
> talking via WebSockets and gRPC-Web to a high-throughput **Go backend orchestrator**, while
> quantum execution runs via isolated **Python gRPC microservices**.
>
> ⚑ **Implementation status (2026-11-10, `de3428c` / `119749e`):** The actual
> Next.js 16 console (`platform/console`, `:3000`) communicates with `kubonad`
> (`:8080`) over HTTP/JSON and SSE (`/api/events/stream` with `Last-Event-ID`,
> `142` modeled / `14` client-only / `0` unmodeled contracts) rather than
> gRPC-Web. Shipped surfaces include Workbench / Orbital Space / Proof DAG /
> Hardware QPU views, `ResultsStudio` (dependency-free SVG `<Chart>` series
> plots, pre-claim measured experiment table, result receipts, and signed
> Ed25519 export), `HardwareLaunchPanel`, `DiscoveryProtocolPanel`, and the
> global **`Cmd+E` / `Ctrl+E` Experiment & Discovery Lab** modal, with native
> `<dialog>` top-layer dismiss, `startViewTransition`, and IME composition guards.
> Rather than purely passive read/approve, operators can submit experiments and
> hardware batches strictly through the same governed Go admission, WebCrypto
> Ed25519 mandate, and budget-hold rail used by agents.
>
> **Implementation update (2026-11-11):** Console contracts now stand at
> **144** modeled / **14** client-only / **0** unmodeled (`131` OpenAPI paths,
> `123` schemas). The `Cmd+E` / `Ctrl+E` **Experiment & Discovery Lab** now opens
> as a non-blocking right drawer in the Arena shell (duplicate Settings entry
> removed), `Hardware QPU` renders directly inside the Arena, `SpatialCanvas` is
> promoted directly from the `Molecule` card and HUD (`Open molecule object view`),
> semantic `Dark graphite` / `Light paper` plus contrast/density/reduced-motion
> axes drive `console-token-lint.mjs` to **0** occurrences,
> Workbench viewports persist via session-scoped `GET/PUT /api/workbench/view`,
> and concurrent SSE invalidations queue via `pendingRefresh` (`resource-store.ts`).

> **Implementation update (2026-11-14):** The reviewer's decision queue now
> lives in the shell, not three levels deep. `ModeRail` carries a
> **Needs you** trigger with an unseen-count badge; `NeedsYouTray`
> (`components/shell/NeedsYouTray.tsx`) lists every open obligation from
> `GET /api/obligations/open` with its verbatim blocking reason, whether the
> server's close rules would pass now (`decidable now`) or refuse (`blocked`),
> and its real waiting age. Opening the tray marks items seen
> (`domain/needs-you-seen.ts`, session-scoped, bounded); a row leaves only
> when it is decided, never because it was dismissed. `Review` routes to the
> surface that owns the subject (Workbench object, Tribunal, Gallery, or the
> Guardrails queue) and decides nothing itself. Obligation rows now carry
> `created_at`/`updated_at` and `unknowns_detail` so the tray never invents a
> reason or a clock — unreadable timestamps render `waiting unknown`, and an
unreadable queue renders as unavailable, never as "nothing waiting". The
> projection is `lib/needs-you.ts` (pure, ordered decidable-first then
> longest-waiting — a stated v1 heuristic, since cost-of-waiting inputs are
> not on obligation rows). Cross-session limitation: the obligations poll
> stays alive at 15 s even while the session-scoped SSE stream is live,
> because that stream cannot carry another session's obligations; an
> `obligation_*` ledger-event domain exists in `domain/invalidation.ts` but
> the ledger does not yet emit those types.
>
> **Typed needs (C2, 2026-11-14):** failed jobs now carry a `need` from a
> closed vocabulary in `platform/contracts/need-kinds.json` —
> `credentials`, `budget_exceeded`, `backend_offline`, `quota_exhausted`,
> `approval`, `anchor_missing`, `retry` — with exactly one remedy each. The
> runner classifies from structured evidence it owns (gRPC status codes and
> its own sentinels), persists the need beside the error, and omits it when
> the cause is not known: a vendor-prose failure emits **no** need, so the
> console never offers a remedy that may not help. Unauthenticated /
> permission-denied map to credentials, resource-exhausted to quota,
> unavailable to backend-offline, and deadline/aborted/internal to retry;
> deterministic refusals (invalid argument, failed precondition, not found,
> unimplemented) emit nothing. The Workbench card renders one sentence (the
> server's reason, verbatim) and the contract's one button, which routes to
> provider settings, hardware settings, the Guardrails queue, or the Copilot
> drawer — and never retries or spends by itself. `isIssueWorkbenchObject`
> and the job card now treat a declared need as the issue signal; the
> former regexes over titles, run statuses, and fact text are gone (a result
> that merely contains the word "failed" no longer manufactures an issue).
> Emitted today: credentials, backend_offline, quota_exhausted, retry.
> `budget_exceeded`, `approval`, and `anchor_missing` are reserved until the
> owning rail records the fact; loops still have no failure field, so loop
> needs wait on that schema.
>
> **Server-owned proposals (B1, 2026-11-14):** launch no longer trusts a
> client-echoed plan. `proposals` rows are minted by the server from stored
> campaign state or a stored assistant message
> (`POST /api/proposals {session_id, source}`), carry a canonical body and
> its SHA-256 digest, and record the campaign revision and the device
> calibration identity they were priced against. `POST /api/chat/launch`
> now takes only `{proposal_id, digest, request_id}` plus the mandate/grant
> fields: the circuit, backend, budget, shots, seed, and `designed_by` come
> from the stored body. The backend is normalized into a closed set
> (exact simulator ids → calibrated hardware profile → simulator-smelling
> free text; provider names are never demoted), and metered plans without a
> numeric `max_cost_usd` are refused at mint — text spend estimates are not
> a bound. A moved campaign revision or calibration digest refuses with
> `proposal_stale`; a wrong digest with `proposal_digest_mismatch`; a spent
> proposal with `proposal_not_offered`. The proposal is burned (CAS
> offered→launched) before dispatch, so a reload cannot launch it twice,
> while an idempotent replay of the same `request_id` still returns the
> admitted job. Chat replies mint at extraction and expose
> `proposal_id`/`proposal_digest`/`proposal_error`; a plan the server cannot
> price honestly stays display-only with no launch button. Offline
> simulation uses a monotone `downgrade: "simulator"` mint (zero-cost,
> circuit from stored state); no other downgrade value is accepted.

> **Calm shell + failing cards (3.9 / C3, 2026-11-14):** API refusals and
> response-contract drift are now **status chips** inside `StatusStrip`
> (orchestrator readiness, loops/spend, and stream state already share that
> one line), not page-wide banners. Chips are `role="status"`, dismissible
> by problem signature (`path|status|message`) persisted in sessionStorage, so
> the same refusal does not nag this session while a different one reappears;
> contract drift keeps its schema name and full detail in the title. The only
> remaining full-width `role="alert"` banner is blocking auth expiry (strict
> contract mode still fails the response closed, surfaced by the affected
> surface).
>
> **Run parts (C1, 2026-11-14):** a loop's run shape is now recorded rather
> than guessed. `loop_part` ledger events carry a closed helper vocabulary
> (`lead`, `tool`, `quantum_job`, `literature`, `prover`) and state set
> (`planned`, `running`, `waiting`, `done`, `failed`, `stopped`). Emitters
> today: supervisor optimizer steps (with the gradient metric), lifecycle
> transitions (paused/resumed/rescoped/completed/diverged/killed), manual loop
> registration, and every dispatched job (linked by `job_id`, with the
> proposal target as goal). `GET /api/loops/{id}/parts` projects them oldest
> first, upgrades a job part's state from the live job row (queued→waiting,
> failed→failed, terminal→ended), and carries the job's typed `need`. The
> session SSE stream already carries each `loop_part`, so the console
> refreshes through the existing `loop_*` invalidation. The agent-run card
> renders `columnsOf(parts)` as time-ordered helper columns with the active
> part marked; live runs poll every 5 s, settled runs load on request (no
> request storm from a canvas of history). Honest gap: copilot tool calls are
> not loop parts (they belong to chat turns), and no rail records
> literature/prover/tool parts yet, so those columns stay empty until the
> owning rails emit them.
>
> **Canvas polish (C4–C7, 2026-11-14):** pending state is projected only
> where the record genuinely means "data is expected" (queued/running job,
> outcome-unknown reservation, a live loop) and renders as a labelled
> `role="status"` line whose pulse runs only while the card is on screen
> (IntersectionObserver; reduced motion disables it). Follow-live is on by
> default: `followCamera` pans the minimum needed to put the live target
> inside a 14% lead band, never zooms, glides for 220 ms (reduced motion
> snaps), pauses on any manual camera move (wheel/pan/zoom/fit/minimap), and
> re-arms on a new live target. New cards animate once on arrival and are
> announced through a polite live region; the first projection is not an
> arrival. Cards beyond a grow-only 3×-viewport near set render as sized empty
> boxes, so panning back never pays a remount and scene extents stay stable. On the canvas, every card and every embedded chart now sits inside
> its own `RenderBoundary`: a render failure costs that item only, shows
> "record is intact" with a Retry, and leaves neighbours and the layout
> untouched. A pure scene sanitizer (`sanitizeWorkbenchObjects` /
> `sanitizeWorkbenchLinks`) drops blank/duplicate ids and dangling, self,
> duplicate, or uncatalogued links, and clips oversized labels/titles/details/
> facts before render — a projection-only guard, never a ledger rewrite.
>
> **Per-fact source chips (C9, 2026-11-14):** facts that come from a named
> record carry a `from` ref; the card renders a small chip that focuses the
> producing job/loop/obligation/task (measurement and proof facts resolve to
> the measured job, else the campaign). Folded job runs resolve to their
> owning card. No source means no chip — the UI only offers a link it can
> honour.
>
> **Plan → tasks (C8, 2026-11-14):** a successful dry-run plan is published
> to the canvas as a `plan` card (workflow id, per-step rows with tool, cost,
> and approval flag, planned cost, approval count, unresolved references).
> The draft is explicitly ephemeral — a plan has not executed, so it is not
> server state. **Make tasks** creates one task per step in plan order through
> `POST /api/tasks/batch` under a single idempotency key (bounded at 50,
> `order` respected); on confirmation the card flips to **Open in Tasks**,
> which focuses the first created task card. A replay of the same request
> returns the same rows, so a double-click cannot duplicate a plan. Unresolved
> references are shown as facts, not invented typed needs (the C2 vocabulary
> has no honest kind for them).
>
> **Steer a live run (C10, 2026-11-14):** the run lane carries a steer box
> with an explicit context disclosure (target loop, part count, active part)
> and the rule stated inline: *applied at the next step boundary — not an
> immediate tool call.* `POST /api/loops/{id}/steer` queues an instruction
> (1–500 runes) for a running/rescoped loop; a paused or terminal loop refuses
> with typed `loop_not_running` (409) because nothing would consume it. The
> next `Step` claims each pending steer with a conditional update (so two
> concurrent steps cannot apply one twice), seals a `loop_steered` ledger
> event, and records a `lead` lane part carrying the instruction as its goal.
> Steers survive as rows until applied; the receipt names the queued id.
>
> **Primitive correctness (D2, 2026-11-14):** `Button` now defaults to
> `type="button"` (a button inside a form can never submit it by accident),
> carries an explicit `pending` state (disabled + `aria-busy` + spinner,
> reduced-motion safe) and a `danger` shorthand. `SegmentedControl` implements
> the radiogroup keyboard contract: roving tabindex, arrows move selection
> (skipping disabled segments), Home/End jump. `Tooltip` puts
> `aria-describedby` on the focusable trigger, not on its wrapper. Tests pin
> each behaviour; the raw-`<button>` ratchet outside `components/ui` remains
> open.

## The nine core observatory surfaces
 
| Surface | Purpose |
|---|---|
| **Spatial research canvas** | Infinite zoomable canvas (Galaxy to Surface); agent co-presence cursors (Buzz pattern); SDK-independent post-QIR universal glyphs & ghost circuits |
| **4D flight recorder** | Replay/scrub multi-hour trajectories; cavity expansion & tree shift replay ([[source-openai-math-corpus]]); speculative counterfactual branching |
| **Epistemic manifold & analytical anchors** | Tripartite credibility wavefunction rings (`{corroborated, contested, unverified}`); deductive certitude ($\rho = 1.0$); inline DMRG / exact analytical anchors |
| **Autonomy cockpit & spend cards** | Calibrated friction architecture; WebCrypto Ed25519 spend mandates (`{tenant_id, job_hash, max_budget_usd, expiry}`); one-glance action gate cards |
| **Execution fleet radar** | Real-time multi-modality fleet status (local simulator, ZNE, IBM, Quantinuum, QuEra, D-Wave); jurisdiction & export-control shield (`policy.py`) |
| **Cognitive LLM mesh** | Zero-config local host port radar (Ollama, OMLX, LM Studio, MLX); sovereign air-gap switch; tiered persona model assignment; arg normalization inspector |
| **Results studio & notebook** | Systematic citable reports with frame-anchored media comments (Buzz pattern); 100% provenance verification links; first-class negative results repository |
| **Epistemic tribunal room** | Dedicated adversarial deliberation chamber; real-time quorum consensus gauges, dissenting arguments, and formal Lean 4 AST citations |
| **Generative scientific UI (A2UI)** | Agent-streamed interactive components: 3D molecular active space (Mol*/Three.js), QIR circuit editor, Lean 4 goal tree, shadow tomography radar |

## Interaction principles

- **Progressive disclosure** — deep circuits default to summarized stats, drill-down on demand.
- **Confidence/uncertainty viz** — distinguish raw vs. error-mitigated values; show technique
  and its variance cost inline.
- **Status & activity** — the loop inspector always answers "what is the agent doing now,
  and why" in one glance.
- **Error handling & recovery** — failed jobs suggest an actionable next step (switch
  backend, request budget, fall back to simulator).

## Two hard rules

- **Read/approve only** — the console is oversight on top of agent actions, never a second
  path to submit jobs (avoids reopening the credential/spend risk).
- **Fixed, audited human surfaces** for approval/circuit/provenance — generative /
  agent-materialized UI (MCP Apps, A2UI) is a v2+ direction for ad-hoc visualizations only,
  given the security sensitivity.

Related: [[qflow-studio]] · [[cost-governance]] ·
[[provenance-and-reproducibility]] · [[qir-mlir-compilation]]