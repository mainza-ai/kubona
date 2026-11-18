---
type: concept
name: Publication pipeline (server-owned manuscripts)
date: 2026-11-18
updated: 2026-11-18
tags: [publication, manuscript, peer-review, latex, signoff, deposit, evidence]
sources: [source-gap-analysis-plan, source-architecture-paper]
status: adopted
---

# Publication pipeline (server-owned manuscripts)

The deployed path from measured campaign to paper artifact. Implemented on
2026-11-18 to close K-01/K-03/K-04 in
[[production-readiness-audit-2026-11-18]]; the operator/developer contract is
`docs/publication-pipeline-2026-11-18.md`.

## Shape

- **Go owns** identity, tenancy, persistence, evidence resolution, the
  canonical content hash, state transitions, and signoff.
- **The Python bridge owns** prose composition, the deterministic peer-review
  rule engine, the LaTeX project, and the pdfTeX build.
- **The console** exposes draft → re-review → build → operator signoff →
  deposit staging, plus artifact download.

A draft is composed from the stored campaign snapshot, never from a
client-authored template. The content hash covers title, abstract, sections,
citations, and figure hashes; a rebuild that would change it is refused
(`409 source_changed`).

## Evidence

- Caller references resolve only against the campaign's **signed** result
  receipts; unresolved references are printed as unresolved and earn no
  reproducibility credit.
- A Lean proof hash resolves only against a comparator event with
  `verdict:"live_kernel_proven"`, `sorry_ax_count:0`, and clean axioms. No
  production producer exists, so manuscripts honestly stop at `REVISE`.
- Reviews are persisted rows and `manuscript_reviewed` events; signoff is a
  `manuscript_signed` commit event over the exact hash.

## Build

pdfTeX is installed in the bridge image; the project `\input`s section files,
includes declared PNG/PDF/JPEG figures, resolves BibTeX, and is
byte-reproducible (fixed `SOURCE_DATE_EPOCH`, pdfTeX date/trailer suppression).
Artifacts (PDF, sources, metadata) are stored in the ledger, so `kubonad
backup` remains a complete restore source.

## Deposit

The Zenodo manifest is generated from **built** artifacts with checksums and
the signoff. It requires an ACCEPT review and a valid, unexpired signoff, and
is explicitly `staged-needs-human-upload` / `auto_publish:false` / `doi:null`.
There is no uploader and no token path; DOI minting happens at the repository.

## Follow-up audit (2026-11-18, second pass)

A self-audit found ten defects in this slice and fixed them with tests:
nested artifacts were not downloadable; the content hash ignored order (a
reordered paper could share a signoff hash — now `kubona-manuscript-v2`);
citation keys could collide and cite the wrong reference; `target_system` was
unescaped LaTeX; provenance lists were unbounded; artifact size caps were
checked after reading; compose/build had no limiter; the schema version was
not bumped (now 2, with a live pre-migration snapshot); preflight never received
the encryption requirement; and figure hashes were trusted from the bridge.
See `docs/publication-followup-audit-2026-11-18.md` (F-01…F-10).

The register and plan that remain:

- **P0 operator:** backup encryption/waiver + require flags; fresh-volume reset
  for the polluted host ledger.
- **P1:** annotation persistence; comparator producer (no ACCEPT path yet);
  literature→citation binding + novelty; artifact retention; durable audit
  events/build telemetry.
- **P2/P3:** create idempotency, draft edit/delete, pagination, `deposit_ready`,
  authenticated blob download, review-detail UX, route-vs-operation counts.

## Review notes, durability, and telemetry (third pass)

- **Annotations are records.** `manuscript_annotations` binds a note to the
  draft's content hash; signoff refuses with `409 annotations_open` while any
  note is open. The panel adds/resolves notes; the older free-text annotation
  prompt is separate.
- **Events are spooled.** Manuscript events use the durable spool and refuse
  if neither the ledger nor the spool accepts; a build ships a scrubbed
  `build.log`.
- **Create is idempotent.** `request_id` is unique per session; replay returns
  the same draft. Deposit staging sets `deposit_ready`.
- **Telemetry:** `/metrics` reports builds by outcome, build seconds, and
  stored artifact bytes.
- Schema version 3 covers the annotation table and the idempotency column.

## Known limits

- No production comparator producer ⇒ no ACCEPT path today (gate tested with a
  synthetic event).
- Annotations are not yet persisted as manuscript review notes.
- Citations are caller-supplied; literature lookup does not auto-feed them.
- Figures are caller artifacts; convergence data is a table, not a plot.
- Server-side signoff uses the commissioned reviewer key on the single trusted
  host; a hardware-held key would be a stronger separation.

## Cross-references

- [[autonomous-manuscript-synthesis-and-peer-review]] — the design and its
  correction block.
- [[provenance-and-reproducibility]] — the evidence model.
- [[production-readiness-audit-2026-11-18]] — the audit that ordered this work.
- [[remaining-work]] — execution queue.
