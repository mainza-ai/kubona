<div align="center">

<img src="assets/milimo_kubona_logo.png" alt="Kubona Logo" width="160" />

# Kubona

### *See, observe, discover — then know.*

**An open-source quantum-computing and AI-enhanced science discovery platform for autonomous AI agents, with an MCP tool layer (5 servers, 29 tools over stdio).**

<p align="center">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License" /></a>
  <a href="https://creativecommons.org/licenses/by/4.0/"><img src="https://img.shields.io/badge/Docs-CC--BY--4.0-orange.svg" alt="Documentation License" /></a>
  <a href="platform/kubona/"><img src="https://img.shields.io/badge/Python-49.7%25-3776AB?logo=python&logoColor=white" alt="Python" /></a>
  <a href="platform/core/"><img src="https://img.shields.io/badge/Go-33.4%25-00ADD8?logo=go&logoColor=white" alt="Go" /></a>
  <a href="platform/console/"><img src="https://img.shields.io/badge/TypeScript-16.1%25-3178C6?logo=typescript&logoColor=white" alt="TypeScript" /></a>
  <a href="platform/tests/"><img src="https://img.shields.io/badge/Tests-961%20Passed-brightgreen" alt="Tests" /></a>
  <a href="wiki/concepts/local-simulation-test-scenarios.md"><img src="https://img.shields.io/badge/Simulation-%240.00%20(Zero--QPU)-success" alt="Simulation" /></a>
  <a href="wiki/"><img src="https://img.shields.io/badge/Wiki-145%20Pages-purple?logo=obsidian&logoColor=white" alt="Obsidian Knowledge Base" /></a>
  <a href="https://www.linkedin.com/in/mainza-kangombe-6214295"><img src="https://img.shields.io/badge/Author-Mainza%20Kangombe-0A66C2?logo=linkedin&logoColor=white" alt="Author" /></a>
</p>

</div>

---

> **“To see is the founding act of science: observe, measure, record, understand.”**  
> **Kubona** draws its name from the Chitonga (*Tonga, Zambia*) infinitive: **ku-** (infinitive prefix) + **-bona** (*to see, observe, perceive*). Kubona systematically surfaces quantum phenomena, orchestrates autonomous agent experiments, and records immutable, reproducible findings.

> [!NOTE]
> **Active Research Platform**: Kubona is under active development. APIs, agent protocols, and the sovereign console evolve rapidly. See [`wiki/roadmap.md`](wiki/roadmap.md) for phased milestones and [`wiki/log.md`](wiki/log.md) for the latest research log.

---

## 📽️ Deep Dive: The AI Quantum Bridge

Watch the architectural walkthrough of how autonomous AI agents interface with the sovereign Go orchestrator and quantum execution runtime inside the Kubona stack:

<div align="center">
  <a href="https://youtu.be/Clo0PAk7Oj0" target="_blank">
    <img src="https://img.youtube.com/vi/Clo0PAk7Oj0/maxresdefault.jpg" alt="Architecting the AI Quantum Bridge Inside the Kubona Stack" width="85%" style="border-radius: 8px; box-shadow: 0 4px 20px rgba(0,0,0,0.15);" />
  </a>
  <p><em>▶️ <strong><a href="https://youtu.be/Clo0PAk7Oj0">Watch: Architecting the AI Quantum Bridge Inside the Kubona Stack</a></strong> (Milimo Quantum)</em></p>
</div>

---

## 🗺️ Platform Architecture & Overview

<div align="center">
  <a href="assets/misc/Kubona_Platform_Overview_Infographic.png">
    <img src="assets/misc/Kubona_Platform_Overview_Infographic.png" alt="Kubona Platform Architecture and Overview Infographic" width="100%" style="border-radius: 8px; box-shadow: 0 4px 20px rgba(0,0,0,0.15);" />
  </a>
  <p><em>System topology, language breakdown, execution pipeline, and verification gates. Click image to expand.</em></p>
</div>

### 📑 Executive Presentation Deck
Explore the comprehensive 10-slide architectural deck:
👉 **[Download / View the Kubona Quantum & AI Discovery Platform Deck (PDF)](assets/misc/Kubona_Quantum_AI_Discovery_Platform.pdf)**
*(Features: epistemic scientific loops, agentic autonomy pillars, polyglot system topology, evaluator-guided circuit co-design, and $0.00 local simulation).*

---

## 🔬 The Epistemic Scientific Loop

Kubona structures scientific inquiry for autonomous agents across four deterministic stages:

```
  ┌──────────────┐       ┌──────────────┐       ┌──────────────┐       ┌──────────────┐
  │  1. Observe  │ ───►  │  2. Measure  │ ───►  │  3. Record   │ ───►  │4. Understand │
  └──────────────┘       └──────────────┘       └──────────────┘       └──────────────┘
  Surface quantum        Execute agent-         Log tamper-proof       Synthesize
  phenomena              directed algorithms    provenance ledger      reproducible
  systematically.        (VQE, QAOA, circuits). (SQLite WAL / edge).   discoveries.
```

- **Agentic Autonomy**: Sovereign multi-user labs, user- and agent-authored workflows, and meta-level self-evolution loops.
- **Quantum Core**: QIR/MLIR execution pipelines, PySCF/Qiskit bridge, evaluator-guided circuit co-design, and built-in cost governance.
- **Science Discovery**: Autonomous autoresearch engine, interactive experiment notebooks, and an organized results studio.

### Verifiable Tool Grounding (MCP Layer)

Every epistemic stage translates into concrete, deterministic tools exposed through the MCP layer (5 federated servers, 29 tools over stdio, digest-pinned in [`platform/contracts/mcp-catalog.json`](platform/contracts/mcp-catalog.json)):

* **Observe & Measure**: `execution` server (circuit execution, VQE sweeps, state-vector simulation, hardware capability probes).
* **Synthesize & Optimize**: `compiler` server (pass portfolio, QIR/OpenQASM3 transpilation, circuit co-design verification).
* **Record & Attest**: `provenance` server (immutable SQLite WAL ledger, Merkle audit chains, confidentiality lattice labels).
* **Govern & Gate**: `cost` & `control` servers (spend mandates, pre-flight budget checks, tenant isolation, operator approval cards).

---

## 🏛️ System Topology & Codebase Layout

| Directory | Layer | Description |
|---|---|---|
| [`platform/core/`](platform/core/) | **Go Orchestrator** | Sovereign orchestrator, immutable provenance ledger, job coordination, tribunal, and core API on `:8080`. |
| [`platform/kubona/mcp/`](platform/kubona/mcp/) | **MCP Tool Layer** | 5 federated Python servers mounting 29 tools over stdio (`compiler`, `execution`, `cost`, `provenance`, `control`)*. |
| [`platform/console/`](platform/console/) | **Sovereign Console** | Next.js sovereign UI on `:3000` with experiment studio, tribunal, flight recorder, and copilot chat. |
| [`platform/kubona/`](platform/kubona/) | **Python Runtime** | Agent execution runtime, evaluator-guided circuit co-design program, and ML pipelines. |
| [`platform/bridge/`](platform/bridge/) | **Quantum Bridge** | PySCF and Qiskit bridge facilitating communication between AI agents, classical chemistry, and quantum hardware/simulators. |
| [`wiki/`](wiki/) | **Obsidian Knowledge Base** | Living blueprint (145+ pages) cataloging entities, concepts, source audits, and ranked open questions. |
| [`raw/`](raw/) | **Immutable Sources** | Read-only ingested source documents, architecture papers, and implementation plans. |
| [`scripts/`](scripts/) | **Tooling & CI** | Automated structural wiki linting (`scripts/wiki-lint.py`) and CI validation runners. |
| [`assets/misc/`](assets/misc/) | **Media & Presentations** | Platform overview infographic and executive presentation deck. |
| [`AGENTS.md`](AGENTS.md) | **Maintainer Operating Rules** | Core engineering standards, production bar, and invariants for human and AI collaborators. |

*\*Note on MCP Serving: The 5 MCP servers currently execute over stdio (`python -m kubona.mcp.servers.<name>`) and in-process test harnesses; standalone compose container serving is in progress.*

---

## ⚡ Quickstart: Run It Locally ($0.00 / Zero-QPU)

Work with ten adopted local-simulation scenarios ([`wiki/concepts/local-simulation-test-scenarios.md`](wiki/concepts/local-simulation-test-scenarios.md)) — including Co-pilot Chat, Tribunal, Flight Recorder, and On-Demand Widgets — at **$0.00 cost** without requiring physical QPU hardware:

### 1. Launch the Go Orchestrator (Backend API)
```bash
cd platform/core
go build -o /tmp/kubonad ./cmd/kubonad
/tmp/kubonad -port 8080
```

### 2. Launch the Sovereign Next.js Console (Frontend)
```bash
cd platform/console
NEXT_PUBLIC_KUBONA_CONSOLE_TOKEN=dev-token pnpm dev --port 3000
```
*Access the sovereign console at `http://localhost:3000`.*

### 3. Run the Verification Suite
```bash
platform/.venv/bin/python -m pytest platform/tests/ -q   # 961 passed, 3 skipped
```

For extended console configuration, see [`platform/console/README.md`](platform/console/README.md).

---

## 🧭 Implementation Roadmap

- **Phases 0–5 (Quantum Core Foundations)**: Federated MCP servers, QIR/MLIR pipelines, cost governance, immutable provenance ledger, autoresearch engine (§11), and meta-level self-evolution loop (§13).
- **Phase 6 (Science Discovery & Synthesis)**: Multi-user lab identities, workflow and agent authoring studio, interactive experiment notebook, scholarly data integrations, and localized model training.

See [`wiki/roadmap.md`](wiki/roadmap.md) for the phased implementation plan and [`wiki/overview.md`](wiki/overview.md) for the architectural synthesis.

---

## 👤 Author & Creator

**Mainza Kangombe**  
Founder & Lead Architect — [Milimo Quantum](https://www.youtube.com/@MilimoQuantum)  
LinkedIn: [linkedin.com/in/mainza-kangombe-6214295](https://www.linkedin.com/in/mainza-kangombe-6214295)  
GitHub: [@mainza-ai](https://github.com/mainza-ai)

---

## 📄 License & Attribution

* **Software & Codebase**: Licensed under the [Apache License, Version 2.0](LICENSE).
* **Documentation & Living Wiki**: Licensed under the [Creative Commons Attribution 4.0 International (CC-BY-4.0)](https://creativecommons.org/licenses/by/4.0/) license.
* **Notices & Contributions**: See [`NOTICE`](NOTICE) for copyright and attribution notices, and [`CONTRIBUTING.md`](CONTRIBUTING.md) for contribution guidelines and Developer Certificate of Origin (DCO) details.
