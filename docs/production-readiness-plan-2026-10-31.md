# Production readiness report and plan — 2026-10-31

**Scope.** This report defines production as **fully functional for a local
private deployment**: one operator, one host (or a private tunnel to it),
loopback-published console/API, local ledger and simulator, optional QPU and
BYOK cloud LLM. It is not a public-SaaS readiness claim.

**Owner:** maintainer. **Review cadence:** after each phase and before any release tag.

**Current verdict.** The code gate is green and a fresh local compose stack is
functional. The platform is ready for a **controlled fresh-volume deployment**
with an operator who follows the runbook. It is **not yet operationally
production-ready for an unattended host** because backup/restore, migration
rollback, and operator reconciliation still depend on manual shell work and
outside evidence.

## Evidence at this revision

- `develop` and `main` at the green code gate; CI 20/20 and Console E2E green.
- Generated counts: 122 Go routes, 765 Go tests, 468 console tests, 180 Python
  test files, 29 Rust tests, 38 MCP tools, 154 wiki pages.
- Local compose acceptance: loopback-only ports, fresh state root, health/ready,
  console boot, dispatch, codesign, refusal state.
- Backup, FK audit/quarantine, gate/rate-card audit, key generation, doctor,
  and release dry-run are implemented.
- Fresh production ledger is the supported path; the historical host DB is
  forensic because of 606 pre-existing chain faults.

## Gap register

### P0 — blocks unattended local production

| ID | Gap | Why it matters | Evidence |
|---|---|---|---|
| P0-1 | **CLOSED 2026-10-31.** `kubonad restore` with dry-run/apply, source chain + integrity verification, atomic install, preservation directory, and rollback-on-verify-failure. | No longer manual. | `platform/core/cmd/kubonad/restore.go`; restore tests. |
| P0-2 | **CLOSED 2026-10-31.** Opt-in compose `backup` profile runs verified scheduled backups, optional `age` encryption, retention, and `backup-status` freshness/failure checks. | No longer manual cadence. | `platform/docker/backup-loop.sh`; `backup` service; `backup-status`. |
| P0-3 | **CLOSED 2026-10-31.** Older schemas take a verified `VACUUM INTO` pre-migration snapshot; a failed snapshot/chain refuses the upgrade. | Upgrade has a rollback point. | `platform/core/pkg/ledger/ledger.go`; `migration_backup_test.go`. |
| P0-4 | **CLOSED 2026-10-31 (code/docs).** Restore, backup-status, scheduled profile, and migration behavior documented; stale schema-version claim removed. | Operators can follow the real path. | `docs/backup-restore.md`, `docs/local-deployment.md`. |
| P0-5 | Vendor runtime rate card is not reconciled; embedded card becomes stale 2026-11-29. Production cloud pricing intentionally refuses without a runtime card. | Cloud LLM/Atlas/QPU estimates cannot be shown as vendor-current; local simulator remains unaffected. | `docs/rate-cards.md`; `docs/remaining-todos.md` production blockers. |
| P0-6 | Historical QPU settlement row is unreconciled: vendor completed/charged 2 s while the local row is cancelled/$0.00. | Ledger spend is not a complete picture of that run until operator evidence is attached. | IBM job `dbha28pq260s73busu70`; local job `39b5e6b832cd4ae3`; `docs/remaining-todos.md`. |

### P1 — production hardening

| ID | Gap | Why it matters |
|---|---|---|
| P1-1 | **CLOSED 2026-10-31.** SSE emits `id:` frames, honors `Last-Event-ID`/`last_event_id`, replays the gap from the ledger cursor, and resets a future cursor to the tip. | Reconnect now replays rather than skipping rows. |
| P1-2 | **PARTIAL 2026-10-31.** Generated OpenAPI 3.1 documents 60 paths with 58 response schemas; draft/OpenAPI drift is gated. Drift now reaches the API banner, and `KUBONA_STRICT_CONTRACTS=1` fails the response. The 68 unmodeled functions still need schemas. | Runtime validation is strict on demand; unmodeled paths remain explicit. |
| P1-3 | Secrets are `.env`-based; the ledger is plaintext SQLite. | Acceptable for a single trusted host, but no Docker secrets/KMS/at-rest encryption path. |
| P1-4 | **PARTIAL 2026-10-31.** `doctor --strict` reports backup freshness/failure; `kubonad preflight` gates stack start on rate/ledger/FK/backup; optional `KUBONA_ALERT_WEBHOOK_URL` posts preflight and scheduled-backup failures. Disk/rate-specific alert routing remains open. | Local start failures and backup failures can reach an external webhook. |
| P1-5 | **PARTIAL 2026-10-31.** E2E rejects unnamed controls/unlabeled fields/missing alt/multiple main landmarks, checks reduced-motion disables ambient animation, and verifies the command palette focus trap. Contrast, other modals, and screen-reader transcripts remain. | Structural, reduced-motion, and modal-focus regressions gate E2E. |
| P1-6 | **PARTIAL 2026-10-31.** Canvas layout now stores the 3D camera position and orbit target (zoom is represented by camera distance); point selection is still campaign state, not layout. | A saved observatory view restores on reopen. |

### P2 — enhancements, not local-production blockers

- API versioning (`/api/v1`) and a generated OpenAPI document.
- Encrypted ledger at rest / KMS-backed secret injection.
- Opt-in OTLP/Sentry exporters, scrubbed and sampled.
- Containerized stdio MCP broker (host-spawned stdio remains supported).
- HA/multi-host operation (explicitly outside the local-private scope).

## Implementation plan

### Phase 1 — recovery and upgrade safety (P0) — CODE LANDED 2026-10-31

1. **`kubonad restore`** — landed with dry-run/apply, chain + integrity
   verification, atomic install, preservation directory, rollback on failed
   post-install verification, and daemon-lock refusal.
2. **Scheduled backup profile** — landed as `backup` compose profile with
   `backup-loop.sh`, optional `age`, sha256 receipts, retention, and
   `backup-status` freshness/failure health.
3. **Migration backup gate** — landed: older schemas take a verified
   `VACUUM INTO` snapshot before migration; failure refuses the open.
4. **Docs truth** — restore, backup profile, backup-status, and schema upgrade
   behavior documented; stale schema-version claim removed.

**Exit criteria:** a scratch restore passes `kubonad verify` and `/api/ready`;
an older schema refuses when its pre-migration snapshot cannot verify; backup
age and last failure are visible to the operator. Remaining Phase-1 work is
operator evidence for P0-5/P0-6.

### Phase 2 — live-state and contract hardening (P1)

1. SSE: emit `id:` frames, honor `Last-Event-ID`, replay from the ledger
   cursor, and keep polling as the fallback. Add a reconnect E2E.
2. Contract generation: derive a checked schema inventory for every console
   path; keep runtime validation non-fatal in production and strict in tests.
3. Preflight: run `doctor --strict` before `up`, surface rate-card/backup/disk
   failures in `/api/ready` or the console banner.
4. Secrets/at-rest: optional Docker secrets/OpenBao path; document the
   plaintext-local boundary; add ledger encryption only with a key-custody plan.
5. Accessibility: axe/contrast/keyboard sweep; screen-reader notes for chat,
   tribunal, and recorder.
6. Canvas: persist camera, zoom, selection, and lens history view state with a
   versioned layout schema.

### Phase 3 — operator reconciliation and optional enhancements

- Attach the vendor runtime card and paid IBM evidence; reconcile the
  historical settlement row with written operator approval.
- Add API versioning/OpenAPI if external agent clients need a stable contract.
- Evaluate OTLP/Sentry only after a backend/DSN choice; keep default off.
- Keep HA, public ingress, and a containerized MCP broker out of scope unless
  the deployment shape changes.

## Definition of done for local private production

- Fresh install: compose up, health/ready, login, local LLM, simulator, and
  optional QPU path all work on a clean volume.
- Recovery: scheduled encrypted backup, `kubonad restore`, restore drill, and
  post-restore verify are automated and tested.
- Upgrade: schema downgrade refusal plus migration backup/rollback tested.
- Truth: doctor/console surface stale rate cards, backup age, disk pressure,
  and unreconciled settlement without inventing values.
- Security: loopback-only, fail-closed writes, non-extractable browser key,
  encrypted recovery kit, and documented secret custody.
- Quality: full CI/E2E green on the shipped commit; no known P0/P1 gap without
  an explicit operator-owned exception.
