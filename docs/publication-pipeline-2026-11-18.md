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
| `GET` | `/api/sessions/{sid}/manuscripts/{mid}/artifacts/{name}` | artifact bytes (`main.pdf`, `main.tex`, `sections/*`, `references.bib`, `metadata.json`, `zenodo_deposit.json`) |

## States and invariants

```
draft ──review──▶ reviewed ──signoff──▶ signed ──build──▶ built
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
- **Signoff**: requires the operator bearer, names the current content hash,
  has a future RFC3339 expiry (default 30 d, maximum 365 d), and is signed by
  the commissioned reviewer key. The signature payload is
  `manuscript-signoff:<mid>:<content_hash>:<expiry>`.
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

## Known limits (intentional)

- **No production comparator producer** — so no manuscript can reach ACCEPT
  today. That is the honest state; the gate is wired and tested with a
  synthetic kernel-proof event.
- **Annotations are not yet review notes.** Results Studio annotations still
  go to the copilot as prompts. Persisting them as `manuscript_reviews`
  comments is the remaining R4 item.
- **Citations are caller-supplied.** Literature lookup does not yet feed the
  draft automatically, and claim→citation binding is manual.
- **No plot generation.** Figures are caller artifacts; convergence data is a
  table. A plotting dependency is deliberately not added yet.
- **Server-side signoff** uses the commissioned reviewer key because this is
  a single trusted host; a hardware-held key would be a stronger separation.
