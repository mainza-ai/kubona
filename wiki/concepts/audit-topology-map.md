---
type: concept
name: Audit Topology & Live Verification Map
date: 2026-10-21
updated: 2026-10-21
tags: [architecture, diagram, audit, deployment, verification]
sources: [source-architecture-paper, source-gap-analysis-plan, source-llm-circuit-codesign-2026]
status: living
---

# Audit Topology & Live Verification Map

> **What this page is.** The deployment topology as *verified live* on
> 2026-10-21 — which process listens where, which rail executes which
> language, what the browser can and cannot reach, and where the audit
> proved failure. Companion to [[platform-architecture-diagram]] (the
> design-side map, snapshot 2026-10-12): this page records the *wiring as
> probed*, including the gaps. Refresh duty: re-verify after any
> compose/Dockerfile/port/auth change — a stale map is worse than none.
> Evidence class: live `curl` + Playwright/Chromium against the compose
> stack and an isolated local rail (`--db /tmp/…`, host bridge).

```mermaid
flowchart TB
    subgraph BROWSER["Operator machine (browser)"]
        UI["console :3000<br/>Next.js, baked NEXT_PUBLIC_API_URL"]
    end

    subgraph DOCKER["compose.prod.yml (kubona-prod_*)"]
        CONSOLE["console :3000<br/>static bundle, no secrets baked"]
        KUBONAD["kubonad :8080<br/>93 routes, bearer|session edge auth"]
        BRIDGE["bridge :50051 (expose only)<br/>Python gRPC, NOT host-reachable"]
        VOL["kubona-state volume<br/>kubona.db (SQLite, single writer)"]
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
    KUBONAD -->|gRPC bridge:50051| BRIDGE
    KUBONAD -->|exec local venv python| VENV
    KLOCAL -->|gRPC 127.0.0.1:50051| HBRIDGE
    KLOCAL -->|exec ../.venv| VENV
    KUBONAD & KLOCAL -->|chat/tribunal turns| STRATA
    KUBONAD --> VOL
    KLOCAL --> SQLITE
    BRIDGE & HBRIDGE --> VENV
    KUBONAD & KLOCAL --> GO
    VENV --> PY

    F001["F-001: in DOCKER, exec-rails 503<br/>(no python3 in kubonad image)<br/>codesign evaluate/budget/packs/families,<br/>chemistry, discovery"]
    F006["F-006: /api/fidelity ADMITS<br/>on empty input (hardcoded 0.94 probe)"]
    RADAR["Radar 50051 probe: dials<br/>--bridge addr (fixed 2026-10-21),<br/>was loopback → false OFFLINE"]
    HF401["HF/agent rails: bearer-gated by<br/>design; console had no token →<br/>silent [] / silent null (fixed:<br/>throw + in-modal refusal)"]

    KUBONAD -.-> F001
    KUBONAD -.-> F006
    KUBONAD -.-> RADAR
    UI -.-> HF401
```

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

- Compose: `platform/docker/compose.prod.yml` (kubonad `--bridge
  bridge:50051`, `KUBONA_ALLOW_PLAINTEXT=1` in-network only).
- Routes: 93 `HandleFunc` in `platform/core/pkg/api/server.go` (recount
  with `grep -o 'HandleFunc("[^"]*"'` — `rg` is broken on this host).
- Radar: `handleRadar` dials `s.bridgeAddr` for 50051
  (`server.go`, `SetBridgeAddr` from `--bridge`).
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

## Refresh checklist (after any topology change)

1. `docker compose up --build -d` + smoke matrix (health/ready/config).
2. Re-run route sweep + console cross-check (scripts in `/tmp` notes).
3. Re-screenshot all 6 surfaces + arena tabs (Playwright).
4. `python3 scripts/wiki-lint.py --strict` must stay clean.

Related: [[platform-architecture-diagram]] · [[frontend-backend-ui-ux-audit]]
· [[implementation-gaps]] · [[evaluator-guided-circuit-codesign]] ·
[[local-simulation-test-scenarios]] · [[circuit-codesign-ux-plan]] ·
[[multimodal-mission-control-and-custom-agents]]
