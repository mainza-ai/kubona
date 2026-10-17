---
type: concept
name: leanrea adoption plan
date: 2026-10-17
updated: 2026-10-17
tags: [plan, leanrea, skill, benchmark, mcp, production]
sources: [source-leanrea-investigation-2026]
status: adopted
---

# Kubona × leanrea: measurement, report + implementation plan

> Production stance: every slice below specifies its tests and refusal
> paths up front, so nothing lands as a stub that must be redone. This
> page **updates and narrows** the 2026-10-17 addendum (uncommitted
> scratch; its durable content is re-verified here, not copied): the
> addendum assumed kubona might share REA's disease. Measurement says
> otherwise — §1 first, proposals second.

## 0. Thesis (corrected by measurement)

leanrea's counter-thesis stands — *every MCP tool definition is a
per-turn token tax, justified only where the wrapper provides what the
raw path cannot* — but kubona does not have REA's disease. Measured
2026-10-17 (method: serialize each mounted tool as
`{name, description, inputSchema}` from live signatures +
docstrings, `json.dumps`, sum bytes; reproduce with the L2 probe):

- Python MCP surface (5 servers, gateway mount table
  `platform/kubona/mcp/servers/_mount.py:27-60`): **29 tools,
  ~8–10 KB, ~2–2.5k tokens** — roughly **1/200th** of REA's
  all-loaded 400.7k tokens ($1.60).
- Tribunal turns (Go, `DefinitionsForLLM(role)`
  `platform/core/pkg/tools/tools.go:142-171`): **8 role-scoped tools
  per turn, ~3–4 KB, ~1k tokens** — already lean by construction
  (role scoping is the leanness; no turn ever loads the full set).

So the program here is **hygiene + knowledge distribution + measurement
discipline**, not emergency surgery. The trust/information boundary
still rules every decision:

| | Trust boundary (wrapper IS the product) | Information boundary (knowledge beats wrappers) |
|---|---|---|
| Kubona tools | `cost` ×9 (ceilings, approvals, x402), `provenance` write/query (sealed, label-scoped), `execution` submit/cancel (admission, CAS fence), `control` pause/resume/kill (override authority) | `list_provider_registry`, read-only tails (`get_job_status`, `session_status`, `list_loops`), `compiler` ×3 (pure compute — weakest justification, see L5) |
| Playbook | REA contract discipline (already built: Waves 0–2, R-series) | leanrea knowledge-first (this plan) |

## 1. Gaps and missing functionality (verified against the tree)

- **G1 — No SKILL.md, no machine-readable operator doc.** Case-sensitive
  globs for `SKILL*`/`skills*`/`*skill*` are empty in-repo (only external
  refs + explicit "hub would be theater" notes). Nearest: `platform/README.md`
  entrypoints, MCP tool descriptions, `CONTRIBUTING.md` test lanes. An agent
  opening kubona today pays full discovery turns with no router, no traps,
  no first-moves.
- **G2 — Thin `doctor`, no validity first-moves.** `platform/kubona/run.py:110-116`
  prints 5 config lines only. One-shot inventory exists as MCP tools
  (`session_status`, `list_provider_registry`) but no CLI first-move
  pastes an agent to answers; mandate validity is MCP/API-only
  (`platform/kubona/cost/mandate.py:130` tuple, no CLI check).
- **G3 — Scribe has no seen/inferred/unknown tags.** R3 enforces machine
  coverage (`Available`+`Reason`, `SealTurn` refusal,
  `platform/core/pkg/agents/collective.go:334-345`) — but the lean
  three-word rule (tag every verdict claim, cite `file:offset` or mark
  unknown, never assert absence) is unadopted. Machine-readable without
  human-readable.
- **G4 — No graded agent-task benchmark.** Evals are real but narrow
  (QASM corpora, sandboxed codeexec, mutation-critic, Lean sandbox,
  HumanEval-136, R6 rule-based transcript rubric recorded-never-enforced).
  No `bench/` dir, no answer-key grading of agent behavior, no tribunal
  cost/quality numbers (DeepSeek-vs-oMLX delta still operator-queued).
  leanrea's template (`tasks.json` keys + `run.py` N×M + published
  caveats) has no kubona equivalent.
- **G5 — Per-tool justification never run.** The 29 were never asked
  leanrea's question (*can the agent already do this cheaper?*), and no
  MCP-side catalog exists (R5 catalog covers the 9 tribunal tools;
  P2-13 generated MCP catalog still open). Low urgency given §0 — hygiene.
- **G6 — Tribunal turn economics unmeasured.** Structure known (4 turns ×
  2 LLM trips, 90 s timeouts, 16k context budget) but no measured
  cost/turn, no p95, no model comparison. "Turns cost more than bytes"
  is unpriced.
- **G7 — Trap knowledge scattered.** Fail-closed arithmetic, cancel
  graph, mandate binding, dispatcher strictness are all encoded in code +
  pinned by tests (see investigation record) — but no consolidated
  agent-facing doc converts them into usable knowledge.

## 2. Improvements and enhancements (beyond the addendum)

- **I1 — ctx-budget probe with teeth.** Adapt `ctx.sh` into a kubona
  probe that measures the live gateway surface bytes per load mode and
  **fails CI past budget** (measured 10 KB today → budget 16 KB).
  Leanness becomes a ratchet, not a vibe.
- **I2 — Capability-scoped skill variants later.** Tribunal roles already
  scope tools per turn; a future skill can ship per-role sections. Not
  in any slice below (no duplication of live enforcement).
- **I3 — Trap list seeded from test names.** Every trap in §G7 has a
  pinning test; the skill's trap section cites `path:line` of the test
  that proves it (durable by construction — dead traps fail lint when
  tests move, if the L1 test resolves anchors).

## 3. Implementation plan (S slices, production bar each)

Conventions for every slice: full logic (validation, refusal, retention
where stateful); negative tests (fail on old code where behavior
changes); live proof where it touches runtime; wiki landed-notes, never
silent edits; rollback stated.

### L1 — `skills/kubona/SKILL.md` (S, first — unblocks nothing, enables everything)
- **Content (knowledge only, zero wrappers):** Rules (work-on-copy +
  state-root hygiene, `rg -uu` over extracted trees, slice-don't-cat,
  batch probes with `---` markers, seen/inferred/unknown tagging);
  Router (fastest path per kubona target: job full-state, mandate
  validity, provider inventory, tribunal transcript, ledger tail);
  First moves (one paste-and-run each — see L4 for what they call);
  Traps (each with the pinning test's `path:line`, per I3); Speed
  patterns (strings/imports first, diff inventories before code —
  translated to ledger/session terms).
- **Tests:** file exists, ≤12 KB size budget (leanness pinned),
  required headings present, every `path:line` anchor resolves into
  the tree (dead-trap lint).
- **Accept:** a fresh agent session reaches a job's full state in ≤3
  tool calls following the skill (measured once, recorded).
- **Rollback:** delete the directory (nothing imports it).
- **Not in slice:** `npx skills add` distribution wiring (paste fallback
  documented; distribution is operator packaging, later).
> ✅ LANDED 2026-10-17: `skills/kubona/SKILL.md` (6827 bytes — Rules,
> Router ×8, 5 verified first-moves, 7 traps with resolving anchors,
> Speed patterns) + `platform/tests/test_skill.py` (existence, 12 KB
> budget, headings, anchor resolution; gates negative-tested). All 5
> first-moves executed live before filing (doctor, JSONL tail,
> tribunal-turn SQL, list_providers, mandate round-trip).

### L2 — `bench/` harness with answer keys (S, needs L1 tasks)

- **Change:** `bench/tasks.json` (hermetic tasks with hand-made regex
  keys: provider inventory, session lifecycle, mandate shape, ledger
  tail); `bench/run.py` (N reps, in-process gateway + crypto + ledger,
  state sandboxed to gitignored `bench/work/`, results JSONL, nonzero
  exit on fail — no model arms: hermetic floors only, keyed comparisons
  stay operator-periodic); `bench/surface.py` (single measurer for
  script + CI, derived from the live gateway registry — never a
  hardcoded tool list); `bench/ctx.sh` (16 KB budget gate);
  `bench/BENCHMARK.md` (method + caveats + first numbers). Tribunal
  structure rides existing Go suites by reference (no duplicated
  harness); `KubonaGateway.mounted_tools()` accessor added (also serves
  L5's catalog).
- **Tests:** 4/4 tasks green ×3 reps; ctx probe fails past budget
  (negative-tested); `platform/tests/test_mcp_surface.py` (budget gate +
  task-file validity).
- **Accept:** first comparison run publishes numbers + caveats (even if
  n=3 — leanrea's honesty standard, not its sample size).
- **Rollback:** delete `bench/` (evals in `platform/tests` untouched).
- **Not in slice:** gating releases on scores (R6 policy stays operator);
  keyed skill/no-skill agent comparison (needs models — queued).
> ✅ LANDED 2026-10-17 (baseline): 4/4 tasks ×3 reps green (all
> sub-millisecond–1 ms, hermetic floors as designed); surface 8720
> bytes vs 16384 budget; Go tribunal floor verified green
> (`TestExecuteTribunal*`, rubric separation, fallback declarations).
> `list_provider_registry` ships a bare payload with no `ok` envelope —
> noted in BENCHMARK.md caveats as follow-up candidate (bench measures,
> does not fix product).

### L3 — Scribe seen/inferred/unknown tagging (S, needs R3 — landed)

- **Change:** scribe prompt instructs verbatim `seen:`/`inferred:`/
  `unknown:` tags with `file:offset`/symbol citations; fallback
  template carries deterministic tags; `SealTurn` already refuses
  untagged coverage maps (no new enforcement — prompt + template only,
  honestly labeled as convention, not gate).
- **Tests:** fallback synthesis carries all three tags; scribe coverage
  still 4/4 facets (existing R3 tests keep passing unmodified).
- **Accept:** recorded tribunal transcripts distinguish quoted evidence
  from inference by reading (reviewed, not run).
- **Rollback:** revert prompt + template (coverage map untouched).

### L4 — Doctor deepening + validity first-moves (S)

- **Change:** `doctor` gains read-only checks within its envelope (no
  network): provider inventory, tool-catalog digest pin match, disk
  thresholds, ledger readability. New `kubona.run mandate-check`
  first-move: offline mandate shape/expiry/binding validation (no
  ledger write, no network — the L1 one-liner calls it).
- **Tests:** each new check has pass + refusal fixtures; unknown
  subcommand still refuses (existing behavior kept).
- **Accept:** L1 first-moves call these and only these.
- **Rollback:** revert `run.py` additions (config checks untouched).

### L5 — Per-tool justification review + MCP catalog (S, last)

- **Change:** verdict table for all 29 tools (keep + named trust
  property, or migrate-to-skill): expected outcome is keep-with-reasons
  for ~25, review for `list_provider_registry` + `compiler` ×3. Generate
  the MCP-side catalog (P2-13 remainder) from live signatures, paired
  with the R5 digest discipline.
- **Tests:** catalog byte-identical to served surface (drift = red);
  review table checked in as the audit record.
- **Accept:** an agent can implement retry-vs-escalate from the catalog
  alone (same bar as R5, reviewed by reading).
- **Rollback:** catalog is additive; no tool removed in this slice
  (removal, if ever, is a separate decided slice per tool).

### Sequencing (corrected per LC3 — baseline before change)

L1 (knowledge, no dependencies) → L2-baseline (record numbers +
caveats) → L3 (tags) → L2-rerun (delta as result) → L4 (needs L1
call-sites; hermetic per LC9) → L5 last (needs the I1 budget to judge
against; removals per LC6 as separate slices). Relation to other plans:
L2 converges with R6 measurement (scores recorded alongside, never as
keys per LC8); L5 closes P2-13; L1 is the week-one form of P2-14's
remainder (skill now, installer only if needed); none touch the trust
boundary (Waves 0–2, R-series untouched by design); distribution gated
per LC1.

### Phased roadmap (entry/exit per phase; each phase pushes independently, CI green throughout)

| Phase | Slices | Entry (gate in) | Exit (gate out) |
|---|---|---|---|
| L-A · Knowledge + baseline | L1, L2-baseline | This page adopted (candidate → adopted) | `skills/kubona/SKILL.md` merged with pin tests green; first bench numbers + caveats published (even n=3) |
| L-B · Convention + checks | L3, L4 | L-A exit | Recorded transcripts carry all three tags by reading; `doctor` deepened with hermetic tests green in CI |
| L-C · Review + catalog | L5 | L-B exit | Verdict table for all 29 tools checked in; MCP catalog generated with digest discipline; P2-13 closed |

Phase gates are verified by reading (bench report) and running (suites),
never by prose. A phase that cannot exit green rolls back to its entry —
slices are additive, so rollback is deletion.

## 4. What NOT to borrow (explicit non-goals)

- Stripping or merging trust-boundary tools to chase a smaller number
  (the tax is ~2k tokens; churn risk exceeds savings).
- REA-parity surfaces (decompilers, providers, bridges) — irrelevant
  (unchanged from the REA plan).
- Reproducing leanrea's benchmark (different domain, n=3, one model) —
  borrow the template and honesty standard, not the numbers.
- Machine-enforcing scribe prose (undecidable; R3's map-level gate is
  the enforcement, L3 is convention labeled as such).

## 5. Open decision points (operator calls)

1. L1 distribution: in-repo `skills/` only, or also `npx skills add`
   packaging?
2. L2 configs: which models × which tribunal shapes for the first
   comparison (keys on oMLX availability)?
3. L5 removals: if the review flags `compiler` ×3, merge into one
   `compile_circuit` or keep three explicit tools?

Conflicts, remediation decisions, and sequenced slices: [[rea-conflicts-and-remediation]].
Skill file (when L1 lands): `skills/kubona/SKILL.md`.

Related: [[leanrea]] · [[rea]] · [[source-leanrea-investigation-2026]] ·
[[integration-topology-and-licence-discipline]] ·
[[provenance-and-reproducibility]] · [[spend-mandates-and-delegation]] ·
[[evaluation-and-benchmarks]] · [[claim-credibility-wavefunctions]] ·
[[cloudroom-adoption-plan]] · [[remaining-work]] · [[risks]]
