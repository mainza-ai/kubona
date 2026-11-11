---
type: concept
name: Audit Topology & Live Verification Map
date: 2026-10-21
updated: 2026-11-10
tags: [architecture, diagram, deployment, verification, audit]
sources: [source-architecture-paper, source-gap-analysis-plan, source-llm-circuit-codesign-2026]
status: living
---

# Audit Topology & Live Verification Map

> **What this page is.** The deployment topology as *verified live* on
> 2026-10-29 — which process listens where, which rail executes which
> language, what the browser can and cannot reach, and where the audit
> proved failure. Companion to [[platform-architecture-diagram]] (the
> design-side map, snapshot 2026-10-21): this page records the *wiring as
> probed*, including the gaps. Refresh duty: re-verify after any
> compose/Dockerfile/port/auth change — a stale map is worse than none.
> Evidence class: live `curl` against the compose stack and isolated local
> rails (`--db /tmp/…`, host bridge + UDS socket), plus the 2026-10-29
> governed live-QPU probe (estimate→reserve→cancel→settle, ledger-sealed).
>
> **Update (2026-11-10, `kubona-live` Compose stack @ `de3428c` / `119749e`):**
> - **Live services verified `(healthy)`:** `kubona-live-bridge-1` (`UDS`),
>   `kubona-live-kubonad-1` (`127.0.0.1:8080`, `140` HTTP routes, compiled
>   `qir-core` bundled at `/usr/local/bin/qir-core`), and `kubona-live-console-1`
>   (`127.0.0.1:3000`, `142` modeled / `14` client-only / `0` unmodeled
>   contracts).
> - **Fresh `kubona-state` volume:** preflight exits 0; live `spin_chain` and
>   `thermodynamic_limit` DMRG jobs (`N=16`, `chi=32`, fixed `TNLinearOperator`)
>   completed in compose with `experiment-result-v1` schemas and signed/hash-chained
>   `result_receipt` records (`2026-11-10`).
> - **`F-006` superseded:** `/api/fidelity` (`server.go:handleFidelity`) routes
>   through `EvaluateAdmissionFloorOnly` with `probe_run:false` (no hardcoded
>   0.94 probe), and `api.Server` now shares its live `FidelityGate` pointer
>   with `jobs.Runner` so `POST /api/fleet/calibration/refresh` updates job
>   admission directly (`de3428c`).
> - **Live IBM `ibm_fez` acceptance (`docs/live-qpu-2026-11-09.md`):** SPSA HILO
>   VQE, ZNE VQE, active-space LiH, HILO QAOA, and `spin_chain_hardware`
>   completed end-to-end on Open Plan (`$0.00`) over the fresh volume;
>   `claimable:false` holds honestly.
>
> ⚑ Update (2026-10-30, local-first production): the supported compose is
> loopback-only (`127.0.0.1:8080` / `127.0.0.1:3000`), read-only services with
> dropped caps, and the checked-in `scripts/local-deployment-check.sh` proves
> no public listener. Live proof: stack up on a fresh volume, health/ready/
> console green, check exit 0. The console/bridge/kubonad are container-
> internal `0.0.0.0`; the host binding is the boundary. C5b added a
> session-scoped `POST /api/codesign/rerun` (BYOK estimate, no QPU-cap debit,
> sealed `trial`), and the bridge gained a stateless `spec` op. B4 remains
> local diagnostics by default: no telemetry leaves the host; Sentry/OTLP
> are opt-in only.

```mermaid
flowchart TB
    subgraph BROWSER["Operator machine (browser)"]
        UI["console :3000<br/>Next.js, baked NEXT_PUBLIC_API_URL"]
    end

    subgraph DOCKER["compose.prod.yml (kubona-prod_*)"]
        CONSOLE["console :3000<br/>static bundle, no secrets baked"]
        KUBONAD["kubonad :8080<br/>140 routes (docs/repo-counts.md)<br/>bearer|session edge auth, shared FidelityGate"]
        BRIDGE["bridge UDS unix:///run/kubona/bridge.sock<br/>shared bridge-sock volume, NO TCP in prod"]
        VOL["kubona-state volume<br/>kubona.db (SQLite, single writer)<br/>bridge-sock volume (UDS only)"]
    end

    subgraph LOCALDEV["Local dev / audit rail"]
        KLOCAL["kubonad-local :18080<br/>--db /tmp/… --bridge 127.0.0.1:50051"]
        VENV["platform/.venv python3<br/>kickmix, chemistry, whisper"]
        HBRIDGE["bridge server.py :50051<br/>host-reachable gRPC"]
        STRATA["Strata :47831/v1<br/>iq3_s, OpenAI-compat, ~36 tok/s"]
        SQLITE["/tmp/*.db<br/>throwaway ledgers"]
    end

    subgraph TRUTH["Validity split (settled by construction)"]
        GO["Go owns numbers<br/>TQ, frontier, ranking, spend math"]
        PY["Python owns validity<br/>kickmix evaluator, anchors, proofs"]
    end

    UI -->|reads public, writes bearer| KUBONAD
    UI -->|same origin /api/config| CONSOLE
    CONSOLE -->|NEXT_PUBLIC_API_URL| KUBONAD
    KUBONAD -->|gRPC unix:///run/kubona/bridge.sock| BRIDGE
    KUBONAD -->|exec local venv python| VENV
    KLOCAL -->|gRPC 127.0.0.1:50051 (dev only)| HBRIDGE
    KLOCAL -->|exec ../.venv| VENV
    KUBONAD & KLOCAL -->|chat/tribunal turns| STRATA
    KUBONAD --> VOL
    KLOCAL --> SQLITE
    BRIDGE & HBRIDGE --> VENV
    KUBONAD & KLOCAL --> GO
    VENV --> PY

    F001["F-001 (NARROWED 2026-11-10): in DOCKER, Python-exec rails 503<br/>(no python3 in kubonad image): codesign eval/budget/packs/families/rerun;<br/>chemistry ADMET & discovery now run via Go + gRPC UDS bridge"]
    F006["F-006 (RESOLVED): /api/fidelity calls<br/>EvaluateAdmissionFloorOnly with probe_run:false;<br/>Server & Runner share live FidelityGate"]
    RADAR["Radar probes the bridge addr<br/>(flag removed 2026-10-29: kubonad<br/>dials KUBONA_BRIDGE_SOCKET unix<br/>socket, TCP refused in prod)"]
    HF401["HF/agent rails: bearer-gated by<br/>design; console had no token →<br/>silent [] / silent null (fixed:<br/>throw + in-modal refusal)"]

    KUBONAD -.-> F001
    KUBONAD -.-> F006
    KUBONAD -.-> RADAR
    UI -.-> HF401
```

## What the arrows proved (2026-10-29, live QPU probe + UDS cutover)

- **kubonad → bridge (UDS):** compose runs the bridge with NO TCP socket —
  `KUBONA_BRIDGE_SOCKET=/run/kubona/bridge.sock` on a shared volume, TCP
  refused in `KUBONA_ENV=production`, kubonad dials the same socket
  (`--bridge` flag removed; env takes precedence over the localhost
  default). Proven live both directions: UDS boot + `channel_ready` OK,
  TCP-in-prod refuses fail-closed.
- **Governed live QPU run (2026-10-29):** 100-shot Bell via `ibm_fez`
  (least_busy): tenant → admin → session ($10 ceiling) → reviewer mandate
  → dry-run estimate ($8.000568) → dispatch with mandate + idempotency key
  → queued ~1h → cancelled via governed API with vendor recall confirmed.
  Settlement $8.000568 → $0.00 on both rails, ledger-sealed (`job_settled`);
  calibrated-subgraph guard recorded physical 146/147, max readout 0.29%.
  The probe caught one real bug first: Go minted fractional-expiry mandates
  the Python rail correctly refused — fixed at the mint (whole-second
  canonical), pinned both rails.
- **Load proof (local handler, not prod):** `scripts/load-flight-recorder.sh`
  against a throwaway kubonad: soak p50 3.6/p99 9.2ms, burst p50 5.8/p99
  76.7ms, 0 errors over 1000 fetches.

## What the arrows proved (2026-10-21)

- **Console → kubonad:** 78 distinct console call paths, zero hit a
  missing backend route (exhaustive cross-check). Bearerless writes 401,
  wrong token 401, plain token on admin rails 403 with envelope mismatch.
- **kubonad → bridge (compose):** krylov/mitigation/qir live through gRPC;
  bridge kill → labeled classical fallback + recovery (no hangs).
- **kubonad → venv (compose): ABSENT** — F-001. The same rails work on the
  local rail (venv present): TQ golden, evaluate valid/invalid (+
  `failure_details` recipes), frontier best-pick, batch, budget union
  bound, families lineage, packs, discovery pipeline.
- **kubonad → Strata:** chat stream, 4-turn tribunal (template fallback when
  LLM absent — contested, obligation open, coverage honest). Turn output
  capped at 1500 tokens (uncapped turns rambled past 90s and 300s ctx on
  the default stack; CompleteCapped + min(operator ceiling, cap)). Wired
  turns name the session id verbatim (before: every session-scoped tool
  call failed "unknown session" and contested the run). Consensus TRUE
  proven live (Strata iq3_s, all facets authored, all tools OK). Mesh
  route qualifies bare default-model IDs ("iq3_s" → "strata/iq3_s";
  before: misrouted as omlx-cloud and refused for spend).
- **Spend honesty:** Strata (default provider) filed spend records and
  refused usage-omitted calls as "blind cloud spend" — fixed to the
  `IsLocalProvider` single home (client x4 + server `recordLoopSpend` +
  copilot `engineNotice` hint + cost-test local list).
- **Browser truth:** first paint 0 console errors / 0 failed requests; all
  6 surfaces + 3 arena tabs render; full chat→tribunal→launch→COMPLETED
  job loop through the UI; kill-banner on backend loss; honest refusals
  for HF search, agent register, settings save, mesh route, chat without
  model. Found visually: radar false-OFFLINE, recorder "Iteration 15 of
  7", orb text upside-down mid-cycle (rotation keyframes), unnamed QIR
  copy button — all fixed and re-verified with screenshots.

## Node evidence

- Compose: `platform/docker/compose.prod.yml` (kubonad WITHOUT `--bridge`
  flag; `KUBONA_BRIDGE_SOCKET=/run/kubona/bridge.sock` on both services;
  `bridge-sock` volume; `KUBONA_ALLOW_PLAINTEXT=1` in-network only).
- Routes: 140 `s.route(` in `platform/core/pkg/api/server.go` as of `2026-11-10`
  (previous count: 121 at `2026-10-29`; authoritative counts generated in
  `docs/repo-counts.md`).
- Radar: kubonad dials `KUBONA_BRIDGE_SOCKET` (unix) when `--bridge` is at
  its localhost default (`cmd/kubonad/main.go`); `SetBridgeAddr` kept for
  explicit overrides.
- Python exec: `runBridgePython` + `findPythonVenv`
  (`server.go:3577+`); kubonad image has no python
  (`platform/docker/Dockerfile.kubonad`).
- LLM: `pkg/llm/client.go` (`CompleteCapped`, spend silence via
  `IsLocalProvider`), `pkg/llm/cost.go`, `pkg/agents/collective.go`
  (4-turn tribunal, `tribunalTurnMaxTokens = 1500`, coverage-gated seal).
- Console honesty: `lib/api.ts` (throw-on-refusal for HF + agent
  create), `CognitiveMesh.tsx` (in-modal `role=alert`), `globals.css`
  (orb pulse without rotation), `FlightRecorder.tsx` (milestone label),
  `FleetRadar.tsx` (bridgeAddr probe display).
- Local-first ingress: `platform/docker/compose.prod.yml` (loopback ports,
  read-only, cap-drop, log rotation) and
  `scripts/local-deployment-check.sh` (public-listener refusal).
- C5b rerun: `platform/core/pkg/api/codesign_rerun.go` (session-scoped,
  idempotent, BYOK guardrail), `platform/bridge/codesign_ops.py` (`spec`),
  and `docs/llm-proposer.md`.

## Refresh checklist (after any topology change)

1. `docker compose up --build -d` + smoke matrix (health/ready/config).
2. Re-run route sweep + console cross-check (scripts in `/tmp` notes).
3. Re-screenshot all 6 surfaces + arena tabs (Playwright).
4. `python3 scripts/wiki-lint.py --strict` must stay clean.

Related: [[platform-architecture-diagram]] · [[frontend-backend-ui-ux-audit]]
· [[implementation-gaps]] · [[evaluator-guided-circuit-codesign]] ·
[[local-simulation-test-scenarios]] · [[circuit-codesign-ux-plan]] ·
[[multimodal-mission-control-and-custom-agents]]
