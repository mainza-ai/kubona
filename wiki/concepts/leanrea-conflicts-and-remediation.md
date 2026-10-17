---
type: concept
name: leanrea conflicts and remediation
date: 2026-10-17
updated: 2026-10-17
tags: [plan, leanrea, conflicts, remediation, production, skill, benchmark]
sources: [source-leanrea-investigation-2026]
status: candidate
---

# Conflicts vs the lean plan — investigation, remediation, implementation

> Every question the leanrea adoption raises, investigated against the
> tree with evidence, then remediated by decision (not deferral) or
> recorded as a sequenced constraint. Precedent and format follow
> [[rea-conflicts-and-remediation]]: `LC` numbers, file:line evidence,
> decided remediations, slices with tests. Nothing here is started.

## LC1. Skill publication vs Q-25 private posture (blocks distribution, not writing)

**Evidence.** Q-25 is closed: the repo stays private while the blueprint
is designed, because a public repo without settled direction invites
contributions the project cannot direct. Skills-sh/`npx` distribution
publishes operational knowledge to the public internet; the skill
documents traps, router paths, and working methods.
**Impact.** A public skill advertises the project before the
contributor-legible gate, and its traps section could expose unsettled
design directions (not code, not secrets — direction).
**Remediation (decided):** L1 writes the skill **in-repo only**
(`skills/kubona/SKILL.md`); any distribution beyond the repo waits on
the public flip — the same gate as code, no separate track. The skill
documents public surfaces only (CLI, MCP tools, ledger reads), so its
content is flip-safe by construction when the day comes. Interim
external sharing is paste-fallback on explicit operator order only.
**Slice L1:** unchanged (write + pin tests); distribution explicitly
out (already §4-adjacent — now a decided gate, not an omission).

## LC2. Skill staleness vs agent trust (shapes L1 tests)

**Evidence.** The skill documents commands and behaviors that evolve;
nothing regenerates it. Stale skill misroutes agents into wasted turns.
**Impact.** Cost only, never safety: every trust property is enforced
in code (mandates, CAS, allowlists, SealTurn refusals) regardless of
what any doc claims. Worst case is priced turns, not forged spend.
**Remediation (decided):** staleness is a cost bug with a mechanical
catch — L1 tests resolve every `path:line` anchor into the tree and pin
the ≤12 KB budget; the skill carries its version/date; skill updates
ride in the same commit as the behavior they document (review
checklist, enforced by reviewers not tooling — stated honestly).
Blast radius labeled cost-only in the skill header so no reader
mistakes guidance for guarantee.

## LC3. L3 prompt change vs L2 baseline (ordering correction)

**Evidence.** L3 alters scribe prompts; L2 measures tribunal behavior
including scribe output quality. The plan page currently sequences
"L2 + L3 in either order."
**Impact.** Measuring after changing confounds the baseline: the delta
this program exists to produce (prompt-convention effect) becomes
unreadable.
**Remediation (decided):** correct the sequence — **L2-baseline first**
(record numbers + caveats), then L3, then **L2-rerun** (the delta is a
result, not an afterthought). Plan page sequencing updated accordingly.
**Slice order:** L1 → L2-baseline → L3 → L2-rerun → L4 → L5.

## LC4. Bench LLM spend on CI (shapes L2 runtime)

**Evidence.** CI has no model keys (`platform/scripts/ci.sh` step 5 runs
`doctor`, suites run hermetic; tribunal tests use the httptest stub
client `platform/core/pkg/agents/collective_test.go:194`
`stubTribunalClient`, deterministic, zero spend).
**Impact.** A bench requiring live models cannot run per-PR: flaky,
priced, keyed CI.
**Remediation (decided):** bench is hermetic by default — stub clients,
recorded fixtures, regex keys (no LLM in the grading path, ever).
Keyed-model comparisons (the DeepSeek-vs-oMLX delta) run local or
periodic only, results committed as reports with caveats. CI runs keys
grading + the ctx-budget probe only.

## LC5. mandate-check offline vs admission confusion (shapes L4 output)

**Evidence.** Offline validation covers shape/expiry/binding/cover
(`platform/kubona/cost/mandate.py:104-130`, pure crypto, no ledger);
admission is a separate ledger event (`mandate_verified_client` receipt
→ `entry_hash` cited as `mandate_ref`).
**Impact.** An agent reading bare "valid" dispatches unadmitted spend —
refused safely at admission, but a wasted round-trip every time.
**Remediation (decided):** output wording contract — `shape_valid` vs
`admitted` as separate fields, never a bare "valid"; skill documents
the two-step (check, then verify-and-cite). Test asserts both fields
present on every path.

## LC6. Tool removal breakage (shapes L5)

**Evidence.** Console, gateway tests, and agents depend on tool names;
the L5 review may recommend merging `compiler` ×3. MCP has no
versioning story and external consumers don't exist pre-public — but
first-party breakage is still breakage.
**Impact.** Silent client breakage disguised as hygiene.
**Remediation (decided):** removal is a separate decided slice per tool
(already L5 rule); catalog digest forces explicit regen (drift = red);
console/gateway e2e must cover every mounted tool before any removal
lands. Merges keep aliases for one release where cheap.

## LC7. Bench key staleness (shapes L2 maintenance)

**Evidence.** Hand-made keys rot as behavior intentionally changes (new
verdict text, new refusal wording).
**Impact.** False reds erode bench trust until keys are ignored — then
the bench is theater.
**Remediation (decided):** keys live next to the behavior's own tests;
key-update discipline mirrors the catalog digest (explicit regen
command, reviewed diff, never silent). Stale-key failure messages name
the update procedure. Bench red without a behavior change is treated as
a bench bug, not a product bug, until proven otherwise.

## LC8. R6 score/bench circularity (constrains L2 grading)

**Evidence.** L2 could grade runs with R6 rubric scores while R6 itself
is validated by separation on fixtures.
**Impact.** Self-grading loop proves nothing (scores validate scores).
**Remediation (decided):** L2 keys are hand-made and independent of
scores (leanrea pattern: keys from the task author, not the system
under test). R6 scores may be RECORDED alongside bench runs for later
correlation studies, never used as grading keys.

## LC9. Doctor deepening vs CI hermeticity (constrains L4)

**Evidence.** `ci.sh` step 5 runs `doctor`; Wave-1 taught the TMPDIR
lesson (macOS-only assertion red on Linux CI until hermetic fix).
**Impact.** Any check touching network, processes, or machine paths
flakes CI.
**Remediation (decided):** L4 checks restricted to hermetic read-only
(config files, local registry, digest match, ledger readability under
`KUBONA_STATE_DIR` override). Anything needing network/process state
is refused from doctor by documented envelope, not added hopefully.
Each check tested with `TMPDIR=/tmp` semantics.

## LC10. Precedence: R3 gate vs L3 convention (clarification, no code)

**Evidence.** Two layers will coexist: `CoverageDecl` map enforced at
`SealTurn` (refusal, hard) and scribe prose tags (auditable, soft).
**Impact.** A contributor implements tags believing they're enforced,
or relies on tags where the map is the enforcement.
**Remediation (decided):** precedence stated in code comments + skill —
map gate refuses (hard), tags are reviewed convention (soft); tests pin
the hard layer only. No new enforcement (undecidable on LLM prose —
stated, not dodged).

## Explicit non-conflicts (checked, no action)

- **Trap documentation aids probing.** Investigated, no conflict:
  enforcement is cryptographic/structural (Ed25519, CAS, allowlists,
  closed taxonomies); exact refusal rules are Kerckhoffs-safe to
  publish. Recorded here so it is never re-litigated.
- **Skill language.** English only; console/docs i18n sits in the
  deferred batch (Bemba/Typst). No action.
- **Skill vs console surfaces.** No overlap by construction: skill is
  operator/agent ops knowledge, console is human UI; the skill never
  documents click-paths. S8's theater decision untouched.
- **"Integration" topology.** Reference class only — no vendoring, no
  submodule, no dependency, no copied code (same rule as REA, structural
  not per-licence; leanrea's MIT wouldn't conflict anyway).
- **questions.md untouched.** No open-question row is closed or opened
  by this investigation (method narrowed, insight unchanged); the
  execution queue lives in [[remaining-work]], not the work-list.
- **overview.md untouched.** The overall picture is unchanged (MCP-native
  + contracts stand; the news is quantitative and lives here).

## Sequencing with L1–L5 (corrected per LC3)

L1 (knowledge, no dependencies) → L2-baseline (record numbers +
caveats) → L3 (tags) → L2-rerun (delta as result) → L4 (needs L1
call-sites; hermetic per LC9) → L5 last (needs the I1 budget to judge
against; removals per LC6 as separate slices). R6 scores ride along
recorded-only per LC8. Distribution gated per LC1. Every phase pushes
independently with CI green throughout.

Related: [[leanrea-adoption-plan]] · [[leanrea]] ·
[[source-leanrea-investigation-2026]] · [[rea-conflicts-and-remediation]] ·
[[integration-topology-and-licence-discipline]] · [[remaining-work]] ·
[[evaluation-and-benchmarks]]
