# Circuit Codesign Engine

Kubona's Circuit Codesign subsystem implements evaluator-guided fault-tolerant circuit synthesis, durable evaluation receipting, multi-objective frontier search, seven-gate circuit claims (`C0`–`C7`), and cross-domain QEC logical-gate and magic-state factory verification.

## Architecture & Module Map

### Live API & Evaluation Spine
| Module | Responsibility |
|---|---|
| `platform/kubona/codesign/kickmix.py` | Single authoritative circuit validator (`TaskSpec`, `KickmixEvaluator`, exact/Clopper–Pearson `eps_upper_exact`, `toffoli_depth`, `circuit_depth`). |
| `platform/kubona/codesign/specs.py` | Named specs (`and2`, `cz-phase`, `copy1`, `xor2`, `steane-cnot`) and exhaustive truth-table specs (`n_inputs <= 8`). |
| `platform/kubona/codesign/tq.py` | Shared cost function `tq_cost(toffoli, qubits, kappa)` and `kappa_sweep`. |
| `platform/kubona/codesign/frontier.py` | `MapElitesFrontier` over `(T, Q)` log-buckets and `ParetoFrontier` over `(tq, toffoli_depth, qubits)`. |
| `platform/kubona/codesign/families.py` | Multi-family island search (`FamilySearch`) with durable `snapshot()` / `from_snapshot()` persistence and explicit stagnation stop rules (`check_stop_rule`). |
| `platform/kubona/codesign/claims.py` | `CircuitClaim` (`C0`–`C7`) verifier, canonical circuit hashing, and Ed25519 sign-off integration. |
| `platform/kubona/codesign/qec_joint.py` | Phase 4 cross-domain QEC verifier: Stim stabilizer preservation, logical Pauli action, exhaustive single-fault propagation (`[[7,1,3]]` Steane CNOT, `[[5,1,3]]` logical gates vs FunSearch benchmark `9` vs `84`), and S5 `[[15,1,3]]` magic-state factory verification (`distance >= 3`, yield exponent `gamma`). |
| `platform/kubona/codesign/packs.py` | `kubona-pack/v1` scaffold registry; refuses `active` promotion without non-empty `anchor_ref` and `proof_ref`. |
| `platform/bridge/codesign_ops.py` | Closed-vocabulary bridge RPC dispatcher (`spec`, `evaluate`, `batch`, `families`, `packs`, `pack_promote`, `qec_joint`, `budget`). |
| `platform/core/pkg/api/codesign_eval.go` | Go orchestrator proxy, durable `codesign_runs` SQLite receipts, session-scoped run reads, and bearer-gated pack promotion. |

### Paper-Scale Reference Modules
The remaining modules under `platform/kubona/codesign/` (`rs12.py`, `bm.py`, `eea12.py`, `qrom.py`, `horner.py`, `hhl.py`, `f12.py`, `f24.py`, `group72.py`, `gaugeloops.py`, `divsteps.py`, `divwalk.py`, `dqi.py`, `hlanczos.py`, `lanczos.py`) implement the paper-scale Reed–Solomon, HHL, and arithmetic synthesis benchmarks verified by unit tests and `verified.json`.

## HTTP API Surface

- `GET /api/codesign/tq`, `GET /api/codesign/frontier`, `GET /api/codesign/verified` — Go-native read endpoints.
- `POST /api/codesign/evaluate` — Stateless Kickmix evaluation when `session_id` is omitted; persists a durable session-scoped `codesign_runs` SQLite receipt (`run_id`, `artifact_sha256`, `spec_table_sha256`) when `session_id` is supplied.
- `GET /api/codesign/runs?session_id=...` and `GET /api/codesign/runs/{id}?session_id=...` — Session-scoped receipt retrieval.
- `POST /api/codesign/evaluate-batch` — Batch evaluation emitting both MAP-Elites and Pareto `(tq, toffoli_depth, qubits)` frontiers plus per-candidate `artifact_sha256`.
- `POST /api/codesign/families` — Island search with optional `campaign_id` snapshot persistence and `stop_rule` reporting.
- `GET|POST /api/codesign/packs` — Live pack registry.
- `POST /api/codesign/packs/{name}/promote` — Bearer-gated promotion requiring non-empty `anchor_ref` and `proof_ref` (refuses `400` otherwise).
- `POST /api/codesign/qec-joint` — Cross-domain QEC logical-gate (`steane_cnot`, `five_qubit`) and magic-state factory (`magic_state_15_to_1`) verification.
- `POST /api/codesign/rerun` — Governed LLM circuit-proposal loop with BYOK spend bounds and sealed ledger receipts.

## MCP Tools (`cost` Server)

External agents have parity with the console through 6 tools on the `cost` MCP server:
- `codesign_spec` (`cost:read`) — Inspect spec qubit layout and exhaustive domain size.
- `codesign_evaluate` (`cost:read`) — Evaluate a Kickmix artifact (and persist a receipt when `session_id` is provided).
- `codesign_frontier` (`cost:read`) — Project candidates onto the MAP-Elites and Pareto frontiers.
- `codesign_verify` (`cost:read`) — Fetch a persisted session-scoped `codesign_runs` receipt.
- `codesign_claim` (`cost:manage`) — Register a server-derived `CircuitClaim` (`kind="circuit"`) from session-owned exhaustive runs.
- `codesign_rerun` (`cost:manage`) — Launch a governed LLM proposal rerun (`budget_steps` in `[1, 48]`).

## `CircuitClaim` Gates (`C0`–`C7`)

Circuit claims share the `discovery_claims` table (`kind="circuit"`) and commissioned Ed25519 trust anchor with QEC code claims:
- `C0`: Artifact syntax and arity valid.
- `C1`: Exhaustive equivalence (`verification_method == "exhaustive"`, `eps_bound == 0`).
- `C2`: Frozen `TQ` computed by `tq.py`.
- `C3`: Advantage over a frozen baseline run on the same `spec_kind` and `kappa`.
- `C4`: Independent reproduction run matching `artifact_sha256` and verdict.
- `C5`: Held-out / fresh-seed check.
- `C6`: Novelty / non-known-circuit check.
- `C7`: Frozen artifact bundle + Ed25519 sign-off by a commissioned tenant-admin or reviewer key.

## Cost Model & Assumptions

- Primary multi-objective vector: `(TQ, toffoli_depth, qubits)` with strict Pareto non-dominance (`ParetoFrontier`). Improving `TQ` at the expense of `toffoli_depth` preserves both points on the frontier.
- Factory diagnostics (`factories.py`) require explicit `factory`, `code_distance`, and `cycle_us` parameters (no silent physics defaults).
- All evaluations are simulator/local-classical; no claim of quantum advantage is made.
