---
type: concept
name: REA conflicts and remediation
date: 2026-10-17
updated: 2026-10-17
tags: [plan, rea, conflicts, remediation, production, taxonomy]
sources: [source-frontier-repo-audit-2026]
status: candidate
---

# Conflicts vs the REA plan — investigation, remediation, implementation

> Follow-up to [[rea-adoption-plan]]: while scoping R0–R6, six concrete
> conflicts surfaced between the plan and the code, between the two rails,
> and between wiki claims and the tree. Each has file:line evidence, a
> remediation decision, and a production implementation slice. Nothing
> below is started. Kubona read at @ `433e7d5`; REA at `e567033`.

## C1. Cross-language taxonomy split (blocks R1)

**Evidence.** Go enforces a closed taxonomy at all three append chokes
(`platform/core/pkg/ledger/events.go:15`, `IsKnownEventType`,
`pkg/ledger/ledger.go` choke checks). Python accepts any `kind` string
(`platform/kubona/provenance/ledger.py:105`
`def append(self, kind: str, ...)` — no validation). Observed Python kinds
with no Go counterpart: `canary`, `comparator_live_kernel_verification`,
`comparator_proof_verification`, `mutation_radius`, `paid_job`,
`workflow_step`, `workflow_dry_run`.
**Impact.** R1's cross-language receipt equivalence is unstateable while
one rail is untyped; audit joins across rails break on spelling drift.
**Remediation (decided):** shared taxonomy file both rails validate
against — `platform/contracts/event-types.json` (single source; Go
`go:embed`s it, Python loads it at import). Python `append` refuses
unknown kinds like the Go chokes. Cross-rail conformance test asserts the
two taxonomies are identical.
**Slice R1a (S, first — R1 dependency):**
> ✅ LANDED 2026-10-17 (`7fc6a7a`): `platform/contracts/event-types.json`
> v1 (114 rows: 73 go-emitter + 41 python-emitter, one shared
> `job_cancelled` row). Go keeps `events.go` as the embedded mirror with
> `TestEventTypesMatchContract` asserting set-identity (drift = red);
> Python loads the file at import and `append` refuses unknown kinds
> (fail-closed at the MCP `write_record` tool boundary too).
> Landing the closed rail caught **8 genuinely unregistered production
> emitters** (`comparator_*` ×2, `paid_job`, `mutation_radius`,
> `intelligence-tag`, `promethium-anchor`, `vqe_step`, `workflow_result`)
> — the exact audit-join breakage class C1 predicted. Test-only `"t"`
> repointed to `trial`, not registered. Suite: Go green, Python 640
> passed (4 new taxonomy tests), live typo refusal proven.
- Files: `platform/contracts/event-types.json` (new),
  `pkg/ledger/events.go` (load-or-generate from it),
  `platform/kubona/provenance/ledger.py` (validate in `append`),
  conformance test both rails.
- Tests: unknown kind refused both rails; taxonomy files byte-identical
  in CI (drift = red); every currently emitted Python kind registered
  with a payload contract.
- Accept: `workflow_step` and friends resolve to registered rows; no
  unregistered kind emitted in full-suite run.
- Rollback: revert validation (registry stays advisory).

## C2. S5 migration misses the Python rail (blocks R1, limits S5)

**Evidence.** S5 exports Go tables only. Python keeps tenant-scoped sidecar
files (`ledger.<tenant>.jsonl` via `ledger_path()`,
`platform/kubona/provenance/ledger.py:84-93`) holding `workflow_step` /
`workflow_dry_run` records — none of which enter the Go manifest.
**Impact.** A migrated lab silently loses its workflow history; the
"single record" activation attests an incomplete migration.
**Remediation (decided):** S5b follow-up slice — export a ninth manifest
section (`python_ledger`: per-tenant digest-pinned record lists, same
64 MB total cap, same validation-before-replay). Until then, the S5 wiki
row carries the residual explicitly (added below).
**Slice S5b (M, after R1a — needs the shared taxonomy for kind checks):**
> ✅ LANDED 2026-10-17 (`671200d`): `python_ledger` ninth section —
> export sweeps the tenant sidecar (`KUBONA_LEDGER`-mirrored resolution,
> `ledger.<tenant>.jsonl`) for the session's rows, digest-pinned +
> enveloped like the rest; import replays with rewritten chain links
> (`prev` := tail, `record_hash` := sha256(prev:content), the exact
> Python formula) under Python's own `.lock`, deduped by content_hash
> (double-activate replays). Layering decided: kind-registry enforced
> at the Python append path (R1a), Go verifies structure + digest-pin
> (malformed lines refuse, never dropped). Strict 10-row digest roster:
> stripped/unknown rows refuse (an undetectable history-drop is worse
> than refusing a day-old manifest — S5 is one day old). Python
> companion test proves the replay algorithm yields `validate_chain`
> -clean files. `omitempty` deliberately absent on the new field (an
> omitted-then-nil `[]` digests as `null`, failing the HTTP round-trip
> — caught by the API round-trip test).
> - Files: `pkg/session/pyrail.go` (new), `pkg/session/migrate.go` (ninth section + roster + replay), `pkg/session/migrate_test.go`, `pkg/session/pyrail_test.go` (real-Python-written fixtures), `platform/tests/test_s5b_sidecar.py`.
> - Tests: filter/round-trip/idempotent-replay/tamper/stripped/unknown-row/malformed/empty-tenant-file; negative automatic (new digest row required).
> - Accept: export → fresh DB import → both rails' chains verify.
> - Rollback: drop the section (manifest v1 readers ignore unknown
>   sections — version gate already handles this).
- Files: `pkg/session/migrate.go` (ninth section + cap accounting),
  Python-side export helper, round-trip + tamper tests.
- Tests: workflow records survive migration with digests intact; tampered
  sidecar section 400s.
- Accept: export → fresh DB import → both rails' chains verify.
- Rollback: drop the section (manifest v1 readers ignore unknown
  sections — version gate already handles this).

## C3. Tribunal turn persistence is caller-dependent (weakens R2/R6)

**Evidence.** Turns accumulate in memory
(`platform/core/pkg/agents/collective.go:332,368,414,449`
`result.Turns = append(...)`); only the HTTP handler seals them
(`platform/core/pkg/api/server.go:2130` `tribunal_turn`). Direct collective
callers (tests, future adapters) leave no turn records — so obligation
closure (R2) and transcript scoring (R6) would reason over incomplete
histories whenever the tribunal isn't driven via HTTP.
**Remediation (decided):** move sealing into the collective behind an
injected ledger handle (nil = current in-memory behavior, so unit tests
without a ledger keep passing); the HTTP handler stops double-sealing
(dedupe by turn index — turns already carry indices).
**Slice (M, with R2):**
> ✅ LANDED 2026-10-17 (`dfeb2cc`, with R2): sealing moved into
> `AgentCollective.SealTurn` behind the injected `Ledger` handle
> (nil = pure in-memory, sessionless callers unaffected); the HTTP
> handler seals nothing (AddMessage only). Dedupe by turn index per
> obligation via the turn-N case keys (persisted — restart-safe, no
> in-memory sets on the singleton). Proven: direct run seals 4,
> HTTP run seals exactly 4 (no doubles), second run 4+4 under distinct
> obligations. New runs use `ExecuteTribunalWithSession`; the old entry
> points delegate sessionless.
- Files: `pkg/agents/collective.go` (ledger port + seal-on-turn),
  `pkg/api/server.go` (remove handler-side sealing), turn-level tests.
- Tests: direct-collective run seals 4 turn records; HTTP run seals
  exactly 4 (no doubles); nil-ledger run unaffected.
- Accept: `tribunal_turn` count == turn count for every tribunal test.
- Rollback: revert to handler-side sealing (single call site).

## C4. `proposal_generated` taxonomy row with no producer (hygiene)

**Evidence.** Row exists (`events.go:65`) but the only emitter is
`pkg/ledger/ledger_test.go:71`. Every other row has a production writer
(verified: `grant_issued` → `pkg/api/grants.go:63`, read back at `:143`).
**Remediation (decided):** wire the real producer — emit
`proposal_generated` in the tribunal handler where a proposal is produced
(beside the existing claim mint; same payload shape as the claim's
evidence). Deleting the row instead would waste a true concept.
**Slice (S, with R6):** one emit + assertion that proposal-bearing
tribunals always seal it.
> ✅ LANDED 2026-10-17 (`217b062`, with R6): `proposal_generated`
> emitted beside the claim mint (claim_record_id + obligation_id +
> has_proposal + consensus — the claim's evidence shape); every
> claim-bearing tribunal response has a matching event (HTTP test
> asserts the pairing, including obligation agreement).
- Accept: every `claim_record_id`-bearing tribunal response has a
  matching `proposal_generated` event in the session stream.

## C5. I5 capabilities vs R5 catalog: two discovery mechanisms (overlap)

**Evidence.** Cloudroom plan I5 proposes runtime `/api/capabilities`
(flags, thresholds, catalog); R5 proposes build-time generated tool
catalog with digest pinning. Unscoped, these become two discovery
mechanisms that rot apart (the exact failure class both plans fight).
**Remediation (decided):** boundary, not merger — catalog = build-time
surface (what *can* be called, digest-pinned); capabilities = runtime
state (what is *currently* available: flags, thresholds, harness
catalog). Capabilities embeds the catalog digest; skew between embedded
and generated digests 500s the endpoint (fail-closed discovery).
**Slice (M, with R5):** single endpoint serving both halves from the two
single sources; skew test.
> ✅ LANDED 2026-10-17 (`1c4993e`): `GET /api/capabilities` serves the
> live tool catalog (9 tools, digest-pinned) + runtime state (disk
> trip points with `guarded` flag, harness ids, turn budget, reviewer/
> bridge flags). Catalog generated by `TestToolCatalogMatchesCheckedIn
> -update` (checked-in JSON + embedded digest const — algorithm shared
> via `tools.CatalogJSON`, so only staleness can disagree); live-vs-
> embedded skew 500s typed `internal`; checked-in hand-edits fail the
> digest self-check. I5 delivered by the same endpoint (console
> degrades from one shape instead of probing).
- Accept: console degrades from one endpoint; digest mismatch pages the
  operator instead of serving stale tools.

## C6. Route-count drift 71 → 76 (wiki hygiene, no code change)

> ✅ FIXED 2026-10-17 ([[overview]]): 71 → 76 with the recount command
> recorded in the note (`rg -o 'HandleFunc\("[^"]*"' … | sort -u | wc -l`
> = 76 distinct paths). The "unaccounted +1" resolved: exactly 76
> distinct paths, no remainder.

**Evidence.** `rg HandleFunc → 76 distinct paths
(`platform/core/pkg/api/server.go`); [[overview]] correction block says
71. New since: `sessions/context|export|import`, `vault/leases/revoke`,
`jobs/{id}/cancel` (+1 unaccounted — likely an S8 route; recount on fix).
**Remediation (decided):** correct the number with a recount command in
the note (no lint change — counts are prose, and prose rots; the command
is the durable part).

## Explicit non-conflicts (checked, no action)

- **Python ledger locking:** `fcntl.flock` sidecar already present
  (`ledger.py:110-117`, fixed 2026-10-12) — no cross-process fork class.
- **Steer N/A, tools-hub reduction, chat-sleep theater:** documented
  reductions in the Cloudroom plan, not conflicts.
- **Q-27 (operator-gated history):** unchanged, still operator-only.

## Sequencing with R0–R6

R1a (taxonomy file) joins R0 first (both are unblocking infra) → R1
envelope → C3 sealing + R2 obligations → C4 event + R6 scoring →
C2/S5b migration + C5 with R5 → C6 prose fix anytime.

Related: [[rea-adoption-plan]] · [[rea]] ·
[[integration-topology-and-licence-discipline]] ·
[[provenance-and-reproducibility]] · [[spend-mandates-and-delegation]] ·
[[multi-agent-research-collective]] · [[implementation-gaps]] ·
[[agentic-memory-and-observability]] · [[workflows-and-agent-authoring]]
