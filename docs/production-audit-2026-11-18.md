# Production audit — 2026-11-18 (local-private)

**Audited head:** `d9a0c1ab` on `develop` (working tree clean at audit start).
**Runtime audited:** the live `platform/docker/compose.prod.yml` stack on this
host (`kubona-prod-kubonad-1`, `-bridge-1`, `-console-1`, `-backup-1`).
**Scope:** fully functional **local-private** deployment on one trusted host —
not a demo, not a public SaaS, not multi-tenant hosting.
**Method:** code read plus live execution. Every claim below names the file and
line or the command that produced it. Prior status prose was treated as a claim
to verify, not as evidence.

> **What this page is.** The audited system is *operationally healthy* and its
> science/job/ledger core is real. Its **publication pipeline is not deployed**
> and its **declared deployment posture does not match its data**. Those two
> are the audit's headline findings. Nothing here upgrades a historical result
> to a stronger claim, and no finding below is closed by this page — the plan
> in §7 is the work-list.

---

## 1. What was verified live (not inferred)

| Check | Command | Result |
|---|---|---|
| Stack | `docker compose ... ps` | 4 services up, all healthy (`kubonad`, `bridge`, `console`, `backup`) |
| Health | `GET /api/health` | 200, `status:"healthy"`, `disk_level:"ok"`, version `dev` |
| Readiness | `GET /api/ready` | `{"ready":true}` |
| Access mode | `GET /api/access` | `mode:"multi_lab"`, `single_lab:false`, `auth_required:true`, `campaign_create:false`, `anonymous_chat:false` |
| Tenants | `GET /api/tenants` | **23** tenants: `default`, `mk-labs`, and **21** `e2e-*` |
| Sessions | `GET /api/sessions` | 24 sessions across **24** distinct tenants |
| Notebooks | `GET /api/results/notebooks?session_id=…` | `{"notebooks":[]}` (campaign `notebook` is `null`, so this is consistent, not a defect) |
| Signed export | `GET /api/sessions/{id}/experiment-export` | `ok:true`, 2 jobs, Ed25519 envelope `signed:true`, payload SHA-256 present |
| Manuscript API | `POST /api/science/manuscripts` | **404** |
| Reports API | `GET /api/reports` | **404** |
| Backups | `kubonad backup-status --dir /backups --max-age 26h` (in `backup`) | `plaintext`; newest `…20261118T053638Z`, age 3 h 28 m, **7** backups |
| Backup integrity | `kubonad verify --db <newest backup>` | `chain clean (all sessions)` |
| Restore drill | `kubonad restore-drill --db …` | `clean (sessions=24 jobs=54 events=659 claims=4)` |
| Preflight (preflight container) | `kubonad preflight …` | clean; rate card `v2` (19 models), schema `1`, FK clean, 2 sealed provider keys, backups fresh |
| Preflight (kubonad container) | same command | `backups warn no backup files found` — the `kubonad` service does not mount `kubona-backups` |
| FK audit | `kubonad fk-audit` | clean |
| Metrics | `GET /metrics` | only `kubona_http_requests_total`, `…_duration_seconds`, `…_inflight` |
| Go build | `go build ./...` | ok |
| Go API tests | `go test ./pkg/api -run 'TestNotebooks\|TestExperimentExport\|TestClaimReview'` | ok |
| Python manuscript/report tests | `pytest tests/test_manuscript.py tests/test_phase6.py` | 29 passed |
| Wiki lint | `python3 scripts/wiki-lint.py --strict` | clean, 161 pages |
| Counts | `python3 scripts/repo-counts.py --check` | OK |
| Contracts | `console-contract-inventory.py --check` | 173 modeled / 14 client-only / 0 unmodeled, 149 schemas |

**Verdict on the operational core:** the ledger, jobs, signed export, backup,
restore, preflight, and FK/secret machinery work on this host. The scientific
execution path (bridge → job → result receipt → campaign fold) is real and
tested.

---

## 2. Executive verdict

1. **The publication pipeline is absent from the deployed product.** The
   manuscript generator, peer-review rule engine, signoff verifier, report
   builder, and Zenodo staging exist as Python library code. None of them is
   reachable from the Go API, the bridge, or the console. The only "manuscript"
   a user can obtain is a **client-side LaTeX string** assembled in the browser
   from in-memory campaign state (`ResultsStudio.tsx`). The two routes that
   would serve the library (`/api/science/manuscripts`, `/api/reports`) exist
   only in `platform/kubona/console_api.py`, which has **no listener and no
   compose service** — verified 404 on the live API.
2. **The declared single-lab posture is contradicted by the live ledger.**
   `compose.prod.yml` declares `KUBONA_SINGLE_LAB=1`, but `singleTenantStore()`
   only reports single-lab while the sessions table holds **one** tenant
   (`tenant_reads.go:44`, `feature_gates.go:28`). Twenty-one E2E labs make the
   effective mode `multi_lab`, which re-requires operator auth and disables
   anonymous local chat. The acceptance script did not check this until this
   audit (now it can: `--expect-single-lab`).
3. **The manuscript artifacts that do exist are not reproducible.** LaTeX is
   assembled but never compiled (no TeX toolchain in any image), figures are
   written but never referenced by the document (`\includegraphics` appears
   nowhere), section files are written but the main document inlines sections
   instead of `\input`-ing them, and the convergence figure is drawn with an
   empty trajectory when no energies are supplied. There is no build hash, no
   compile test, and no artifact store.
4. **Peer review is a deterministic rule engine over caller-attested numbers.**
   It is not the "adversarial tribunal" the wiki describes, the provenance
   receipts are never verified against the ledger, and the only path that even
   validates the evidence envelope is the unserved `console_api`. The module
   docstring previously claimed the endpoint records the review into the ledger;
   it does not (docstring corrected in this audit).
5. **Backups are plaintext, same-host, and not a proven restore source for the
   scheduled file.** Encryption is off, preflight only warns when backups are
   missing, and `restore-drill` proves the *live* ledger round-trips — it does
   not prove the newest scheduled backup file is readable.
6. **Observability is HTTP-only.** There are no job/queue, bridge, backup,
   ledger, or scientific-work metrics, and the alert rail is a webhook that is
   not configured on this host.

None of these are demo-grade failures of the science core. All of them block
the sentence "this deployment can produce a report, a paper, and a recoverable
record, end to end".

---

## 3. Findings register

Severity is about **the local-private production bar**, not about code quality.

### P0 — blocks a production claim

| ID | Finding | Evidence |
|---|---|---|
| **K-01** | **Reporting/publication pipeline is not deployed.** No Go route serves manuscripts, peer review, reports, or builds. The Python implementations are library-only; `console_api.py` (the only route layer that calls them) is unserved. Live checks return 404 for `/api/science/manuscripts` and `/api/reports`. | `platform/core/pkg/api/server.go:621-876` (route table contains no manuscript/report route); `platform/kubona/console_api.py:528` (`science_manuscript_create`); 404s recorded in §1 |
| **K-02** | **Effective access mode contradicts the declared posture.** `KUBONA_SINGLE_LAB=1` + 23 tenants ⇒ `/api/access` reports `multi_lab`, `auth_required:true`, `campaign_create:false`. E2E labs created on the live stack are the cause; the ledger is append-only so there is no supported "delete the labs" repair. | `platform/docker/compose.prod.yml:166`; `platform/core/pkg/api/tenant_reads.go:44`; `platform/core/pkg/api/feature_gates.go:28`; live `/api/access` and `/api/tenants` |
| **K-03** | **Manuscript artifacts are not buildable or reproducible.** No TeX engine in `Dockerfile.kubonad`, `Dockerfile.bridge`, or `Dockerfile.console`; `compile_latex()` emits `\bibliography{references}` with no bibtex step; figures are generated but never included (`\includegraphics` absent); `export_project()` writes `sections/*.tex` that `main.tex` does not `\input`; no test compiles LaTeX. | `platform/kubona/science/manuscript.py:136-175`, `:177-219`, `:570-572`; `grep -R 'texlive\|pdflatex\|latexmk\|tectonic'` → none |
| **K-04** | **Peer review and signoff are caller-attested, not ledger-bound.** Scores derive from caller-supplied fields; `provenance_receipts` are strings that are never resolved against the ledger; the signoff verifier is real Ed25519 but nothing in production calls it. Only `console_api` builds `ReviewEvidence` from a request, and it too does not append a `peer_review` event. | `platform/kubona/science/manuscript.py:27-40`, `:578-668`, `:45-85`; `platform/kubona/console_api.py:528-590` (no `AppendEvent`); ledger has `MintClaimReview` but no manuscript route |

### P1 — required for a complete researcher workflow

| ID | Finding | Evidence |
|---|---|---|
| **K-05** | **Backups are plaintext and same-host; the acceptance gate does not force encryption or a real backup-file restore.** `KUBONA_BACKUP_AGE_RECIPIENT` is empty, `REQUIRE_ENCRYPTION=0`, status prints `plaintext`. `KUBONA_PREFLIGHT_REQUIRE_BACKUP=0` means a fresh host starts with no backup and only a warning. `restore-drill` snapshots the live DB, so it never proves `kubona.db.backup-*` is readable. | `platform/docker/backup-loop.sh:14-34`; `.env.example:101-105`; live `backup-status`; `platform/core/cmd/kubonad/restore_drill.go` |
| **K-06** | **Observability covers HTTP only.** No metrics for job queue depth/age, bridge health, backup age, ledger chain faults, settlement, or scientific runs. Alerts exist (`alerts.Notify`) but require `KUBONA_ALERT_WEBHOOK_URL`, unset on this host. | `platform/core/pkg/api/metrics.go`; `platform/core/pkg/alerts/alerts.go:17`; live `/metrics` |
| **K-07** | **Report persistence is campaign-state-only.** `ExperimentReport`/`ReportStore` are never used by a served route; reports are built in `discovery/campaign.py` and written to `reports.json` only when that Python path runs. There is no report table, no report route, and no report contract. | `platform/kubona/reports/notebook.py:68`; `platform/kubona/discovery/campaign.py:251`; `platform/kubona/console_api.py:150` (unserved) |
| **K-08** | **Review surfaces are split and partly ephemeral.** `ResultsStudio` annotations are React state sent to chat as a prompt (`sendChatMessage`); they are not persisted and do not create a claim review. A real claim-review flow exists in `ClaimHistory` (`reviewClaim` → `/api/sessions/{id}/claims/{cid}/review`), but it is not connected to the manuscript/annotation surface. | `platform/console/src/components/surfaces/ResultsStudio.tsx:611-645`; `platform/console/src/components/common/ClaimHistory.tsx:118`; `platform/core/pkg/api/server.go:2545-2575` |
| **K-09** | **No in-product bibliography or deposit path.** `bibtex_entries` is empty unless a caller supplies entries; literature lookup (`/api/literature`) never becomes BibTeX; `stage_zenodo_deposit` is library-only and unwired; DOI minting is correctly refused (good), but there is no staged deposit artifact anywhere in the deployed stack. | `platform/kubona/science/manuscript.py:704-752`; `platform/core/pkg/api/server.go:706`; `platform/console/src/components/surfaces/ResultsStudio.tsx:447` |
| **K-10** | **Compose lifecycle gaps.** `local-reset.py` targets a host path and cannot reach the `kubona-state` volume or take the lock while the compose daemon runs. `docker compose down -v` deletes `kubona-backups` along with state. `kubonad` does not mount `/backups`, so running preflight there gives a false "no backups" warning. | `scripts/local-reset.py:40-60`; `platform/docker/compose.prod.yml:257-260`, `:354-357`; live preflight discrepancy |
| **K-11** | **Citations are display-only.** Literature records are projected to a table; there is no citation verification, no DOI resolution, no claim→citation binding, and no reference of record for a manuscript. | `platform/core/pkg/api/server.go:1985-2005`; `ResultsStudio.tsx` literature tab |

### P2 — enhancements / hygiene

| ID | Finding | Evidence |
|---|---|---|
| **K-12** | **LaTeX hygiene.** `\usepackage{cite}` is deprecated; `\bibliography{references}` requires an external bibtex run; no `\includegraphics`, captions, labels, cross-references, or supplementary material; the "bilingual" path is a title/abstract stub, not a second-language document. | `platform/kubona/science/manuscript.py:136-175` |
| **K-13** | **No single canonical result object end-to-end.** Result JSON, campaign projections, signed export, and UI types still carry separate shapes; the prior audit's P0 item remains open. | `platform/core/pkg/experimentresult/`, `platform/core/pkg/agents/campaign_synth.go` |
| **K-14** | **Contracts stop at the served surface.** There are no OpenAPI schemas or console contract entries for manuscripts/reports because there are no routes; the publication feature is invisible to contract tooling. | `platform/contracts/console-openapi.json`; `platform/contracts/console-contract-inventory.json` |

---

## 4. Axis A — local-private operations and security

**Working:**

- Loopback-only publishing is enforced and checked (`scripts/local-deployment-check.sh`), and the compose header documents the SSH-tunnel/WireGuard path.
- Writes fail closed without `KUBONA_API_TOKEN`; the production guard refuses an ephemeral reviewer signer.
- Secrets at rest: `KUBONA_SECRET_KEY` seals UI-entered provider keys (preflight reports 2 sealed keys).
- Ledger chain verify, FK audit, schema-version refusal on newer DBs, and pre-migration verified snapshots are implemented.
- Backup loop is scheduled, verifies the copy (`VACUUM INTO` + `VerifyChain`), writes a SHA-256 sidecar, and prunes to `KEEP`.

**Gaps:**

- **Posture mismatch (K-02).** This is the single most misleading operational
  fact on the host: the console can be in `multi_lab` while the operator
  believes it is `local_private`. The new `--expect-single-lab` assertion makes
  it observable; the repair is a fresh-volume reset (documented now).
- **Plaintext backups (K-05).** On a single trusted host this is a defensible
  choice, but it must be a *recorded* choice. Today it is a default with a
  warning.
- **Backup blind spot.** `restore-drill` is valuable but tests a fresh snapshot
  of the live DB. The scheduled file is verified by `verify` (chain) but no
  documented step restores it. A corrupt `.db` with a clean chain is possible.
- **Alerting is opt-in and off.** `KUBONA_ALERT_WEBHOOK_URL` is empty, so disk,
  calibration, rate-limit, and settlement-skip alerts go nowhere.
- **Metrics surface (K-06).** Prometheus can see HTTP 5xx but cannot see a
  stuck job queue, an ageing obligation, a bridge that stopped accepting work,
  or a backup that silently stopped producing files.

---

## 5. Axis B — evidence, provenance, persistence

**Working:**

- The ledger is append-only and hash-chained; `verify` is clean on the live DB
  and on the newest backup.
- Jobs persist a `result_receipt`; the experiment export is Ed25519-signed over
  the exact payload bytes (`experiment_export.go`).
- Claim rows are immutable; reviews are append-only `ClaimReview` projections
  with a closed decision vocabulary and a `peer_review` event.
- Campaign folding records measurement provenance, backend, claim ids, and a
  provenance hash into the systematic notebook.
- Tenant-scoped reads bind the caller to the session tenant; ghost sessions are
  refused rather than fabricated.

**Gaps:**

- **The export is the strongest artifact and it stops at JSON.** There is no
  human-readable, signed, buildable document derived from it.
- **Review evidence is not tied back to the ledger (K-04).** A `provenance_receipt`
  string can be invented; nothing resolves it against `ledger_records`.
- **Annotations are not evidence (K-08).** A human review comment never becomes
  a persisted artifact, so it cannot be cited, audited, or attached to a
  signoff.
- **No artifact registry.** Manuscripts, PDFs, figures, and deposit manifests
  have no table, no content hash, no retention rule, and no tenant scope.

---

## 6. Axis C — results, reports, documentation, academic papers

### 6.1 What exists (and where)

| Layer | Implementation | Reachable in production? |
|---|---|---|
| Campaign result model | `pkg/agents/campaign_synth.go` (`CampaignState`, `SystematicNotebook`) | Yes |
| Signed experiment export | `pkg/api/experiment_export.go` | Yes (`/api/sessions/{id}/experiment-export`) |
| Notebook read | `pkg/api/server.go:2009` (`/api/results/notebooks`) | Yes |
| Result receipt | `pkg/jobs/jobs.go` | Yes (inside job result) |
| Structured report | `kubona/reports/notebook.py` (`ExperimentReport`, `ReportStore`) | **No** |
| Manuscript generator | `kubona/science/manuscript.py` (`ManuscriptGenerator`) | **No** |
| Peer review | `manuscript.py:578` (`automated_peer_review`) | **No** |
| Signoff verifier | `manuscript.py:45` (`ManuscriptSignoff`) | **No** |
| Zenodo staging | `manuscript.py:704` (`stage_zenodo_deposit`) | **No** |
| Console results UI | `ResultsStudio.tsx` | Yes (client-side exports only) |
| Claim review UI | `ClaimHistory.tsx` + `/claims/{id}/review` | Yes (claims, not manuscripts) |

### 6.2 What the console actually does

- **Signed export:** downloads the server-built, Ed25519-signed JSON bundle
  (`handleExportSigned`, `ResultsStudio.tsx:511`). This is the only
  server-authored artifact a user can obtain.
- **Result JSON / CSV:** built in the browser from campaign state
  (`handleExportResult`, `handleExportCSV`).
- **Markdown finding:** built in the browser and downloaded
  (`ResultsStudio.tsx:601`).
- **LaTeX tab:** a template string with `% UNPUBLISHED DRAFT — DOI is minted by
  Zenodo/arXiv at deposit time, never here` (`ResultsStudio.tsx:447`). It is a
  correct refusal to invent a DOI, but it is not the manuscript engine.
- **Literature tab:** queries `/api/literature` and renders records; it does not
  produce citations.
- **Annotations:** optimistic React state, then a chat prompt
  (`sendChatMessage`), marked `addressed` when the transport accepts the
  message. No persistence, no review row, no link to a claim.

### 6.3 Truth defects in the generator

1. **Figures are decorative and unreferenced.** `create_discovery_manuscript`
   attaches `crystal_lattice.svg`, `bloch_sphere.svg`, and
   `energy_convergence.svg` (`manuscript.py:570-572`). None is included by
   `compile_latex()`; the PDF (if one were built) would contain no figures.
   The crystal and Bloch graphics are fixed illustrations, not plots of the
   run's data.
2. **Empty convergence figure.** With no energies, `generate_convergence_svg`
   emits axes, a title "Energy Convergence vs Analytical Anchor", and an
   **empty** polyline (`manuscript.py:283-310`). A reader sees a convergence
   chart with no convergence in it.
3. **Sections are duplicated, not composed.** `export_project` writes
   `sections/*.tex`, but `compile_latex` inlines each section's content. The
   two representations can drift, and the "modular project" is cosmetic.
4. **No build.** Nothing invokes LaTeX, so none of the above is caught by a
   compiler, and no test asserts a buildable document.
5. **Review provenance is asserted.** Receipts are listed, not resolved
   (K-04). The module docstring's claim that the manuscripts endpoint records
   the review into the ledger was false; corrected in this audit.

### 6.4 Gap matrix: publication capability

| Capability | Local-private bar | Present | Gap |
|---|---|---|---|
| Draft from measured results | Server-generated, tenant-scoped | Browser template only | **K-01** |
| Manuscript persistence | Versioned row + content hash | None | **K-01/K-07** |
| Peer review | Ledger-verified evidence, recorded decision | Library rule engine, caller-attested | **K-04** |
| Human signoff | Ed25519 over content hash, verified server-side | Verifier exists, unserved | **K-04** |
| PDF build | Deterministic, reproducible, hashed | No TeX engine, no build step | **K-03** |
| Figures | Generated from measured data, referenced in document | Static/unreferenced | **K-03** |
| Bibliography | Verified entries, stable keys | Empty unless caller supplies | **K-09/K-11** |
| Deposit staging | Manifest + checksums, human upload | Library-only | **K-09** |
| Artifact retention | Registry + export | None | **K-07** |
| Regeneration | Same inputs ⇒ same bytes | Not tested | **K-03** |

---

## 7. Implementation plan

Slices are ordered by dependency. Each slice is done only when its acceptance
criteria are executable and green; none of them is satisfied by prose.

### R0 — Truth containment (P0, small)
- Stop presenting unbuilt capabilities as available: no "publish", "download
  PDF", or "submit for review" affordance until R3/R4 exist.
- Make figures honest: attach the convergence figure only when energies exist;
  label crystal/Bloch as illustrations, or drop them from manuscript metadata.
- Keep the "DOI is minted at deposit time" refusal.
- **Accept:** manuscript metadata cannot claim figures the document does not
  reference; a no-energy manuscript contains no convergence chart; docstrings
  match behavior.

### R1 — Server-side report and manuscript persistence (P0)
- New SQLite tables: `manuscripts` (id, session, tenant, title, state,
  content_hash, created/updated), `manuscript_sections`, `manuscript_reviews`,
  `manuscript_artifacts` (kind, path, sha256, bytes).
- Go routes with tenant binding and append-only events:
  `POST/GET /api/sessions/{id}/manuscripts`,
  `GET/PUT /api/sessions/{id}/manuscripts/{mid}`,
  `POST /api/sessions/{id}/manuscripts/{mid}/reviews`,
  `POST /api/sessions/{id}/manuscripts/{mid}/signoff`,
  `GET /api/sessions/{id}/manuscripts/{mid}/artifacts`.
- Build the draft **server-side** from the campaign projection (same source as
  the signed export), never from a client template.
- **Accept:** tenant-scoped CRUD, content hash stable across reads, 401/403 for
  cross-tenant access, OpenAPI + contract-inventory entries, integration tests
  against a real ledger.

### R2 — Evidence binding (P0)
- Resolve every `provenance_receipt` against ledger records; unknown receipts
  falsify reproducibility (no silent pass).
- Resolve the Lean proof hash against the comparator record that produced it;
  no hash-only acceptance.
- Append `peer_review` and `manuscript_signoff` events; make the recorded
  decision the projection of record.
- **Accept:** a forged receipt cannot reach ACCEPT; a replayed review cannot be
  duplicated; ledger `verify` stays clean after the new event types.

### R3 — Reproducible build (P0)
- Add a TeX toolchain (Tectonic preferred for determinism/size) to a build
  service or the bridge image; build in an isolated job with a fixed
  `SOURCE_DATE_EPOCH`.
- Emit `main.tex` that `\input`s `sections/*.tex`, `\includegraphics` for every
  declared figure, and a resolved `references.bib`.
- Store `manuscript.pdf`, `main.tex`, `references.bib`, and figure files with
  SHA-256; record the build hash.
- **Accept:** two builds of the same manuscript produce byte-identical output
  (or a documented deterministic exception); a missing figure or bib key fails
  the build; a compile test runs in CI.

### R4 — Console publication workflow (P1)
- Results Studio: draft → review → signoff → build → download, backed by R1/R3
  routes; annotations persist as `manuscript_reviews` and are included in the
  review projection.
- Show the build hash and the signoff state; never show "published".
- **Accept:** browser test covers draft persistence across reload, annotation
  persistence, signoff refusal on content change, and download of the built
  PDF.

### R5 — Deposit staging (P1)
- Generate the Zenodo manifest from the **built** artifacts (not from a
  regenerated LaTeX string), with checksums and the signoff.
- Refuse staging without an ACCEPT review and a valid signoff; keep
  human-upload-only, no token, no auto-publish.
- **Accept:** manifest references exactly the stored artifacts; a changed
  artifact after signoff refuses staging.

### R6 — Operations hardening (P1)
- Encrypted off-host backup profile (`age` recipient set,
  `REQUIRE_ENCRYPTION=1`), or a signed plaintext waiver.
- `KUBONA_PREFLIGHT_REQUIRE_BACKUP=1` once the profile is live.
- A real restore-from-backup acceptance step (not only `restore-drill`).
- Metrics: job queue depth/age, bridge up/refusals, backup age, chain faults,
  settlement skips; alert on backup staleness and queue age.
- **Accept:** `scripts/local-deployment-check.sh --expect-single-lab` plus a
  documented backup-restore drill pass on a fresh host; `/metrics` exposes the
  new gauges; a stale backup produces a non-zero preflight.

### R7 — Bibliographic integrity (P2)
- Persist literature records used by a manuscript; emit BibTeX with stable
  keys; bind citations to claims and to the review.
- Record novelty distance as measured or explicitly unmeasured.
- **Accept:** a manuscript's bibliography resolves to stored records; a claim
  cites a real reference or says it has none.

---

## 8. Verification log (commands and outputs)

Key commands run for this audit (abridged):

```
docker compose --env-file .env -f platform/docker/compose.prod.yml ps
curl -fsS http://127.0.0.1:8080/api/health
curl -fsS http://127.0.0.1:8080/api/ready
curl -fsS http://127.0.0.1:8080/api/access
curl -fsS -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/tenants
curl -fsS -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/api/sessions
curl -sS -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:8080/api/science/manuscripts
curl -sS -o /dev/null -w '%{http_code}' http://127.0.0.1:8080/api/reports
docker exec kubona-prod-backup-1 kubonad backup-status --dir /backups --max-age 26h
docker exec kubona-prod-backup-1 kubonad verify --db /backups/kubona.db.backup-20261118T053638Z
docker exec kubona-prod-kubonad-1 kubonad restore-drill --db /home/kubona/.kubona/kubona.db
docker exec kubona-prod-kubonad-1 kubonad preflight --db … --backup-dir /backups
docker exec kubona-prod-kubonad-1 kubonad fk-audit --db …
curl -fsS -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/metrics
cd platform/core && go build ./... && go test ./pkg/api -run 'TestNotebooks|TestExperimentExport|TestClaimReview'
cd platform && ./.venv/bin/python -m pytest tests/test_manuscript.py tests/test_phase6.py -q
python3 scripts/wiki-lint.py --strict
python3 scripts/repo-counts.py --check
python3 scripts/console-contract-inventory.py --check
scripts/local-deployment-check.sh --expect-single-lab
```

The last command **fails on this host** with
`mode=multi_lab single_lab=false`, which is the reproduction for K-02. It
passes on a fresh volume (CI's `docker-smoke` job now asserts it).

---

## 9. Residual risk and operator-only items

- **Operator decision required:** the fresh-volume reset that would restore
  `local_private`. This audit did not delete the operator's ledger or the E2E
  labs. The procedure is documented in `docs/local-deployment.md` ("Reset a
  compose ledger") and deliberately preserves a verified snapshot.
- **Operator-only evidence:** manual VoiceOver/NVDA pass, reviewer-key
  confirmation on the deployment host, paid-QPU instance/run id or invoice,
  runtime rate-card record.
- **Not verified by this audit:** hosted CI status of `d9a0c1ab` (the local
  gates above were run, but this page does not claim a protected remote run).
- **Deliberately out of scope:** multi-host/HA, public SaaS multi-tenancy,
  OTel/Sentry SaaS backends, plugin marketplace.

---

## 10. Corrections to prior claims

| Prior claim | Correction |
|---|---|
| "Automated bilingual publication and peer-review tribunal" (wiki concept page, `adopted`) | The engine is a deterministic Python rule set; it is **not served**, the review is not adversarial/LLM-authored, and the bilingual output is a title/abstract stub. See the correction block added to `wiki/concepts/autonomous-manuscript-synthesis-and-peer-review.md`. |
| "Results Studio publishes with DOI/Zenodo" | The studio generates client-side exports and a draft LaTeX string. DOI minting is correctly refused; Zenodo staging is library-only. |
| `ReviewEvidence` docstring: "the manuscripts endpoint records the review into the ledger" | No endpoint records it; the `console_api` handler does not append an event. Docstring corrected in `platform/kubona/science/manuscript.py`. |
| "Acceptance check proves local-private posture" | It proved loopback/health/console only. It now also asserts `/api/access` when `--expect-single-lab` is passed, and CI passes the flag. |


---

## 11. Implementation record (2026-11-18, same session)

The R0–R7 plan was executed against the audit findings. Every claim below is
backed by a test or a live run.

### Findings status

| ID | Status | Evidence |
|---|---|---|
| K-01 pipeline not deployed | **Fixed** | Go routes + bridge RPC + publication panel; live draft/build on a disposable stack |
| K-02 single-lab posture mismatch | **Gate added** | `--expect-single-lab` fails on the polluted host and passes on a fresh volume (live both ways) |
| K-03 unbuildable manuscripts | **Fixed** | pdfTeX in the bridge image; `\input` sections; deterministic byte-identical rebuild; artifact hashes |
| K-04 caller-attested review | **Fixed at the boundary** | receipts resolved against signed campaign receipts; proof hashes resolved against comparator events; unresolved named; signoff binds content hash |
| K-05 backup trust | **Partly fixed** | real-backup restore check script; encryption requirement enforced by `backup-status`/preflight; actual encryption/waiver remains operator-owned |
| K-06 observability | **Improved** | job/manuscript/bridge/backup gauges; `kubona-backups` mounted read-only in kubonad |
| K-07 report persistence | **Fixed for manuscripts** | manuscript rows, sections, citations, reviews, artifacts, signoffs |
| K-08 review split | **Partly fixed** | claim review unchanged; manuscript signoff is a first-class persisted action; annotation persistence still open |
| K-09 bibliography/deposit | **Partly fixed** | citations persist + BibTeX; deposit manifest staged from built artifacts; no auto-upload by design |
| K-10 compose lifecycle | **Fixed (docs + mounts)** | reset procedure, `down -v` warning, backup mount, preflight caveat |
| K-11 citations display-only | **Partly fixed** | caller citations become BibTeX and are cited; literature auto-feed and claim binding remain |

### Live verification (disposable stack, fresh volume)

```
GET /api/access                         -> mode:"local_private" single_lab:true
POST /api/experiments (H2 VQE)          -> job 2a2ff5a4… completed
campaign                                -> notebook delta 15.19 mHa, CONTESTED
POST …/manuscripts {build:true}         -> 201, state built, 8 artifacts
main.pdf                                -> %PDF-1.7, sha256 eb77c3db…,
                                           X-Kubona-SHA256 matches bytes
POST …/build (again)                    -> build hash eb77c3db… identical
POST …/signoff (anonymous)              -> 403 forbidden
POST …/signoff (operator)               -> 200, signature over
                                           manuscript-signoff:<id>:<hash>:<expiry>
POST …/deposit                          -> 409 review_not_accepted (REVISE)
ledger events                           -> manuscript_drafted, manuscript_built,
                                           manuscript_signed
kubonad verify                          -> chain clean (new event types included)
GET /metrics                            -> kubona_jobs{status=…},
                                           kubona_manuscripts{state="built"},
                                           kubona_bridge_configured 1
```

The first live build exposed a real defect: the primary VQE point lives in
`campaign.measurement`/`notebook`, not `campaign.experiments`, so the results
section said "no measured result" for a completed run. The composer now
promotes the primary point exactly once; a regression test pins it.

### Backup-file restore (live main stack)

`scripts/backup-restore-check.sh` restored
`kubona.db.backup-20261118T053638Z` into a scratch ledger, re-verified the
chain, and reported `clean`. This is the check `restore-drill` cannot make: it
reads the scheduled artifact, not a fresh snapshot of the live DB.

### What is still open

- Operator-only: age recipient + `KUBONA_BACKUP_REQUIRE_ENCRYPTION=1` (or a
  signed plaintext waiver), `KUBONA_PREFLIGHT_REQUIRE_BACKUP=1`, and the
  fresh-volume reset for the polluted host ledger.
- R4 annotation persistence as manuscript review notes.
- R7 literature auto-feed / claim→citation binding.
- No production comparator producer, so no manuscript can honestly reach
  ACCEPT yet; the gate is wired and tested with a synthetic event.
