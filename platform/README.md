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

## Phase boundaries (honest stubs)

- Compiler: validates + canonicalizes QASM 2 (real QIR/MLIR lowering = Phase 1, Rust core).
- Auth: dev token only (OAuth 2.1 + ML-KEM = Phase 3).
- Cost: static estimates + pre-flight (burn-rate telemetry, ceilings, two-stage
  approval, x402-compatible interface = Phase 1).
- Provenance: write/query over JSONL ledger (ledger-as-MCP-resource = Phase 3).
- Providers: local simulator runs; ibm-qiskit and cuda-q-simulator are registered
  capability data (ibm-qiskit needs credentials + Qiskit/Aer optional deps).
