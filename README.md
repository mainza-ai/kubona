<div align="center">

<img src="assets/milimo_kubona_logo.png" alt="Kubona Logo" width="140" />

# Kubona

### *See, observe, discover — then know.*

**An open-source quantum-computing and AI-enhanced scientific discovery platform for autonomous agents and human operators.**

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License" /></a>
  <a href="https://creativecommons.org/licenses/by/4.0/"><img src="https://img.shields.io/badge/Docs-CC--BY--4.0-orange.svg" alt="Documentation License" /></a>
  <a href="platform/kubona/"><img src="https://img.shields.io/badge/Python-Runtime%20%26%20Bridge-3776AB?logo=python&logoColor=white" alt="Python" /></a>
  <a href="platform/core/"><img src="https://img.shields.io/badge/Go-Orchestrator-00ADD8?logo=go&logoColor=white" alt="Go" /></a>
  <a href="platform/crates/"><img src="https://img.shields.io/badge/Rust-QIR%20%26%20IFC-DEA584?logo=rust&logoColor=white" alt="Rust" /></a>
  <a href="platform/console/"><img src="https://img.shields.io/badge/Next.js-Sovereign%20Console-3178C6?logo=typescript&logoColor=white" alt="TypeScript" /></a>
  <a href="wiki/"><img src="https://img.shields.io/badge/Blueprint-156%20Wiki%20Pages-purple?logo=obsidian&logoColor=white" alt="Wiki" /></a>
</p>

</div>

---

**Kubona** (from Chitonga *ku-bona*, "to see, observe, perceive") is a local-first platform that pairs a sovereign Go orchestrator and hash-chained SQLite provenance ledger with a Python quantum/chemistry compute bridge (PySCF, Qiskit Aer, PennyLane), a Rust QIR/IFC compiler core, a 6-server MCP tool layer (38 tools over stdio), and a Next.js operator console.

> [!IMPORTANT]
> **Deployment Target — Local-Private Production:** Kubona is engineered for single-host local-private operation (`127.0.0.1` bindings for API `:8080` and Console `:3000`, Unix Domain Socket bridge transport, Ed25519 mandate/session signing, and fail-closed budget/fidelity gates). All simulators, classical anchors, and local LLMs run offline at `$0.00`; cloud QPU dispatch and cloud LLMs (BYOK) are explicit opt-ins. See [`docs/local-deployment.md`](docs/local-deployment.md).

---

## Architecture Overview

<div align="center">
  <a href="assets/misc/Kubona_Platform_Overview_Infographic.png">
    <img src="assets/misc/Kubona_Platform_Overview_Infographic.png" alt="Kubona Platform Architecture and Overview Infographic" width="100%" style="border-radius: 8px;" />
  </a>
</div>

- **Architectural Walkthrough (Video):** [Architecting the AI Quantum Bridge Inside the Kubona Stack](https://youtu.be/Clo0PAk7Oj0)
- **Executive Presentation Deck (PDF):** [`assets/misc/Kubona_Quantum_AI_Discovery_Platform.pdf`](assets/misc/Kubona_Quantum_AI_Discovery_Platform.pdf)
- **Living Blueprint & Audit Trail:** [`wiki/overview.md`](wiki/overview.md) · [`wiki/roadmap.md`](wiki/roadmap.md) · [`docs/repo-counts.md`](docs/repo-counts.md)

### Core Principles

1. **Receipts Don't Lie:** Every spend reservation, calibration check, experiment result, and agent action is recorded in a hash-chained SQLite WAL ledger with SHA-256 result receipts.
2. **Fail-Closed Refusal:** No fabricated metrics, silent fallbacks, or unanchored quantum advantage claims. Missing credentials, stale calibration (>24h), or unconfigured sandboxes refuse explicitly.
3. **Classical Anchoring:** Every quantum experiment computes or binds an explicit classical reference (PySCF RHF/CASCI/FCI, exact diagonalization, or MPS/DMRG) alongside measured outputs.

---

## System Topology

| Path | Layer | Role |
|---|---|---|
| [`platform/core/`](platform/core/) | **Go Orchestrator (`:8080`)** | HTTP/SSE API, hash-chained provenance ledger, job admission & fidelity gates, campaign replanning, flight recorder, and Docker/`runsc` sandbox runner. |
| [`platform/bridge/`](platform/bridge/) | **Quantum Bridge (`UDS / gRPC`)** | PySCF, Qiskit Aer, and PennyLane compute service over `unix:///run/kubona/bridge.sock` for chemistry anchors, lattice solvers, and QPU dispatch. |
| [`platform/crates/`](platform/crates/) | **Rust Core** | `qir-core` (OpenQASM 3.0 to LLVM IR lowering) and `ifc-core` (information-flow confidentiality lattice). |
| [`platform/kubona/`](platform/kubona/) | **Python Runtime & MCP** | 6 stdio MCP servers (38 tools: `execution`, `compiler`, `cost`, `provenance`, `control`, `world`), mixed-state autodiff, ansatz transfer priors, and register packing. |
| [`platform/console/`](platform/console/) | **Sovereign Console (`:3000`)** | Next.js operator workbench with Spatial Canvas (3D Observatory), Results Studio, Experiment & Discovery Lab (`Cmd+E`), Epistemic Tribunal, Flight Recorder, and WebCrypto Ed25519 signing. |
| [`platform/contracts/`](platform/contracts/) | **Shared Contracts** | JSON schemas (`experiment-result-v1`, OpenAPI, console contract inventory), error catalog, rate cards, and device calibration profiles. |
| [`wiki/`](wiki/) | **Blueprint Wiki** | 156 cross-linked design, entity, and audit pages validated in CI by `scripts/wiki-lint.py --strict`. |

---

## Scientific Capabilities

All 13 experiment kinds execute through the governed Go job rail, validate against [`platform/contracts/experiment-result-v1.schema.json`](platform/contracts/experiment-result-v1.schema.json), fold into campaign state, and render in the Console's **Experiment & Discovery Lab** (`Cmd+E` / `Ctrl+E`) and **Results Studio**:

| Capability | Kind | Measurement | Classical Anchor |
|---|---|---|---|
| **Molecular VQE** | `vqe` (`vqe_run`) | Shot-sampled Pauli expectation energy | Live PySCF RHF / CASCI / FCI |
| **Geometry Optimization** | `geometry_optimization` | VQE at classically relaxed bond geometry | Curated scan or caller-geometry RHF/STO-3G BFGS + FCI |
| **Repeated VQE** | `vqe_repeats` | Multi-seed mean energy, shot σ, and between-run SE | Full FCI at identical geometry |
| **Potential-Energy Scan** | `pes` (`vqe_pes`) | Bond-distance energy curve across geometry grid | Per-point live RHF & FCI |
| **Hardware PES** | `hardware_pes` | Single QPU batch across source PES geometries | Per-point FCI anchors (optional ZNE) |
| **Excited State** | `excited_state` | Ground + overlap-deflated first excited state (`H2`, `LiH`, `BeH2`, `H2O`) | Exact active-space spectrum |
| **Reaction Energy** | `reaction` | Reactant-to-product ΔE (`H2`, `LiH`, `BeH2`, `H2O`) | Active-space endpoint + exact atomic FCI |
| **1D Spin Chain** | `spin_chain` | Variational energy, magnetization, and gap (`heisenberg`, `xxz`, `tfim`) | Exact diagonalization (`COBYLA` + multi-start `L-BFGS-B`) |
| **DMRG Scaling** | `thermodynamic_limit` | Finite-size MPS/DMRG energy per site & $1/N$ extrapolation | Bethe/Pfeuty analytic limit (`heisenberg`/`tfim`) |
| **2D Quench Dynamics** | `lattice_dynamics` | Exact finite-size 2D TFIM $\langle Z \rangle(t)$ trajectory | Sparse Krylov `expm_multiply` time evolution |
| **MaxCut QAOA** | `qaoa` (`qaoa_run`) | Shot-sampled cut value and approximation ratio | Exact brute-force MaxCut |
| **Math Conjecture** | `conjecture` | Exact symbolic/rational search + holdout verification | Finite-scope holdout certificate + known-law registry |
| **Discovery Protocol** | `/api/discovery/{plan,run}` | 1–5 sequential governed jobs (`then` / `->` / `;` pipelines) | Deterministic planner + ledger-linked job summaries |

---

## Quickstart: Local-Private Docker Stack (Recommended)

The production stack binds strictly to `127.0.0.1:8080` (API) and `127.0.0.1:3000` (Console), communicating with the Python bridge over an internal Unix Domain Socket volume.

```bash
# 1. Configure environment & keys
cp .env.example .env
chmod 600 .env
# Populate KUBONA_API_TOKEN and KUBONA_SECRET_KEY in .env (e.g. openssl rand -hex 32)
# Optional: generate a reviewer mandate key via (cd platform/core && go run ./cmd/kubonad keygen)

# 2. Build and start bridge, preflight gate, kubonad, and console
docker compose --env-file .env -f platform/docker/compose.prod.yml up --build -d

# 3. Verify health, readiness, and loopback-only exposure
curl -fsS http://127.0.0.1:8080/api/health
curl -fsS http://127.0.0.1:8080/api/ready
scripts/local-deployment-check.sh

# 4. Run the deployment doctor
cd platform && .venv/bin/python -m kubona.run doctor --env-file ../.env
```

Open **`http://localhost:3000`** in your browser. Press **`Cmd+E`** (`Ctrl+E`) or click **Experiment Lab** in the top bar to launch single experiments, multi-step discovery protocols, or governed QPU runs.

---

## Quickstart: Native Development ($0.00 / Zero-QPU)

```bash
# 1. Start the Python Quantum Bridge (Unix socket or localhost:50051)
cd platform/bridge
../.venv/bin/python server.py

# 2. Start the Go Orchestrator (:8080)
cd platform/core
go run ./cmd/kubonad --port 8080

# 3. Start the Next.js Sovereign Console (:3000)
cd platform/console
pnpm dev --port 3000
```

### Example API Calls

```bash
# Run an XXZ spin-chain experiment with anisotropy Δ = 1.5
curl -fsS -X POST http://127.0.0.1:8080/api/experiments \
  -H "Authorization: Bearer $KUBONA_API_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"session_id":"<session-id>","spec":{"kind":"spin_chain","model":"xxz","n_sites":4,"coupling":1.0,"anisotropy":1.5,"field":0.4,"layers":3,"shots":8192},"budget_usd":1}'

# Plan and execute a multi-step discovery protocol
curl -fsS -X POST http://127.0.0.1:8080/api/discovery/plan \
  -H "Content-Type: application/json" \
  -d '{"goal":"optimize LiH geometry then run PES scan then check excited state"}'
```

---

## Verification & Quality Gates

```bash
# Go Orchestrator unit & integration suite
cd platform/core && go test ./...

# Rust native QIR compiler & IFC lattice
cargo test --manifest-path platform/crates/qir-core/Cargo.toml
cargo test --manifest-path platform/crates/ifc-core/Cargo.toml

# Python runtime, bridge, DMRG, and scientific solvers
PYTHONPATH=platform/bridge:platform platform/.venv/bin/pytest platform/bridge/ platform/tests/

# Console TypeScript, ESLint, Vitest, and OpenAPI/contract gates
cd platform/console && pnpm exec tsc --noEmit && pnpm lint && pnpm test
python3 scripts/console-contract-inventory.py --check
node scripts/console-openapi.mjs --check

# Strict wiki blueprint lint
python3 scripts/wiki-lint.py --strict
```

---

## Author & License

- **Author:** Mainza Kangombe — Founder & Lead Architect, [Milimo Quantum](https://www.youtube.com/@MilimoQuantum) ([LinkedIn](https://www.linkedin.com/in/mainza-kangombe-6214295) · [@mainza-ai](https://github.com/mainza-ai))
- **Software License:** [Apache License 2.0](LICENSE) (see [`NOTICE`](NOTICE) and [`CONTRIBUTING.md`](CONTRIBUTING.md))
- **Documentation & Wiki License:** [Creative Commons Attribution 4.0 International (CC-BY-4.0)](https://creativecommons.org/licenses/by/4.0/)
