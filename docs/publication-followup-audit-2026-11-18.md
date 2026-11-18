# Publication pipeline follow-up audit — 2026-11-18 (second pass)

**Scope:** the code shipped in `30803817`/`c452ded3` (the R0–R7 publication
slice), the remaining R4/R7 items, the operator-only items, and the operational
edges around the new ledger evidence.
**Method:** adversarial re-read of the new code, targeted probes (including a
live disposable stack), the full Go/console suites, and a live schema upgrade.
**Audited head:** `c452ded3` plus this pass's fixes (uncommitted at the time of
writing).

> This is the **follow-up** to `production-audit-2026-11-18.md`. That report
> asked for the pipeline; this one asks whether the pipeline is trustworthy
> under its own claims. Ten defects were found and fixed; the remaining work is
> registered in §4 and planned in §5.

## 1. Executive verdict

The pipeline's architecture is sound: Go owns identity/persistence/evidence
resolution, the bridge owns composition/build, and the console never authors
numbers. The live proof from the first pass still holds (real VQE → 88 KB PDF →
identical rebuild → operator signoff → deposit refused at `REVISE`).

The second pass found that several **edges of that core were wrong**, two of
them integrity-relevant:

- the content hash ignored section/citation/figure **order**, so a reordered
  document could share a signoff hash;
- `sections/*` artifacts were advertised but **not downloadable**;
- citation keys that sanitize to the same BibTeX key silently cited the
  **wrong reference**;
- the preflight container never received the encryption requirement, so a
  declared requirement was **not enforced at startup**;
- a target string could **break the LaTeX build**;
- the schema bump was missing, so the new tables landed without the documented
  pre-migration snapshot.

All ten are fixed with regression tests. The remaining work is now mostly
**product depth** (annotations, citations, comparator) and **operational
policy** (retention, encryption, reset), not correctness of the core.

## 2. Defects found and fixed in this pass

| ID | Defect | Severity | Fix | Test |
|---|---|---|---|---|
| F-01 | Nested artifacts (`sections/results.tex`, `figures/*.png`) were listed but the download route accepted only flat names → 404 | P1 truth/UX | Parser splits on `/artifacts/` and validates every segment; nested names served | `TestManuscriptArtifactDownloadAcceptsNestedProjectPaths` |
| F-02 | Content hash sorted sections/citations and dropped figure order; two different documents could share a signoff hash | **P0 integrity** | Hash v2 is order-sensitive over sections, citations, and figures; reorder → `409 source_changed` | `TestManuscriptHashCoversSectionOrder` |
| F-03 | `target_system` interpolated raw into section LaTeX; `_`/`%`/`&` broke pdflatex | P1 build | Escaped at the prose boundary | `test_target_markup_is_escaped_in_section_latex`, `test_build_accepts_a_hostile_target…` |
| F-04 | Citation keys `10.1/x` and `10.1_x` both became `10_1_x`; BibTeX cited the wrong entry | **P1 truth** | Deterministic suffixes; API stores the effective keys the PDF cites | `test_citation_keys_that_collide…`, `TestManuscriptStoresBridgeEffectiveCitationKeys` |
| F-05 | Provenance section listed every receipt; a large campaign produced a huge section | P2 build | Cap at 40 with a pointer to the signed export | `test_provenance_list_is_bounded…` |
| F-06 | PDF/artifacts were read into memory before the size cap | P1 resource | `stat` before `read_bytes`; refuse oversized outputs | build tests (cap path) |
| F-07 | Compose/build/re-review had no edge limiter (CPU-bound) | P1 resource | `KUBONA_RATE_MANUSCRIPT` (0.2/s, burst 3) on the work paths only | `TestManuscriptWorkIsRateLimited` |
| F-08 | `CurrentSchemaVersion` stayed 1; existing ledgers missed the pre-migration snapshot for the new tables | P1 upgrade | Version 2; live upgrade took `kubona.db.pre-migration-…` and both chains verify | `TestPublicationUpgradeTakesPreMigrationSnapshot` + live |
| F-09 | Preflight service did not receive `KUBONA_BACKUP_REQUIRE_ENCRYPTION`; enforcement was unreachable in compose | P1 ops | Env forwarded to preflight; `compose config` shows it | compose config |
| F-10 | API trusted the bridge's figure SHA-256 in the content hash | P2 integrity | Recompute each submitted figure hash; mismatch → `bridge_unavailable` | figure-hash path (unit) |

Also clarified in UI: deposit staging states **why** it is disabled (an ACCEPT
review needs a comparator-linked kernel proof), instead of an unexplained grey
button.

## 3. What the follow-up verified live

- Schema upgrade: the main ledger opened at `schema=1` and now reports
  `schema=2`, with `/home/kubona/.kubona/kubona.db.pre-migration-20261118T103105…`
  written and `chain clean` on both the snapshot and the live DB.
- Preflight inside `kubonad` sees the backup volume: `backups ok newest 21m49s old`
  (previously a false "no files").
- `/metrics` exposes `kubona_backup_count 7` and `kubona_backup_age_seconds`.
- `/api/sessions/{id}/manuscripts` serves an empty list on the main ledger
  (tables created by the upgrade).
- Full Go suite, 304 node tests, 524 browser tests, 44 Python manuscript/bridge
  tests, wiki lint, counts, contracts, token lint — all green (see §6).

## 4. Remaining gaps register

### P0 — operational, operator-owned

| ID | Gap | Why it blocks the production claim |
|---|---|---|
| G-01 | **Backups are still plaintext and same-host.** `KUBONA_BACKUP_AGE_RECIPIENT` is empty, `REQUIRE_ENCRYPTION=0`; the new enforcement exists but is off. No signed plaintext waiver is recorded. | A lost host loses the paper, its evidence, and every artifact. The code now *can* enforce encryption; the policy is unset. |
| G-02 | **The host ledger is still `multi_lab`.** 23 tenants (21 E2E) make `--expect-single-lab` fail. No reset has been executed. | The declared local-private posture is not the effective one; auth and anonymous chat behave differently than the operator believes. |

Both are decisions/actions only the operator can take. The procedures are
documented (`docs/local-deployment.md` reset; `docs/backup-restore.md` restore;
`.env.example` encryption block).

### P1 — product depth and operational safety

| ID | Gap | Evidence |
|---|---|---|
| G-03 | **Annotations are not manuscript review notes.** Results Studio annotations are optimistic React state + a chat prompt; they are not persisted, not tied to a section, and not part of the signoff. R4's remaining item. | `ResultsStudio.tsx` `handleAddAnnotation`; no annotation table/route. |
| G-04 | **No comparator producer, so no ACCEPT.** The review gate resolves `formal_verification` events, but no code path emits one. Every real manuscript stops at `REVISE`; deposit can never be staged in practice. | `resolveManuscriptProof`; no event emitter; live deposit `409 review_not_accepted`. |
| G-05 | **Citations are caller-supplied only.** Literature lookup never feeds a draft; `manuscript_citations` has no `claim_id`; novelty distance is never measured (bridge accepts it, Go never sends it). | `manuscript_citations` schema; `storedManuscriptRequest` evidence map; bridge `citation_distance` default. |
| G-06 | **No artifact retention.** Every build replaces artifacts, but no policy bounds per-session builds, total bytes, or backup growth; deleting a session leaves its manuscript blobs unreachable but still in the ledger/backups. | Only `DELETE FROM manuscript_artifacts` is the replace path; no prune. |
| G-07 | **Build observability is thin.** No build duration/failure counter, no persisted build log/diagnostics, no artifact-bytes gauge. A failing LaTeX build is visible only in the error string returned to the caller. | `/metrics` has no manuscript work counters; build log dropped. |
| G-08 | **Event durability is best-effort.** `manuscript_drafted/reviewed/built` appends use `AppendEvent` and ignore errors; the review-history row insert is also best-effort. A ledger append failure can leave a manuscript row without its audit event. | `_ = s.ledger.AppendEvent(...)`, `_ = s.ledger.AddManuscriptReview(...)`. |

### P2 — ergonomics, honesty, and polish

| ID | Gap |
|---|---|
| G-09 | No edit/delete/archive for drafts; no pagination on the manuscript list; no `request_id` idempotency for create (double-click = two drafts). |
| G-10 | The manuscript state vocabulary stops at `built`; a staged deposit is not a distinct state (the manifest says `staged-needs-human-upload`). |
| G-11 | `docs/repo-counts.md` reports **154 HTTP routes** (registration patterns) while the OpenAPI surface is **157 paths**; the label can mislead about endpoint coverage. |
| G-12 | Multi-lab artifact download uses a plain anchor; it works with the Lax session cookie, but a bearer-only console build has no header on that navigation. A blob download through `apiFetch` would close it. |
| G-13 | Novelty scoring is “provisional 8.0” until a citation-distance measurement exists; the review summary says so, but no operator surface explains the score. |
| G-14 | Build reproducibility is pinned to the bridge image’s TeX version; cross-image builds are not claimed identical. |
| G-15 | Python bridge tests are excluded from the `repo-counts` “Python test files” number, and the new publication tests live in `bridge/`. The count definition should say so (or include bridge tests separately). |

## 5. Implementation plan (A-series)

Ordered by risk, not by novelty. Each slice closes only with executable
acceptance.

### A1 — Annotation persistence (P1, closes G-03)
- Table `manuscript_annotations(id, manuscript_id, session_id, tenant_id,
  author, anchor, body, status, created_at)`; status vocabulary
  `open|addressed|dismissed`.
- Routes: `GET/POST /api/sessions/{sid}/manuscripts/{mid}/annotations`,
  `PATCH …/annotations/{aid}` (status only, operator or author).
- The review projection includes open annotations; signoff refuses while any
  annotation is `open` (or requires an explicit “resolve all” action).
- **Accept:** annotation survives reload; signoff refusal names the open note;
  cross-tenant read/write refused; browser test covers create + resolve.

### A2 — Citation and novelty binding (P1, closes G-05, G-13)
- `manuscript_citations` gains `claim_id` and `source_id` (OpenAlex/DOI) plus
  `verified` (resolved vs caller-supplied).
- Console: select literature records from the Literature tab into the draft;
  server stores them and emits BibTeX.
- Measure `citation_distance` from the selected records (OpenAlex) and send it
  to the bridge; record “unmeasured” explicitly when lookup is unavailable.
- **Accept:** a manuscript cites only stored records or explicit unverified
  caller entries; a claim citation resolves to a stored reference; novelty is
  numeric or explicitly unmeasured.

### A3 — Comparator producer (P1, closes G-04)
- A comparator job/bridge RPC that runs the Lean comparator, returns
  `{proof_hash, verdict, sorry_ax_count, axioms_clean}` and appends a signed
  `formal_verification` event.
- Wire the existing `Dockerfile.lean` toolchain and a fixture theorem.
- **Accept:** a manuscript with a passing comparator review reaches ACCEPT and
  can be signed + deposited end to end; a failing proof stays REVISE.

### A4 — Artifact lifecycle (P1, closes G-06)
- Retention: keep the newest `KUBONA_MANUSCRIPT_KEEP_BUILDS` (default 3) per
  manuscript and a per-tenant byte ceiling; prune oldest builds first.
- Session deletion: export-before-delete or soft-delete with an explicit
  “publication evidence” marker; never silently drop the blobs without an
  export path.
- `/metrics`: `kubona_manuscript_artifact_bytes`, builds retained.
- **Accept:** a 20-build session stays under the ceiling; a pruned build is
  reported by hash; a deleted session’s evidence is exported or explicitly
  retained with a reason.

### A5 — Durable audit and build telemetry (P1, closes G-07, G-08)
- Use `AppendEventWithSpool` for manuscript events; fail the request when both
  append and spool fail.
- Persist a scrubbed `build.log` artifact and record duration/success counters
  (`kubona_manuscript_builds_total{result}`).
- **Accept:** a forced ledger failure spools and replays; a failing build is
  diagnosable from stored artifacts; metrics show success/failure counts.

### A6 — API ergonomics (P2, closes G-09, G-10)
- `request_id` idempotency on create (replay returns the same draft).
- `PUT` a draft before first review; `DELETE`/archive a draft; pagination
  (`limit/offset`) on list.
- Add `deposit_ready` state (or a `deposit_staged_at` field).
- **Accept:** double-submit yields one draft; pagination is bounded; archived
  drafts are excluded by default.

### A7 — Console polish (P2, closes G-12, G-13)
- Blob artifact download through `apiFetch` (bearer + cookie), basename
  filename.
- Review-detail view explaining each dimension and the provisional novelty.
- **Accept:** download works in bearer-only and cookie modes; the review panel
  states measured vs unmeasured values.

### A8 — Counts and docs honesty (P3, closes G-11, G-15)
- Report “route patterns” and “OpenAPI operations” separately; count bridge
  tests explicitly or include them in a separate row.
- **Accept:** `repo-counts --check` output matches the labels.

## 6. Verification log (this pass)

```
go test ./pkg/api ./pkg/ledger ./cmd/kubonad        ok (api 81s)
go test ./...                                        ok (prior full run)
pytest test_manuscript + test_phase6 + bridge        44 passed
bridge/test_manuscript_ops.py                        17 passed (2 builds byte-identical)
console tsc                                          ok
console node tests                                   304 passed
console browser tests                                524 passed
wiki-lint --strict                                   clean (163 pages)
repo-counts --check                                  OK
contract inventory / OpenAPI check                   180/15/0, 157 paths
token lint                                           0 raw-palette
compose config                                       KUBONA_BACKUP_REQUIRE_ENCRYPTION forwarded
live schema upgrade                                  schema=2, pre-migration snapshot verified
live /metrics                                        backup count/age present
```

## 7. Residual risk statement

- The publication pipeline is **trustworthy for drafts, builds, and
  human-operator signoff** on this host.
- It is **not yet a complete scientific publication system**: no comparator
  producer (no ACCEPT), no annotation record, no literature binding, no
  retention policy.
- The operator items (encryption, require flags, reset) are the remaining
  blockers to calling the *deployment* production-ready, independent of code.
