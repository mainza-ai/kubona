# Publication pipeline — implementation record (2026-11-18)

The local-private stack now owns a real publication path: a campaign becomes a
server-stored manuscript, an evidence-gated review, a reproducible PDF build,
an operator signoff, and a staged (never auto-published) deposit manifest.

This page is the operator/developer contract. The audit that motivated it is
[`production-audit-2026-11-18.md`](production-audit-2026-11-18.md); the slices
are tracked in `active-implementation-todos.md`.

## Division of labour

| Layer | Owns |
|---|---|
| Go API (`pkg/api/manuscripts.go`, `pkg/ledger/manuscripts.go`) | identity, tenancy, persistence, evidence resolution, content hash, state, signoff |
| Python bridge (`bridge/manuscript_ops.py`, `pkg/bridge/proto`) | prose composition, peer-review rule engine, LaTeX project, pdfTeX build |
| Console (`PublicationPanel.tsx`, `lib/api.ts`) | draft/review/build/signoff/deposit workflow and artifact download |
| Ledger | append-only events, content hashes, signoff row, artifacts (bytes) |

The bridge never decides whether evidence is real. The API never authors
prose or LaTeX.

## Endpoints

All paths are session-scoped and tenant-bound. A cross-tenant or unknown
session refuses before any body is read.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/sessions/{sid}/manuscripts` | list drafts (summary) |
| `POST` | `/api/sessions/{sid}/manuscripts` | compose a draft from the stored campaign; `build:true` also compiles |
| `GET` | `/api/sessions/{sid}/manuscripts/{mid}` | draft + sections + citations + reviews + artifacts + signoff |
| `POST` | `/api/sessions/{sid}/manuscripts/{mid}/reviews` | recompose and re-run the review over the stored source |
| `POST` | `/api/sessions/{sid}/manuscripts/{mid}/build` | compile the stored source; refuses a changed source |
| `POST` | `/api/sessions/{sid}/manuscripts/{mid}/signoff` | operator bearer only; signs the exact content hash |
| `POST` | `/api/sessions/{sid}/manuscripts/{mid}/deposit` | stage a Zenodo manifest from built artifacts |
| `GET` | `/api/sessions/{sid}/manuscripts/{mid}/artifacts/{name}` | artifact bytes (`main.pdf`, `main.tex`, `sections/*`, `references.bib`, `metadata.json`, `build.log`, `zenodo_deposit.json`) |
| `GET` | `/api/sessions/{sid}/manuscripts/{mid}/annotations` | review notes |
| `POST` | `/api/sessions/{sid}/manuscripts/{mid}/annotations` | add a note (`{anchor, body}`) |
| `PATCH` | `/api/sessions/{sid}/manuscripts/{mid}/annotations/{aid}` | set `open`, `addressed`, or `dismissed` |

## States and invariants

```
draft ──review──▶ reviewed ──signoff──▶ signed ──build──▶ built ──deposit──▶ deposit_ready
  │                  │                     │                 │
  └──── immutable after the first review; edits require a new draft ────┘
```

- **Content hash** covers title, abstract, every section (slug/title/LaTeX),
  every citation (key/title/authors/year/DOI), and every figure name+SHA-256.
  It is computed in Go so it cannot drift with Python formatting.
- **Draft immutability**: a reviewed/signed/built manuscript cannot be edited.
  A campaign that moved on requires a new draft.
- **Source-change refusal**: build/re-review recompose from the stored
  campaign snapshot and refuse with `409 source_changed` if the new source
  hash differs from the stored one. A PDF is never attached to a signoff that
  describes different content.
- **Evidence**: caller references resolve only against the campaign's *signed*
  result receipts. Empty references default to the campaign's own receipts.
  Anything else is printed as unresolved and earns no reproducibility credit.
- **Lean proof**: a supplied hash resolves only against a session event of
  type `formal_verification` / `comparator_verification` / `lean_verification`
  carrying `verdict:"live_kernel_proven"`, `sorry_ax_count:0`, and
  `axioms_clean:true`. No production producer exists yet, so a supplied hash
  stays unverified — review returns `REVISE`, and deposit refuses.
- **Annotations** are human review notes bound to the draft's content hash.
  They are not part of the rendered document and do not enter the content
  hash, but an `open` note blocks signoff with `409 annotations_open`.
- **Signoff**: requires the operator bearer, names the current content hash,
  has a future RFC3339 expiry (default 30 d, maximum 365 d), has no open
  annotations, and is signed by the commissioned reviewer key. The signature
  payload is `manuscript-signoff:<mid>:<content_hash>:<expiry>`.
- **Idempotent create**: a non-empty `request_id` is unique per session; a
  replay returns the existing draft (200) instead of creating a second one.
- **Deposit** sets `deposit_ready`; re-depositing replaces the manifest and
  keeps the same build hash.
- **Deposit**: requires an ACCEPT review, a valid unexpired signoff, and a
  built artifact set. The manifest is `staged-needs-human-upload`,
  `auto_publish:false`, `doi:null`. There is no uploader and no token path.

## Build requirements

- The **bridge image** installs `texlive-latex-base`, `texlive-latex-recommended`,
  and `texlive-fonts-recommended`. Without pdfTeX the RPC returns
  `build_unavailable` (503) rather than a fake PDF.
- The preamble uses only `amsmath`, `amssymb`, and `graphicx`; `unsrt.bst` is
  the bibliography style.
- Reproducibility: `SOURCE_DATE_EPOCH` is fixed per manuscript,
  `FORCE_SOURCE_DATE=1`, and pdfTeX date/trailer primitives are suppressed.
  Two builds of the same source are byte-identical (pinned by
  `bridge/test_manuscript_ops.py` and verified live).
- Figures must be PNG/PDF/JPEG bytes supplied by the caller. SVG is refused
  by `compile_latex`; there are no decorative or fabricated figures.
- Artifacts are stored as BLOBs in the ledger (bounded: 8 MiB per artifact,
  32 MiB per build), so `kubonad backup` remains a complete restore source.
- A scrubbed `build.log` ships with the build; the project temp path is
  replaced with `<project>`.

## Review scoring (fail-closed)

The deterministic rule engine in `kubona/science/manuscript.py` scores rigor,
novelty, grounding, hygiene, and reproducibility. It reaches `ACCEPT` only with
a comparator-verified kernel proof (hygiene 10.0) and composite ≥ 9.0. A
manuscript with measured results but no proof record is `REVISE`; a manuscript
with no measured point explicitly says so in the results section instead of
printing a template value.

## Operator checks

```bash
# Metrics include manuscript/job/backup gauges
curl -H "Authorization: Bearer $KUBONA_API_TOKEN" http://127.0.0.1:8080/metrics \
  | grep -E 'kubona_(jobs|manuscripts|backup|bridge)'

# Prove the scheduled backup file is restorable (not just the live ledger)
docker run --rm -v kubona-prod_kubona-backups:/backups:ro \
  -v "$PWD/scripts:/scripts:ro" --entrypoint bash kubona/kubonad:prod \
  -c 'KUBONAD_BIN=/usr/local/bin/kubonad bash /scripts/backup-restore-check.sh /backups'

# Declared encryption is enforced, not aspirational
KUBONA_BACKUP_REQUIRE_ENCRYPTION=1 kubonad backup-status --dir /backups
```

## Follow-up corrections (2026-11-18, second pass)

A self-audit of this slice found and fixed ten defects; each has a regression
test unless noted:

1. **Nested artifacts were unreachable.** The download parser accepted only
   `mid/artifacts/<flat-name>`, so `sections/results.tex` and
   `figures/*.png` were listed but 404'd. Nested project paths are now served
   with per-segment traversal checks.
2. **The content hash was order-insensitive.** Sections and citations were
   sorted and figure order was dropped, so a reordered document could share a
   signoff hash. The hash is now `kubona-manuscript-v2`: section order,
   citation order, and figure order all contribute. A reordered source
   refuses with `409 source_changed`.
3. **Preflight never received the encryption requirement.** The compose
   preflight service now forwards `KUBONA_BACKUP_REQUIRE_ENCRYPTION`, so a
   declared requirement is actually enforced at startup.
4. **Target markup could break the build.** `target_system` was interpolated
   raw into section LaTeX (the abstract was already escaped). It is escaped
   now; a hostile target compiles.
5. **Citation key collisions produced the wrong reference.** Two keys that
   sanitize to the same BibTeX key (`10.1/x`, `10.1_x`) now get deterministic
   suffixes; the API stores the effective keys the PDF cites.
6. **Provenance lists were unbounded.** A large campaign produced a huge
   section; the list is capped at 40 with a pointer to the signed export.
7. **Artifact size caps were checked after reading.** Both the PDF and each
   project file are now `stat`-checked before `read_bytes`, so a runaway build
   cannot exhaust bridge memory.
8. **Compose/build had no edge limiter.** `KUBONA_RATE_MANUSCRIPT`
   (default `0.2,3`) paces the CPU-bound operations; signoff/deposit stay
   human-paced and operator-authenticated.
9. **The schema version was not bumped.** `CurrentSchemaVersion` is now `2`,
   so an existing ledger takes a verified pre-migration snapshot before the
   additive publication DDL. Verified live: the main ledger upgraded to
   `schema=2` with `kubona.db.pre-migration-…` and both chains clean.
10. **Figure hashes were trusted from the bridge.** The API recomputes each
    submitted figure's SHA-256 and refuses a mismatch, so the content hash
    never depends on an unverified compute-plane assertion.

## Known limits (intentional)

- **No production comparator producer** — so no manuscript can reach ACCEPT
  today. That is the honest state; the gate is wired and tested with a
  synthetic kernel-proof event.
- **Annotations are persisted** (table + routes + panel) and gate signoff.
  Results Studio's older free-text annotation path still prompts the copilot;
  the publication notes are the ones of record.
- **Citations are caller-supplied.** Literature lookup does not yet feed the
  draft automatically, and claim→citation binding is manual.
- **No retention policy yet** for builds/artifacts; the next slice bounds
  per-session builds and bytes.
- **No plot generation.** Figures are caller artifacts; convergence data is a
  table. A plotting dependency is deliberately not added yet.
- **Server-side signoff** uses the commissioned reviewer key because this is
  a single trusted host; a hardware-held key would be a stronger separation.
