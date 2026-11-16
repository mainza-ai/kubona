---
type: concept
name: Remaining work queue
date: 2026-10-17
updated: 2026-11-14
tags: [plan, queue, production]
sources: [source-gap-analysis-plan, source-frontier-repo-audit-2026]
status: tracked
---

# Remaining work queue

> Every item the program still owes, with its owner and unblock
> condition. Landed work is NOT here (see [[full-tree-roadmap]] Landed,
> [[rea-adoption-plan]], [[cloudroom-adoption-plan]]). Durable blueprint
> questions live in [[questions]] — this page tracks execution leftovers
> only. Screened-out items name their reason; nothing rots silently.
>
> ⚑ **State update (2026-11-10, `119749e` / `de3428c`):** Since `2026-11-01`:
> - **Science engine & contracts closed:** all 19 `experiment-result-v1` kinds
>   decode in `pkg/experimentresult`, fold into `CampaignExperiment` state
>   (including failed-job negative records and `result_receipts`), and export
>   via Ed25519 `/api/sessions/{id}/experiment-export`. Added multidimensional
>   RHF/STO-3G BFGS `geometry_optimization`, `vqe_repeats`, `H2`/`LiH`/`BeH2`/`H2O`
>   active-space excited states & atomization reactions, 1D `spin_chain`
>   (`xxz`/`heisenberg`/`tfim` with multi-start `L-BFGS-B`), `quimb`
>   open-boundary DMRG (`thermodynamic_limit` with `TNLinearOperator` fix
>   verified through `N=16`), 2D TFIM quench (`lattice_dynamics`), opt-in
>   OpenAlex lookup in `conjecture`, calibration-aware `hardware_pes`, reviewed
>   rate-card preview, and multi-step `DiscoveryProtocol` (`then`/`->`/`;`).
> - **Live IBM `ibm_fez` acceptance (`docs/live-qpu-2026-11-09.md`):** completed
>   4-step SPSA HILO VQE (`da0b1703`), ZNE VQE (`5f0064eb`), active-space LiH
>   (`1b6bb0ec`), HILO QAOA (`7014cd0c`), and `spin_chain_hardware` (`1ad0e5fb`)
>   with live calibration refresh and signed receipts (Open Plan `$0.00`;
>   honestly `claimable:false` where noise exceeds thresholds).
> - **Contracts & Console UX:** inventory stands at **142 modeled / 14
>   client-only / 0 unmodeled** (`121` schemas, `130` OpenAPI paths;
>   `console-contract-inventory.py --check`, `console-openapi.mjs --check`).
>   Added the `Cmd+E` **Experiment & Discovery Lab** modal, `HardwareLaunchPanel`,
>   `DiscoveryProtocolPanel`, `ResultsStudio` linear series `<Chart>` plots &
>   pre-claim report tables, native `<dialog>` dismissal, `startViewTransition`,
>   and IME guards; `api.Server` shares one live `FidelityGate` with `jobs.Runner`,
>   `obligation.go` locks all DB calls, and `sandbox/docker.go` refuses without
>   `kubona-quantum-runner:1.1.0`.
> - **Repo & stack hygiene closed:** all 8 `dependabot/*` branches pruned on
>   origin (Ruleset `25762910` excludes `dependabot/**/*`, 0 open PRs); `Co-Authored-By`
>   `noreply` trailers excised via `git-filter-repo` (`1047` commits under
>   `mainza-ai`); `kubona-live` Docker Compose production stack running cleanly
>   on a fresh `kubona-state` volume (`127.0.0.1:8080` / `:3000`, `bridge` UDS).
> - **2026-11-11 Workbench, Token & Transport Consolidation:** `docs/repo-counts.md`
>   now records **141** HTTP routes, **868** Go test functions, **127** Console
>   test files, and **667** Console test cases; console contract inventory holds
>   **144 modeled / 14 client-only / 0 unmodeled** (`123` schemas, `131` OpenAPI
>   paths), adding session-scoped `GET/PUT /api/workbench/view` viewport
>   persistence (`pkg/api/server.go`, `route_policy.go`) and campaign-scoped
>   `replan.go` filtering. Semantic warm-graphite (`Dark graphite`) and `Light paper`
>   themes plus contrast/density/reduced-motion axes drive `console-token-lint.mjs`
>   down from the `865` ratchet to a **14**-occurrence baseline
>   (`platform/contracts/console-token-baseline.json`). The `Cmd+E` **Experiment &
>   Discovery Lab** is now a non-blocking right drawer (duplicate Settings entry
>   removed), `SpatialCanvas` promotes directly from the `Molecule` card and HUD,
>   Workbench folds repeatable jobs across pins with a Web Worker DAG (`proof-dag.worker.ts`)
>   and hover/focus-gated handles, `page.tsx` separates mount-only bootstrap
>   from polling, `resource-store.ts` queues concurrent SSE `pendingRefresh`
>   invalidations, and `.github/workflows/console-e2e.yml` uploads Playwright traces
>   on failure.
> - **Still open (human/operator & v2 scope):** manual VoiceOver/NVDA pass;
>   third-party security review (P3); external provider sidecar (P4); QPU
>   chemical/spin accuracy & live `hardware_pes` run; general SMILES/open-shell/
>   periodic chemistry; single unified `ExperimentResult` model replacing
>   remaining UI projections; retiring the final `14` raw-palette baseline
>   occurrences (`console-token-lint.mjs`, down from `865`);
>   Lean toolchain container; `gho_…` provider revocation confirmation; OpenAlex
>   `KUBONA_CONTACT_EMAIL` / HF token rotation; vendor invoice runtime rate
>   card; and optional [[optional-enhancement-plan-2026-11-01]] (E1–E4).

## Phase-0/1/2 execution record (2026-10-28, `develop`)

- Phase-0 + Phase-1 hardening landed on `develop` with the full gate
  green (20 checks, run 40786519756): settlement, admission crypto, A2A
  binding, approval gate, bridge confinement, manuscript evidence, route
  policy; then terminal settlement everywhere, atomic admission, sandbox
  confinement + escape tests, principal-bound tenancy, RAM admission,
  Rust measurement fidelity, CI depth (vet/gofmt, ruff ratchet, clippy,
  semgrep, Trivy, coverage floors).
- Phase-2 credibility hygiene landed: stale README numbers corrected to
  the recounted values at that commit (116 routes, 154 wiki pages, 57
  console files / 276 vitest tests); those values are now stale again.
  The generated source of truth is `docs/repo-counts.md` (2026-10-29:
  120 routes, 74 console files / 434 tests). The simulator-backend parity
  contract is `platform/tests/test_simulator_parity.py`; all 7 dependabot
  branches were triaged (PRs #1–#7 closed unmerged; branch deletion was
  blocked by the repository ruleset).
- Some §A deeds were closed after this list was written; closed entries
  are marked below and kept as a record. No §B queued item was retired by
  these phases; landed program items continue to live in
  [[full-tree-roadmap]] Landed per this page's rule.

> ⚑ State correction (2026-10-31): the 20-job **push** gate is green on the
> latest `develop` and `main`; the exact runs live in `AGENTS.md` and the
> current counts in `docs/repo-counts.md`. The **nightly thesis-scale** gate
> remains green at `c7d98f6` (`41041244412`: canary + both slice-60 jobs).
> Older red runs are history.

## Interface/runtime upgrade program (2026-11-01)

> Scope: the console interface and runtime projection program. This is
> **prototype scaffold** evidence unless a gate names a protected run; it is
> not a claim that the public console is production-ready.

Landed on `develop` and `main` with the console/Go gates green:

- Inspector shell: Details/Evidence/Jobs/Copilot tabs, closed by default,
  keyboard-resizable, overlay below `lg`, selected-workbench-object details;
  the page now owns the shared jobs projection.

- Contract/a11y foundation: canonical empty transcripts, job-status wire
  union, typed auth state, semantic tokens, readable data/control type,
  opened-on-demand copilot, and shared Button/Badge/EmptyState primitives.
- Campaign workbench: default arena view, typed cards, drag/resize, camera
  and position persistence, explicit links, keyboard nudge/open, a
  text/table equivalent, and a persisted spatial area.
- Versioned workbench storage: dedicated v2 table, bounded validation,
  revision/CAS saves, legacy-layout migration, and a typed 409 conflict.
- Data/artifacts: dependency-free SVG chart kit with exact values, CSV,
  provenance, accessible summaries; ResultsStudio, FleetRadar,
  EpistemicManifold, and Codesign now use it. At-rest fleet/observatory
  motion removed.
- Projection/transport: one authoritative session telemetry stream with
  poll fallback; campaign/loop/session projections moved out of the page;
  `/api/v1/...` aliases with `Kubona-API-Version: 1`.
- Rendering: the 3D observatory stops its rAF loop while hidden or offscreen.
- Runtime: measured QIR lowering benchmark; warm worker deferred because the
  measured workload does not justify it.

Landed in later slices:

- **Needs-you tray (2026-11-14):** open obligations surface in the shell
  (`ModeRail` badge + `NeedsYouTray`) with verbatim blocking reasons, a
  decidable/blocked state from the server's close rules, and a real waiting
  age; `Review` routes to the owning surface. Obligation rows now expose
  `created_at`/`updated_at`/`unknowns_detail`. Remaining: ledger
  `obligation_*` events for push invalidation, and the ageing escalation
  after 1 h described in [[runtime-honesty-and-commit-policy]].
- **Role-named scales (D1, 2026-11-14):** tokens are now exposed through
  Tailwind `@theme inline` (runtime aliases avoid custom-property cycles), and
  the default radius/text scales are reset, so a size-named radius or text
  utility compiles to nothing. A codemod moved **80 files** off
  `rounded-sm/md/lg/xl/2xl/3xl/full` and `text-xs/sm/base/lg/xl/2xl/3xl`, and
  the ladder gained exact role tokens (`chip` 4px, `sheet` 24px,
  `body-strong` 14, `body-large` 16, `subtitle` 18, `title-small` 20,
  `title-large` 24, `display-small` 30, `display-large` 36) with the old
  line heights, so the reset is not a restyle. `console-scale-lint.mjs` fails
  on any size-named use; `console-theme-check.mjs` proves the compiled CSS
  has no default scales and that role utilities resolve. The legacy
  `@theme inline` color aliases (including the `--color-card` collision) are
  gone. The suggested stylelint dependency is replaced by the palette suite's
  globals.css literal scan (keyframe glows and the modal backdrop are the only
  sanctioned raw colours). **Colour codemod landed:** all 80 `@theme` colours
  are exposed through runtime aliases and every arbitrary
  `bg/text/border/ring/...-[var(--color-*)]` utility was converted (4,532
  replacements across the source); `console-color-utility-lint.mjs` keeps the
  count at zero. A browser test imports the real token sheets and asserts the
  radius/type ladder, the card-vs-raised distinction, and dark/light
  resolution. Remaining: none in the arbitrary-colour class.
- **Import boundaries (D6, 2026-11-14):** deterministic layer check in CI
  (`types < lib < domain < components < app`); upward type imports moved to
  `types`, and three domain stores moved to `lib` with re-export shims.
  Remaining: the 67 downward `components → lib/api` imports are legal but
  still a coupling to reduce via domain projections.
- **Bundle budgets (D7, 2026-11-14):** `console-bundle-budgets.mjs` enforces
  first-load ceilings on the real build — recorded **1108.5 KiB raw / 326.8
  KiB gzip** plus **40 / 24 KiB** growth allowance (ceiling 1148.5 / 350.8),
  largest chunk 410 KiB — keeps three/elkjs/KaTeX out of first-load, and
  requires large lazy chunks to carry a stable marker + reason with stale
  records failing. The check found KaTeX in first-load; `MathText` now
  lazy-loads it only for `$…$` prose (−257 KiB raw / −75 KiB gzip). Remaining:
  no other route has a budget yet (the console is single-route), and the
  first-load total still leans on one 395 KiB chunk worth splitting.
- **Browser component tests (D8, 2026-11-14):** a separate Vitest browser
  project (`vitest.browser.config.ts`, Playwright provider, Chromium,
  `vitest-browser-react`) runs every `*.component.test.tsx` against a real
  page, so focus, keyboard, layout, and CSS are observable. The first suite
  pins `SegmentedControl` roving tabindex/arrow/Home/End behavior and
  `IconButton` non-submitting `type="button"` inside a form. A baseline
  ratchet (`console-test-renderer-ratchet.mjs`, 95 files) freezes new
  `react-test-renderer` use; the list may only shrink. Browser tests run in
  the Console E2E lane, where Chromium/Chrome is already installed. The
  token-sheet browser test now consumes the real palette/tokens/axes CSS.
  Remaining: migrate the 95 grandfathered `react-test-renderer` files.
- **Chart accessibility/refusal (D5, 2026-11-14):** announced summary via
  `<desc>` + `aria-describedby`, all-null refusal, compact exact-values table,
  actionable evidence buttons. **Split landed:** pure geometry/tick/scale
  planning lives in `chart-layout.ts` with five browser-independent tests,
  the eight renderers moved to `chart-parts.tsx`, and `DataChart.tsx` is now
  ~380 lines. `LazyBoundary` wraps the lazy chart and offers **Retry** for a
  render failure or **Reload** for a stale chunk (detected by
  `isChunkLoadError`); a browser test proves the retry remount. Per-chunk
  bundle budgets (D7) landed separately below.
- **Reduced motion + motion vocabulary (D3/D4, 2026-11-14):** OS preference
  and in-app axis both collapse motion globally; `lib/motion.ts` unifies
  imperative decisions; press/exit durations and ease-in/emphasized tokens
  added; `transition: all` is lint-banned in CSS **and** `transition-all` in
  source (112 utilities replaced with an explicit property list; the source
  scan is part of the token gate). Programmatic smooth scrolling now falls
  back to `auto` when either reduced-motion signal is set. Remaining: the
  per-component motion review (only the workbench glide consumes the hook;
  everything else relies on the global CSS collapse).
- **Recorded decision evals (B12, 2026-11-14):** 12 copilot fixtures
  (10 scored + 2 `unscored`) with exact labels `supported|refuted|unverified`,
  `measured|inferred` trust, and per-fixture SHA-256 pins; editing a prompt
  without re-pinning fails. Deterministic scoring computes multiclass Brier
  and 10-bin ECE only from complete probability vectors; missing, extra,
  malformed, or sha-mismatched predictions refuse. `kubonad eval-llm --role
  copilot --predictions file` records a report under `~/.kubona/evals/`;
  `GET /api/evals/llm` serves evaluated/unevaluated/stale status and
  ModelPicker shows `evaluated N/M · date` or `unevaluated`. A **strict
  gate** (`KUBONA_REQUIRE_LLM_EVAL=1`) refuses a changed copilot model that is
  unevaluated/stale unless the same write carries
  `acknowledge_unevaluated:true`; Settings surfaces that acknowledgement
  button and the backend records `llm_eval_acknowledged`. Remaining: the CLI
  scores recorded predictions only (no live model call), and only the copilot
  role has fixtures.
- **Runtime skills (B11, 2026-11-14):** strict `SKILL.md` loader (16 KiB,
  64 skills, name charset, no symlinks, closed `SkillFault` codes), six
  embedded seeds, and user skills in `~/.kubona/skills/`. Only names and
  descriptions enter the copilot prompt; the body is read through the
  read-only `load_skill` tool. `GET /api/skills` and Settings → Runtime
  skills show the catalogue and every rejected file. `load_skill` accepts an
  optional section heading and returns only that part; unknown headings name
  the available sections, and headings are matched case-insensitively. See
  [[workflows-and-agent-authoring]]. Remaining: no skill editor/installer UI,
  and no per-skill enable/disable.
- **Declared model capabilities (B13, 2026-11-14):** substring guessing
  removed; rate-card + OpenRouter metadata + Ollama `/api/show` are the only
  sources; reasoning effort normalized per provider/model and never sent to a
  model that does not declare reasoning; verification stays GET-only.
  Remaining: a `light` role for titles/summaries, and moving the session id
  out of the tool-context system note to keep cross-session cache prefixes
  shareable.
- **Memory hygiene (B10, 2026-11-14):** shared secret-shape screen on
  explicit and automatic retention plus recall-time defense; retraction
  tombstones (content cleared, embedding removed, event sealed, hidden from
  recall); explicit facts remain a separate bank. The shared screen now also
  refuses credential-shaped prompts at the cloud-LLM request edge (S9).
  Remaining: general ledger-write screening, and the memory list/inventory UI
  already exists in `MemoryPanel` (retraction included).
- **LLM budget reservation (B3, 2026-11-14):** a client-level spend guard
  reserves before every metered call, applies an affordable `max_tokens`
  ceiling, and releases after measured spend; unpriced/unmetered/unreadable
  spend refuses. Remaining: in-process reservations do not coordinate multiple
  daemons sharing one ledger (single-daemon deployment is the supported path).
- **Primitive correctness (D2, landed 2026-11-14):** safe Button default,
  pending/danger states, SegmentedControl roving tabindex + arrow keys,
  Tooltip trigger description. Remaining: the raw-`<button>` ratchet outside
  `components/ui`, and the motion, chart, boundaries, bundle, and
  browser-test items (D3–D8).
- **Run-scoped allowances (C11, Q-60):** reviewed with the recommended
  answers and implemented server-side (schema, signed mint, atomic draw, taint
  exclusion, rate-card/calibration + air-gap suspension, revocation, reader).
  Console control and launch-under-allowance wiring also landed. Remaining:
  hardware parent rails (`hardware_experiment`, `qaoa_hardware`,
  `spin_chain_hardware`) are still per-job only.
- **Mid-run steering (2026-11-14, report C10):** queued instruction rows,
  applied once at the next step boundary with a ledger event and a lane part;
  the run lane shows target/active-part disclosure and the postponement rule;
  non-running loops refuse typed 409. Remaining: no cancel/edit of a queued
  steer before the boundary, and the agent loop does not yet read the
  instruction text into its prompt (it is recorded and surfaced; consumption
  is a later wiring step).
- **Plan → tasks (2026-11-14, report C8):** dry-run plans publish a canvas
  `plan` card; batch task creation (`POST /api/tasks/batch`) is idempotent,
  ordered, and bounded; the card flips to Open in Tasks. Remaining: the plan
  draft is ephemeral (lost on reload; tasks persist), and unresolved tool
  references are facts rather than typed needs.
- **Per-fact source chips (2026-11-14, report C9):** job/loop/approval/task
  facts name their record; measurement/proof facts resolve to the measured
  job (else campaign); chips focus the owning card, including folded runs.
  Remaining: chart series facts and campaign-level facts do not carry refs
  yet (the chart's producing jobs are not attributed per point).
- **Citation-checked numeric claims (2026-11-14, report B5):** observation
  ids in the tool loop, a prompt rule, deterministic post-check with
  tolerance, streaming-safe citation marks, persisted `citations_json`, and
  a per-reply console badge that names unverified numbers, inline highlights
  for unverified prose and table-cell numbers. Table cells are scanned by the
  checker (regression-tested). Remaining: rendered math values are not
  scanned.
- **Claim provenance rows (2026-11-14, report B4):** typed `claim_evidence`
  rows (job_result/classical_anchor/literature/tribunal_turn/general_knowledge),
  automatic measured links at the job-claim mints, a provenance endpoint with
  a measured/complete summary, and an acceptance gate that refuses a verified
  claim without measured or explicit general-knowledge evidence. Remaining:
  the console has no provenance panel yet (endpoint only), and citation-number
  checking in copilot replies (B5) is the companion piece.
- **Canvas polish (2026-11-14, reports C4–C7):** labelled pending lines
  (on-screen-only pulse), least-shift follow-live on by default with glide and
  manual-pan pause, one-shot arrival animation plus a polite announcement,
  and a grow-only near set with sized placeholders for far cards. Remaining:
  the chart-kit split (D5) and the per-chunk bundle budgets (D7); no new gaps
  introduced by these.
- **Run parts / lanes (2026-11-14, report C1):** supervisor steps, lifecycle
  transitions, manual registrations, and dispatched jobs record `loop_part`
  events; the parts endpoint enriches job state + typed need; the agent-run
  card renders helper columns with an active marker. Remaining: parts from
  literature/prover/tool rails (none record them yet), copilot tool-call
  parts (chat turns are not loops), pending placeholders (C4), follow-live
  (C5), arrival announcements (C6), and viewport culling (C7).
- **Untrusted tool-output boundary (2026-11-14, report B6/S4):** one shared
  provenance fence and system rule across the copilot and tribunal rails;
  payload markers neutralized; effectful tools refuse after untrusted output
  unless they opt in (`submit_local_sim` is the reviewed exception); taint is
  audited on `tool_called` and refusals emit `tool_taint_refused`; the
  deterministic injection corpus is contract-pinned. Remaining: a
  model-behaviour injection eval (does the model actually resist?) and a
  structured `effect` class on tool definitions (today `ReadOnly` is the only
  signal).
- **Calm status strip + card isolation (2026-11-14, reports 3.9/C3):** API
  refusals and contract drift render as dismissible `role="status"` chips in
  `StatusStrip` (keyed by problem signature in sessionStorage); the only
  full-width alert left is blocking auth expiry. Each Workbench card and chart
  has its own `RenderBoundary`, and a pure sanitizer drops malformed edges/ids
  and clips text before render. Remaining: split the 1,055-line chart kit
  (D5), viewport culling (C7), and the arrival-motion/announcement work (C6).
- **Server-owned proposals (2026-11-14, report B1):** launch is bound to a
  server-minted `{proposal_id, digest}`; the client never supplies the
  circuit, backend, budget, shots, seed, or attribution at launch. Minting
  records the campaign revision + calibration digest as basis and is
  content-idempotent; launch refuses stale basis, wrong digests, and spent
  proposals; the proposal is burned before dispatch; chat replies carry
  `proposal_id`/`proposal_digest`/`proposal_error`, and unpriced metered
  plans stay display-only. A session proposal history/list surface now exists
  (`GET /api/proposals?session_id=...` + the Copilot drawer panel). Remaining:
  pricing model proposals that do not carry a numeric bound (today they are
  refused rather than guessed).
- **Typed job needs (2026-11-14, report C2):** jobs carry a closed-vocabulary
  `need` (`platform/contracts/need-kinds.json`) classified from structured
  evidence, persisted with the failure, and rendered as one sentence + one
  remedy on the card; free-text failure inference removed from
  `isIssueWorkbenchObject` and the job card. Emitted: credentials,
  backend_offline, quota_exhausted, retry. Remaining: emitters for
  `budget_exceeded` / `approval` / `anchor_missing` once the owning rail
  records the fact; loop needs need a recorded failure field;
  `ProofDagView`'s watchdog/branch status matching is campaign-trajectory
  classification, not job-failure inference, and waits for typed campaign
  verdicts.
- The workbench object-kind set is covered: job, agent-run, table, approval,
  and note cards join the campaign/hypothesis/circuit/measurement/evidence/
  proof/manuscript/chart cards. Records now carry distinct refs; links, area
  assignments, and content bodies are validated at the API. Areas can be
  renamed, moved, resized, and assigned; a 409 exposes the current revision and
  a Reload action instead of overwriting. The canvas virtualizes past 24
  objects; 300+ object projection/serialization has a <250 ms median guard.
  An opt-in Playwright harness rendered 361 mocked objects and measured
  60.7 fps / p95 16.7 ms in headless Chrome on 2026-11-01 — a single-machine
  observation, not a CI gate.
- Backend readiness, spend, fleet/radar, flight-recorder telemetry, and
  job progress now share a serialized, visibility-gated, backoff-bounded
  polling store with terminal-stop support. Copilot tribunal-turn polling is
  a local, run-scoped exception.
- Revisioned domain events beyond the ledger cursor. The stream now gates
  stale frames and refetches a snapshot after a forward gap; jobs, obligations,
  fleet/radar, cost, and backend readiness have named domain projections backed
  by a ref-counted store registry; simultaneous consumers share one store and
  the last unsubscribe stops/resets it. Campaign create/delete use shared
  write settlement; the Copilot transcript is a shared per-session store.
  The composer attaches readable text files into that transcript and links to
  Model Hub; per-send model switching has no backend contract and is not
  faked. Autoresearch and metasearch writes use the shared settlement
  vocabulary; collapsed transport/refusal is shown as unknown, never applied; generated contract
  types, operation metadata, and a checked `requestOperation()` client are
  landed; a Go drift test checks every generated console path/method against
  the route-policy table (it caught a missing GET on observatory lens).
  Close-obligation, quantum cancel, custom-agent delete, and metasearch
  approve/reject/apply, LLM/quantum settings, canvas layout, chat feedback,
  loop actions, HF cancel, vault-lease revoke, and quantum canary now use
  generated operation metadata: tenant ceiling/status/autonomy/revoke/refund/
  admin-key and session refund followed; auth logout/revoke, session delete,
  and tenant-grant issue/revoke too; codesign budget and session/ledger
  export/import complete the set, so the inventory is 124 modeled /
  13 client-only (from 91/40) with zero unmodeled operations. The remaining names are transport/browser/
  auth helpers plus the `fetchSessions` delegating wrapper; none is an
  unmodeled operation; the generator emits alias operations
  sharing one REST path without duplicating the OpenAPI operation;
  session, loops, events, jobs, fidelity, ADMET, agents, LLM/quantum
  settings, radar, cost, obligations, notebooks, capabilities,
  session-context, abort, workflow, quantum-submit, messages, job-detail,
  memory-recall, autoresearch, metasearch, epistemics, quantum-job, QIR
  compile, calibration, prediction history/latest/seal/evaluate/assert,
  lens-history/status, vault lease, ledger export, mesh routing, memory
  retain, lab creation, observatory asset/lens resolution, LLM models/test,
  HF search/details/downloads, and auth challenge/login/whoami/sessions
  wrappers use it, plus codesign packs/verified, HF download start,
  custom-agent create, literature search, ledger verify, lens request,
  campaign synthesis, all autoresearch loop actions, fleet, canvas layout,
  codesign evaluate/batch/family/rerun, chat send, tribunal run/resume,
  proposal launch, and discovery pipeline.
  Typed operation errors carry the backend refusal body. Layout and
  obligation-close writes render accepted/applied/refused/unknown settlement.

> **Update (2026-11-10):** Console contract inventory expanded to **142 modeled /
> 14 client-only / 0 unmodeled** (`121` schemas, `130` OpenAPI paths) with the
> addition of science experiment, discovery protocol, calibration refresh,
> claim review, and signed experiment-export endpoints. Additionally,
> `api/experiments` + **Experiment & Discovery Lab** (`Cmd+E`),
> `HardwareLaunchPanel`, and `DiscoveryProtocolPanel` intentionally expose the
> governed UI forms for all live measured kinds (`vqe`, `geometry_optimization`,
> `vqe_repeats`, `pes`, `hardware_pes`, `excited_state`, `reaction`, `spin_chain`,
> `thermodynamic_limit`, `lattice_dynamics`, `qaoa`, `conjecture`, and
> `/api/discovery/{plan,run}`), superseding the 2026-10-17 note declining
> standalone unbacked experiment forms. All 8 stale `dependabot/*` branches were
> deleted from origin on `2026-11-10` after scoping Ruleset `25762910` to exclude
> `dependabot/**/*`.

Still open (as of 2026-11-10):

- Manual VoiceOver/NVDA evidence: **NOT RUN**; the operator explicitly skipped
  it. Automated axe, forced-colors, 200%-zoom, reduced-motion and settings
  workspace audits are green, but they are not a manual pass.
- Design-system migration: the shared primitive set exists (Button through
  Tooltip), and Separator/KeyHint are used by the shell; most product surfaces
  still use raw utility classes. The CI ratchet now holds **865** occurrences,
  down from 873. The shell has a global campaign header outside the arena, the
  status strip is at the bottom, one banner slot outranks API refusal behind
  auth expiry, Settings is a single-column section workspace (186 visible DOM),
  four font families were reduced to Geist + JetBrains, decorative gradients
  and glass were removed from surfaces, and rest-state float/pulse was removed
  from the shell. Remaining: migrate all forms to Field/Select/Toggle and retire
  the raw-palette debt rather than hold it.
- Budgets and evidence: `e2e/runtime-budget.spec.ts` now measures **425 KB
  static / 435 KB all-resource / 473 DOM** on the normal mocked boot, plus
  **186 visible Settings DOM**; the dense 361-object boot is **425 KB static /
  473 DOM**. LCP/INP/heap/request-count gates remain open. The full nightly is green at `d1a48ed` (run `41418158035`: canary, 100/300 FPS,
  slice60 composition + determinism).
- Transport hardening: the typed auth phase store now bootstraps through
  `whoami`; anonymous 401s no longer raise an outage toast or expired banner;
  the session stream is fetch-based, bearer-capable, bounded-backoff reconnecting,

> **Update (2026-11-11):** Semantic tokens and appearance axes (`tokens.css`,
> `axes.css`, `appearance.ts`, `AppearanceSettings.tsx`) cut `console-token-lint.mjs`
> raw-palette debt from **865** to **14** baseline occurrences (ratchet held in
> `platform/contracts/console-token-baseline.json`); `Experiment & Discovery Lab`
> (`Cmd+E`) moved from a blocking modal to a non-blocking right drawer;
> `SpatialCanvas` is now directly promoted from the `Molecule` card and HUD
> alongside session-scoped `GET/PUT /api/workbench/view` viewport persistence;
> `page.tsx` isolates mount-only bootstrap from polling and `resource-store.ts`
> queues concurrent SSE `pendingRefresh` invalidations; mobile `390×844` drawer
> overlays and `runtime-budget.spec.ts` / `stream-invalidation.spec.ts` pass
> against `kubona-live`. Remaining from above: retiring the final 14 baseline
> token occurrences, remaining form primitive migrations, and LCP/INP/heap
> budget gates.
  and carries `Last-Event-ID`; telemetry invalidates campaign/jobs/tasks/loops/
  obligations and pauses their fallback polls while live; ledger history pages
  backwards. Operation receipts are queryable and layout/task writes are
  idempotent with retained request ids; unknown writes reconcile against the
  receipt. Remaining: make every mutation use the receipt rail, and give fleet/
  spend/readiness an explicit external-poller policy rather than calling them
  fallbacks.
- Contract truth: job status is now a canonical lowercase string with a
  `status_code` compatibility field; empty lists are canonical; contract tests
  cover all 123 unique modeled REST operations (124 modeled functions, one
  duplicate binding) and v1 parity for 12 modeled reads; the console sends
  `Kubona-API-Version: 1` when opted in. No modeled operation is left
  handler-untested; the OpenAPI document is still generated from the
  client response schemas, not the Go route policy. `/api/v1` remains opt-in.
- Legacy retirement: `WidgetStage` and the legacy molecule canvas stay until
  object cards reach five-widget parity and the old aliases have one release of
  support. Recorder and Manifold still behave as separate full-page products
  rather than Workbench views. `KUBONA_CONSOLE_WORKBENCH=0` is the tested
  rollback: the arena removes the Workbench tab and keeps the legacy orbital
  surface plus the molecule object view reachable.
- Workbench content: approval cards now expose review and close-ready close,
  tasks complete inline, job status is canonical, campaign cards show recorded
  spend, links render only for the selection, and the 3D control stack collapses
  behind one disclosure. Evidence cards now carry typed source links or plain
  references, explicit basis text, and no invented quote/confidence; measurement
  cards distinguish the VQE-FCI discrepancy from uncertainty and state the
  chemical-accuracy limit. Remaining: new-result highlight, camera-follow, and
  agent-presence fade. The molecule opens as a named object-view dialog; the
  3D loop publishes a frame counter and suspended reason, and a browser E2E gate
  proves hidden/offscreen frames stop.
- Rust: the Rust core returns a structured JSON receipt (QIR text, qubit,
  depth, gate counts); the Go API consumes those fields directly and Python no
  longer re-parses QIR on the shipped path. Both frontends split QASM by
  statement (one-line programs), expand whole-register `measure q -> c`, and
  use summed multi-register offsets. The live endpoint reports
  `rust-qir-core` / `BaseProfile`; invalid QASM is a 400. A full-process
  benchmark (`scripts/qir-bench-full.py`) records p50/p95/p99, receipt bytes,
  and child RSS and runs in CI as a signal. A warm worker remains deferred until
  the wrapper measurement justifies it.
- Per-send model override has no backend contract; the composer links to Model
  Hub instead of faking one.
- The 14 remaining client-only names are intentional transport/browser/auth
  helpers (plus the delegating `fetchSessions`/`fetchCampaignState` wrappers),
  not unmodeled operations.

## Local-first production (2026-10-30, landed)

> Scope correction: "production ready" here means **fully functional for a
> local private deployment**, not a public SaaS. The supported shape is one
> operator on their own machine/lab host, loopback only; the only cloud
> egress is QPU dispatch and opt-in cloud LLM (BYOK).
>
> - ✅ **B2 local ingress:** `compose.prod.yml` publishes `127.0.0.1:8080` /
>   `127.0.0.1:3000` only, services are read-only with dropped caps, and
>   `scripts/local-deployment-check.sh` refuses a public listener. Live
>   proof 2026-10-30: fresh volume, health/ready/console green, check exit 0.
>   Full runbook: `docs/local-deployment.md`.
> - ✅ **C5b governed rerun:** `POST /api/codesign/rerun` is session-scoped (anonymous refused),
>   rate-limited, idempotent (`request_id` required), bounded by steps/
>   tokens/time/USD, and sealed as a `trial`. Cloud LLM is BYOK: the provider
>   bills the operator's key; Kubona records an estimate and never debits the
>   QPU session cap. Local providers are free. The bridge stays stateless
>   (new `spec` op; no LLM keys). Docs: `docs/llm-proposer.md`.
> - ✅ **B4 local diagnostics:** logs, `/metrics`, health/ready, request IDs
>   and local traces remain on-host; no telemetry by default. Sentry/OTLP
>   stay opt-in, not release requirements.
> - ✅ **Bootstrap:** `kubonad keygen` emits reviewer/operator Ed25519 seeds;
>   `.env.example` and the runbook name the token/key requirements.
> - ✅ **FK quarantine (2026-10-30):** 127 orphans quarantined from
>   `~/.kubona/kubona.db` (50 chat_messages, 40 campaign_versions, 23 jobs,
>   12 loops, 2 claim_records); `PRAGMA foreign_key_check` is clean and all
>   rows are preserved in `_fk_orphans`. The raw pre-repair snapshot is
>   `~/.kubona/backups/kubona-pre-fk-20261030T024721Z.db`
>   (sha256 `c15bffd6843e7263de8c64bb1a9d3b8d9c00c4898ba851cfce4990eb53146f3b`).
>   **Caveat:** `kubonad backup` refused it as a chain-clean backup (606
>   pre-existing ledger chain faults: 258 unhashed legacy v1 events,
>   unsigned commit events, and their link cascade). Resolved by disposition
>   on 2026-10-30: do not backfill/retro-sign; the host DB is forensic only,
>   and production must start on a fresh ledger. The snapshot is
>   SQLite-consistent (integrity_check ok, 1470 events) but not a verified
>   provenance backup.
> - **Rate card (2026-10-30):** the embedded bootstrap was refreshed to current
>   provider models/prices (retired entries removed); production cloud pricing
>   now **refuses the embedded card** and requires a runtime vendor-derived
>   card (`KUBONA_RATE_CARD_PATH`/`URL`). `kubonad rates-audit` verifies the
>   assigned models against live `/models`, checks provider-qualified prices,
>   and can write a used-model-only card. Remaining operator work: replace the
>   bootstrap with a vendor-invoice/list card before 2026-11-11.
> - **Nightly green (2026-10-30):** `Builder` stores ops in compact typed
>   columns and streamed evaluation cut the paper-scale peaks below 5 GiB.
>   Manual run `41041244412` passed the canary and both slice-60 jobs at
>   `c7d98f6`; push CI is 20/20 at the same commit. Keep re-verifying the
>   exact commit; do not cite the older red runs as current.
> - Remaining operator-side for a production claim: the vendor runtime card,
>   attaching the paid QPU run evidence, and the historical
>   cancelled-vs-completed settlement row. The 606 chain faults remain a
>   disposition (fresh ledger for production), not a rewrite target. Provider
>   keys are present in `.env`. These are not code gaps.
> - ✅ **Console rerun panel** (2026-10-30): Codesign Cockpit session picker,
>   request_id retry, step/token/USD bounds, sealed receipt; 439 console tests.
> - ✅ **MCP rerun wrapper** (2026-10-30): `cost/codesign_rerun` forwards to
>   the governed Go route with the operator bearer and a scoped budget envelope;
>   no LLM key in the MCP server. Catalog now 38 tools (regenerated).
> - ✅ **Deployment doctor** (2026-10-30): `kubona doctor --env-file .env`
>   reports rate-card freshness, FK orphans, key presence, LLM/air-gap posture,
>   bridge, and daemon readiness; `--strict` exits non-zero. On this host it
>   found 127 FK orphans in `~/.kubona/kubona.db` and a persisted
>   `active=nvidia`/air-gap-off setting with `KUBONA_LLM_CLOUD` unset — both
>   operator-visible now.
> - Optional remaining: opt-in OTel/Sentry exporters (backend/DSN choice);
>   the local release dry-run is done.
> - Short execution list: `docs/remaining-todos.md`. A new blocker surfaced
>   on 2026-10-30: the host ledger has **606 pre-existing chain faults**, so
>   the FK-quarantine snapshot is forensic, not a verified restore source.

## Optional enhancement program (2026-11-01)

Post-production work is sequenced in
[[optional-enhancement-plan-2026-11-01]]: API versioning, external alert
routing, deeper assistive-technology audit, and deferred at-rest/observability
hardening. None blocks the supported local-private deployment.

## A · Operator deeds (blocked on the human — no slice can deliver these)

- `gho_…` token: `credential.helper=store` was dropped globally and
  `~/.git-credentials` was reported empty ([[risks]]), but the token
  revocation itself is not proven by the repo record. Treat this as
  **open until the token is revoked at the provider**, not merely removed
  from the helper.
- ~~Q-27 history flip~~ — ✅ CLOSED 2026-10-26: `git filter-repo` scrub,
  force-push, re-clone, and ruleset re-enable are recorded in [[risks]].
- ~~Advance `main` to `develop`~~ — ✅ CLOSED 2026-10-29: `main` and
  `develop` are both `5fda062`.
- Tenant reviewer key (`KUBONA_REVIEWER_PRIVKEY_HEX`): AGENTS and the
  2026-10-29 log say it is commissioned; this queue still said open. The
  authoritative state is the deployment environment, not the repo record.
  Confirm `reviewer_key_configured=true` on the target host before
  citing it.
- QPU credentials + backends: ✅ IBM credentials are present in `.env`
  (`KUBONA_IBM_QISKIT_TOKEN`, instance, `ibm_quantum_platform`, 120 s cap);
  per-run backend choice and a GPU host for local training remain operator
  choices, not missing secrets.
- `KUBONA_CONTACT_EMAIL` (console mailto + polite-pool), HF-token rotation,
  EPO registration.
- ~~First paid QPU run~~ — ⚠️ OPERATOR-CONFIRMED 2026-10-28, evidence caveat
  (2026-10-30): IBM job `dbha28pq260s73busu70` (`ibm_fez`, DONE, 2 s,
  0.028795 RU, 100 shots, counts 00:58/11:38/10:1/01:3) maps to local job
  `39b5e6b832cd4ae3` (estimate $8.000568, local cancelled, settled $0.00).
  The configured IBM instance reports **Open Plan** (600 s/month, 20 s
  consumed), so a paid invoice cannot be proven from it. The cancel path
  now fetches vendor status and settles measured usage; the historical row
  remains for operator reconciliation. Attach the paid instance/invoice or
  run id before quoting a paid cost.
- DeepSeek-vs-oMLX transcript delta (keyed providers; harness ready).
- Device/emulator recalibration (keyed hardware; canary verifies
  live-vs-declared meanwhile per Q-14).
- Protogen venv re-pin (`grpcio-tools` 1.71.x) + regen Python pb2 stubs
  (`EvidenceEnvelope` message waits on it). ✅ CLOSED 2026-10-29: scratch
  venv `/tmp/protogen` re-pinned to 1.71.0, regen reproduced byte-identical
  output (1-line stamp delta only), `EvidenceEnvelope` imports cleanly
  under the pinned 5.29.6 runtime; deterministic regen proven.
- R-series defaults CONFIRMED 2026-10-29 (operator decision, no code change —
  all four were already implemented as recorded): R1 extend (+tenant
  mandatory, +mandate_ref optional; envelope.go both rails + byte-identical
  vectors); R2 tribunal-quorum authority = the four seated roles as required
  authorities + `ledger-receipt` for mandate settlement (AttemptClose
  enforces; Lean proof / human sign-off stay future authorities, not
  required today); R0 lanes fast+boundary required, changed advisory
  (CONTRIBUTING.md + CI mapping); P1-4 consolidation stays queued behind
  R1–R3 (single maintainer, gate is green without it — revisit when the
  contracts drive a need, not before).
- R6 gating policy RESOLVED 2026-10-29 (operator decision, recorded in
  `platform/core/pkg/agents/scoring.go` header): advisory-only, permanently.
  Substring heuristics are gameable — no gate may branch on scores. Scores
  surface in tribunal responses + the review queue (human prioritization).
- Failed-mandate review queue ✅ LANDED 2026-10-29: `GET
  /api/obligations/open` (lab-scoped list + close-readiness reasons) and
  `POST /api/obligations/{id}/close` (owner session or owner-key signature;
  bearer alone never sufficient; double-close idempotent; no oracle).

## B · Queued programs (unblocked, beyond a single slice)

- UI/Docker production program (report 2026-10-17: U0 → D1 → U1 → U2 → U3 → D2).
  > ✅ U0 LANDED: console tests + lint in CI (19 lint errors fixed:
  > 6 render-phase key-change resets — no stale-session bleed — + 13
  > typed test casts); quantum-runner == pins measured from the shipped
  > image + rebuilt + import-verified; openbao digest-pinned + dev-only
  > header.
  > ✅ D1-skeleton LANDED: Dockerfiles for kubonad (CGO/sqlite,
  > nonroot, healthcheck) / console (standalone, CI-identical toolchain,
  > nonroot) / bridge (pinned extras, grpc healthcheck); `.dockerignore`
  > ×3; `compose.prod.yml` (health-gated, resource-limited, vault +
  > pgvector deliberately absent); CI `docker-smoke` job wired into the
  > release gate. Live findings fixed: fresh state roots never became
  > ready (daemon now ensures the models dir, regression-tested);
  > plaintext-compose needs the explicit opt-in the server demands.
  > Next: U1 (health banner, tenant writes, loop override, metasearch UI).
  > ✅ U1 LANDED 2026-10-17: backend status strip (down vs degraded vs
  > healthy, 15s poll — ends "empty vs down" ambiguity); loop
  > pause/resume/kill roster in the recorder (status-gated buttons,
  > confirm on kill, reasons verbatim, proven live pause→resume→kill);
  > metasearch approve/reject/apply panel in the cockpit (+ the missing
  > reject client; signer `console-operator`; proven live list-shape +
  > gated refusal); LabsView tenant writes (ceiling/suspend/refund +
  > admin bootstrap) with byte-exact admin envelopes (Go-generated
  > golden hashes, 4 vectors; full live cycle on compose: bootstrap →
  > ceiling → wrong-key 403 → suspend → frozen-write 403 → resume →
  > no-bearer 401).
  > ✅ U2 WIRED 2026-10-17: session export/import + ledger export
  > (JSON/Markdown download, import switches to the session) in the
  > recorder — proven live as export→import idempotent replay
  > (replayed:true, no duplicate); re-synthesize-campaign button in the
  > tribunal room (same 75% quorum bar as commit; proven live, full
  > campaign state returned). Correction to the U2 brief: the modal's
  > "Synthesize Reference Baseline" path was already wired server-side
  > (`fresh:false` synthesizes at creation) — only re-synthesis of
  > existing sessions was missing.
  > ⏸️ U2 DECIDED 2026-10-17 (wire-or-decide outcomes, each with trigger):
  > autoresearch UI deferred — engine is eval-driven (nightly canary),
  > no operator workflow designed (who starts loops, with what artifact,
  > toward what end); trigger = workflow design, no theater before it.
  > krylov / mitigation / fidelity / ADMET / discovery-pipeline console
  > forms declined — agent-consumed compute stages (task_executor,
  > campaign_synth, bridge); the console already surfaces their results,
  > a hand-run form duplicates agent paths with no operator action
  > behind it (discovery curation belongs to the blueprint layer,
  > [[questions]] §D). notebooks-endpoint UI declined — ResultsStudio
  > already renders notebook content from campaign state (redundant read
  > path). canvas layout save/load deferred — waits for an interactive
  > SpatialCanvas (currently static display). server-signed mandate
  > console path declined — client-mint is the console path,
  > server-signed stays daemon/CLI-side; unify at IdP (U3).
  > ✅ UI COMPLETION 2026-10-17 (opencode-deepseek is the dev LLM; oMLX
  > parked): fetchJob threads session_id (job detail/JobProgress 400s
  > fixed, proven live old-400/new-200 on current build — which also
  > exposed the dev daemon as a stale binary: the /tmp/kubonad-s8a file
  > was removed during cleanup while PID 54943 lives on; rebuild from
  > current tree before any restart); fleet reports credential_state
  > (ready vs credentialless) + measured bridge liveness (dial, not the
  > never-nil client) with a NO CREDENTIALS console badge, proven live;
  > copilot no-proposal hint names the offline exits when the engine
  > notice fires. Bridge red test fixed to assert the W2.18 refusal at
  > equal strength + stale Tier-1 comment corrected; the bridge suite now
  > runs in CI (it lived outside testpaths, which is how the red went
  > unnoticed). Findings needing the operator: opencode returns 402
  > INSUFFICIENT FUNDS — tribunal/chat/LLM features are down until
  > top-up (oMLX serves locally with models as the fallback; repoint
  > roles on order).
  > ✅ M1–M6 LANDED 2026-10-17 (no hardcoded models): Go NVIDIA plumbing
  > (provider type + endpoint + env-key wiring, zero model IDs);
  > env-driven selection (KUBONA_LLM_PROVIDER, *_MODEL incl. NVIDIA_MODEL,
  > per-role overrides, .env.example); settings schema_version +
  > stale-blob stamping (no value rewrites); write-path validation
  > (unknown provider/empty model refuse, unchanged entries
  > grandfathered); GetModelForRole names the exact env var + refuses
  > silent fallback; kill-switch plumbed end-to-end (snapshot + update +
  > console toggle); RecordSuccess last_ok. Console: editable provider
  > defaults, active-provider select, free-text models with datalist
  > suggestions, last-success display, active-provider-biased fetch,
  > new-agent model required. Pricing: LoopIterationBound refuses unknown
  > metered models (new unpriced_model code) instead of over-reserving.
  > Python router refuses instead of inventing (discovery first, explicit
  > only); seeds unpinned (Go custom.go + Python user_defined.py);
  > legacy PUT /api/settings now validated. U3 runtime backend URL via
  > /api/config (no rebuild per backend). D2 digest-pinned base images
  > (all Dockerfiles rebuild + serve green).
  > ✅ M0 DONE 2026-10-17: daemon verified idle (no live loops/jobs, 38
  > sessions persisted, loops reconcile on boot), killed gracefully,
  > restarted on a current-tree rebuild with .env (NVIDIA key + provider
  > + model), all 5 roles repointed via authenticated save and proven
  > live — QUARK via private/nvidia/nemotron-3.5-super in 1.1s.
  > ✅ Kill-switch UI DONE: DSML toggle in LLM Settings + round-trip
  > test (toggle flips, persists via save).
  > ✅ A1–A4 Session-based Auth (U3 remainder) DONE 2026-10-17:
  > Ed25519 challenge-response login (`POST /api/auth/challenge|login|logout|revoke`,
  > `GET /api/auth/whoami|sessions` in `pkg/api/auth_session.go`) bound to each
  > lab's registered admin pubkey; `httpOnly` + `SameSite=Lax` cookie
  > (`kubona_session`, SHA-256 token hash only in `auth_sessions`, 60s single-use
  > challenge nonce, 12h TTL via `KUBONA_SESSION_TTL`); `csrfAllowed` Origin check
  > on unsafe cookie-authenticated methods (including `PUT /api/llm/settings`,
  > `POST /api/auth/logout`, and `POST /api/auth/revoke`); `RotateAdminKey`
  > immediately revokes active tenant sessions; `AuthPanel.tsx` mounted in
  > `LabsView.tsx` with WebCrypto Ed25519 signing + Vitest coverage; 7 missing Go
  > ledger event types registered in `platform/contracts/event-types.json` +
  > `pkg/ledger/events.go` (121 total rows: 80 Go + 41 Python).
  > ✅ LocalModelRegistry `ports=[]` fix DONE 2026-10-17: `registry.py` now uses
  > `DEFAULT_PORTS if ports is None else list(ports)` (and skips `_lms_endpoint()`
  > when `self.ports` is empty), so `test_llm_registry_has_no_stale_cache` passes
  > deterministically even when a local LLM server is active on `:8787`.
- P1-4 subsystem consolidation execution — design: [[agent-subsystem-consolidation]].
  > ✅ STEP 1 LANDED 2026-10-17: conformance freezes on all three
  > registry homes (Go + 2× Python) + boot-crash hardening
  > (negative-proven). Step 2 scoped: wire-shape freezes both sides,
  > canonical mapping left for the operator (three Go vocabularies,
  > not one). Steps 3–5 await operator scheduling + identity-home
  > decision. Page status stays `candidate` — correctly.
- Lean skill slices L1–L5 (plan: [[leanrea-adoption-plan]] §3, conflicts:
  [[leanrea-conflicts-and-remediation]] — S-sized, ordered L1 → L2-baseline
  → L3 → L2-rerun → L4 → L5 (LC3: baseline before change), none touch the
  trust boundary; distribution gated on the public flip (LC1).
  > ✅ L1 + L2-baseline LANDED 2026-10-17 (Phase L-A exit met):
  > `skills/kubona/SKILL.md` + pins, `bench/` harness + first numbers.
  > ✅ L3 + L2-rerun + L4 LANDED 2026-10-17 (Phase L-B exit met):
  > scribe tags (convention, R3 gate unchanged), bench delta zero on
  > record, doctor deepening + `mandate-check` (LC5 wording contract).
  > ✅ L5 LANDED 2026-10-17 (Phase L-C exit met): MCP catalog +
  > 29 keep-verdicts (P2-13 closed) + unscoped-tool guard + `not_found`
  > code. Lean program COMPLETE — all phases exited green.
- Bundle export endpoint — needs run-archival design (post-run turns
  cannot be reconstructed from events+chat; collective-level flow exists).
- Sidecar rotation — needs chain-aware reads (rotation breaks the
  tail-chained ledger assumption).
- S8c full hub — needs skills/packs/automations backends (reduced: no
  theater built).
- Steer — needs engine surgery (documented N/A: synchronous model has
  no background turn to steer into).
- ~~Assess principal threading on the remaining MCP servers
  (cost/execution/provenance write paths)~~ — ASSESSED 2026-10-17, no
  change (by design, threat argument): control already threads
  `requester` with label scoping (`test_control_tenants.py`); provenance
  reads thread `requester` via `readable()` with ambient resources
  public-only; provenance writes take caller-stated `label` (default =
  file's own tenant, files sharded per tenant, label bound into the
  content hash so forgery breaks validation); cost/execution run a
  possession model (unguessable `uuid4` session/job ids) + gateway scope
  gates for WHO — consistent with the `step_loop` precedent (possession
  of loop_id, documented). Known limit, stated: the gateway never binds
  `principal.sub` → `requester`, so label isolation is honest-caller +
  file-sharding + scopes, not cryptographic binding — the documented
  local-deployment model. Re-open if MCP serves remote cross-lab
  clients.
- Deferred batch: IdP (Phase 4), human lab membership (Phase 4),
  pgvector activation, sharding, full LLVM lowering, KEMTLS re-eval,
  CKT, Cirq/Q#/Serverless/Braket/rQOPS/CQM providers, GQE transformer,
  Bemba/Typst surfaces, quantum multi-geometry, loop load/chaos tests,
  cgroup kill, peercred sidecar (Q5 Linux hardening), workspace-files v2.
- Lean image — blocked on toolchain (no `lake`/`elan`, image
  unresolvable); comparator stays AST-first.

> Current reconciled backlog and phased plan: [[open-backlog-2026-11-15]].

## E · Feedback and notification closure

See [[feedback-loops-and-notifications]] for the full audit. The short queue:

- **Landed 2026-11-15:** durable votes + GET hydration/toggle/actor/reason;
  advisory-review labelling + caller identity; proposal launched/dismissed
  events; derived notification inbox with read/resolve state, SSE domain,
  coalescing, and Notification Center; feedback-reason UI and proposal-dismiss
  UI/reason; server-synced quiet/muted notification preferences; skill
  enable/disable persisted across restarts; steer edit/cancel and autoresearch
  prompt consumption. Claim reviews are **advisory permanently** (decided).
- **Open P1:** proposal-dismiss reasons used in ranking, Attention Director
  wiring for every producer, one polite live-region batch, removal of the
  remaining duplicate per-surface toasts/chips, skill editor/installer.
- **Open P2:** feed votes/reviews/dismissals/steering outcomes into
  eval/ranking with an explicit policy, and measure delivery/read/action
  latency.

## C · Conditional (wire when a consumer exists)

- Python↔Go mandate crossing — bytes agree three-way (W0.9 vector);
  both rails mint and verify their own by design. Wire only if a flow
  needs cross-rail mandates (none does today).

## D · Standing questions

- All 48 questions in [[questions]] Sections A–E (including Q-01 through Q-48 and Q-27 history scrub) are **settled and closed** as of 2026-10-26 (0 open questions remaining across the entire blueprint).

Related: [[questions]] · [[roadmap]] · [[risks]] · [[full-tree-roadmap]] ·
[[rea-adoption-plan]] · [[cloudroom-adoption-plan]] ·
[[agent-subsystem-consolidation]]
