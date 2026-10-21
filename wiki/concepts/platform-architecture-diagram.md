---
type: concept
tags: [architecture, diagram, implementation, tripartite]
date: 2026-10-12
updated: 2026-10-21
status: living
sources: [source-frontier-repo-audit-2026]
---

# Platform Architecture Diagram

> **Snapshot, not blueprint.** Every node below was verified against the running
> implementation at `mainza-ai/kubona` @ `86afb3a` (2026-10-21, branch `develop`).
> Previous snapshot: `dd600c2` (2026-10-12) — superseded, kept in [[log]].
> Design intent lives on the linked concept pages; this page records only what the
> prototype does today. **Refresh duty:** re-verify node-by-node after any slice that
> adds/removes a service, port, store — a stale map is worse than none.

```mermaid
flowchart TB
    subgraph HUMAN["Human surface"]
        CONSOLE["platform/console :3000<br/>Next.js observatory surfaces"]
        SIGN["WebCrypto Ed25519<br/>mandate cards + proposals"]
    end

    subgraph EDGE["kubonad edge :8080"]
        API["pkg/api<br/>bearer + per-IP limiters + CORS allowlist"]
        RPC["POST /api/a2a/v1<br/>JSON-RPC: card, status, 402-gated submit"]
    end

    subgraph COG["Cognition — Go core"]
        TRIB["Tribunal<br/>4-turn debate, per-role LLMs"]
        REPLAN["Replan + MW fold<br/>claim records clm-xxxxxxxx-xxxx"]
        AGENTS["Custom agents + triage<br/>seed-immutable, advisory cap 50"]
        META["Metasearch evolution<br/>approve / reject / apply"]
    end

    subgraph EXEC["Execution — Go core"]
        JOBS["Jobs runner<br/>QASM-validated, autonomy-gated dispatch"]
        LOOPS["Autoresearch loops<br/>5 harnesses, session-attributed"]
        QIR["QIR Analyze gate<br/>text-IR only"]
        SB["Sandbox + watchdog<br/>timeouts, abs paths"]
    end

    subgraph TRUST["Trust — Go core"]
        LEDGER["Ledger<br/>SQLite single-writer flock, sealed events"]
        TENANT["Tenants = labs<br/>suspend freeze, verifyAdmin choke"]
        GRANTS["Grants + mandates<br/>narrow-only, depth 2, revocable"]
        COST["Budgets + spend caps<br/>150 USD default, global LLM cap"]
    end

    subgraph MEM["Memory"]
        RECALL["Hindsight SQLite<br/>live retain / recall, tenant-scoped"]
        PGVEC["Postgres + pgvector<br/>DORMANT BY DESIGN"]
    end

    subgraph BRIDGE["platform/bridge :50051 gRPC"]
        BSRV["server.py<br/>enveloped errors, never silent"]
        CHEM["chemistry.py<br/>PySCF FCI classical anchor"]
        KRY["krylov + cutting + ZNE<br/>emulation labelled"]
        LIT["literature.py<br/>offline returns empty, never cached"]
    end

    subgraph BACKENDS["Backends"]
        SIM["Local sim + Qiskit Aer<br/>credential-free default"]
        QPU["Runtime / Braket / Neutral atom<br/>4-tier fallback, 150 USD ceiling"]
    end

    subgraph MODELS["LLMs (verified 2026-10-21)"]
        STRATA["Strata :47831/v1<br/>DEFAULT provider, iq3_s, keyless local<br/>compose ships STRATA_MODEL + host route"]
        CLOUD["cloud deepseek via OPENCODE_API_KEY<br/>metered, spend-gated"]
        OMLX["oMLX :8787<br/>standby local engine"]
    end

    subgraph CODESIGN["Circuit co-design rail (verified 2026-10-21)"]
        CDPROXY["Go proxies :8080<br/>tq, frontier, evaluate(-batch),<br/>budget, families, packs, verified"]
        PYEVAL["kickmix evaluator (venv python)<br/>validity owner; failure recipes"]
        F001["F-001: prod kubonad image has<br/>no python → exec rails 503<br/>(compose); local rail works"]
    end

    subgraph SECRETS["Secrets"]
        BAO["OpenBao sidecar or env<br/>leases scoped tenant/id"]
    end

    LEGACY["platform/kubona console_api.py<br/>legacy shim, 23 routes remain (was 19;<br/>codesign trio now duplicated on Go rail)"]

    CONSOLE -->|bearer token| API
    SIGN -->|signed approvals| API
    RPC -->|same executor + tenant rules| TRIB
    API --> TRIB
    API --> REPLAN
    API --> AGENTS
    API --> META
    API --> JOBS
    API --> LOOPS
    API --> RECALL
    API --> CDPROXY
    CDPROXY -->|local venv exec| PYEVAL
    CDPROXY -.->|no python in prod image = 503| F001
    JOBS -->|dispatch| QIR
    QIR -->|lower| BSRV
    JOBS -->|gRPC| BSRV
    LOOPS -->|gRPC| BSRV
    BSRV --> CHEM
    BSRV --> KRY
    BSRV --> LIT
    CHEM --> SIM
    JOBS -->|4-tier, mandate-gated| QPU
    TRIB -->|per-role routing (1500-token turn caps)| STRATA
    TRIB -->|per-role routing, metered| CLOUD
    TRIB -->|standby| OMLX
    TENANT -->|verifyAdmin on every mutation| API
    JOBS -->|autonomy check| TENANT
    LEDGER <-->|sealed events| API
    LEDGER <-->|sealed events| JOBS
    LEDGER <-->|sealed events| TENANT
    GRANTS -->|admit or refuse| JOBS
    COST -->|caps at entry| API
    BAO -->|lease creds| API
    BAO -->|lease creds| BSRV
    PGVEC -.->|dormant, activation needs operator PG| RECALL
    API -.->|read mirror| LEGACY
```

## How to read it

- **Solid arrows** are live call paths, each covered by a Go test and at least one
  live probe against `:8080` (see [[implementation-gaps]] for the proof log).
- **Dotted arrows** are explicitly non-paths: dormant code or a legacy mirror.
- **Ports:** console `:3000` needs `NEXT_PUBLIC_KUBONA_CONSOLE_TOKEN`
  (unset in the stock build — bearer-gated console calls refuse honestly);
  `kubonad :8080` binds `127.0.0.1` by default (`--bind` / `--tls-cert` /
  `--tls-required` for the edge posture); bridge `:50051` loopback in
  local dev, `bridge:50051` in compose (unpublished — the radar probes
  the configured `--bridge` addr, fixed 2026-10-21 after a false-OFFLINE);
  local engines `:8787` oMLX, `:11434` Ollama, `:1234` LM Studio,
  `:47831` Strata (default; compose routes it via
  `host.docker.internal`, verified live 2026-10-21).
- **State:** `~/.kubona/kubona.db` behind `<db>.lock` (second daemon refuses);
  `campaign_versions` capped at 50; canvas layouts capped at 256KB.

## Node evidence (spot-check anchors, re-verified 2026-10-21 @ `86afb3a`)

- Edge auth + limiters: `platform/core/pkg/api/edge_auth.go`, `requireBearer`
  on mutations; tribunal/jobs/launch/autoresearch 404 ghost sessions (G7).
- Tribunal: `platform/core/pkg/agents/collective.go:ExecuteTribunal`
  (method, line 403); 4 turns capped at 1500 tokens (`CompleteCapped`);
  wired turns name the session id; spend entries bill the named session's
  tenant, suspended tenants refuse.
- A2A: `platform/core/pkg/api/server.go:handleA2A` (line 3115) over
  `platform/core/pkg/agents/a2a.go` — external JSON-RPC, not internal-only (Q-35).
- Tenants: `platform/core/pkg/tenants/tenants.go:verifyAdmin` (line 155) is the single
  choke; `POST /api/tenants/status` mutator (Q-33).
- Memory: `platform/core/pkg/memory/hindsight.go` live;
  `platform/core/pkg/memory/postgres.go` dormant with contract test (Q-34).
- Bridge: `platform/bridge/proto/kubona.proto` is the IDL source of truth,
  regenerated by `scripts/regen-proto.sh` (pinned toolchain).
- Console gating: `MeasuredOutputs` renders live job folds;
  `COLLAPSED_VERIFIED` gates verified claims (`replan.go:394`);
  Action Gate behind `isMeteredBackend`.
- Retired surface: 18 Python shadow routes removed (K5); `pkg/mcp/`
  absent (gateway deleted, F-5); `useTelemetry` deleted 2026-10-20 (Phase 4);
  console_api.py holds 23 unserved routes (codesign trio now duplicated on
  the Go rail — documented overlap, migration only on surface need).
- Counts this snapshot: 93 routes (`server.go`), 28 Go packages,
  194 Vitest tests across 41 files, 1011 pytest green.

Related: [[overview]] · [[implementation-gaps]] · [[protocol-strategy]] ·
[[user-identity-and-profiles]] · [[agentic-memory-and-observability]] · [[a2a]] ·
[[integration-topology-and-licence-discipline]] · [[spend-mandates-and-delegation]]
