---
type: concept
name: Remaining work queue
date: 2026-10-17
updated: 2026-10-28
tags: [plan, queue, production]
sources: [source-gap-analysis-plan, source-frontier-repo-audit-2026]
status: tracked
---

# Remaining work queue

> Every item the program still owes, with its owner and unblock
> condition. Landed work is NOT here (see [[full-tree-roadmap]] Landed,
> [[rea-adoption-plan]], [[cloudroom-adoption-plan]]). Durable blueprint
> questions live in [[questions]] — this page tracks execution leftovers
> only. Screened-out items name their reason; nothing rots silently.

## Phase-0/1/2 execution record (2026-10-28, `develop`)

- Phase-0 + Phase-1 hardening landed on `develop` with the full gate
  green (20 checks, run 40786519756): settlement, admission crypto, A2A
  binding, approval gate, bridge confinement, manuscript evidence, route
  policy; then terminal settlement everywhere, atomic admission, sandbox
  confinement + escape tests, principal-bound tenancy, RAM admission,
  Rust measurement fidelity, CI depth (vet/gofmt, ruff ratchet, clippy,
  semgrep, Trivy, coverage floors).
- Phase-2 credibility hygiene landed: stale README numbers corrected to
  the recounted values at that commit (116 routes, 154 wiki pages, 57
  console files / 276 vitest tests); those values are now stale again.
  The generated source of truth is `docs/repo-counts.md` (2026-10-29:
  120 routes, 74 console files / 434 tests). The simulator-backend parity
  contract is `platform/tests/test_simulator_parity.py`; all 7 dependabot
  branches were triaged (PRs #1–#7 closed unmerged; branch deletion was
  blocked by the repository ruleset).
- Some §A deeds were closed after this list was written; closed entries
  are marked below and kept as a record. No §B queued item was retired by
  these phases; landed program items continue to live in
  [[full-tree-roadmap]] Landed per this page's rule.

> ⚑ Stale-state correction (2026-10-29): the 20-job **push** gate is green
> (run `40914979580`), but the **nightly thesis-scale** gate is red (runs
> `40854405074`, `40666624246`, `40478035440`; Slice60 was cancelled by
> runner shutdown, not by an assertion). Do not read "full gate green" as
> covering nightly until it is green. Counts are generated in
> `docs/repo-counts.md`.

## Local-first production (2026-10-30, landed)

> Scope correction: "production ready" here means **fully functional for a
> local private deployment**, not a public SaaS. The supported shape is one
> operator on their own machine/lab host, loopback only; the only cloud
> egress is QPU dispatch and opt-in cloud LLM (BYOK).
>
> - ✅ **B2 local ingress:** `compose.prod.yml` publishes `127.0.0.1:8080` /
>   `127.0.0.1:3000` only, services are read-only with dropped caps, and
>   `scripts/local-deployment-check.sh` refuses a public listener. Live
>   proof 2026-10-30: fresh volume, health/ready/console green, check exit 0.
>   Full runbook: `docs/local-deployment.md`.
> - ✅ **C5b governed rerun:** `POST /api/codesign/rerun` is session-scoped (anonymous refused),
>   rate-limited, idempotent (`request_id` required), bounded by steps/
>   tokens/time/USD, and sealed as a `trial`. Cloud LLM is BYOK: the provider
>   bills the operator's key; Kubona records an estimate and never debits the
>   QPU session cap. Local providers are free. The bridge stays stateless
>   (new `spec` op; no LLM keys). Docs: `docs/llm-proposer.md`.
> - ✅ **B4 local diagnostics:** logs, `/metrics`, health/ready, request IDs
>   and local traces remain on-host; no telemetry by default. Sentry/OTLP
>   stay opt-in, not release requirements.
> - ✅ **Bootstrap:** `kubonad keygen` emits reviewer/operator Ed25519 seeds;
>   `.env.example` and the runbook name the token/key requirements.
> - ✅ **FK quarantine (2026-10-30):** 127 orphans quarantined from
>   `~/.kubona/kubona.db` (50 chat_messages, 40 campaign_versions, 23 jobs,
>   12 loops, 2 claim_records); `PRAGMA foreign_key_check` is clean and all
>   rows are preserved in `_fk_orphans`. The raw pre-repair snapshot is
>   `~/.kubona/backups/kubona-pre-fk-20261030T024721Z.db`
>   (sha256 `c15bffd6843e7263de8c64bb1a9d3b8d9c00c4898ba851cfce4990eb53146f3b`).
>   **Caveat:** `kubonad backup` refused it as a chain-clean backup (606
>   pre-existing ledger chain faults); the snapshot is SQLite-consistent
>   (integrity_check ok, 1470 events) but is forensic, not a verified
>   provenance backup.
> - Remaining operator-side for a production claim: vendor rate-card
>   reconciliation before 2026-11-11 (`docs/rate-cards.md`), a green nightly
>   thesis-scale proof, attaching the paid QPU run evidence, and resolving
>   those 606 pre-existing ledger chain faults before trusting a restore.
>   Provider keys are present in `.env`. These are not code gaps.
> - ✅ **Console rerun panel** (2026-10-30): Codesign Cockpit session picker,
>   request_id retry, step/token/USD bounds, sealed receipt; 439 console tests.
> - ✅ **MCP rerun wrapper** (2026-10-30): `cost/codesign_rerun` forwards to
>   the governed Go route with the operator bearer and a scoped budget envelope;
>   no LLM key in the MCP server. Catalog now 38 tools (regenerated).
> - ✅ **Deployment doctor** (2026-10-30): `kubona doctor --env-file .env`
>   reports rate-card freshness, FK orphans, key presence, LLM/air-gap posture,
>   bridge, and daemon readiness; `--strict` exits non-zero. On this host it
>   found 127 FK orphans in `~/.kubona/kubona.db` and a persisted
>   `active=nvidia`/air-gap-off setting with `KUBONA_LLM_CLOUD` unset — both
>   operator-visible now.
> - Optional remaining: opt-in OTel/Sentry exporters and a local release
>   dry-run/checksum script.
> - Short execution list: `docs/remaining-todos.md`. A new blocker surfaced
>   on 2026-10-30: the host ledger has **606 pre-existing chain faults**, so
>   the FK-quarantine snapshot is forensic, not a verified restore source.

## A · Operator deeds (blocked on the human — no slice can deliver these)

- `gho_…` token: `credential.helper=store` was dropped globally and
  `~/.git-credentials` was reported empty ([[risks]]), but the token
  revocation itself is not proven by the repo record. Treat this as
  **open until the token is revoked at the provider**, not merely removed
  from the helper.
- ~~Q-27 history flip~~ — ✅ CLOSED 2026-10-26: `git filter-repo` scrub,
  force-push, re-clone, and ruleset re-enable are recorded in [[risks]].
- ~~Advance `main` to `develop`~~ — ✅ CLOSED 2026-10-29: `main` and
  `develop` are both `5fda062`.
- Tenant reviewer key (`KUBONA_REVIEWER_PRIVKEY_HEX`): AGENTS and the
  2026-10-29 log say it is commissioned; this queue still said open. The
  authoritative state is the deployment environment, not the repo record.
  Confirm `reviewer_key_configured=true` on the target host before
  citing it.
- QPU credentials + backends: ✅ IBM credentials are present in `.env`
  (`KUBONA_IBM_QISKIT_TOKEN`, instance, `ibm_quantum_platform`, 120 s cap);
  per-run backend choice and a GPU host for local training remain operator
  choices, not missing secrets.
- `KUBONA_CONTACT_EMAIL` (console mailto + polite-pool), HF-token rotation,
  EPO registration.
- ~~First paid QPU run~~ — ✅ CLOSED 2026-10-28 (operator-confirmed paid run;
  see [[log]] and [[ibm-quantum-platform]]). Evidence note: this workstation's
  `~/.kubona/kubona.db` on 2026-10-30 holds only the 2026-10-29 free-plan
  `ibm_fez` probe, so attach the deployment ledger/run id before citing its
  measured backend/cost.
- DeepSeek-vs-oMLX transcript delta (keyed providers; harness ready).
- Device/emulator recalibration (keyed hardware; canary verifies
  live-vs-declared meanwhile per Q-14).
- Protogen venv re-pin (`grpcio-tools` 1.71.x) + regen Python pb2 stubs
  (`EvidenceEnvelope` message waits on it). ✅ CLOSED 2026-10-29: scratch
  venv `/tmp/protogen` re-pinned to 1.71.0, regen reproduced byte-identical
  output (1-line stamp delta only), `EvidenceEnvelope` imports cleanly
  under the pinned 5.29.6 runtime; deterministic regen proven.
- R-series defaults CONFIRMED 2026-10-29 (operator decision, no code change —
  all four were already implemented as recorded): R1 extend (+tenant
  mandatory, +mandate_ref optional; envelope.go both rails + byte-identical
  vectors); R2 tribunal-quorum authority = the four seated roles as required
  authorities + `ledger-receipt` for mandate settlement (AttemptClose
  enforces; Lean proof / human sign-off stay future authorities, not
  required today); R0 lanes fast+boundary required, changed advisory
  (CONTRIBUTING.md + CI mapping); P1-4 consolidation stays queued behind
  R1–R3 (single maintainer, gate is green without it — revisit when the
  contracts drive a need, not before).
- R6 gating policy RESOLVED 2026-10-29 (operator decision, recorded in
  `platform/core/pkg/agents/scoring.go` header): advisory-only, permanently.
  Substring heuristics are gameable — no gate may branch on scores. Scores
  surface in tribunal responses + the review queue (human prioritization).
- Failed-mandate review queue ✅ LANDED 2026-10-29: `GET
  /api/obligations/open` (lab-scoped list + close-readiness reasons) and
  `POST /api/obligations/{id}/close` (owner session or owner-key signature;
  bearer alone never sufficient; double-close idempotent; no oracle).

## B · Queued programs (unblocked, beyond a single slice)

- UI/Docker production program (report 2026-10-17: U0 → D1 → U1 → U2 → U3 → D2).
  > ✅ U0 LANDED: console tests + lint in CI (19 lint errors fixed:
  > 6 render-phase key-change resets — no stale-session bleed — + 13
  > typed test casts); quantum-runner == pins measured from the shipped
  > image + rebuilt + import-verified; openbao digest-pinned + dev-only
  > header.
  > ✅ D1-skeleton LANDED: Dockerfiles for kubonad (CGO/sqlite,
  > nonroot, healthcheck) / console (standalone, CI-identical toolchain,
  > nonroot) / bridge (pinned extras, grpc healthcheck); `.dockerignore`
  > ×3; `compose.prod.yml` (health-gated, resource-limited, vault +
  > pgvector deliberately absent); CI `docker-smoke` job wired into the
  > release gate. Live findings fixed: fresh state roots never became
  > ready (daemon now ensures the models dir, regression-tested);
  > plaintext-compose needs the explicit opt-in the server demands.
  > Next: U1 (health banner, tenant writes, loop override, metasearch UI).
  > ✅ U1 LANDED 2026-10-17: backend status strip (down vs degraded vs
  > healthy, 15s poll — ends "empty vs down" ambiguity); loop
  > pause/resume/kill roster in the recorder (status-gated buttons,
  > confirm on kill, reasons verbatim, proven live pause→resume→kill);
  > metasearch approve/reject/apply panel in the cockpit (+ the missing
  > reject client; signer `console-operator`; proven live list-shape +
  > gated refusal); LabsView tenant writes (ceiling/suspend/refund +
  > admin bootstrap) with byte-exact admin envelopes (Go-generated
  > golden hashes, 4 vectors; full live cycle on compose: bootstrap →
  > ceiling → wrong-key 403 → suspend → frozen-write 403 → resume →
  > no-bearer 401).
  > ✅ U2 WIRED 2026-10-17: session export/import + ledger export
  > (JSON/Markdown download, import switches to the session) in the
  > recorder — proven live as export→import idempotent replay
  > (replayed:true, no duplicate); re-synthesize-campaign button in the
  > tribunal room (same 75% quorum bar as commit; proven live, full
  > campaign state returned). Correction to the U2 brief: the modal's
  > "Synthesize Reference Baseline" path was already wired server-side
  > (`fresh:false` synthesizes at creation) — only re-synthesis of
  > existing sessions was missing.
  > ⏸️ U2 DECIDED 2026-10-17 (wire-or-decide outcomes, each with trigger):
  > autoresearch UI deferred — engine is eval-driven (nightly canary),
  > no operator workflow designed (who starts loops, with what artifact,
  > toward what end); trigger = workflow design, no theater before it.
  > krylov / mitigation / fidelity / ADMET / discovery-pipeline console
  > forms declined — agent-consumed compute stages (task_executor,
  > campaign_synth, bridge); the console already surfaces their results,
  > a hand-run form duplicates agent paths with no operator action
  > behind it (discovery curation belongs to the blueprint layer,
  > [[questions]] §D). notebooks-endpoint UI declined — ResultsStudio
  > already renders notebook content from campaign state (redundant read
  > path). canvas layout save/load deferred — waits for an interactive
  > SpatialCanvas (currently static display). server-signed mandate
  > console path declined — client-mint is the console path,
  > server-signed stays daemon/CLI-side; unify at IdP (U3).
  > ✅ UI COMPLETION 2026-10-17 (opencode-deepseek is the dev LLM; oMLX
  > parked): fetchJob threads session_id (job detail/JobProgress 400s
  > fixed, proven live old-400/new-200 on current build — which also
  > exposed the dev daemon as a stale binary: the /tmp/kubonad-s8a file
  > was removed during cleanup while PID 54943 lives on; rebuild from
  > current tree before any restart); fleet reports credential_state
  > (ready vs credentialless) + measured bridge liveness (dial, not the
  > never-nil client) with a NO CREDENTIALS console badge, proven live;
  > copilot no-proposal hint names the offline exits when the engine
  > notice fires. Bridge red test fixed to assert the W2.18 refusal at
  > equal strength + stale Tier-1 comment corrected; the bridge suite now
  > runs in CI (it lived outside testpaths, which is how the red went
  > unnoticed). Findings needing the operator: opencode returns 402
  > INSUFFICIENT FUNDS — tribunal/chat/LLM features are down until
  > top-up (oMLX serves locally with models as the fallback; repoint
  > roles on order).
  > ✅ M1–M6 LANDED 2026-10-17 (no hardcoded models): Go NVIDIA plumbing
  > (provider type + endpoint + env-key wiring, zero model IDs);
  > env-driven selection (KUBONA_LLM_PROVIDER, *_MODEL incl. NVIDIA_MODEL,
  > per-role overrides, .env.example); settings schema_version +
  > stale-blob stamping (no value rewrites); write-path validation
  > (unknown provider/empty model refuse, unchanged entries
  > grandfathered); GetModelForRole names the exact env var + refuses
  > silent fallback; kill-switch plumbed end-to-end (snapshot + update +
  > console toggle); RecordSuccess last_ok. Console: editable provider
  > defaults, active-provider select, free-text models with datalist
  > suggestions, last-success display, active-provider-biased fetch,
  > new-agent model required. Pricing: LoopIterationBound refuses unknown
  > metered models (new unpriced_model code) instead of over-reserving.
  > Python router refuses instead of inventing (discovery first, explicit
  > only); seeds unpinned (Go custom.go + Python user_defined.py);
  > legacy PUT /api/settings now validated. U3 runtime backend URL via
  > /api/config (no rebuild per backend). D2 digest-pinned base images
  > (all Dockerfiles rebuild + serve green).
  > ✅ M0 DONE 2026-10-17: daemon verified idle (no live loops/jobs, 38
  > sessions persisted, loops reconcile on boot), killed gracefully,
  > restarted on a current-tree rebuild with .env (NVIDIA key + provider
  > + model), all 5 roles repointed via authenticated save and proven
  > live — QUARK via private/nvidia/nemotron-3.5-super in 1.1s.
  > ✅ Kill-switch UI DONE: DSML toggle in LLM Settings + round-trip
  > test (toggle flips, persists via save).
  > ✅ A1–A4 Session-based Auth (U3 remainder) DONE 2026-10-17:
  > Ed25519 challenge-response login (`POST /api/auth/challenge|login|logout|revoke`,
  > `GET /api/auth/whoami|sessions` in `pkg/api/auth_session.go`) bound to each
  > lab's registered admin pubkey; `httpOnly` + `SameSite=Lax` cookie
  > (`kubona_session`, SHA-256 token hash only in `auth_sessions`, 60s single-use
  > challenge nonce, 12h TTL via `KUBONA_SESSION_TTL`); `csrfAllowed` Origin check
  > on unsafe cookie-authenticated methods (including `PUT /api/llm/settings`,
  > `POST /api/auth/logout`, and `POST /api/auth/revoke`); `RotateAdminKey`
  > immediately revokes active tenant sessions; `AuthPanel.tsx` mounted in
  > `LabsView.tsx` with WebCrypto Ed25519 signing + Vitest coverage; 7 missing Go
  > ledger event types registered in `platform/contracts/event-types.json` +
  > `pkg/ledger/events.go` (121 total rows: 80 Go + 41 Python).
  > ✅ LocalModelRegistry `ports=[]` fix DONE 2026-10-17: `registry.py` now uses
  > `DEFAULT_PORTS if ports is None else list(ports)` (and skips `_lms_endpoint()`
  > when `self.ports` is empty), so `test_llm_registry_has_no_stale_cache` passes
  > deterministically even when a local LLM server is active on `:8787`.
- P1-4 subsystem consolidation execution — design: [[agent-subsystem-consolidation]].
  > ✅ STEP 1 LANDED 2026-10-17: conformance freezes on all three
  > registry homes (Go + 2× Python) + boot-crash hardening
  > (negative-proven). Step 2 scoped: wire-shape freezes both sides,
  > canonical mapping left for the operator (three Go vocabularies,
  > not one). Steps 3–5 await operator scheduling + identity-home
  > decision. Page status stays `candidate` — correctly.
- Lean skill slices L1–L5 (plan: [[leanrea-adoption-plan]] §3, conflicts:
  [[leanrea-conflicts-and-remediation]] — S-sized, ordered L1 → L2-baseline
  → L3 → L2-rerun → L4 → L5 (LC3: baseline before change), none touch the
  trust boundary; distribution gated on the public flip (LC1).
  > ✅ L1 + L2-baseline LANDED 2026-10-17 (Phase L-A exit met):
  > `skills/kubona/SKILL.md` + pins, `bench/` harness + first numbers.
  > ✅ L3 + L2-rerun + L4 LANDED 2026-10-17 (Phase L-B exit met):
  > scribe tags (convention, R3 gate unchanged), bench delta zero on
  > record, doctor deepening + `mandate-check` (LC5 wording contract).
  > ✅ L5 LANDED 2026-10-17 (Phase L-C exit met): MCP catalog +
  > 29 keep-verdicts (P2-13 closed) + unscoped-tool guard + `not_found`
  > code. Lean program COMPLETE — all phases exited green.
- Bundle export endpoint — needs run-archival design (post-run turns
  cannot be reconstructed from events+chat; collective-level flow exists).
- Sidecar rotation — needs chain-aware reads (rotation breaks the
  tail-chained ledger assumption).
- S8c full hub — needs skills/packs/automations backends (reduced: no
  theater built).
- Steer — needs engine surgery (documented N/A: synchronous model has
  no background turn to steer into).
- ~~Assess principal threading on the remaining MCP servers
  (cost/execution/provenance write paths)~~ — ASSESSED 2026-10-17, no
  change (by design, threat argument): control already threads
  `requester` with label scoping (`test_control_tenants.py`); provenance
  reads thread `requester` via `readable()` with ambient resources
  public-only; provenance writes take caller-stated `label` (default =
  file's own tenant, files sharded per tenant, label bound into the
  content hash so forgery breaks validation); cost/execution run a
  possession model (unguessable `uuid4` session/job ids) + gateway scope
  gates for WHO — consistent with the `step_loop` precedent (possession
  of loop_id, documented). Known limit, stated: the gateway never binds
  `principal.sub` → `requester`, so label isolation is honest-caller +
  file-sharding + scopes, not cryptographic binding — the documented
  local-deployment model. Re-open if MCP serves remote cross-lab
  clients.
- Deferred batch: IdP (Phase 4), human lab membership (Phase 4),
  pgvector activation, sharding, full LLVM lowering, KEMTLS re-eval,
  CKT, Cirq/Q#/Serverless/Braket/rQOPS/CQM providers, GQE transformer,
  Bemba/Typst surfaces, quantum multi-geometry, loop load/chaos tests,
  cgroup kill, peercred sidecar (Q5 Linux hardening), workspace-files v2.
- Lean image — blocked on toolchain (no `lake`/`elan`, image
  unresolvable); comparator stays AST-first.

## C · Conditional (wire when a consumer exists)

- Python↔Go mandate crossing — bytes agree three-way (W0.9 vector);
  both rails mint and verify their own by design. Wire only if a flow
  needs cross-rail mandates (none does today).

## D · Standing questions

- All 48 questions in [[questions]] Sections A–E (including Q-01 through Q-48 and Q-27 history scrub) are **settled and closed** as of 2026-10-26 (0 open questions remaining across the entire blueprint).

Related: [[questions]] · [[roadmap]] · [[risks]] · [[full-tree-roadmap]] ·
[[rea-adoption-plan]] · [[cloudroom-adoption-plan]] ·
[[agent-subsystem-consolidation]]
