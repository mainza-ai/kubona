# Production Audit & Hardening Close — 2026-11-20 (local-private)

**Base commit:** `baed702d` → **Landed commits:** `0fdba803`, `853a476e` (`develop` / `main`).  
**Scope:** Full-stack audit and implementation across Go Core (`kubonad`), Python Bridge & Science, Rust (`qir-core` / `ifc-core`), Next.js Sovereign Console, Contracts/OpenAPI, Docker Compose/DR, and CI gates for local-private single-host deployment.

---

## 1. Key Fixes & Capabilities Landed

### Phase 0 — Boot & Process Resilience
- **Rate-card expiry & local-only posture:** Refreshed `platform/contracts/rate-cards.json` (`2026-11-20`) and updated `kubonad preflight` (`platform/core/cmd/kubonad/preflight.go`) to warn rather than fail on stale cloud rate cards when `KUBONA_LLM_CLOUD` and paid QPU credentials are unset.
- **SSE slow-client crash:** Fixed `platform/core/pkg/api/stream_hub.go` and `server.go` (break after drop, clear write deadline, regression test).
- **Secret scanner:** Fixed underscore boundary bypass in `platform/core/pkg/secret/secret.go` and added Gemini, Hugging Face, OpenBao, `github_pat_`, and `AGE-SECRET-KEY-1` detection.
- **Compose MCP, `kubona` CLI & Manuscript PDF security:** Fixed `compose.prod.yml` `mcp` healthcheck/env (`${KUBONA_API_TOKEN:-}` so tokenless fail-closed `docker-smoke` and `image-scan` interpolate cleanly), `kubona/run.py` UDS permissions (`0o700`), `manuscript_ops.py` (`-no-shell-escape`, `killpg`, BibTeX escaping), `manuscript.py` (full-manifest Ed25519 signoff hash, slug traversal guard), and `kubonad` CLI (`version` via `debug.ReadBuildInfo`, exit `2` on unknown subcommands, `KUBONA_DB_PATH`, CORS `PATCH`).

### Phase 1 — Security, Multi-Lab Isolation & Ledger Integrity
- Separated `requireOperatorBearer` on host-config routes from per-session tenant checks.
- Revoked `auth_sessions` on `SetAdminKey` rotation; enforced `pubkey_fp` verification, 32-byte key length, bootstrap CAS, and `admin_action_replays`.
- Closed cross-tenant IDORs (`GET /manuscripts/{id}/reviews`, `POST /proposals`), multi-lab `authorizeKnownSession`, `/formal-verifications`, `singleLabDeclared()`/`singleTenantStore()`, and UCAN expiry/NaN/subject edge cases.
- Hardened `pkg/llm` air-gap/DNS-rebinding/Strata URL validation and Anthropic `HardMaxTokens` `SpendGuard` enforcement.
- Enforced fail-closed bridge UDS token in production, 36 MiB gRPC limits, signed `peer_review` (`MintClaimReviewSigned`) & `manuscript_deleted` events, SHA-256 session-prefixed claim/review IDs, and tenant-scoped `HindsightStore` in `AgentCollective` & `LoopEngine`.

### Phase 2 — Jobs, Autonomy, Plans, Local LLM & Sandbox
- Fixed `hardware_pes` admission, `LiH`/`BeH2` casing, `ReconcileJobsOnBoot`, CAS `executeJob` / `failJobWithNeed`, `Stop()` channel safety, and `resultSigner` lock.
- Fixed `DecideExecute` guards, `CASPlanRunning`, `*float64` `$0` plan budgets, `PATCH /plans/{id}` terminal guards, `ToolPermissions` preservation on halt/rollback, `"loop_aborted"` event registration, and serialized supervisor DB updates.
- Hardened `pkg/llm` (`TimeoutSec`, streaming ceiling, `screenCloudPrompt`, `WireError`, local `stream_options`, Gemini `ThoughtsTokenCount`), Tribunal direct-text reuse, `rates` staleness checks, `sandbox` (4 MiB bounded output, `0777` workspace, case-insensitive `unconfined` refusal, opt-in `KUBONA_SANDBOX_ALLOW_RUNC=1`), `pkg/hf`, and `pkg/epistemics` source deduplication.

### Phase 3 — Scientific Correctness, Memory & Benchmarks
- Fixed Pauli measurement endianness (`term[::-1]`), per-term seeds, and physical `(n_alpha, n_beta)` Fock projection in `platform/bridge/experiments.py`.
- Enforced Rayleigh-Ritz variational bounds and quantum-advantage realism in `platform/bridge/epistemics.py`.
- Fixed true unitary ZNE folding (`bridge/mitigation.py`), TKET half-turns (`tket_provider.py`), iterative recurrence checking (`conjecture.py`), URL-encoded arXiv queries (`bridge/literature.py`), Q3 `max_cut_opt = 12.0` + Meyer-Wallach entanglement (`autoresearch/harness.go`), `bench_agentic` `gte` scoring, `pkg/verify`, `pkg/prov`, and `pkg/memory`.

### Phase 4 — Console Parity, Export & UX Fixes
- Exposed `GET /api/sessions/{id}/manuscripts/{mid}/export` (RO-Crate 1.1 + W3C PROV-JSON + CRediT) in `platform/core/pkg/api/manuscripts.go`.
- Wired Console surfaces: `ResearchPipelinePanel` (Hypotheses, Elo tournament, `$0` Plans, Research Graph, Lab Knowledge), `OversightGovernancePanel` (Autonomy Policy, Operator Halt/Unlock, History/Rollback, Receipts/Undo, `/api/ops/science`), `PublicationPanel` (Verify, RO-Crate/PROV Export, Draft Edit/Delete, Zenodo Manifest, Formal Verifications), live `ApprovalInbox` invalidation, real `MemoryPanel` retraction (`POST /api/memory/retract`), and `LabOpsPanel` Tenant Agentic Default.
- Fixed keyless `LabsView` bootstrap, `useCampaignProjection` session reset, `MathText` KaTeX hardening, `/api/lens-image` GCS allowlist/timeout/`nosniff`, scoped 403 `authPhase`, `streamChatMessage` stall/timer cleanup, `useSessionStream` watchdog, and non-Arena Copilot docking.
- Wired restart-orphan `sleeping` loop settlement (`POST /api/sessions/{id}/loops/settle`) into the **Delete campaign** dialog (`Settle sleeping loops & delete`) and `LoopControlList` (`Settle orphan`).
- Defaulted Attention Director ambient sound (`platform/console/src/lib/ambient-audio.ts`) to **muted** and added `useDismissiblePopover` click-outside / `Escape` dismissal across `Campaigns`, `NotificationCenter`, `NeedsYouTray`, and `CommandDock` flyouts.

### Phase 5 — Rust `qir-core`, Docker/DR & CI
- Hardened `platform/crates/qir-core/src/lib.rs` & `main.rs` (`close <= o` slice panic, zero-size QASM3 guard, `checked_add` + `MAX_QUBITS=1024`, 4 MiB input cap, `pi`/`sdg`/`tdg` support).
- Hardened `backup-loop.sh`, `backup-restore-check.sh` (`.sha256` & file arg), `reset-compose-state.sh`, contract volume upgrade refresh, `ci.yml` (all bridge suites + `dco-check`/`performance-gate` in `canary-gate`), `secret-scan.sh`, and `perf-gate.sh`.

---

## 2. Verification Snapshot (2026-11-20)

- **Generated counts (`docs/repo-counts.md`):** `156` HTTP route patterns, `188` OpenAPI paths (`191` schemas, `220` modeled / `16` client-only / `0` unmodeled), `48` Go packages (`1167` tests), `189` Python test files + `7` bridge test files, `160` Console test files (`852` source cases; `304` Node + `549` Chromium Vitest runtime), `34` Rust tests, `6` MCP servers (`38` tools), `168` wiki pages.
- **Live rebuilt stack E2E:** All 18/18 checks passed against an isolated stack (CLI, UDS bridge, `kubonad`, Streamable-HTTP MCP `:18123`, live H2 VQE `Δ = 4.29 mHa`, Bell QASM, Hypotheses/Plans/Graph/Knowledge, Autonomy/Receipts/Memory Retract, Manuscript Verify + RO-Crate/PROV Export, `kubonad verify`, DR backup + SHA-256 restore, and Playwright Chromium UI E2E with 0 page errors). Local Strata (`127.0.0.1:47831`) was offline during that run.
- **Relaunched `compose.prod.yml`:** All 5 containers (`kubonad`, `bridge`, `mcp`, `console`, `backup`) verified healthy on `127.0.0.1:8080`, `:3000`, and `:8123`.
