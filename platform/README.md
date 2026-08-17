# Kubona Platform — Phase 0 Skeleton

The four federated MCP servers (compiler · execution · cost-governance ·
eval-provenance), gateway, provider trait, and provenance ledger — a runnable
Phase-0 skeleton. See `../wiki/roadmap.md` for the phase plan and `../wiki/overview.md`
for the architecture.

## Quick start

```bash
cd platform
uv venv .venv
uv pip install --python .venv/bin/python -e ".[dev]"
.venv/bin/python -m pytest -q
```

## The P0 gate

`tests/test_phase0_gate.py` proves the roadmap's Phase-0 exit criterion: an agent
submits a simulator job end-to-end through the MCP chain (over the **real stdio MCP
transport**, servers spawned as subprocesses) and gets results back with a
content-addressed, hash-chained provenance record written — using only
open-source dependencies.

## Layout

| Path | What it is |
|---|---|
| `kubona/mcp/servers/` | The four MCP servers (each runnable: `python -m kubona.mcp.servers.execution`) |
| `kubona/mcp/gateway.py` | Gateway: auth + transport interfaces, per-call dispatch, audit |
| `kubona/mcp/auth.py` | AuthProvider contract; DevAuthProvider; OAuth 2.1 stub (Phase 3) |
| `kubona/mcp/transport.py` | TransportProvider contract; in-process + stdio |
| `kubona/providers/` | Provider trait + `LocalSimulator` (pure numpy) + optional Qiskit adapter |
| `kubona/schema/providers/` | Capability-schema JSON — onboarding = data + plugin (3 initial entries) |
| `kubona/provenance/ledger.py` | Content-addressed, hash-chained append-only ledger |
| `kubona/cost/estimator.py` | Pricing-model estimate + mandatory simulator pre-flight |
| `kubona/circuits/` | SDK-independent circuit IR (QASM 2 subset parser) |

## Phase 1 additions (cost governance + sessions + ZNE + QIR)

- **Session abstraction** (`kubona/session.py`) — `job_id` sessions persist to
  `KUBONA_STATE_DIR`; jobs and spend are threaded to a session.
- **Cost governance** (`kubona/cost/governance.py`) — hard per-session budget
  ceilings (enforced at write time), two-stage approval gate (REVIEW -> HOLD ->
  APPROVED/REJECTED), real-time burn-rate telemetry.
- **x402-compatible payment interface** (`kubona/cost/payment.py`) — quote/verify
  contract; free tier now, X402 stub (activation Phase 5+).
- **ZNE mitigation** (`kubona/mitigation/`) — noise-aware density-matrix backend +
  zero-noise extrapolation (unitary folding, Richardson) — the first mitigation.
- **QIR lowering** — Python reference pipeline (`kubona/qir/`) + the **Rust core**
  crate (`crates/qir-core`, cargo test: parse -> transpile u3/u2/u1 -> QIR-like IR).
- **Phase 1 gate test** (`tests/test_phase1_gate.py`) — budget-enforced VQE loop
  with step-level provenance, plus the paid-provider approval + x402 quote flows.

Run everything: `uv run --project . pytest -q` or `.venv/bin/python -m pytest -q`.

## Phase 2 additions (multi-provider + control plane)

- **Providers (6):** + quantinuum (pytket), rigetti (pyQuil), neutral-atom
  (Pulser) capability entries with optional-dependency plugins — onboarding is
  data + plugin; registry remains honest when deps/credentials are absent.
- **Circuit cutting** (`kubona/cutting/`) — LOCC wire cut of a CNOT for
  Z-diagonal target expectations, numerically verified vs. the uncut simulator;
  production path (Qiskit Circuit Knitting Toolbox) is hardening.
- **Identity groundwork** (`kubona/identity/`) — Ed25519 keypairs, owner-delegated
  authority with scopes/expiry, instant cascade revocation (NIP-OA pattern).
- **Control plane backend** (`kubona/control/` + `control` MCP server) — loop
  supervisor: register/step/list/status/pause/kill — the console's watch+interrupt.
- **LangGraph shim** (`kubona/orchestration/`) — Kubona tools as LangGraph tools +
  a reference VQE StateGraph (third-party framework drives Kubona; `[orchestration]` extra).
- **Dart console scaffold** (`console/`) — models mirroring MCP payloads, gateway
  JSON-RPC client, the two priority surface contracts; widgets build on a
  Flutter-equipped machine.
- **Phase 2 gate test** (`tests/test_phase2_gate.py`) — watch + interrupt a loop
  over the control MCP server (44 tests total).

## Qiskit-first development & the console (maintainer direction, 2026-08-16)

- **Qiskit as the dev provider:** `qiskit-aer-simulator` (free, GA) runs the full
  governed chain via Qiskit 2.x + AerSimulator — no credentials needed; real
  hardware providers (ibm-qiskit, quantinuum, rigetti, neutral-atom) keep
  capability entries + plugins that fail with actionable guidance until
  credentials/SDKs exist. Open-source-first: Qiskit/Aer (Apache-2.0), Flutter (BSD-3).
- **Console REST API** (`kubona/console_api.py`): read/approve-only surface the
  console talks to — sessions, burn rate, approvals (hold/approve/reject), loops
  (pause/resume/kill). **No job-submission endpoints** (ui-ux-vision hard rule #1).
  CORS `*` is dev-only (Phase-3 hardening). Run: `uv run --project . kubona-console-api`
  (or `python -m kubona.console_api`, default http://127.0.0.1:8000).
- **Flutter console** (`console/`): builds for web (`flutter build web`), two
  Phase-2 surfaces live (Cost & Approvals, Live Loops) wired to the REST API.
  Requires Flutter SDK (installed via Homebrew 2026-08-16) and
  `XDG_CONFIG_HOME` writable (see ~/.kubona-config on this machine).
- **Known upstream quirk:** mcp 2.0's stdio client hangs when closing one of
  several concurrently-open sessions — tests use one session at a time.

## Phase 3 additions (security hardening + provenance depth)

- **Hybrid PQC transport** (`kubona/security/hybrid_transport.py`) — X25519 +
  ML-KEM768 (FIPS 203) combined via HKDF into AES-256-GCM sessions; swappable
  suite (the Phase-0 cryptographic-agility interface pays off); handshake,
  tamper/wrong-peer tests.
- **Per-call identity-aware authorization** (`kubona/security/policy.py`) —
  fine-grained tool->scope map replacing coarse roles; agent delegation checked
  live (revocation immediate); wired into the gateway.
- **Immutable audit** — every gateway call lands in the hash-chained ledger
  (kind="audit"), doubling as the cost ledger.
- **Canary CI** (`kubona/canary/`) — `python -m kubona.canary` runs a Bell-state
  canary against executable providers, flags unexecutable ones, writes results to
  the ledger; a regression test proves it catches calibration-drift-style bugs.
- **Provenance as an MCP resource** — `provenance://records` + `provenance://best`
  (historical "what worked best per circuit hash" aggregation) on the provenance server.
- **Console follow-ons** — REST `/api/provenance` + `/api/fleet`; Flutter web app
  now has four tabs (Cost & Approvals, Live Loops, Provenance, Fleet).
- **Security review package** — `docs/security-review.md` (threat model, controls
  map, third-party checklist, known gaps).

## Phase 4 additions (evaluation + autoresearch engine + release prep)

- **Autoresearch engine** (`kubona/autoresearch/`) — the four-part harness
  contract (§11.2) + Orchestrator loop: keep-or-discard, structural write-scope,
  simulator-only, with a coarse-to-fine search (exploration + golden-section
  refinement). Three harnesses ship (§11.5): **VQE ansatz**, **mitigation
  (ZNE config)**, **correctness regression** (no mutation; gates releases).
  QAOA + compilation-pass follow once the engine is proven.
- **Search Mode (§11.4)** — `run_search_mode` autonomously finds an ansatz
  **beating a hand-designed baseline** (tested); files the result into the library.
- **GQE harness (4th, v2)** — `kubona/autoresearch/gqe.py`, same four-part
  contract with an n-gram proposal (transformer via Unsloth is the production path).
- **Algorithm library** (`kubona/library/`) — provenance-backed entries; the
  simulator -> cost-approved -> library pipeline is tested.
- **Meta-loop (§13, stretch)** — `kubona/autoresearch/meta.py`: propose-only
  strategy changes, shadow-tested, human-approved; write scope stops at search
  strategy (never platform code).
- **Evals** (`kubona/eval/`) — built-in code-gen corpus (bell/ghz3/qft2) with
  invariant checks, + agent-behavior evals (budget compliance, simulator-first,
  WBP-detection response). QuantumKatas/HumanEval/PennyLang plug into the same contract.
- **Canary now gates releases** — the correctness-regression harness runs in
  `canary.run_all` (a failing correctness check blocks).
- **Provider-registration SDK** (`kubona/sdk/register_provider`) — schema
  validate + sandbox smoke-test + data write; `docs/contribution-governance.md`
  (tool-surface security boundary). The Phase-4 gate's third-party-merge proof.

## Phase 5 additions (scale & breadth)

- **Breadth providers:** + `xanadu-strawberry-fields` (photonic/CV) and
  `dwave-annealing` (annealing). Annealing is QUBO/CQM, not gate-circuit — the
  AnnealingProvider has a separate credential-free `solve_qubo` path (small-n
  exact, larger via dimod-simulated-annealing when available); Xanadu plugin
  fails closed until credentials + Strawberry Fields wiring.
- **Routing / export control** (`kubona/routing/`) — jurisdiction +
  export-control-aware routing (`route()`), enforced at the routing layer
  before a job reaches a provider (§2.9). 'none' jurisdiction = world-open.
- **Multi-tenant scale** — `Ledger` and `SessionStore` accept a `tenant`
  (per-lab sharding, no cross-tenant collisions); ledger appends are atomic
  (O_APPEND) and hash-chain-guarded by a lock so concurrent writers are safe.
- **x402 activation + library metering** (`kubona/cost/metering.py`) — the
  algorithm library is a data refinery: free low-resolution summary, sub-threshold
  x402 microtransaction for the full validated config; the ledger doubles as the
  payment-proof ledger (payment + library_release records).
- **Meta-loop auto-application (§13.5)** — `MetaLoop.auto_apply_candidate`:
  an APPROVED proposal is auto-applied ONLY after the safety period elapsed AND
  it won in shadow mode across a minimum number of runs; otherwise it stays
  staged (never premature, never auto-applied without evidence).

## Phase boundaries (honest stubs)

- Compiler: validates + canonicalizes QASM 2 (real QIR/MLIR lowering = Phase 1, Rust core).
- Auth: dev token only (OAuth 2.1 + ML-KEM = Phase 3).
- Cost: static estimates + pre-flight (burn-rate telemetry, ceilings, two-stage
  approval, x402-compatible interface = Phase 1).
- Provenance: write/query over JSONL ledger (ledger-as-MCP-resource = Phase 3).
- Providers: local simulator runs; ibm-qiskit and cuda-q-simulator are registered
  capability data (ibm-qiskit needs credentials + Qiskit/Aer optional deps).
