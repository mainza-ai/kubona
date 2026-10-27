# Kubona Platform — production-hardened (Phases 0-6 + M0/M1 hardening)

The federated MCP servers (compiler · execution · cost-governance ·
eval-provenance), gateway, provider trait, provenance ledger, plus the production
hardening: a single state root, fail-closed OAuth2 auth, package entrypoints
(`kubona serve`/`doctor`/`serve-gateway`), a persistent async job farm, the Rust
`qir-core` on the compiler path, an HTTP/ML-KEM transport, persistent audit, and
a reproducible CI pipeline. See `../wiki/roadmap.md` for the phase plan and
`../wiki/overview.md` for the architecture.

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
| `kubona/mcp/gateway.py` | Gateway: auth + per-call authz + dispatch + audit (in-memory + persistent JSONL export) |
| `kubona/mcp/auth.py` | AuthProvider contract + DevAuthProvider (local/CI; the real OAuth2 issuer is `security/oauth.py`) |
| `kubona/mcp/transport.py` / `kubona/mcp/http_gateway.py` | TransportProvider contract; in-process/stdio, and a real HTTP `/rpc` surface (OAuth2 bearer + X25519+ML-KEM envelope) |
| `kubona/providers/` | Provider trait + `LocalSimulator` (pure numpy) + Qiskit adapter; Quantinuum (pytket) &
  Rigetti (pyquil) conversions are **real-SDK validated**; IBM has a Qiskit Runtime path; D-Wave a QUBO solver |
| `kubona/schema/providers/` | Capability-schema JSON — onboarding = data + plugin (8 provider entries, backend targets set) |
| `kubona/provenance/ledger.py` | Content-addressed, hash-chained append-only ledger (tenant-sharded, atomic appends) |
| `kubona/cost/estimator.py` | Pricing-model estimate + mandatory simulator pre-flight |
| `kubona/circuits/` | SDK-independent circuit IR (QASM 2 subset parser); Rust `qir-core` lowers it on the compiler path |
| `kubona/config.py` / `run.py` | Single state root + `kubona serve`/`doctor`/`serve-gateway` package entrypoints |
| `kubona/jobs.py` | Persistent async job farm (restart-safe, `cancel_job`) |
| `kubona/secrets.py` | Per-provider credential store (env-backed, KMS-ready), fails closed |
| `kubona/eval/corpus_loader.py` | Declarative JSON eval-corpus loader (volume corpora drop in) |

## Phase 1 additions (cost governance + sessions + ZNE + QIR)

- **Session abstraction** (`kubona/session.py`) — `job_id` sessions persist to
  `KUBONA_STATE_DIR`; jobs and spend are threaded to a session.
- **Cost governance** (`kubona/cost/governance.py`) — hard per-session budget
  ceilings (enforced at write time), two-stage approval gate (REVIEW -> HOLD ->
  APPROVED/REJECTED), real-time burn-rate telemetry.
- **x402-compatible payment interface** (`kubona/cost/payment.py`) — quote/verify
  contract; free tier live, X402 **fails closed** (no proof accepted unless a real
  verifier is injected; activation Phase 5+).
- **ZNE mitigation** (`kubona/mitigation/`) — noise-aware density-matrix backend +
  zero-noise extrapolation (unitary folding, Richardson) — the first mitigation.
- **QIR lowering** — Python reference pipeline (`kubona/qir/`) + the **Rust core**
  crate (`crates/qir-core`, cargo test: parse -> transpile u3/u2/u1 -> QIR-like IR).
- **Phase 1 gate test** (`tests/test_phase1_gate.py`) — budget-enforced VQE loop
  with step-level provenance, plus the paid-provider approval + x402 quote flows.

Run everything: `uv run --project . pytest -q` or `.venv/bin/python -m pytest -q`.

## Phase 2 additions (multi-provider + control plane)

- **Providers (8):** local-simulator, qiskit-aer-simulator (both GA, credential-free),
  ibm-qiskit (Qiskit Runtime route), quantinuum (pytket), rigetti (pyQuil),
  neutral-atom (Pulser), xanadu (Strawberry Fields), dwave (annealing). The
  first two SDK connecters have **real** circuit->SDK conversions validated
  against the installed SDKs; IBM has a Qiskit Runtime route; D-Wave a
  credential-free QUBO solver; Pulser/SF are modality-gated (gate->AHS/CV is a
  distinct program model) and fail closed honestly. (Removed the phantom
  `cuda-q-simulator` entry — it had no execution plugin; CUDA-Q remains a
  considered-not-integrated optional route, not a shipped provider.)
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
  Flutter-equipped machine. (Retired per Q-22 — the human surface is
  TypeScript/Next.js; see `wiki/questions.md`.)
- **Phase 2 gate test** (`tests/test_phase2_gate.py`) — watch + interrupt a loop
  over the control MCP server.

## Qiskit-first development & the console (maintainer direction, 2026-08-16)

- **Qiskit as the dev provider:** `qiskit-aer-simulator` (free, GA) runs the full
  governed chain via Qiskit 2.x + AerSimulator — no credentials needed; real
  hardware providers (ibm-qiskit, quantinuum, rigetti, neutral-atom) keep
  capability entries + plugins that fail with actionable guidance until
  credentials/SDKs exist. Open-source-first: Qiskit/Aer (Apache-2.0), Flutter (BSD-3).
- **Console REST API:** the Next.js console talks **only to the Go
  orchestrator** (`platform/core`, `:8080`) — sessions, chat, loops,
  jobs, mandates, tenants, HF, mesh, fleet, events. `kubona/
  console_api.py` (`:8000` when run via `kubona serve`) keeps only
  routes with no Go equivalent (health, approvals, providers,
  reports, workflows, agents, labs, models, settings, science/*);
  its sessions/chat/loops/events/hf/mesh/fleet/provenance/
  custom-agent duplicates were retired 2026-10-12 (separate stores
  behind the same paths = split-brain). **No job-submission
  endpoints** on either surface (ui-ux-vision hard rule #1).
- **Flutter console** (`console/`): builds for web (`flutter build web`), two
  Phase-2 surfaces live (Cost & Approvals, Live Loops) wired to the REST API.
  Requires Flutter SDK (installed via Homebrew 2026-08-16) and
  `XDG_CONFIG_HOME` writable (see ~/.kubona-config on this machine).
  (Retired per Q-22 — see above.)
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

## Phase 6 additions (science discovery & collaboration)

- **Workflow engine** (`kubona/workflows/`) — YAML DSL: steps -> MCP tools with
  cost/capability gates; `{{prev.<step>.<field>}}` templating; agent-created
  workflows require human approval (§13.4); every step ledger-recorded; workflow
  library (user/agent-created, reusable).
- **Agent registry & personas** (`kubona/agents/`) — agent = persona + scopes +
  budget; **no privilege escalation** on agent-created agents (scopes must be a
  subset of the creator's) + `create_as_agent`.
- **Experiment notebook / results studio** (`kubona/reports/`) — structured
  reports (objectives · methods · protocol · results · analysis · conclusions ·
  citations · how-to-reproduce); every claim carries a ledger/citation ref.
- **Scholarly integrations** (`kubona/integrations/`) — governed, provenance-first
  sources (arXiv, OpenAlex, + in-repo `mock`); each fetch writes {source, license,
  fetched_at, freshness}; offline-tolerant (never crashes a campaign).
- **Full discovery loop** (`kubona/discovery/`) — ground → hypothesize → design →
  experiment (Search Mode) → analyze (ZNE) → report → archive → meta-learn,
  inside the cost gates, tenant-scoped. **The Phase-6 gate test runs it end to end.**

## Hardening & remaining-implementation closures (2026-08-16)

- **OAuth 2.1-shaped tokens** (`kubona/security/oauth.py`) — a real issuer signs
  short-lived, scoped Ed25519 tokens; gateway verifies them (swappable with dev
  auth). Expiry/tamper/scope tested.
- **ML-KEM secure RPC** (`kubona/security/secure_rpc.py`) — encrypts a governed
  gateway call envelope end-to-end over the hybrid transport; tamper rejected.
- **Routing enforcement in execution** — `submit_job` offers
  `caller_jurisdictions`/`caller_export_class`/`require_review`; ineligible jobs
  are blocked before execution (the §2.9 compliance gate on the call path).
- **Eval corpus volume** — the built-in code-gen corpus grew to **7 tasks**
  (bell, ghz3, ghz4, bell-pairs, qft2, qft3, grover-2) with invariant checkers.
- **Phase-6 console surfaces** — REST `/api/reports` `/api/workflows` `/api/agents`
  `/api/labs`; the Flutter web console is now **6 tabs** (… Notebook · Labs &
  Workflows); `ReportStore` persists campaign notebooks; `Lab` registry.
- ReportStore: `kubona/reports/notebook.py`; Labs: `kubona/labs.py`.


## Production hardening (M0/M1 closures)

- **Single non-`/tmp` state root** — `kubona/config.py`: `state_root()` (default
  `~/.kubona`, or `KUBONA_STATE_DIR`); sessions, ledger, governance, identity,
  agents, the algorithm library, reports, labs, workflows, settings, meta-proposals
  all persist under one env-configurable root. **No more data loss on reboot.**
- **Fail-closed production auth** — OAuth2 signed tokens carry `aud`/`nbf`
  (verified); `KUBONA_ENV=production` requires `KUBONA_CONSOLE_TOKEN` (else
  every request is unauthorized); console uses constant-time token compare; the
  `kubona run`/`serve-gateway` gateway wires the real issuer.
- **Package entrypoints** — `kubona serve / doctor / serve-gateway`
  (`[project.scripts]`); starlette + uvicorn are hard deps.
- **Persistent async job farm** — `kubona/jobs.py`: `JobStore` (JSONL,
  restart-safe) + background `JobRunner`; `submit_job(wait=True/False)`,
  `cancel_job` (scoped to `execution:override`); provenance on every transition.
- **Rust QIR core on the compiler path** — `lower_circuit` runs `qir-core`
  (`engine: rust-qir-core`) with a Python fallback; CI builds + wires
  `KUBONA_QIR_CORE`.
- **HTTP transport + secrets** — `kubona/mcp/http_gateway.py` (real uvicorn `/rpc`
  with OAuth2 bearer + a genuine two-party X25519+ML-KEM envelope, tamper-rejected);
  `kubona/secrets.py` per-provider credentials that fail closed.
- **x402 fail-closed** — no proof is ever accepted unless a real verifier is injected.
- **Persistent audit** — gateway audit writes a restart-safe JSONL export by default.
- **CI reproducible & green locally** — `platform/scripts/ci.sh` runs the entire
  pipeline (compile, Rust build+`--lib`, pytest, canary, correctness release-gate,
  doctor + fail-closed check); `.github/workflows/ci.yml` + `nightly-canary.yml`.
  The canary treats credential/SDK gaps as SKIP, never a provider regression.
- **Production end-to-end test** — `tests/test_e2e_production.py` proves OAuth →
  HTTP/ML-KEM → job farm → provider → result → audit → scoped-deny composes as
  ONE system.

## Local LLM integration (Ollama · OMLX · LM Studio · llama.cpp)

- **LLM layer** (`kubona/llm/`): `AbstractLLM` + OpenAI-compatible adapter and
  an Ollama-native adapter, with `json_mode` (forced-JSON) for reliable tool-call
  output. Offline-tolerant with a deterministic `ScriptedLLM` fallback.
- **Auto-detect (`LocalModelRegistry`)** scans a range of ports (incl. OMLX's
  managed-server port **8787**, Ollama 11434, LM Studio 1234, MLX/llama.cpp 8080,
  …) probing /v1/models (OpenAI-compat) and /api/tags (Ollama); finds the real
  port, classifies the provider, lists the models. Override via `KUBONA_LLM_PORTS`
  / `KUBONA_LLM_BASE_URL`. Verified live against the on-host OMLX server.
- **LLM agent loop** (`kubona/agents/loop.py`): a governed ReAct loop — the local
  model picks tools (exact signatures are fed), executes under the cost/capability
  gates, with an **arg-normalization layer** that maps model-arg-synonyms onto the
  tool's real parameters. Verified live: the OLMX Qwen model transpiled a circuit
  and drove the gateway.
- **Settings** (`kubona/settings.py`): persisted user settings (provider/model/
  temperature); `GET/PUT /api/settings`; live host-model snapshot kept separate.
- **Console**: a **Models** tab shows the models auto-detected on this host and
  lets the user set the active provider/model/temperature (saved to settings).

## Current program (2026-10): evaluator-guided circuit co-design

- **What:** independent verification of the evaluator-guided
  quantum-circuit co-design in Gu et al. `arXiv:2610.22367v1` (SI held
  locally, never committed): divstep walks, branch decoders, trace
  readout, gauge-loop scheduling, Hermitian/Hermitian-locator walks,
  and the Reed–Solomon decode chain — all proven by exhaustive local
  simulation (Qiskit Aer / exact field math). No QPU, no LLM calls
  in tests.
- **Python** (`kubona/codesign/`, 37 modules): step/walk builders,
  minimized + factored Boolean emitters, BM/Chien/Forney/phase-bit
  machinery, gauge scheduler + local search, Hermitian walkers.
  49 slice test files (`tests/test_codesign_slice2.py` …
  `test_codesign_slice50.py`) plus harness/API/audit suites.
- **Go** (`core/pkg/codesign/`, `core/pkg/api/codesign.go`): TQ cost
  model + frontier served read-only to the console.
- **Console:** `CodesignCockpit` surface (read-only scoring).
- **Blueprint:** `../wiki/` is the design record (overview, questions
  work-list, per-source summaries, decision log in `log.md`).
- **Run it:** `.venv/bin/python -m pytest platform/tests/test_codesign_slice50.py -q`
  (single slice) or the full suite (961 passed, 3 skipped as of 2026-10-19).
- **Bar:** real logic or fail-closed refusal; no weakened asserts;
  every number measured and pinned, paper deltas stated, never closed
  by invention.

## Phase boundaries (honest stubs)

- Compiler: validates + canonicalizes QASM 2 (real QIR/MLIR lowering = Phase 1, Rust core).
- Auth: dev token only (OAuth 2.1 + ML-KEM = Phase 3).
- Cost: static estimates + pre-flight (burn-rate telemetry, ceilings, two-stage
  approval, x402-compatible interface = Phase 1).
- Provenance: write/query over JSONL ledger (ledger-as-MCP-resource = Phase 3).
- Providers: local simulator runs; ibm-qiskit and cuda-q-simulator are registered
  capability data (ibm-qiskit needs credentials + Qiskit/Aer optional deps).

## Operator refresh: calibration and rate cards

Both refreshes are **dry-run by default**; `--write` is explicit. They refuse
unmeasured or unverified input rather than stamping stale data as fresh.

```bash
# Live device calibration from a provider plugin (needs that vendor's key,
# e.g. QISKIT_IBM_TOKEN for ibm-qiskit). Unmeasurable => exit 2, no write.
cd platform
.venv/bin/python -m kubona.fidelity.refresh \
  --device ibm-heron --provider ibm-qiskit --write

# Or from a captured snapshot:
.venv/bin/python -m kubona.fidelity.refresh \
  --device ibm-heron --input snapshot.json --write

# Verified vendor rate card (date + pricing URL + plan notes are required).
.venv/bin/python -m kubona.cost.rate_card \
  --provider ionq --input rate-card.json --write
```

`refresh` recomputes `calibrated_at`/`version`/`source`; the fidelity gate
treats a missing or old stamp as stale. `rate_card` validates positive rates,
USD, an ISO `verified` date, and the vendor URL before atomically replacing
the provider schema's `pricing` object. Vendor pricing pages are not scraped:
an operator records the verified card. CI runs both suites without keys.
