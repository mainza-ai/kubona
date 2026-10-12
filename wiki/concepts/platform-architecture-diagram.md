---
type: concept
tags: [architecture, diagram, implementation, tripartite]
date: 2026-10-12
updated: 2026-10-12
status: living
sources: [source-frontier-repo-audit-2026]
---

# Platform Architecture Diagram

> **Snapshot, not blueprint.** Every node below was verified against the running
> implementation at `mainza-ai/kubona` @ `dd600c2` (2026-10-12, branch `develop`).
> Design intent lives on the linked concept pages; this page records only what the
> prototype does today. **Refresh duty:** re-verify node-by-node after any slice that
> adds/removes a service, port, or store — a stale map is worse than none.

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

    subgraph MODELS["LLMs"]
        CLOUD["cloud deepseek<br/>via OPENCODE_API_KEY"]
        OMLX["oMLX :8787<br/>online, daemon stays on cloud"]
    end

    subgraph SECRETS["Secrets"]
        BAO["OpenBao sidecar or env<br/>leases scoped tenant/id"]
    end

    LEGACY["platform/kubona console_api.py<br/>legacy shim, 19 routes remain"]

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
    JOBS -->|dispatch| QIR
    QIR -->|lower| BSRV
    JOBS -->|gRPC| BSRV
    LOOPS -->|gRPC| BSRV
    BSRV --> CHEM
    BSRV --> KRY
    BSRV --> LIT
    CHEM --> SIM
    JOBS -->|4-tier, mandate-gated| QPU
    TRIB -->|per-role routing| CLOUD
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
- **Ports:** console `:3000` needs `NEXT_PUBLIC_KUBONA_CONSOLE_TOKEN`;
  `kubonad :8080` binds `127.0.0.1` by default (`--bind` / `--tls-cert` /
  `--tls-required` for the edge posture); bridge `:50051` loopback; oMLX `:8787`.
- **State:** `~/.kubona/kubona.db` behind `<db>.lock` (second daemon refuses);
  `campaign_versions` capped at 50; canvas layouts capped at 256KB.

## Node evidence (spot-check anchors)

- Edge auth + limiters: `platform/core/pkg/api/edge_auth.go`, `requireBearer`
  on all mutations; tribunal/jobs/launch/autoresearch 404 ghost sessions (G7).
- Tribunal: `platform/core/pkg/agents/collective.go:ExecuteTribunal`
  (method, line 231);
  spend entries bill the named session's tenant, suspended tenants refuse.
- A2A: `platform/core/pkg/api/server.go:handleA2A` over
  `platform/core/pkg/agents/a2a.go` — external JSON-RPC, not internal-only (Q-35).
- Tenants: `platform/core/pkg/tenants/tenants.go:verifyAdmin` is the single
  choke; `POST /api/tenants/status` mutator (Q-33).
- Memory: `platform/core/pkg/memory/hindsight.go` live;
  `platform/core/pkg/memory/postgres.go` dormant with contract test (Q-34).
- Bridge: `platform/bridge/proto/kubona.proto` is the IDL source of truth,
  regenerated by `scripts/regen-proto.sh` (pinned toolchain).
- Console gating: `ResultsStudio` renders only `COLLAPSED_VERIFIED`;
  `MeasuredOutputs` only `COMPLETED`; Action Gate behind `isMeteredBackend`.
- Retired surface: 18 Python shadow routes removed (K5); `pkg/mcp/gateway.go`
  deleted (F-5); `useTelemetry` still staged.

Related: [[overview]] · [[implementation-gaps]] · [[protocol-strategy]] ·
[[user-identity-and-profiles]] · [[agentic-memory-and-observability]] · [[a2a]] ·
[[integration-topology-and-licence-discipline]] · [[spend-mandates-and-delegation]]
