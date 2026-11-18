---
type: concept
name: Production Readiness Audit — 2026-11-18 (local-private)
date: 2026-11-18
updated: 2026-11-18
tags: [audit, production, local-private, publication, backup, access-mode, plan]
sources: [source-gap-analysis-plan, source-architecture-paper]
status: tracked
---

# Production Readiness Audit — 2026-11-18 (local-private)

> **What this page is.** A code-plus-runtime audit of the local-private
> deployment at `d9a0c1ab`, focused on the bar "fully functional on one trusted
> host", not "demo works". The full register, evidence log, and slice-level
> acceptance criteria are in
> [`docs/production-audit-2026-11-18.md`](https://github.com/mainza-ai/kubona/blob/develop/docs/production-audit-2026-11-18.md).
> This page is the durable summary; it is `tracked`, not `settled`.

## Verdict

The operational core is real: ledger, jobs, signed experiment export, scheduled
verified backups, restore drill, preflight, FK audit, sealed provider keys, and
the bridge/UDS confinement all work on the audited host. The **publication
pipeline is not deployed**, and the **declared single-lab posture is
contradicted by the live ledger**. Those are the two findings that block the
sentence "this deployment can produce a paper, its evidence, and a recoverable
record, end to end".

## P0 findings

| ID | Finding |
|---|---|
| K-01 | No production route serves manuscripts, peer review, reports, or builds. `/api/science/manuscripts` and `/api/reports` return 404; the Python engine is reachable only through the unserved `console_api.py`. |
| K-02 | `KUBONA_SINGLE_LAB=1` but `/api/access` reports `mode:"multi_lab"`. `singleTenantStore()` counts distinct session tenants; 21 E2E labs plus `mk-labs` keep the store multi-tenant, so auth is re-required and anonymous local chat is off. The ledger is append-only, so the repair is a fresh volume, not a row delete. |
| K-03 | Manuscript artifacts are not buildable or reproducible: no TeX toolchain in any image, figures never `\includegraphics`-ed, `sections/*.tex` written but not `\input`-ed, no compile test. The convergence figure renders an empty trajectory when no energies are supplied. |
| K-04 | Peer review and signoff are caller-attested. Provenance receipts are strings, not ledger lookups; the only validating path is unserved; no `peer_review` event is appended for a manuscript. The `ReviewEvidence` docstring's ledger-recording claim was false and is corrected. |

## P1 findings

- **K-05 Backups:** plaintext, same-host, `REQUIRE_ENCRYPTION=0`; preflight only
  warns when no backup exists; `restore-drill` proves the live ledger round-trips,
  not that the scheduled backup file is readable.
- **K-06 Observability:** `/metrics` is HTTP-only. No job-queue, bridge, backup,
  chain-fault, settlement, or scientific-run metrics; the alert webhook is unset.
- **K-07 Reports:** `ExperimentReport`/`ReportStore` exist but no served route or
  table; reports are campaign-state-only.
- **K-08 Review split:** Results Studio annotations are ephemeral React state
  sent as chat prompts. Real claim review lives in `ClaimHistory`; the two are
  not connected.
- **K-09 Deposits:** no in-product bibliography or artifact registry;
  `stage_zenodo_deposit` is library-only.
- **K-10 Lifecycle:** `local-reset.py` cannot reach the compose volume or take
  the daemon's lock; `down -v` deletes `kubona-backups`; `kubonad` does not
  mount `/backups`, so preflight there reports a false "no backups".
- **K-11 Citations:** literature lookup is display-only; no BibTeX, DOI
  resolution, or claim→citation binding.

## Plan (dependency order)

1. **R0 truth containment** — no publish/PDF affordances before the build exists;
   figures honest or absent.
2. **R1 persistence** — manuscript/review/artifact tables and tenant-scoped Go
   routes; drafts built server-side from the campaign projection.
3. **R2 evidence binding** — receipts and proof hashes resolved against the
   ledger; `peer_review`/signoff events appended.
4. **R3 reproducible build** — Tectonic-class toolchain in an isolated job,
   `\input` sections, real figures, resolved `references.bib`, hashed artifacts.
5. **R4 console workflow** — draft → review → signoff → build → download, with
   persistent annotations.
6. **R5 deposit staging** — manifest from built artifacts, human upload only.
7. **R6 operations** — encrypted off-host backups, require-backup preflight,
   restore-from-backup acceptance, operational metrics/alerts.
8. **R7 bibliographic integrity** — stored literature, stable BibTeX keys,
   measured or explicitly unmeasured novelty.

## Implementation status (2026-11-18, same session)

The R0–R7 plan was executed. The operational pipeline is now
[[publication-pipeline]] (adopted), with the contract in
`docs/publication-pipeline-2026-11-18.md`.

| Finding | Status |
|---|---|
| K-01 not deployed | Fixed: Go routes + bridge RPC + console panel |
| K-02 posture mismatch | Gate added (`--expect-single-lab`); operator reset remains |
| K-03 unbuildable | Fixed: pdfTeX, `\input`, byte-identical rebuild |
| K-04 caller-attested review | Fixed at the boundary: signed-receipt resolution, comparator-event binding, hash-bound signoff |
| K-05 backup trust | Partly: real-backup restore check + encryption enforcement; key/waiver operator-owned |
| K-06 observability | Improved: job/manuscript/bridge/backup gauges |
| K-07 report persistence | Fixed for manuscripts |
| K-08 review split | Partly: signoff persisted; annotations still chat prompts |
| K-09 bibliography/deposit | Partly: persisted citations + staged manifest, no auto-upload |
| K-10 compose lifecycle | Fixed in docs + mounts |
| K-11 citations | Partly: caller citations become BibTeX; no auto-feed |

Live proof: a fresh stack ran a real H2 VQE job, composed and built an 88 KB
PDF (`eb77c3db…`), rebuilt it to the identical hash, refused anonymous
signoff, accepted the operator signoff, refused deposit with `REVISE`, and
kept the ledger chain clean. The first build exposed a real composing defect
(primary VQE point in `measurement`, not `experiments`), now fixed with a
regression test.

### Third pass (2026-11-18): annotations, durability, idempotency

A1/A5/A6a/A8 from the follow-up plan landed: persisted review notes that gate
signoff, spooled manuscript events, a scrubbed build log, build/artifact
metrics, idempotent create, and `deposit_ready`. Schema version 3. Remaining:
citations/novelty, comparator producer, retention, draft edit/delete/pagination,
and authenticated blob download.

### Follow-up pass (2026-11-18, second)

`docs/publication-followup-audit-2026-11-18.md` re-audited the shipped
pipeline and fixed ten edge defects (F-01…F-10), including an
order-insensitive content hash, unreachable section artifacts, citation-key
collisions, and a missing schema bump. The remaining work is product depth
(annotations, comparator, citations, retention) plus the two operator policy
items; it is tracked in [[publication-pipeline]] and [[remaining-work]].

## What this audit changed

- `scripts/local-deployment-check.sh` now asserts `/api/access` with
  `--expect-single-lab`; CI's docker smoke passes the flag. It **fails on the
  audited host** (`mode=multi_lab`), which is the reproduction for K-02.
- `docs/local-deployment.md` gained a compose-aware reset procedure, the
  `down -v` warning, the preflight-mount caveat, and backup acceptance items.
- `platform/kubona/science/manuscript.py` no longer claims that a served
  endpoint records reviews into the ledger.

## Cross-references

- [[remaining-work]] — execution queue this plan feeds.
- [[autonomous-manuscript-synthesis-and-peer-review]] — carries a correction
  block for the "deployed tribunal/PDF/DOI" description.
- [[provenance-and-reproducibility]] — the evidence model the build must obey.
- [[production-readiness-audit-2026-10-26]] — the earlier full-tree audit
  (different scope: code/P0 blockers, not the publication pipeline).
- [[roadmap]] — phases read as acceptance criteria.
