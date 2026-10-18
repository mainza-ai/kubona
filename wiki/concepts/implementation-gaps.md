---
type: concept
date: 2026-08-16
updated: 2026-10-17
tags: [kubona, production, gaps]
sources: [source-gap-analysis-plan]
status: tracked
---

# Prototype Gap Register (production vs. scaffold)

> **What this page is.** The honest register of what the [[overview|prototype]] does vs.
> what production requires. It is a *code* register, not a design register: it answers
> "what does the scaffold already do", never "what should Kubona be". Design questions
> live in [[questions]]. This page exists so a skeleton/engine implementation is never
> mistaken for a deployable one, and so each gap is closed with evidence, not intent.

## Status legend
- 🟢 **Closed (M0)** — structural, cheap-now fixes landed.
- 🟠 **Open + required before real deployment** — the "we'd redo it later" items.
- 🔵 **Deferred by design** — matches the plan's own "functional product first" sequencing.
- 🔴 **Regressed / failing now** — verified broken at the stated date; not a future concern.

## Forensic Audit (2026-10-04 historical snapshot): Codebase Reality vs. Blueprint

> **Historical note (updated 2026-10-17):** The numbered findings below record the state of
> the tree at the 2026-10-04 forensic baseline (see [[forensic-audit-2026]]) before Phases
> 1–6, Waves 0–2, Cloudroom S1–S8, REA R0–R6, leanrea L1–L5, M0–M6, and A1–A4 landed. See
> the `## Critical (structural)` table below for current verified status.

A systematic audit on 2026-10-04 revealed that over 90% of what was documented in the blueprint across the
89 wiki pages, the 7 Observatory Surfaces, and 25 audited dev-docs repositories was at that time either
unimplemented, stubbed, or disguised behind static UI facades:

1. **The 7 Observatory Surfaces (at 2026-10-04 baseline):**
   - Surface 1 (Spatial Living Lab): No infinite zoomable canvas, no React Flow node graph, no post-QIR universal glyphs, no ghost circuits.
   - Surface 2 (4D Flight Recorder): Was reduced to a plain static HTML table; lacked interactive scrub bar, parameter trajectory animation, and counterfactual branching.
   - Surface 3 (Epistemic Manifold): Rendered static placeholder cards rather than dynamic SVG/Canvas wavefunction rings ($\alpha|\text{corr}\rangle + \beta|\text{cont}\rangle + \gamma|\text{unver}\rangle$) or stagnation telemetry ($\kappa$).
   - Surface 4 (Autonomy Cockpit & AP2): Unsigned JSON payload without Ed25519 cryptographic spend mandate verification.
   - Surfaces 5, 6, 7 (Fleet Radar, Cognitive LLM Mesh, Results Studio): Completely absent from the frontend.
2. **Multi-Agent Research Collective (at 2026-10-04 baseline):**
   - The 5 distinct specialist personas (`epistemic-scribe`, `ansatz-scout`, `device-auditor`, `disconfirmation-officer`, `human-reviewer`) with declarative `role.yaml` capability grants and keypair identities were unbuilt.
3. **The Five-Ring Research Loop (at 2026-10-04 baseline):**
   - Ring 0 (Reflex): Lacked automated re-scoping and snapshot recovery.
   - Ring 1 (Epistemic): Uncollapsed claims were not blocked from being asserted in agent outputs.
   - Ring 3 (Methodological): No fidelity accounting, no device noise profiles ($T_2 \le 2T_1$), and no Clifford stabilizer probes.
   - Ring 4 (Reflexive): No Karpathy Autoresearch harness runners or §13 meta-search evolution flywheel.
4. **Security, Confidentiality & Isolation (at 2026-10-04 baseline):**
   - Buzz IFC `can_flow_to` reader-set lattice was missing from queries.
   - OpenBao secrets vault with dynamic single-use leases was unintegrated.
   - Code execution was unsandboxed on the host instead of container-isolated.

## The 6-Phase Master Implementation Plan

- **Phase 1: Architecture Core, Governance & Isolation:** Ed25519 AP2 spend mandate engine in Go, Buzz IFC reader-set lattice in Rust/Go, Paperclip 4-plane MCP access gateway with `low_trust_review`, and OpenBao secrets vault sidecar.
- **Phase 2: Quantum Physics Engines, Real Anchors & Data Grounding:** PySCF numerical electronic structure solver (RHF, CISD, FCI), hardware noise profiles ($T_2 \le 2T_1$) with `stim` Clifford probes ($\le 64$ shots), PennyLane `qcut` KaHyPar circuit cutting, and live arXiv/OpenAlex connectors.
- **Phase 3: Multi-Agent Collective & Closed Rings 0-1:** 5 specialized agent personas with declarative `role.yaml`, Ring 0 supervisor with automated re-scoping, Ring 1 assertion gating with epistemic curvature ($\kappa$, drift, refusal ratio), and Hindsight tripartite memory banks.
- **Phase 4: Autoresearch Engine, Fidelity Accounting & Rings 3-4:** Karpathy Autoresearch 4-part harness registry, autonomous ansatz genome search with cheap pre-checks, Ring 3 fidelity accounting gate (predict $\to$ probe $\to$ admit/downgrade/refuse), and §13 meta-search self-evolution engine.
- **Phase 5: The Seven Core Observatory Surfaces (Next.js 16 + TypeScript):** Surface 1 (Spatial Canvas via React Flow), Surface 2 (4D Scrubbable Flight Recorder), Surface 3 (Dynamic Wavefunction Rings), Surface 4 (Cryptographic Mandate Cockpit), Surface 5 (Fleet Radar & ITAR Shield), Surface 6 (Cognitive Mesh & Air-Gap Switch), Surface 7 (Citable Results Studio).
- **Phase 6: Multi-Tenant Sandboxing, Production CI & Zero-Placeholder Verification:** Docker workload container isolation, unified multi-language CI pipeline, automated end-to-end campaign tests, and zero-placeholder audit across the codebase.

## Critical (structural)

| Gap | Status | What's real / what's missing |
|---|---|---|
| **Persistent state root (data-loss)** | 🟢 M0 | Route through `kubona/config.py` (`state_root()`) — default `~/.kubona`. |
| **Prod auth & Ed25519 operator sessions (A1–A4)** | 🟢 landed 2026-10-17 | Unified `checkAuth` in Go core (`pkg/api/edge_auth.go`, `auth_session.go`): accepts `kubona_session` `httpOnly` + `SameSite=Lax` cookies minted via Ed25519 challenge-response against the lab's registered `admin_pubkey_hex` (with `csrfAllowed` Origin check on unsafe methods and session revocation on `RotateAdminKey`), or constant-time `KUBONA_API_TOKEN` bearer for CLI/CI callers. Console mounts `AuthPanel.tsx` in `LabsView.tsx`. |
| **Deployment entrypoint** | 🟢 M0 | `kubona serve` / `kubona doctor` package entrypoints + `kubonad` Go daemon with Docker Compose production stack (`platform/docker/docker-compose.yml`). |
| **CI / release-gate** | 🟢 green | Single verdict `release_gate()` in `canary.py`, verified in hosted Actions (`ci.yml` + `docker-smoke`). |
| **Pillar 1: Workload Container Sandbox** | 🟢 verified | Fail-closed Docker runner in production with `--read-only`, `--network=none`, `--cap-drop=ALL`, unprivileged user, and memory clamps (`platform/core/pkg/sandbox/docker.go`). |
| **Pillar 2: OpenBao Secrets Vault Sidecar** | 🟢 verified | AppRole token exchange, dynamic leased QPU credentials, and automatic lease revocation (`platform/core/pkg/vault/client.go`). Manifest at `platform/docker/docker-compose.vault.yml`. |
| **Pillar 3: Post-Quantum Spend Mandates** | 🟢 verified | Hybrid NIST FIPS 203 ML-KEM-768 + AES-256-GCM + RFC 8032 Ed25519 spend mandates in Go (`platform/core/pkg/mandates/pqc.go`) and TypeScript (`platform/console/src/lib/pqc.ts`). |
| **Pillar 4: Tripartite Vector Memory** | 🟢 verified | 384-dimensional semantic embeddings, SQLite binary persistence, cosine similarity search, and automated memory injection for Tribunal Turns 1 & 2 (`platform/core/pkg/memory/hindsight.go`). |
| **Pillar 5: Autoresearch Pareto & Git Branches** | 🟢 verified | Multi-objective Pareto frontier engine (`platform/core/pkg/metasearch/pareto.go`) and Git Counterfactual Branch Manager (`platform/core/pkg/autoresearch/branch_manager.go`) managing ephemeral branches and signed fast-forwards. |
| **Pillar 6: Rust QIR LLVM Lowering Core** | 🟢 verified | QIR AST to LLVM IR lowering module (`platform/crates/qir-core/src/codegen.rs`) emitting BaseProfile text IR with full QIS runtime declarations and `--json` CLI output. |
| **Pillar 7: A2A Protocol & x402 Payment Rails** | 🟢 verified | JSON-RPC 2.0 A2A server (`platform/core/pkg/agents/a2a.go`) with capability cards and task delegation. HTTP 402 challenge/response verification (`platform/core/pkg/cost/payment.go`) requiring cryptographic signatures. |
| **Pillar 8: Observatory Cockpit & SSE Streaming** | 🟢 verified | Server-Sent Events (SSE) streaming endpoint (`/api/events/stream`), spatial canvas layout persistence (`/api/canvas/layout`), and Next.js 16 `useTelemetry` React hook (`platform/console/src/lib/useTelemetry.ts`). |
| **x402 fail-closed** | 🟢 M0 | Rejects proofs unless real verifier injected; reboot-refusal check on persisted nonces (`pkg/cost/payment.go`). |
| **Chat/Copilot & Env-Driven LLM Routing (M0–M6)** | 🟢 landed 2026-10-17 | Zero hardcoded model IDs in Go/Python/TS (`pkg/llm/config.go`, `kubona/mesh/model_router.py`); env-driven selection (`KUBONA_LLM_PROVIDER`, `*_MODEL`), live 4-turn tribunal debate pipeline with PySCF classical anchor injection, and DSML tool-call fallback toggle. |
| **Circuit Cutting & Knitting** | 🟢 verified | Genuine PennyLane & QPD circuit cutting decomposing circuits into valid subcircuits with preserved gates and measurements in `platform/bridge/executor.py`. |
| **Local Classical-Quantum Testing Gate** | 🟢 verified | Verified in `scripts/verify_real_world_e2e.py`: PySCF ab-initio RHF/FCI, Qiskit Aer simulation, Clifford canary probes ($\le 64$ shots), Meyer-Wallach entanglement $Q=1.000000$. |
| **Hybrid LLM Configuration Plane** | 🟢 verified | Settings view in Next.js console (`/llm` and `/settings/llm`) with Sovereign Air-Gap switch, local port radar sweep (8787, 11434, 1234, 8080, 50051), and Go Core `/api/llm/settings` persistence. |
| **Loop Supervisor Persistence** | 🟢 verified | Go Core WAL SQLite audit ledger (`platform/core/pkg/ledger/ledger.go`), event stream, and live job submission/status. |
| **Classical Chemistry Anchors** | 🟢 verified | Live PySCF STO-3G exact RHF and FCI calculation on molecular geometries ($H_2$ -1.137270 Ha, $LiH$ -7.882392 Ha) with strictly negative correlation energies. |
| **TypeScript/Go Core Migration** | 🟢 verified | Live production stack: Next.js 16 console (port 3000), Go Core daemon `kubonad` (port 8080), Python gRPC bridge (port 50051), and local/cloud LLM providers. |
| **Gap M-1: Memory Lifecycle & LRU Eviction** | 🟢 verified | Hard capacity limits (500 items/bank, 250k tokens) with importance-weighted LRU eviction and SQLite WAL synchronization (`platform/core/pkg/memory/hindsight.go#L238-L278`). |
| **Gap M-2: IFC Multi-Tenant Scoping** | 🟢 verified | Multi-tenant boundary checks and Buzz IFC `ReaderSet.CanFlowTo` lattice flow rules across semantic memory recall (`platform/core/pkg/memory/hindsight.go#L322-L385`). |
| **Gap M-3: Quantum Simulation RAM Pre-Flight Bounds Clamp** | 🟢 verified | Theoretical RAM scaling checks clamping exact statevector at 28 qubits (4.29 GB) and Aer at 30 qubits, automatically degrading to PennyLane KaHyPar wire-cutting (`platform/bridge/executor.py#L77-L140`). |
| **Gap M-4: Autonomous Epistemic Reflection Daemon** | 🟢 verified | 5-discard stagnation hook automatically triggers LLM/heuristic diagnosis consolidating failed mutations into `BankMentalModels` (`platform/core/pkg/autoresearch/engine.go#L400-L455`). |
| **Gap M-5: Buzz RFC 8032 Per-Event Signatures** | 🟢 verified | SHA-256 parent hash chaining combined with RFC 8032 Ed25519 per-event attestations, tampering detection, and verification (`platform/core/pkg/ledger/ledger.go#L112-L215`). |
| **Gap M-6: Enterprise PostgreSQL + pgvector Profile** | 🟢 verified | Docker Compose profile (`platform/docker/docker-compose.hindsight.yml`) and HNSW cosine index schema (`platform/docker/sql/init_hindsight.sql`). |
| **Gap M-7: Distributed Session Leases** | 🟢 verified | Distributed advisory locks and lease renewals on PostgreSQL sidecar (`platform/core/pkg/memory/postgres.go#L200-L245`). |
| **Gap M-8: GQE Checkpoint Pipeline** | 🟢 verified | LoRA weight delta distribution manifest and architecture specification for Qwen2.5-Coder base model (`platform/models/adapters/manifest.json`). |
| **Campaign Pre-Population & Element Parsing Fix** | 🟢 verified | Fresh vs Baseline discovery mode, element symbol whitelist in `SpatialCanvas.tsx`, and session deletion lifecycle (`platform/core/pkg/session/session.go`, `platform/console/src/components/modals/NewCampaignModal.tsx`). |
| **Elimination of UI Hardcoded Mocks** | 🟢 verified | All 8 surfaces dynamically wired to live session state (`CampaignState`), OpenQASM 2.0 multi-qubit visualizer, dynamic milestones, and centralized `API_BASE` (`platform/console/src/lib/api.ts`). |
| **Spend idempotency & reservation safety (S1 + Waves 0–2)** | 🟢 landed 2026-10-17 | Content-bound `idempotency_key` across all 4 spend mutations (`jobs`, `launch`, `tribunal`, `autoresearch-start` — replay=cached, mutated-body=409), persistent `spend_reservations` table with crash-recovery reconciliation, and `/api/tenants/refund` primitive (`full-tree-roadmap`). |
| **Disk guard with hysteresis (S2)** | 🟢 landed 2026-10-17 | `platform/core/pkg/disk/` poller (WARN 2 GB / BLOCK 1 GB / RESUME 1.5 GB, env-overridable, `0<block<resume<warn` enforced at boot) watching DB dir + HF models dir via `statfs` (minimum wins); BLOCK 503s `storage_blocked` on all new-work entries (spend mutations, chat writes, loop starts) with `Retry-After`, freezes `SubmitJob` defense-in-depth; reads/health/recovery stay; `/api/ready` 200/503 probe + health disk fields; transitions sealed as ledger events. No process killing (needs cgroup work). |
| **Loop sleep + drain + mid-run budget (S3)** | 🟢 landed 2026-10-17 | `loops` table finally has a writer (`SaveLoopState` + `harness_id/last_activity/drain/reserved_amount` cols; `ReconcileLoopsOnBoot` sleeps non-terminal rows — proven live: 20+1 rows). Engines: `sleeping` status, activity clock, `SetDrained` latch (Step refuses), `SetBudgetCheck` hook pausing on mid-run breach (never abort). API: `sleep`/`wake` (S1-keyed replay)/`drain` actions + `SweepIdleLoops` (10 min, 60 s poll) + sweeper goroutine in `kubonad`. Chat/tribunal deliberately excluded (no harness held — sleeping them would be theater). |
| **Vault lease session/tenant binding (S4)** | 🟢 landed 2026-10-17 | Portable `session_id` + `tenant_id` + `scope` binding on vault leases plus `/api/vault/leases/revoke` and canary test coverage. |
| **Session migration + delete cascade (S5 + S5b)** | 🟢 landed 2026-10-17 | `POST /api/sessions/export|import` with manifest v1 (per-section digests across 9 sections including `python_ledger`, 64 MB / 50-version / 256 KB caps, validation-before-replay, ONE `session_imported` record, event IDs preserved). `DeleteSession` refuses live/non-terminal/reserved work, else cascades all 9 tables (receipt survives). `ledger.CanonicalHash` unified (I1). |
| **Redaction by construction & shared event taxonomy (S6 + R1a)** | 🟢 landed 2026-10-17 | `sandbox.BoundOutput` (16 KiB, UTF-8-safe, flagged) at the harness-parse choke (incl. failure reasons); job results refuse above 1 MB (`result_too_large`); closed 121-row `platform/contracts/event-types.json` (80 Go + 41 Python) enforced at all Go and Python append chokes + cross-rail conformance tests. |
| **Queue ops reframed + shipped (S7)** | 🟢 landed 2026-10-17 | No prompt queue exists (synchronous model) — shipped the real gaps: `POST /api/jobs/{id}/cancel` (CAS unwind, terminal replay, ungated by disk by design) + rewrite CAS/session-404/disk-gate. Steer N/A documented (no background turn). |
| **Console context + progress + operator visibility (S8 + S8a + U0–U3 + D1–D2)** | 🟢 landed 2026-10-17 | `GET /api/sessions/context` (measured transcript vs BudgetHistory caps — no vendor window) + composer readout + pressure-gated overflow card + `JobProgress` in-flight polling + `OperatorBar` disk/drain/loops/bundle controls + runtime `/api/config` + digest-pinned Docker production stack (`platform/docker/`). |
| **REA contracts & leanrea skill/catalog (R0–R6, C1–C6, L1–L5)** | 🟢 landed 2026-10-17 | Cross-rail `EvidenceEnvelope` vectors, `platform/contracts/obligation-rules.json` (7 rules), coverage declarations (`CoverageMap`), bounded tribunal context + provenance markers, 22-code `platform/contracts/error-codes.json` + `/api/capabilities`, 5-dimension transcript rubric, `skills/kubona/SKILL.md` (≤12 KB), `bench/` harness, and 29-tool `platform/contracts/mcp-catalog.json`. |

## Medium

| Gap | Status | Note |
|---|---|---|
| Structured observability/metrics/audit export | 🟢 partial | Gateway audit now persists to a JSONL export (restart-safe, `audit_export()`, wired by default in `kubona run`/`serve-gateway`). Structured logging + OTel/Prometheus metrics remain. |
| Perf-regression guards (G12) | 🟢 | tests/test_perf_guard.py guards the lower + local-simulator hot paths against accidental O(exponential) regressions (passed in ~0.1s). |
| Volume eval corpora (QuantumKatas/HumanEval/PennyLang) | 🟢 landed 2026-10-10 | Katas transcribed (16 tasks at 1.0) + HumanEval 136 golden with 15 named exclusions via the loader; PennyLang refused-named under Q-26 (licence). Loader + licence gate + provenance quarantine all enforced. |
| GQE transformer (Unsloth) training | 🔵 | n-gram stand-in today; drop-in `propose` replacement on a GPU host. |
| Full circuit knitting | 🟡 partial | LOOC-Z single-CNOT wire cut now additionally reconstructs the **full post-cut target distribution** (`cut_histogram`, exact Z-diagonal, verified against uncut — strictly more informative than a single ⟨Z⟩). Honest scope: a single-CNOT cut reaches one target qubit, so multi-qubit-gate/product-Z cuts still need the full QPD/Qiskit-CKT integration (documented; that is the remaining piece). |
| Perf/load baselines | 🟢 guards landed | tests/test_perf_guard.py guards the lower + local-simulator hot paths (passed in ~0.1s). Full loop load/chaos tests remain open. |
| Session-attribution fallback (G7) | 🟢 closed 2026-10-12 | `ensureSession` used to adopt the most-recent session (or mint silently) for unknown/missing ids — unattributed spend and cross-session content writes. Now: spend entries (jobs, launch, tribunal, autoresearch) `requireSession` (404 missing/unknown); chat writes 404 unknown / mint fresh on empty; reads 404 unknown without adopting (epistemics keeps most-recent for the console global view — read-only, bearer-gated). `TestSessionAttributionStrictness` + live 404/201 matrix both directions. |

## Deferred by design (correct, per plan)
- x402 activation on a real rail (Phase 5+), meta-loop auto-apply (§13.5, Phase 5+), Unsloth personas (Phase 6), real third-party provider PR, third-party security review execution.

## Blueprint integrity (the wiki is the deliverable, so it is linted)

| Gap | Severity | Where |
|---|---|---|
| The blueprint accumulated **build language** for months — phase gates written as "implemented", `status` values encoding finish state. The failure mode where a blueprint quietly becomes a changelog | high (fixed 2026-09-27) | reframed in [[overview]] / [[roadmap]] / [[index]]; vocabulary defined in `AGENTS.md` |
| `scripts/wiki-lint.py` existed and exited 0, but **nothing ran it** | medium → closed 2026-09-27 | wired into `ci.yml` as a *Wiki structural lint* step in the `python-tests` job — deliberately added only after the release-gate fix above, because a step added to a pipeline that already fails 17/17 adds no signal. The check is text-only over `wiki/**`, so it needs neither `dev-docs/` nor the Rust toolchain on the runner |
| `AGENTS.md` drifted from the wiki it governs (stale page counts, an outdated repo-status rule, inline-cite keys that no longer matched source pages) | medium (fixed 2026-09-27) | schema now states purpose, vocabulary, exemptions, and the two-register rule |

The lint is deliberately narrow: it checks **structural** invariants — traceability, link
integrity, classification, index coverage — not prose quality or whether a decision is
*correct*. Those stay human judgement. What a script can guarantee is that a decision is
findable, linked, and traceable to a source, which is the property that makes a blueprint
accumulate instead of rot.

## Doc-overstatement corrections (see also [[overview]] / [[roadmap]])
- "canary CI gates releases" → **CI was absent**; now scaffolded (M0), first hosted run pending.
- "OAuth-shaped token issuance" → issuer existed but was **not wired**; now the production default with aud/nbf.
- "9 providers" → 2 execute; the rest are capability entries.
- "QIR (Rust core)" in Platform P1 → Rust core exists but was **off the compiler path**; integration is an open gap.
