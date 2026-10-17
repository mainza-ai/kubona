---
type: concept
name: REA adoption plan
date: 2026-10-17
updated: 2026-10-17
tags: [plan, rea, prior-art, receipts, contracts, tribunal, production]
sources: [source-frontier-repo-audit-2026]
status: candidate
---

# Kubona × REA: updated improvement report + implementation plan

> Production stance: every slice below specifies full logic — concurrency,
> restart, validation, retention, and refusal paths — so nothing lands as a
> stub that must be redone. This page **updates and supersedes** the
> 2026-10-16 cross-repo investigation (uncommitted scratch; its durable
> content is re-verified here, not copied). Method: REA read live at
> `morluto/rea` @ `e567033` (2026-10-17 — `docs/mcp-contracts.md`,
> `docs/architecture.mermaid`, repo metadata); Kubona read at
> `mainza-ai/kubona` @ `433e7d5` (develop — code graph, `ci.yml`,
> `pyproject.toml`, wiki record). REA internals taken as documented
> (its `docs:check` gate is the basis for doc/code parity); Kubona claims
> verified against code or the wiki's own landed-notes. Nothing below is
> started.

## 0. Thesis (unchanged, sharpened)

REA's excellence is platform mechanics, not domain — and Kubona has since
*converged* on several of those mechanics independently (S1 idempotency,
S5 validated bundles, S6 closed taxonomy, disk/storage gating). The
remaining gap is narrower and more valuable than in October: **contract
discipline for the features Kubona already has** — canonical receipt
identity across the two ledgers, machine-enforced claim closure, coverage
declarations, provenance-marked inputs, typed errors, and a CI that stops
flaking. No new features are proposed here that Kubona hasn't already
decided it wants; several prior proposals are now closed or reduced.

## 1. Prior proposals, re-verdict against S1–S8 state

| # | Prior proposal | Verdict today | Evidence |
|---|---|---|---|
| P0-1 | Canonical cross-ledger evidence envelope | **OPEN, still highest leverage** | Two ledgers remain (`platform/core/pkg/ledger/ledger.go` SQLite WAL + Ed25519; `platform/kubona/provenance/ledger.py` hash-chained). S5's manifest digests + I1 `CanonicalHash` are envelope *thinking*, not cross-language equivalence — no shared test vectors exist. |
| P0-2 | Obligation ledgers for claim/mandate closure | **OPEN** | Claim records carry amplitudes; nothing machine-enforces closure. No `obligation` construct in either tree (grep clean). |
| P0-3 | Coverage discipline ("unknown, not absence") | **OPEN** | No `facet_availability`/coverage declarations on verdicts (grep clean). The Disconfirmation Officer can still certify unobserved as disproven. |
| P1-4 | Consolidate two agent subsystems | **OPEN, still biggest refactor** | Go `platform/core/pkg/agents/` + Python `platform/kubona/agents/` (`collective.py`, `loop.py`, `registry.py`) both live. Defer until R-series contracts below stabilize it. |
| P1-5 | Provenance-mark tribunal inputs | **OPEN** | `ObservedCall{name, ok, error}` has no origin/provenance field (no `untrusted_content` marker anywhere in Go/Python trees). |
| P1-6 | Validated session bundles | **SUBSTANTIALLY CLOSED by S5** | Manifest v1 (per-section digests, validation-before-replay, tamper→400 proven live). Remainder: tribunal-*turn* portability across agents (turns are ledger events — exportable, but no turn-scoped bundle + resume flow exists). Reduced to R4. |
| P1-7 | Detail/summary projection | **OPEN** | No `detail`/`summary` projection, no continuation pattern; tribunal context is unbounded per turn (90 s timeouts price it). |
| P1-8 | No-silent-fallback discipline | **PARTIALLY CLOSED** | Fidelity gate *refuses* mitigate/downgrade verdicts with reason (`admission.go:136`, `execution.py:46,140`) — refusal, not silent swap. Remainder: QPU→simulator tier substitutions need the same explicit ledger-recorded treatment where they still occur. |
| P1-9 | Residual-unknowns audit | **OPEN** | No unknowns ledger, no blocking-unknowns construct. Ties to P0-2. |
| P2-10 | CI overhaul | **PARTIALLY DONE** | Qiskit pinned (`==2.5.2/aer/runtime`, `pyproject.toml:31`) ✓. Still open, re-verified today: pnpm-only caching (no pip/Go/cargo cache), no `needs:` DAG, mega `python-tests` job (Rust build + suite + doctor + wiki-lint in one), wiki-lint buried at step 55, tag-pinned (`v4`/`v5`) not SHA-pinned actions. |
| P2-11 | Transcript scoring | **OPEN** | No session-transcript scoring; tribunal quality unevaluated and model-dependent. |
| P2-12 | Typed error contract | **OPEN** | No `isError` catalog (grep clean across Go/Python). Failures are fail-closed but machine-illegible across MCP/REST/gRPC/CLI. |
| P2-13 | Generated MCP catalog | **OPEN** | 5 servers (`compiler`, `control`, `cost`, `execution`, `provenance` + gateway registry) hand-maintained; no catalog digest (grep clean). |
| P2-14 | Plan-preview onboarding | **PARTIALLY CLOSED** | Workflows `dry_run=True` landed (trigger discipline slice) — plan-then-approve exists for workflows. Remainder: setup/doctor trust pattern for any future agent-config editing. |
| P2-15 | Anti-mock lane | **PARTIALLY CLOSED** | [[quantum-advantage-realism]] settled (backend recorded, anchor required); simulator-vs-hardware distinction in receipts still absent — keep the receipt-level distinction open. |

## 2. New findings from this investigation (not in the prior report)

- **N1 — Kubona now has its own contract precedent: extend it, don't restart.**
  S6's enforced `KnownEventTypes` taxonomy (~60 rows, all three append chokes,
  conformance test) is the same *shape* as REA's closed vocabularies — and it
  caught real misses on landing. The R-series below builds on this proven
  pattern (payload contracts per type, cross-language vectors) rather than
  importing a foreign schema.
- **N2 — S1 keys already speak REA's idempotency dialect.** Same-ID replay,
  same-ID-divergent-content 409, ghost-before-store. R1 (below) only adds
  the *identity envelope* around them, not new mechanics.
- **N3 — The two-ledger split now has a seam to exploit.** S5 manifests
  already normalize both worlds' rows into one digest-pinned document. The
  canonical envelope (R1) can be defined *as* the manifest-section shape
  plus identity fields — migration, not greenfield.
- **N4 — CI's flake surface is unchanged since October.** Same structure
  ( Mega python job, no cache, no DAG) re-verified in today's `ci.yml`.
  With S1–S8 cadence (a landing every day), flakes tax every slice — R6 is
  positioned first for that reason.
- **N5 — Provider-fallback honesty is half-done.** Refusal paths are
  explicit; *successful* tier substitutions still lack the typed,
  ledger-recorded event REA requires. Small slice, high reproducibility
  value.

## 3. Production implementation plan

Conventions for every slice: full logic (concurrency, restart, validation,
retention, refusal); negative tests (fail on old code); race suites where
concurrent; live both-directions proof on `:8080`; wiki Correction/landed
notes, never silent edits; rollback stated.

### R0 — CI cache + DAG + lane split (S, first — unblocks everything)

> ✅ LANDED 2026-10-17: `wiki-lint` is its own fail-fast job (was buried
> behind the Python suite); `secret-scan` + `wiki-lint` gate every heavy
> job via `needs:`; `canary-gate` needs all suites (a red suite can no
> longer coexist with a green gate); cargo cache enabled for the
> Python-job qir-core build; actions were already SHA-pinned (W2.4) and
> pip/Go/pnpm caches already present. `CONTRIBUTING.md` defines the
> named lanes (`test:fast` / `test:changed` / `test:boundary`, required
> before commit / code-touch / push). Operator default recorded: lanes
> `test:fast` + `test:boundary` required, `test:changed` advisory
> (necessary, not sufficient). Job IDs unchanged (branch protection
> safe). Accept criterion (3 consecutive green runs, no network flake)
> is pending observation — this push is run #1 of 3.

- **Threat:** every run is a full network fetch; flakes tax each landing.
- **Change:** pip/Go/cargo/pnpm cache; split mega `python-tests` job
  (build / test / lint); wiki-lint first for fail-fast; `needs:` fan-in so
  the release gate actually gates; SHA-pin actions; named lanes
  (`test:fast`, `test:changed`, `test:boundary`) in CONTRIBUTING.
- **Tests:** cache-hit rate visible in logs; lane mapping documented.
- **Accept:** 3 consecutive green runs with no network-flake failure.
- **Rollback:** revert workflow file (no code touched).

### R1 — Canonical evidence envelope, both ledgers (M, highest leverage)

> ✅ LANDED 2026-10-17 (`8a9e9a5` + `3404976`): `EvidenceEnvelope` in the
> IDL + sealed canonical-JSON bytes (sorted keys, compact, UTF-8, finite
> floats; `ev_` + sha256). Operator default recorded: REA's four fields
> + `tenant_id` mandatory and optional `mandate_ref` from day one.
> `platform/contracts/evidence-vectors.json` (3 fixtures) reproduced
> byte-exactly by both rails (Go `pkg/ledger/envelope.go`, Python
> `kubona/provenance/envelope.py`) — agreement across the file IS the
> cross-language proof. Missing profile/method/tenant/result refuse;
> NaN/Inf refuse; tamper refuses. S5 manifests seal all 9 sections as
> envelopes at export; import verifies the set (forged/swapped/partial
> refuse; pre-envelope manifests still import on digests).
> Deviations: Python `kubona_pb2` NOT regenerated — `/tmp/protogen`
> drifted to grpcio-tools 1.84 (stamps 7.x gencode the 5.29.6 runtime
> refuses); Python gencode reverted per the regen script's own stop
> rule. Operator action: re-pin the scratch venv to grpcio-tools 1.71.x
> and re-run `./scripts/regen-proto.sh` (Go stubs already carry the
> message; envelope sealing needs no stubs).

- **Threat:** two receipt formats for one "sealed ledger" story — an
  auditor cannot verify what has two canonical forms.
- **Change:** define once (protobuf IDL, the existing IDL home):
  `evidence_id` (`ev_` + 64 hex, content-addressed), `normalized_result`,
  nullable provider-native `raw_result`, mandatory `analysis_profile`
  (missing ⇒ rejected, REA rule). Implement in Go + Python against the
  SAME checked-in test vectors; cross-language equivalence assertion in
  CI. S5 manifest sections adopt the envelope (migration path, N3).
- **Tests:** vector suite both languages; equivalence CI job; omission of
  `analysis_profile` refused both sides.
- **Accept:** one fixture verified byte-identical from both ledgers.
- **Rollback:** envelope additive alongside existing rows (no migration).

### R2 — Obligation ledger for claim + mandate closure (M, needs R1)

> ✅ LANDED 2026-10-17, tribunal half (`dfeb2cc`): `obligations` +
> `obligation_cases` tables (idempotent open on subject, set-once owner,
> upsert cases, closed-history immutable, residual unknowns with IDs);
> `AttemptClose` enforces owner + all-required-pass + no-contradiction
> anywhere + required authorities + closed deps + zero unknowns +
> non-empty; tribunal runs open one obligation each (run-unique subject),
> seal turn-N cases with authored pass/fail, record `deliberation` fail
> on tool contest and `classical-anchor` unknown on fallback anchors;
> the session owns, closes, and reads the disposition AS the verdict
> (claim evidence + response carry the obligation id). Operator default
> recorded: tribunal quorum = the four seated roles. Suite: store (4) +
> lifecycle (2, every refusal) + collective C3 (1) + HTTP (1) green.
> Sequenced follow-up R2b (NOT hidden): mandate issuance opening
> obligations with settlement cases needs a `job.mandate_ref` column +
> admission threading + settled-hook chain — the linkage does not exist
> yet, and opening unclosable obligations would be theater.

- **Threat:** vocabulary without enforcement — claims close on prose.
- **Change:** obligations (ID, version, required case kinds incl.
  negative/malformed/cancellation, required authorities, dependencies,
  residual unknowns, disposition). Close requires unique owner + every
  case + passing verifier of comparable authority; contradictions,
  duplicate owners, or residual unknowns keep it open; empty input ⇒
  `unknown`, never closure. Tribunal verdicts + mandate completions ride
  it. Unknowns get IDs and can block closure (absorbs P1-9).
- **Tests:** each refusal reason (contradiction, dup owner, residual,
  weak authority, empty) refused; happy-path closes.
- **Accept:** no claim reaches `closed` without verifier authorities on
  record — asserted by query, not by reading code.
- **Rollback:** obligations additive; verdicts keep current path until cutover.

### R3 — Coverage declarations + provenance-marked inputs (M)

> ✅ LANDED 2026-10-17 (`5a4a6ce`): every `ObservedCall` carries
> `source` + `untrusted` (all tool outputs, success AND error) and the
> history bytes embed the tag inside the observation budget (data
> truncated to leave room — provenance can never push past the cap);
> injection fixture proves the directive is preserved-as-data and never
> re-dispatched. Every `TribunalTurn` carries per-facet coverage
> (`ansatz`/`anchor`/`audit`/`proposal`; `available:false` + reason =
> unobserved, distinct from fail = disproven — the fallback run's turn-2
> anchor facet demonstrates it on record); `SealTurn` refuses
> coverage-less turns and facet-incomplete scribes; `tool_called` audit
> mirrors the provenance fields. Suite: agents green (4 new tests).

- **Threat:** false negatives certified as receipts (P0-3); prompt
  injection via tool results (P1-5).
- **Change:** verdicts carry per-facet `{available: true}` /
  `{available: false, reason}`; Scribe + ledger reject undeclared
  verdicts. Every tribunal observation carries origin provenance
  (`untrusted_content`-equivalent flag + source) on success AND error;
  roles weight by source. One slice: both are "declare what you saw".
- **Tests:** undeclared verdict refused; injected directive in a marked
  observation reported, never followed (fixture).
- **Accept:** Disconfirmation Officer output distinguishes disproven from
  unobserved in every recorded verdict.

### R4 — Bounded tribunal context: projection + continuation (M)

- **Threat:** context bloat is the tribunal's biggest cost/quality risk
  (90 s timeouts price it).
- **Change:** full evidence server-side; per-turn bounded views with
  `parent_evidence_id` + links; stable offset/limit paging; oversized
  results return a ready-to-call continuation, never truncation; pinned
  response budget returning `resource_constraint` pre-construction.
  (S8a's BudgetHistory pressure readout becomes the client-side gauge.)
- **Tests:** paging stable across growth; continuation round-trips
  losslessly; budget breach returns the code, not a clipped string.
- **Accept:** p95 turn context bytes under the pinned budget in soak.

### R5 — Typed errors + generated catalog (M)

- **Threat:** machine-illegible failures (agents can't choose
  retry-vs-escalate); hand-maintained tool surface skews silently.
- **Change:** canonical `{code, category, message, retryable,
  remediation, details}` on MCP/REST/gRPC/CLI + generated error catalog
  (CSV, CI-checked); contracted CLI exit statuses
  (`0` incl. partial / `1` incomplete / `128+N` signals); generated
  tool catalog with digest (`expected_catalog_digest` pinning);
  `docs:check`-equivalent gate (wiki-lint pattern exists — extend to
  generated artifacts).
- **Tests:** catalog drift fails CI; every code in the catalog has a
  fixture; exit-status contract test.
- **Accept:** an agent can implement retry-vs-escalate from the catalog
  alone (reviewed by reading, not by running).

### R6 — Transcript scoring pilot (S, alongside R2)

- **Threat:** tribunal quality unevaluated, model-dependent.
- **Change:** per-role rubric over recorded transcripts (falsification
  attempted? canary checked? anchor computed?) + scenario evidence
  review; scores recorded, not enforced — measurement before gating.
- **Tests:** rubric fixtures (good/bad transcripts score apart).
- **Accept:** DeepSeek-vs-oMLX quality delta stated with numbers.

### Sequencing

R0 (unblocks everything) → R1 (receipts become real) → R2 + R3 (closure
becomes enforceable) → R4 + R5 (cost + legibility) → R6 alongside.
P1-4 subsystem consolidation stays queued behind R1–R3 (contracts first,
refactor second — unchanged from October). I2/I3/I5 follow-ups from the
Cloudroom plan are unaffected.

## 4. What NOT to borrow (updated)

- REA's domain machinery (decompilers, providers, bridges) — irrelevant
  (unchanged).
- REA's scale of process (release-please, 19-language i18n, mutation
  testing) — still premature for one maintainer (unchanged).
- Hopper/Ghidra/IDA provider stack — no Kubona analogue; the *pattern*
  (immutable provider binding, no silent replacement) transfers, the
  code does not.
- The trap stands: claims in wiki prose ahead of implementation. Kubona
  now has two `docs:check`-shaped gates (wiki-lint, catalog proposal in
  R5) — use them.

## 5. Open decision points (operator calls)

1. R1 envelope field set: adopt REA's four fields verbatim, or extend
   with Kubona's mandate/tenant binding from day one?
2. R2 authority model: which verifiers count as "comparable authority"
   for claim closure (tribunal quorum? Lean proof? human sign-off)?
3. R0 lanes: which lanes are required vs advisory for a one-maintainer
   repo?
4. P1-4 timing: still quarter-scale, or do R1–R3 make it sooner?

Conflicts, remediation decisions, and sequenced slices: [[rea-conflicts-and-remediation]].

Related: [[rea]] · [[integration-topology-and-licence-discipline]] ·
[[provenance-and-reproducibility]] · [[spend-mandates-and-delegation]] ·
[[multi-agent-research-collective]] · [[claim-credibility-wavefunctions]] ·
[[evaluation-and-benchmarks]] · [[fidelity-accounting]] ·
[[cloudroom-adoption-plan]] · [[implementation-gaps]] · [[risks]]
