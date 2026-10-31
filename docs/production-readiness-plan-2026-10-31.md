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
- Generated counts: 122 Go routes, 745 Go tests, 467 console tests, 179 Python
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
| P0-1 | Restore is a manual shell procedure; no `kubonad restore` command, no atomic verified swap, no restore-drill test in CI. | A backup that has never been restored is not a recovery plan. Operator error during an incident is likely. | `docs/backup-restore.md` restore drill; `platform/core/cmd/kubonad` has `backup`/`backup-prune`, no `restore`. |
| P0-2 | No scheduled, encrypted backup path in the compose deployment. | A verified backup that is not taken on a schedule still loses data. Plaintext off-host copies expose spend/provenance. | `platform/docker/compose.prod.yml` has no backup service; backup doc leaves cadence/encryption manual. |
| P0-3 | Schema versioning exists, but migrations have no mandatory pre-migration verified backup or automated compatibility test against the prior on-disk schema. | An upgrade failure needs a known-good rollback point; a newer binary is protected, an upgrade in progress is not. | `platform/core/pkg/ledger/ledger.go` (`CurrentSchemaVersion`, `checkSchemaVersion`, `recordSchemaVersion`). |
| P0-4 | Operational documentation drift. `docs/backup-restore.md` still says there is no schema version table; the wiki gate snapshot cites an older head. | Operators make decisions from stale safety claims. | `docs/backup-restore.md` “Current limits”; `wiki/concepts/remaining-work.md` state correction. |
| P0-5 | Vendor runtime rate card is not reconciled; embedded card becomes stale 2026-11-29. Production cloud pricing intentionally refuses without a runtime card. | Cloud LLM/Atlas/QPU estimates cannot be shown as vendor-current; local simulator remains unaffected. | `docs/rate-cards.md`; `docs/remaining-todos.md` production blockers. |
| P0-6 | Historical QPU settlement row is unreconciled: vendor completed/charged 2 s while the local row is cancelled/$0.00. | Ledger spend is not a complete picture of that run until operator evidence is attached. | IBM job `dbha28pq260s73busu70`; local job `39b5e6b832cd4ae3`; `docs/remaining-todos.md`. |

### P1 — production hardening

| ID | Gap | Why it matters |
|---|---|---|
| P1-1 | SSE has no event cursor/`Last-Event-ID` replay; the server emits the latest five events every 2 s and the client dedupes by id. | A reconnect longer than five events can miss rows until the next full poll; replay semantics are implicit. |
| P1-2 | Response contracts cover major surfaces but are advisory, hand-maintained, and not a generated OpenAPI/schema artifact. | Drift is logged, not prevented; new endpoints can ship without a contract. |
| P1-3 | Secrets are `.env`-based; the ledger is plaintext SQLite. | Acceptable for a single trusted host, but no Docker secrets/KMS/at-rest encryption path. |
| P1-4 | `doctor --strict`, backup freshness, disk pressure, and rate-card expiry are not enforced at stack start or surfaced as alerts. | Failures are detectable but not proactive. |
| P1-5 | Deep accessibility audit is incomplete: contrast, every modal, reduced-motion coverage, and screen-reader transcripts. | Keyboard/mobile smoke is covered; full WCAG pass is not. |
| P1-6 | Canvas layout persistence stores only 2D/3D mode, not camera/zoom/selection. | “Save layout” does not restore the view an operator arranged. |

### P2 — enhancements, not local-production blockers

- API versioning (`/api/v1`) and a generated OpenAPI document.
- Encrypted ledger at rest / KMS-backed secret injection.
- Opt-in OTLP/Sentry exporters, scrubbed and sampled.
- Containerized stdio MCP broker (host-spawned stdio remains supported).
- HA/multi-host operation (explicitly outside the local-private scope).

## Implementation plan

### Phase 1 — recovery and upgrade safety (P0)

1. **Add `kubonad restore`.**
   - `--from`, `--db`, `--dry-run`, `--apply`.
   - Verify the source chain and `PRAGMA integrity_check` before touching the
     live path.
   - Preserve the current DB as `*.pre-restore-<UTC>`, remove stale WAL/SHM,
     atomically install the verified copy, then re-verify.
   - Refuse while the daemon holds the state lock.
   - Tests: happy path, chain-fault refusal, existing-target preservation,
     dry-run no-write, post-restore verify.
2. **Add a scheduled backup profile.**
   - Compose profile or sidecar: verified `kubonad backup`, retention via
     `backup-prune`, optional `age`/`gpg` encryption.
   - Surface last-success timestamp and failure count.
   - Document restore cadence and key custody.
3. **Gate migrations on a verified backup.**
   - If the on-disk schema is older than the binary, require a verified backup
     path (or emit one automatically before migration).
   - Add a fixture test that upgrades a prior-schema DB and reopens it.
4. **Make docs truthful.**
   - Correct the schema-version claim, current gate head, and restore procedure.
   - Link this report from `docs/remaining-todos.md`.

**Exit criteria:** a scratch restore passes `kubonad verify` and `/api/ready`;
an upgrade from the prior schema refuses without a backup and succeeds with
one; backup age is visible to the operator.

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
