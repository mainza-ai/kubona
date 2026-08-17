# Kubona
## The Ultimate Quantum-AI Agentic Platform: Gap Analysis, Architectural Expansion, and Implementation Plan

*Companion document to "The Ultimate AI-Quantum Agentic Platform: Architectural Design and Integration Strategy"*

---

## 1. Executive Summary

**Kubona** — Tonga (Zambia), *to see* — is the name for this platform going forward: the ultimate, **completely open-source** quantum-computing toolset for AI agents, unifying Qiskit, PennyLane, Q#, Cirq, and the major quantum cloud providers behind a single, agent-native MCP interface. This is a greenfield build: the entire stack — compiler, hardware-abstraction layer, MCP servers, cost-governance service, provenance ledger, and control-plane UI — is designed to be owned and shipped in the open under a permissive license, with no dependency on any closed commercial platform for core functionality. Where a commercial or proprietary service (e.g., qBraid) offers useful reach, it's treated strictly as an *optional, swappable* provider behind Kubona's own abstraction — never as infrastructure the open-source project depends on to function.

The source document lays a genuinely strong theoretical foundation: MCP as the agent-to-tool bus, QIR/MLIR as the hardware-agnostic compilation core, Rust for the performance-critical compiler and WASM edge path, and a sound rationale for the Python/TypeScript/Dart split. That part doesn't need to be re-argued.

**Scope revision (rev 3):** Kubona's mission extends beyond a quantum toolset for agents. The
platform is also an **AI-enhanced science discovery platform**: users and agents design, run,
and archive experiments across quantum hardware and classical/data resources; users and
agents can create their own workflows and agents for anything — including new scientific
research — and the platform can conduct its own experiments and return results in an
organized, systematic, reproducible form. Quantum processors remain the differentiating
instrument, but they are one instrument family inside a full discovery loop (ground →
hypothesize → design → experiment → analyze → report → learn). The science-discovery layer is designed in §14; it deliberately reuses the governance, provenance, and safety machinery of
§§4–5, §9, §11, and §13 instead of inventing parallel rails. Everything already planned in
Phases 0–5 is unchanged.

What it's missing is everything that turns a well-reasoned white paper into a buildable system: a concrete data/control-plane architecture, a real competitive/prior-art map (this space is less greenfield than the paper implies), a security model that accounts for the fact that MCP servers holding cloud QPU credentials are now a documented 2026 attack surface, a hardware catalog that stops at four providers when the addressable market has at least ten, an observability and cost-governance layer, a testing/eval pipeline beyond code-gen benchmarks, and — critically — a phased implementation plan with staffing, milestones, and a defensible MVP.

This document is organized in five parts:

1. **Gap analysis** — what's missing or underspecified, organized by architectural layer
2. **Competitive landscape** — who has already built pieces of this, and where the real whitespace is
3. **Expanded architecture** — the missing layers, filled in
4. **Implementation plan** — phased roadmap, team shape, and milestone criteria
5. **Science discovery & collaboration layer** (§14) — the human/AI workspace layer of the discovery platform: identities, user/agent-created workflows and agents, the discovery loop, systematic results, and data/model integrations

---

## 2. Gap Analysis

### 2.1 Hardware & provider coverage gaps

The original document covers IBM, PennyLane-adjacent simulators, Microsoft/Q#, Google Cirq, and D-Wave. That's four of at least eleven modalities/providers an AI agent would realistically need to route across in 2026:

| Missing provider | Modality | Why it matters |
|---|---|---|
| **IonQ** | Trapped ion | Referenced only via AWS Braket pricing table, never as a first-class SDK target. IonQ now runs its own direct API and EQC (error-mitigated) execution mode, and reports ~99.99% two-qubit fidelity — a routing-relevant differentiator the agent should know about directly, not just infer from Braket pricing. |
| **Quantinuum** | Trapped ion | Absent entirely except a passing QIR mention. Quantinuum's tket compiler and Helios system (48 error-corrected logical qubits at ~2:1 encoding) are currently the highest logical-qubit-density platform available — this is exactly the kind of routing decision an "ultimate" agent needs to make. |
| **Rigetti** | Superconducting | Mentioned once as a QIR target, never integrated as its own SDK (pyQuil) or accessed via its Cepheus multi-chip architecture. |
| **QuEra / Pasqal / Infleqtion** | Neutral atom | QuEra appears only inside the Braket pricing table. Neutral-atom analog Hamiltonian simulation (AHS) and Pasqal's Pulser SDK are a distinct programming paradigm the agent needs a dedicated toolchain for, not a pricing footnote. |
| **Xanadu (Strawberry Fields / PennyLane-photonics)** | Photonic (CV) | Not mentioned at all. Continuous-variable photonic QC is architecturally distinct (no discrete qubit model) and increasingly relevant for Gaussian boson sampling and ML-adjacent workloads. |
| **Microsoft Majorana / topological** | Topological | Q# is covered as a language, but the underlying hardware target (Majorana 1) isn't discussed as a distinct routing option with its own risk profile (early-stage, unverified error correction as of this writing). |
| **IQM / Oxford Quantum Circuits / NVIDIA cuQuantum-as-a-service** | Superconducting / GPU-accelerated simulation | Omitted; these are meaningful GPU-simulation and EU-sovereign hardware routes respectively. |

**Fix:** the provider abstraction layer needs a real capability schema (qubit count, modality, native gate set, logical-qubit availability, geographic/export-control zone, current queue depth, pricing model) rather than a fixed four-row table, so new providers can be onboarded as data, not code.

### 2.2 The document ignores an important prior-art reference point — qBraid

This matters for research and design purposes even though Kubona won't depend on it: **qBraid** is, as of mid-2026, a production *commercial* platform doing a large fraction of the hardware-abstraction work described in the original document — a runtime with cross-framework transpilation across 18+ quantum SDKs, centralized access to 34+ devices (IonQ, Rigetti, IQM, QuEra, Pasqal, Quantinuum, AWS, Azure), a CUDA-Q remote-target integration, and an on-demand GPU fleet. Strangeworks occupies similar territory for enterprise workflow orchestration. Neither is open source.

That's clarifying, not disqualifying, for a greenfield open-source build:

- **qBraid and Strangeworks are closed, human/notebook-first platforms.** They're useful as a design reference for what a mature hardware-abstraction API surface looks like, and they're worth being interoperable *with* (an optional connector for users who already have a qBraid account), but Kubona's own provider-abstraction layer needs to be built and shipped as genuine open source — the same way Qiskit, PennyLane, and Mitiq are — so the project isn't structurally dependent on a commercial vendor's roadmap, pricing, or continued existence.
- **This is real, substantial build scope**, not a wrapper task: direct SDK integrations (Qiskit, PennyLane, Q#, Cirq, pyQuil, tket, Ocean, Pulser) against each provider's native API, maintained in the open, with a plugin architecture (§4.4) that lets the community add new backends without waiting on Kubona maintainers.
- **The agentic control layer** — schema-safe tool calls, prompt-driven multi-step workflows, autonomous cost/routing decisions, agent-specific RBAC, MCP-native design — remains the genuinely novel part, because nobody (commercial or open source) has built that layer yet. But it needs a real open-source hardware-abstraction foundation under it, not a rented one.

### 2.3 MCP security model is dangerously underspecified

The original document's security section (RBAC + secrets manager) is a reasonable start but stops at 2023-era MCP security thinking. Two things changed by 2026 that the document needs to account for:

1. **MCP servers holding live cloud-QPU credentials are now a documented target class.** Security research through 2026 has converged on "shadow IT" MCP proliferation, static-API-key persistence, and lack of identity-aware, per-call scoping as the primary real-world MCP breach vectors — not theoretical. A quantum-hardware MCP server is a *higher*-value target than most, because a compromised session can both exfiltrate proprietary circuit IP and run up five/six-figure QPU compute bills before anyone notices.
2. **"Harvest now, decrypt later" (HNDL) is a first-class design constraint for a quantum platform specifically**, because the platform's own outputs (optimized circuits, algorithm results, potentially cryptographically sensitive computations) are exactly what an adversary sitting on encrypted MCP traffic today would want to decrypt once large-scale fault-tolerant hardware exists. Shipping a quantum-computing platform in 2026 without post-quantum-ready transport (hybrid TLS 1.3 + ML-KEM) is a real credibility gap for security-conscious enterprise or research customers.

**Fix, concretely:**
- OAuth 2.1 + scoped, short-lived tokens replacing static API keys for every MCP client-server handshake (this is now table-stakes, not aspirational)
- Per-call, identity-aware authorization (agent A can call `qiskit_simulator` but not `submit_production_qpu_job`) rather than coarse role buckets
- Hybrid PQC transport (ML-KEM alongside classical TLS) on the MCP gateway, with cryptographic agility (swappable cipher suites) built in from day one rather than retrofitted
- Immutable, append-only audit log of every tool call, every credential use, and every dollar spent — this doubles as the cost-governance ledger (§2.5)
- A sandboxed circuit-execution boundary that rate-limits and financially caps *before* a job reaches a paid queue, not just RBAC on the tool call

### 2.4 No agent-orchestration framework compatibility layer

The document treats "the AI agent" as a monolith. In practice, this platform needs to be consumable by whatever orchestration framework a given team already runs — LangGraph, CrewAI, AutoGen, or a custom multi-agent mesh. There's no discussion of:

- How multiple concurrent agents (e.g., a swarm doing parallel ansatz search) share rate limits, budget caps, and hardware queue slots without stepping on each other
- Whether the platform exposes a single MCP server or a federation of scoped servers (compile server, execution server, cost server, eval server) — the document's "context flooding" discussion argues for code-execution mode but doesn't resolve this at the server-topology level
- State/session handling across a long-running variational loop that may span hours and multiple agent invocations — MCP servers are nominally stateless; a VQE loop is not

**Fix:** define a session/job abstraction that survives across MCP calls (a `job_id` the agent threads through every tool call), and a federated MCP topology (compiler server / hardware-routing server / cost-governance server / eval server) instead of one monolithic server, so different agent frameworks can selectively mount only the capabilities they need.

### 2.5 No cost-governance or financial-telemetry *system* — only a routing heuristic

Section on cloud pricing describes routing logic in prose but never specifies the actual governance mechanism: hard budget ceilings, pre-flight cost estimation as a mandatory gate (not optional), spend approval workflows for anything above a threshold, or real-time burn-rate alerting. Given that a single VQE run mis-configured with the wrong shot count can burn thousands of dollars on trapped-ion hardware in minutes, this is a functional requirement, not a nice-to-have.

**Fix:** a dedicated cost-governance service sitting in front of every hardware submission — mandatory simulator pre-flight, hard per-job and per-agent-session spend ceilings, a two-stage approval gate for anything exceeding a configurable threshold, and real-time burn-rate telemetry surfaced back to the agent so it can self-correct before hitting a ceiling rather than after.

### 2.6 No experiment provenance / reproducibility layer

Nowhere does the document address how a circuit run, its parameters, the noise model, the mitigation technique applied, and the resulting data get tracked so results are reproducible and auditable — this matters enormously for research and regulatory contexts (pharma, finance). This is the quantum-computing analog of MLflow/W&B experiment tracking, and it's absent.

**Fix:** every job submission gets a content-addressed experiment record (circuit hash, backend, noise model, mitigation config, raw + mitigated results, cost, timestamp) written to an immutable store, queryable by the agent as an MCP *resource* so it can reason over past runs ("what mitigation technique worked best for this ansatz on this backend last time?").

### 2.7 Testing/eval framework is code-generation-only

QuantumKatas, Qiskit HumanEval, and PennyLang are good choices for *code-generation* correctness, but the document stops there. Missing:

- **Integration/regression testing** for the platform itself (does the QIR lowering pipeline still produce correct bitcode after a compiler change?)
- **Hardware-in-the-loop CI** — a nightly job that runs a canary circuit against each connected backend to catch silent provider API breakage or calibration drift before an agent hits it mid-workflow
- **Agent-behavior evals**, distinct from code-gen evals: does the agent correctly choose simulator-first, does it respect budget ceilings, does it correctly detect and respond to a barren plateau or WBP signal rather than looping forever

### 2.8 Client/frontend scope is thin

Dart/Flutter is assigned "observability dashboard" duties and nothing else. For a platform positioned as *the* agent-facing quantum toolkit, the human-facing surface needs more: a circuit visualizer, a cost/budget console, an approval-queue UI for the spend-governance gate above, and a way for a human to inspect and override an agent's in-flight VQE loop. None of this is in the original scope.

### 2.9 Licensing, export control, and legal surface not addressed

A platform that federates access to IBM, IonQ, Quantinuum, Rigetti, Microsoft, and D-Wave hardware inherits each provider's terms of service, and quantum hardware access can intersect with export-control regimes (EAR/ITAR-adjacent concerns have been raised around quantum computing as a dual-use technology). An autonomous agent routing jobs across borders/providers without geographic/entity-level awareness is a real compliance exposure the document doesn't mention at all.

**Fix:** provider capability schema (§2.1) includes a jurisdiction/export-control flag, and the RBAC layer (§2.3) enforces entity-level routing restrictions, not just tool-level permissions.

### 2.10 Open-source project's own defensibility isn't addressed

If this is meant to be genuinely open source and adopted by other builders (rather than a single internal tool), the document doesn't address: license choice (Apache-2.0 is the de facto standard in this ecosystem — qBraid, Mitiq, Qiskit addons all use it), a plugin/provider-registration model so third parties can add new backends without forking core, or a governance model for accepting community contributions to the MCP tool surface (which is a security-sensitive surface, per §2.3).

---

## 3. Competitive / Prior-Art Landscape (2026 snapshot)

| Project | What it actually does | Relationship to Kubona |
|---|---|---|
| **qBraid** | Hardware-agnostic runtime, 18+ SDK cross-framework transpilation, 34+ devices, CUDA-Q remote target, on-demand GPU fleet, Lab (Jupyter/VS Code) environment | Closed/commercial. Useful design reference and an optional interop connector for users who already have accounts there; **not a dependency** of Kubona's own open-source hardware-abstraction layer. |
| **NVIDIA CUDA-Q / cuQuantum** | GPU-accelerated statevector/tensor-network simulation, unified kernel language (`@cudaq.kernel`) with multi-vendor QPU targets | The GPU-simulation components are open source (Apache-2.0) and can be depended on directly as the default simulator backend without compromising Kubona's open-source posture. |
| **Strangeworks** | Enterprise quantum + "unconventional computing" workflow platform | Adjacent, closed/commercial. Not a dependency; watch for MCP moves. |
| **Mitiq / Qiskit Circuit Knitting Toolbox** | Error mitigation, circuit cutting | Open source (Apache-2.0). Correctly identified in the original document; deepen the integration per this plan. |
| **arXiv 2604.08318 (Shiraishi et al., 2026)** | An MCP server specifically for quantum execution in hybrid quantum-HPC environments, with an OpenQASM interpretation pipeline and CUDA-Q-based async execution against a Quantinuum emulator | Direct proof that the MCP-for-quantum concept is already being validated in the research literature — useful design reference, and a signal that this is a competitive-but-open field, not blue ocean. No indication it's a maintained open-source project Kubona could build on directly; treat as prior-art, not a dependency. |
| **PennyLang / QuantumKatas / Qiskit HumanEval** | Correctly cited in the original as the eval-corpus foundation | Open source. Keep, but extend per §2.7. |

**Net positioning:** Kubona's defensible product is a **fully open-source, MCP-native quantum toolkit purpose-built for autonomous AI agents** — the hardware-abstraction layer, compiler, cost-governance, and provenance systems all built and maintained in the open (Apache-2.0), with the agentic control/governance layer as the genuinely novel contribution nobody else has shipped. Closed platforms like qBraid and Strangeworks are useful references for API design and optional interop targets, never load-bearing dependencies. That's the correct posture for a true greenfield open-source project, and it should drive every build-vs-wrap decision in the plan below.

---

## 4. Expanded Architecture — Filling the Gaps

### 4.1 Revised layer model — Kubona architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Agent Layer (LangGraph / CrewAI / AutoGen / custom mesh)    │
├─────────────────────────────────────────────────────────────┤
│  MCP Gateway  — OAuth 2.1, PQC-hybrid transport,             │
│                 identity-aware per-call RBAC, audit ledger   │
├───────────────┬───────────────┬───────────────┬─────────────┤
│ Compiler MCP  │ Execution MCP │ Cost-Gov MCP  │ Eval/Provenance│
│ Server        │ Server        │ Server        │ MCP Server     │
│ (QIR/MLIR,    │ (job submit,  │ (pre-flight,  │ (experiment    │
│ Rust core,    │ session/job_id│ budget caps,  │ ledger, hw     │
│ WASM edge)    │ threading,    │ approval gate,│ canary tests,  │
│               │ qBraid/CUDA-Q │ burn-rate)    │ regression CI) │
│               │ backend)      │               │                │
├───────────────┴───────────────┴───────────────┴─────────────┤
│  Hardware Abstraction: Kubona-native, open source (Apache-2.0)│
│  Direct SDK integrations — Qiskit, PennyLane, Q#, Cirq,      │
│  pyQuil, tket, Ocean, Pulser — + CUDA-Q for GPU simulation   │
│  (open-source components only). Optional connectors for      │
│  closed platforms (qBraid, Strangeworks) live outside core.  │
├─────────────────────────────────────────────────────────────┤
│  Backends: IBM · IonQ · Quantinuum · Rigetti · Azure ·       │
│  Google · D-Wave · QuEra · Pasqal · Xanadu · local sim (GPU) │
├─────────────────────────────────────────────────────────────┤
│  Dart/Flutter Control Plane: circuit viz, budget console,    │
│  human-in-loop approval queue, live VQE loop inspector        │
└─────────────────────────────────────────────────────────────┘
```

### 4.2 Federated MCP servers instead of one monolith

Splitting into four scoped servers (compiler, execution, cost-governance, eval/provenance) directly addresses §2.3 (finer-grained RBAC — an agent can be scoped to `compiler` + `execution:simulator-only` without ever touching the cost-governance or production-QPU surfaces) and §2.4 (different orchestration frameworks mount only what they need).

### 4.3 Job/session abstraction

Every multi-step hybrid loop gets a `job_id` issued by the Execution MCP server on first call. All subsequent tool calls (submit, poll, mitigate, re-optimize) thread that `job_id`, letting the nominally-stateless MCP servers reconstruct full session state from the provenance ledger (§2.6) rather than holding it in memory — this also makes the whole system horizontally scalable and crash-recoverable, which the original document never addresses.

### 4.4 Provider capability schema (replaces the fixed 4-row pricing table)

```json
{
  "provider": "quantinuum",
  "modality": "trapped_ion",
  "physical_qubits": 98,
  "logical_qubits_available": 48,
  "native_gate_set": ["rzz", "rz", "rx"],
  "queue_depth_estimate_s": 120,
  "pricing_model": "per_shot_plus_task",
  "jurisdiction": "US/UK",
  "export_control_flag": "review_required_non_allied",
  "sdk_bindings": ["pytket", "qiskit_via_qir"],
  "status": "GA"
}
```
This makes onboarding a new provider (say, IQM or a future entrant) a data change, not a code change — directly resolving §2.1.

### 4.5 Cost-governance service (concrete mechanism)

1. **Mandatory simulator pre-flight** — no job reaches a paid queue without a successful local/GPU simulator dry-run and a cost estimate computed from actual shot count × provider pricing model.
2. **Per-agent-session budget ceiling** — set by the human operator via the Dart console; enforced at the MCP gateway, not just logged.
3. **Two-stage approval gate** for anything above a configurable threshold (mirrors Mainza's existing MilimoClaw Finance Claw REVIEW→HOLD pattern — the same design already validated in that system maps directly onto quantum spend approval).
4. **Real-time burn-rate telemetry** streamed back to the agent as an MCP resource, so the agent can self-throttle (e.g., reduce shot count or switch to a cheaper backend) rather than discovering the ceiling only via a hard rejection.

### 4.6 Provenance/eval server

Every execution writes an immutable, content-addressed record: circuit hash → backend → noise model → mitigation technique → raw results → mitigated results → cost → wall-clock time. This is queryable as an MCP *resource*, letting the agent ask "what mitigation technique worked best for this ansatz class on this backend historically?" — turning the platform into something that gets smarter with use, which the original static-tool-list design doesn't support.

### 4.7 Security hardening summary

- OAuth 2.1 scoped tokens, no static keys, anywhere in the MCP chain
- Hybrid classical + ML-KEM transport on the gateway, cryptographically agile (swappable suites)
- Per-call identity-aware authorization, not coarse roles
- Immutable audit ledger doubling as the cost ledger
- Sandboxed, rate- and budget-capped execution boundary in front of every paid queue

---

## 5. UI/UX Investigation for Kubona

The original document assigned Dart/Flutter a single job — "observability dashboard" — and left it there. That undersells what the human-facing surface actually needs to do, and it's worth grounding in where both quantum-tool UX and agent UX have actually landed as of 2026, rather than designing from first principles.

### 5.1 Two converging fields, one interface

Kubona's UI sits at the intersection of two design disciplines that have each matured independently but rarely been combined:

1. **Quantum circuit visualization** — a genuinely distinct visual design space. A recent design-space study of quantum circuit visualizations (2026) makes the point directly: most existing tools (Qiskit's `circuit_drawer`, Cirq's text diagrams) are static, provider-specific, and don't generalize — and it calls out **platform-agnostic quantum circuit visualization as a real open problem**, since visualization approaches are tightly coupled to whichever SDK produced the circuit. That's a gap Kubona inherits directly by being multi-SDK from day one, and it means Kubona can't just embed Qiskit's renderer and call it done.

2. **Agent UX** — also now recognized as its own discipline, distinct from both traditional dashboards and chatbot UI. The consistent 2026 framing across agent-UX practitioners is that an agent interface has to do four things traditional UI doesn't: show what the system is doing, explain why it chose an action, allow override at any point, and recover gracefully from errors — because the interface is functioning as the accountability layer between user intent and autonomous action, not a display layer bolted on afterward.

Kubona's control plane has to do *both* — visualize a fundamentally non-classical computation *and* narrate an autonomous agent's reasoning and spending decisions about that computation — which is a combination no existing tool (quantum or agentic) currently covers.

### 5.2 What "good" looks like, borrowed from adjacent prior art

**From quantum tooling (IBM Quantum Composer, the current reference implementation for this category):**
- A drag-and-drop circuit editor stays in sync with generated code (OpenQASM/Qiskit) bidirectionally — editing either view updates the other
- Live visualizations (Q-sphere, statevector, measurement histograms) run against a fast local simulator, decoupled from — and much cheaper than — actual QPU execution
- Job results pages show the original circuit *and* the transpiled circuit side by side, plus a histogram, so the user can see what the compiler actually did to their intent
- A relevant agent-specific prior-art example: **QFlow Studio**, a tool built specifically to let users design circuits visually, keep code and diagram in sync, route jobs through a pre-flight check, and **share evidence of a run without exposing credentials** — that credential-safe evidence-sharing pattern maps directly onto Kubona's provenance ledger (§4.6) and is worth adopting rather than reinventing.

**From AI agent observability (AgentOps, Confident AI, and the broader 2026 agent-UX consensus):**
- **Session replay / "time-travel"** — the ability to rewind an agent's execution step by step and see exactly where a reasoning path diverged, which for Kubona means being able to scrub through a multi-hour VQE loop's parameter updates, not just see the final result
- **A dedicated HITL (human-in-the-loop) approval module** that pauses execution and requests explicit approval for high-stakes actions — for Kubona this is the natural home for the cost-governance two-stage approval gate (§4.5), not a bolted-on modal
- **Anomaly/drift surfacing** rather than requiring a human to watch every trace — for Kubona, this means the console should proactively flag things like a sudden fidelity drop on a backend, a burn-rate spike, or a detected barren plateau, instead of requiring an operator to notice it in a wall of telemetry
- **Framework-agnostic trace capture** — since Kubona is meant to work under LangGraph, CrewAI, or a custom mesh (§2.4), the console's trace/session view needs to consume a common event schema regardless of which orchestration framework produced it, not a Kubona-specific one

### 5.3 Kubona UI surfaces (replacing the single "observability dashboard" line item)

| Surface | Purpose | Key interactions |
|---|---|---|
| **Circuit visualizer** | Platform-agnostic rendering of any Kubona-compiled circuit (post-QIR-lowering, so it's SDK-independent by construction) | Bidirectional code↔diagram sync, transpiled-vs-original comparison, Q-sphere/statevector/histogram views backed by local simulation |
| **Cost & approval console** | The human-facing side of §4.5's cost-governance gate | Real-time burn-rate view, pending-approval queue with one-click approve/deny, per-agent and per-session budget ceilings, spend history |
| **Live session / loop inspector** | Watch and interrupt an in-flight multi-step hybrid loop | Session replay/scrub, step-by-step parameter-update trace, pause/override/kill controls, barren-plateau and WBP alerts surfaced inline |
| **Provenance/experiment browser** | Human-readable front end to §4.6's provenance ledger | Search by circuit hash/backend/mitigation technique, side-by-side run comparison, credential-safe evidence export (à la QFlow Studio) for sharing results outside the platform |
| **Provider/fleet status view** | Aggregated health across every connected backend | Queue depth, fidelity trend, canary-CI pass/fail (§2.7), jurisdiction/export-control flags surfaced per provider |
| **Plugin/provider-registration console** (Phase 4+, per the deferred-governance decision in §9) | Lets a contributor register and test a new provider integration against the plugin interface (§4.4) before submitting it | Schema validation, sandboxed test run against the capability contract |

### 5.4 Interaction-pattern principles to apply consistently

Pulling from the current agent-UX pattern catalog, four patterns matter most for Kubona specifically:

- **Progressive disclosure** — a circuit with 200 qubits and a deep transpiled form should default to a collapsed/summarized view (gate-count and depth stats, critical-path highlight) with drill-down on demand, not a firehose diagram nobody can read
- **Confidence/uncertainty visualization** — mitigated results should visually distinguish raw vs. error-mitigated values and show the mitigation technique and its known cost/variance tradeoff (§2.3's mitigation table) inline, not as a footnote
- **Status & activity patterns** — the loop inspector should always answer "what is the agent doing right now and why" in one glance (current step, current backend, current spend, current confidence), which is the core agent-UX accountability requirement above
- **Error handling & recovery** — a failed job (provider API error, calibration drift, budget rejection) should surface a clear, actionable next step (retry on a different backend, request budget increase, fall back to simulator) rather than a raw error dump

### 5.5 Platform implications for the Dart/Flutter build

- Flutter's single-codebase, multi-target model (desktop, web, mobile) is well suited here specifically because the operator persona (a researcher or ops lead approving spend and watching a loop) plausibly needs to check in from a phone mid-run, not just from a desktop — the original document's rationale for Dart stands, it just needs the richer scope above attached to it.
- Keep the console strictly **read/approve**, never a second path to *submit* jobs outside the MCP-governed flow — the console is a human oversight layer on top of the agent's actions, not a parallel API surface, which avoids reopening the credential/spend risk the security model (§2.3, §4.5) is designed to close.
- Worth tracking rather than building yet: **generative/agent-materialized UI** (MCP Apps, A2UI, CopilotKit-style patterns) that shipped across major agent platforms in 2026, where the agent constructs an interactive UI at response time rather than a human pre-building every screen. For Kubona this is a plausible v2+ direction for ad hoc visualizations (e.g., an agent rendering a custom comparison view for a specific run) — but the core console surfaces above (approval, circuit view, provenance) should be fixed, audited, human-designed surfaces given their security-sensitive role, not agent-generated.



## 6. Revised Polyglot Stack (with rationale for gaps closed)

| Language | Original scope | Added scope |
|---|---|---|
| **Rust** | QIR/MLIR compiler core, WASM edge | + Cost-governance service (needs the same correctness/performance guarantees as the compiler), + audit ledger writer |
| **TypeScript/JS** | MCP servers, Node.js/VS Code integration | + Federated server topology (4 scoped servers instead of 1), + OAuth 2.1 gateway implementation |
| **Python** | Agent cognition, Qiskit/PennyLane/Cirq bindings | + direct SDK bindings (tket, pyQuil, Pulser, Ocean) and CUDA-Q (open-source components) integration, + eval-harness orchestration (QuantumKatas/Qiskit HumanEval/PennyLang + new agent-behavior evals) |
| **Dart/Flutter** | Read-only observability dashboard | + Full console scope per §5.3: circuit visualizer, cost/approval console, live session/loop inspector, provenance browser, provider/fleet status view |
| **Go (new)** | — | Consider for the hardware-canary CI service (§2.7) and provenance ledger's storage layer — Go's concurrency model suits fan-out canary polling across 10+ providers better than Node, and it's a lighter operational footprint than adding a sixth language to the agent-facing stack. Optional if team bandwidth doesn't support a sixth language; Rust can absorb this instead. |

---

## 7. Implementation Plan

### Phase 0 — Foundations (Weeks 1–4)
- Stand up the federated MCP server skeleton (4 servers, stub tool schemas)
- OAuth 2.1 auth on the gateway; defer PQC transport to Phase 3 but design the transport layer to be swappable from day one (cryptographic agility, not a retrofit)
- Build Kubona's own open-source provider-abstraction interface, with direct Qiskit and local/GPU-simulator (open-source CUDA-Q components) integrations as the first two implementations — architected as a plugin trait/interface from day one so additional providers are pure additions, not core rewrites
- Apache-2.0 license and public repo from day one (license choice is a one-time decision, cheap to make now and expensive to change later) — but no formal contribution/governance process yet; the priority through Phase 3 is a working product, not community-readiness scaffolding
- Provider capability schema (§4.4) with 2–3 initial entries (IBM via Qiskit, local GPU simulator)
- **Exit criteria:** agent can submit a simulator job end-to-end through the MCP chain and get results back with a provenance record written, using only open-source dependencies.

### Phase 1 — Core execution + cost governance (Weeks 5–10)
- Job/session abstraction with `job_id` threading (§4.3)
- Cost-governance service: pre-flight estimation, budget ceilings, two-stage approval gate
- QIR/MLIR lowering pipeline for OpenQASM 3.0 and Qiskit-generated circuits (Rust core)
- Mitiq integration (ZNE first — highest value-to-effort ratio of the three mitigation techniques)
- **Exit criteria:** a VQE loop can run end-to-end on a real QPU backend (direct provider SDK integration) with a budget ceiling enforced and every step provenance-logged.

### Phase 2 — Multi-provider + Dart control plane (Weeks 11–16)
- Add Quantinuum (via pytket), Rigetti (via pyQuil), QuEra/Pasqal (via Pulser) to the provider schema as direct, open-source SDK integrations
- Circuit-cutting (Qiskit Circuit Knitting Toolbox) for width/depth-constrained circuits
- Dart/Flutter console: cost/approval console and circuit visualizer (highest-priority two surfaces per §5.3; provenance browser and fleet status view follow in Phase 3)
- Agent-orchestration compatibility shim for at least one framework (LangGraph recommended given its native support for long-running stateful graphs, which maps well onto the VQE session model)
- **Identity groundwork (rev 3, §14.2):** keypair identities for users and agents plus owner-delegated authorization beneath the existing OAuth/PQC layer — scaffolding for the Phase-6 collaboration layer, cheap now, expensive to retrofit
- **Exit criteria:** a human operator can watch, and interrupt, an in-flight multi-hour hybrid loop from the Dart console.

### Phase 3 — Security hardening + provenance depth (Weeks 17–22)
- Hybrid PQC transport (ML-KEM) on the MCP gateway
- Per-call identity-aware authorization (replace coarse RBAC roles)
- Hardware-canary CI (nightly regression against every connected backend)
- Provenance server becomes queryable as an MCP resource ("what worked best historically" queries)
- **Exit criteria:** third-party security review of the MCP gateway passes; canary CI catches at least one real provider-side regression in staging before it reaches an agent.

### Phase 4 — Evaluation, autonomous ansatz search, and open-source release prep (Weeks 23–30)
- Integrate QuantumKatas + Qiskit HumanEval + PennyLang eval corpora
- Add agent-behavior evals (budget compliance, simulator-first compliance, WBP-detection response)
- Ship **Kubona Search Mode** (§11.4) — the autoresearch-pattern autonomous ansatz-search loop, simulator-only by default, as the low-effort MVP for algorithmic optimization
- GQE-based circuit optimization module (transformer-driven ansatz search, per original document's §"Algorithmic Optimization") as a v2 upgrade to the same harness, once Search Mode has validated the sandboxed-write-scope pattern; retraining runs on Unsloth (fast LoRA, fits Apple Silicon / consumer GPUs) so the §13.3 flywheel stays affordable (rev 3, §14.6)
- Apache-2.0 licensing pass, plugin/provider-registration SDK for third-party backend contributions, contribution governance doc for the tool-surface security boundary
- **Exit criteria:** public repo with documented plugin API; at least one third-party-contributed provider integration merged as a validation of the extensibility model; Search Mode has produced at least one ansatz that beats a hand-designed baseline on a benchmark problem, entirely in simulation.

### Phase 5 — Scale & photonic/annealing breadth (Weeks 31+)
- Xanadu/Strawberry Fields (photonic, CV) integration
- D-Wave Ocean SDK deepening (already scoped in original doc — execute per that spec)
- Export-control/jurisdiction enforcement in the routing layer (§2.9)
- Horizontal scaling of the provenance ledger and cost-governance service for multi-tenant/enterprise deployment

---

## 8. Team Shape & Effort Estimate

| Role | Phases | Notes |
|---|---|---|
| Rust systems engineer (compiler/WASM/cost-gov) | 0–5 | Highest-leverage single hire; QIR/MLIR + cost-gov correctness is the platform's core defensibility |
| TS/Node MCP platform engineer | 0–3 | Owns gateway, federated server topology, OAuth/PQC |
| Python quantum SDK integration engineer | 0–5 | Owns direct Qiskit/PennyLane/Q#/Cirq/pyQuil/tket/Pulser bindings, CUDA-Q (open-source components) integration, eval harness |
| Dart/Flutter engineer | 2–5 | Control-plane console; can be part-time/contract given narrower scope |
| Workflow-DSL / UX engineer (rev 3, optional) | 6 | Workflow/agent authoring surfaces, experiment notebook & results studio; part-time/contract |
| Fine-tuning / ML engineer (rev 3, optional) | 4, 6 | Unsloth-based GQE retraining and persona/domain models (§14.6); part-time/contract |
| Security reviewer (contract, Phase 3) | 3 | PQC transport + MCP threat-model review — don't skip an external pass given the credential/spend blast radius |

Given the existing Milimo Quantum foundation (Qiskit v2.2, 14+ agents, VQE, Apple Silicon/CUDA HAL already built), Phases 0–1 are substantially de-risked — the HAL and agent-mesh patterns from that project are directly reusable for the MCP execution server and cost-governance gate (the Finance Claw two-stage approval pattern maps almost exactly onto §4.5).

---

## 9. Key Risks

| Risk | Mitigation |
|---|---|
| Building a full hardware-abstraction layer from scratch is real, substantial scope, and closed competitors (qBraid, Strangeworks) already have a head start on breadth | Phase provider onboarding deliberately (2–3 providers in Phase 0, expand through Phase 2) rather than trying to match qBraid's 34-device breadth on day one; differentiate on the agentic governance layer, which no open *or* closed competitor has shipped, while the provider list grows |
| MCP credential compromise → runaway cloud QPU spend | Cost-governance service is non-optional in Phase 1, not deferred |
| Barren plateaus / WBPs silently degrade agent-driven VQA quality without the agent noticing | Agent-behavior evals (Phase 4) must include automated WBP-detection-response testing, not just code-gen correctness |
| User/agent-created workflows and agents (Phase 6) widen the attack surface — an agent-created agent could escalate privileges or burn budget | Owner-delegated authority with instant revocation (§14.2), no privilege escalation through creation (§14.3), per-agent budget envelopes, capability sign-off + shadow mode for every new workflow/agent (reuses §13.4) |
| Scholarly/data integrations carry licensing and data-quality risk (TOU violation, stale or wrongly cited sources) | License/TOU flags + freshness/trust scores on every fetched source, citation provenance written into the ledger, curate Tier-1 sources first (§14.5) |
| Six-language stack (Rust/TS/Dart/JS/Python/+Go) raises hiring and maintenance cost | Treat Go as optional; default to Rust for the canary/ledger service if team stays at five languages |
| Export-control exposure from cross-border autonomous job routing | Jurisdiction field in provider schema + routing-layer enforcement before Phase 5 multi-tenant scale-out |
| Open-source contribution governance for the security-sensitive MCP tool surface is undefined | Deliberately deferred — Apache-2.0 licensing is in place from Phase 0, but a formal contribution/review process isn't needed until Kubona has a working core and outside contributors actually show up (targeted for Phase 4, alongside the plugin API). Building governance process before the product works is effort spent on the wrong problem. |

---

## 10. Summary of What Changed

- **Hardware coverage**: 4 providers → 11+ via a data-driven capability schema, built as direct, open-source SDK integrations owned by Kubona (not a wrapped commercial platform)
- **Security**: RBAC + secrets manager → OAuth 2.1 + PQC-hybrid transport + per-call identity-aware auth + immutable audit ledger
- **Orchestration**: monolithic MCP server → 4 federated, scoped servers + job/session abstraction
- **Cost**: routing heuristic in prose → a concrete, mandatory pre-flight + budget-ceiling + two-stage-approval service
- **Reproducibility**: absent → content-addressed provenance ledger, queryable by the agent
- **Testing**: code-gen evals only → + hardware canary CI + agent-behavior evals
- **Frontend**: read-only dashboard → circuit visualizer + approval console + live loop inspector
- **UI/UX**: single "observability dashboard" line item → six defined console surfaces (§5.3) grounded in both quantum-visualization prior art (IBM Composer, QFlow Studio) and 2026 agent-UX practice (session replay, HITL approval, anomaly surfacing, progressive disclosure)
- **Legal/licensing**: unaddressed → Apache-2.0 from day one + export-control-aware routing + contribution governance for a fully open-source project
- **Competitive awareness**: none → explicit qBraid/CUDA-Q/Strangeworks positioning as prior-art reference and optional interop, never a dependency
- **Scope (rev 3)**: quantum toolset for agents → AI-enhanced quantum **+ science discovery** platform: multi-user labs, user/agent-created workflows and agents, autonomous experiments with systematic reproducible results, scholarly/data integrations, and local model training (Unsloth) — see §14
- **Naming (rev 4)**: the project is named **Kubona** (Tonga (Zambia): *to see*) — the name reflects the expanded mission (observe → experiment → know); no prior public name exists open-source build
- **Algorithm R&D**: none → a self-improving algorithm engine (§11), built on the autoresearch orchestration pattern, spanning ansatz/circuit optimization *and* correctness regression testing
- **Self-evolution**: an object-level loop that only improves artifacts → a second, meta-level loop (§13) that improves the search strategy itself — a genuine flywheel where accumulated results make future searches better, gated by stricter versioned/reversible/human-reviewed controls than the object-level loop

---

## 11. The Kubona Autoresearch Engine — a Self-Improving Algorithm R&D Loop

The brief evolved from "borrow the pattern for ansatz search" to something bigger: use [karpathy/autoresearch](https://github.com/karpathy/autoresearch)'s orchestration loop as the literal *base* for a general-purpose subsystem that lets Kubona autonomously improve and test quantum algorithms across families — not just VQE ansätze, but QAOA, error-mitigation configuration, transpilation/compilation strategy, and algorithm-correctness regression. This section designs that subsystem.

### 11.1 What it actually is (recap)

[karpathy/autoresearch](https://github.com/karpathy/autoresearch) is a minimal single-GPU LLM training repo (a simplified [nanochat](https://github.com/karpathy/nanochat)), MIT-licensed, built around a three-file split: fixed infrastructure the agent never touches (`prepare.py`), the one file the agent is allowed to mutate (`train.py`), and a lightweight human-authored instruction file (`program.md`). The loop — mutate, run for a fixed budget, evaluate one metric, keep or discard, repeat, unattended — is genuinely reusable as *orchestration scaffolding*, independent of the fact that the original repo happens to apply it to GPT training.

### 11.2 From one harness to a pluggable harness architecture

The generalization from "ansatz search" to "improve and test quantum algorithms" is really the move from *one* fixed `prepare.py`/`train.py` pair to a **registry of harnesses**, each implementing the same four-part contract the original repo hard-codes for LLM training:

```
Harness {
  fixed_setup()          // problem instance, backend, noise model -- not agent-editable
  mutable_artifact()     // the one file/object the agent is allowed to change
  metric(result)         // single, comparable, normalized score
  validity_check(result) // is this result even a legal/correct output?
}
```

The orchestrator itself — propose a mutation, run it within budget, score it, keep-or-discard, log, repeat — is forked directly from autoresearch's loop logic and is domain-agnostic; it doesn't need to know whether it's optimizing a VQE ansatz or a transpilation pass. Only the harness changes per algorithm family:

| Harness | Fixed setup | Mutable artifact | Metric | What "testing" means here |
|---|---|---|---|---|
| **VQE ansatz search** | Molecular/spin Hamiltonian, simulator, noise model | `ansatz.py` — circuit structure, parameterization, optimizer choice | Energy error per qubit (normalized) | Optimization: find a better ansatz for this problem class |
| **QAOA search** | Graph/combinatorial instance, simulator | Mixer/cost-operator structure, layer count (p), classical optimizer | Approximation ratio | Optimization: find a better circuit for this instance class |
| **Mitigation-strategy search** | Fixed circuit + backend noise profile | Mitigation config (ZNE fold factors/extrapolation order, PEC sampling budget, CDR training set size — per §2.3's mitigation table) | Distance from noiseless reference (on a circuit class with a known classically-computable answer) | Optimization: which mitigation config recovers the most signal per shot spent, per backend |
| **Compilation-pass search** | Fixed logical circuit, target backend's native gate set/topology | Transpiler pass sequence/parameters (qubit mapping, routing, optimization level) | Resulting circuit depth/gate count, weighted by that backend's error rates | Optimization: minimize the thing that actually determines fidelity on this hardware |
| **Algorithm correctness regression** | Known-answer problem instances (Grover with a marked item, Deutsch-Jozsa, Bernstein-Vazirani) | *Nothing mutated* — this harness runs unmodified reference implementations | Pass/fail against the known-correct output | Testing, not optimizing: this is the harness that catches a QIR-lowering regression or a provider API change silently breaking correctness — feeds directly into the hardware-canary CI from §2.7 |

That last row matters: the same engine that autonomously *improves* algorithms is also the natural home for the *testing* half of the original request — correctness regression doesn't need a search loop, but it fits the identical harness contract (fixed setup, no mutation, metric = pass/fail), so it's nearly free to add once the engine exists.

### 11.3 Required adaptations — still non-negotiable at this larger scope

The safety adaptations from the narrower ansatz-search proposal apply with more force now that the engine spans multiple harnesses, including one (compilation-pass search) that touches the compiler pipeline itself:

1. **Structural write-scope, per harness, enforced by the sandbox — not by the harness's own honesty.** Each harness declares exactly which artifact it may mutate; the agent sandbox running that harness has no filesystem or tool access outside that declared scope, and never has access to the Execution MCP server's real-hardware-submission tool. A compilation-pass search agent, for instance, can mutate pass *parameters* fed to the existing Rust QIR/MLIR compiler — it never gets write access to the compiler's own source.
2. **Simulator-only by default, for every harness, always.** No harness runs against real QPU time unless a human explicitly promotes a specific candidate through the cost-governance gate (§4.5). This is now a property of the orchestrator itself (a global setting, not something each harness author has to remember to set correctly), so a new community-contributed harness can't accidentally default to burning real hardware budget.
3. **Correctness-regression harnesses run with even less trust than optimization harnesses** — since their whole job is catching platform bugs, they should run in CI (§2.7's hardware-canary job) with results gating releases, not just logging to the provenance ledger for later review.

### 11.4 The accumulating output: a validated algorithm library, not just a log

The original autoresearch repo's output is a log a human reviews in the morning. Kubona's provenance ledger (§4.6) already captures every run, but at this scope it's worth being explicit about the product this produces over time: a continuously growing, **provenance-backed library of validated algorithm/backend configurations** — "for this Hamiltonian class on this backend, this ansatz + this mitigation config is currently the best known combination, discovered on [date], validated on real hardware on [date], N simulator iterations behind it."

- Promotion path: simulator-discovered candidate → cost-governance-approved real-hardware validation run → if it holds up, promoted from "search result" to a citable library entry
- This is exactly the resource the original architecture document's agent-facing MCP *Resources* primitive was designed for: an agent solving a new problem can query "what's the best known ansatz for a problem like this on this backend" before running its own search from scratch, turning every future agent's work into a query against accumulated results rather than a cold start
- Surfaced in the provenance browser (§5.3) as a searchable library view, not just a raw run log

### 11.5 Where this fits in the roadmap

Revise **Phase 4** (§7) to build the general engine, not a single feature: the pluggable harness interface and orchestrator first, then ship with three harnesses (VQE ansatz search, mitigation-strategy search, and the correctness-regression harness — the highest-value, lowest-risk trio), with QAOA and compilation-pass search following once the engine has proven out. GQE (transformer-driven ansatz search) becomes a fourth harness implementation plugged into the same engine, rather than a separate system — it only needs to implement the same four-part contract as everything else in §11.2.

- **Exit criteria addition for Phase 4:** the correctness-regression harness is wired into the hardware-canary CI and has caught at least one real regression in staging; the algorithm library has at least one entry that went through the full simulator → cost-governance-approved → real-hardware-validated pipeline.

---

## 12. Integrating the Agent Resource Economy — x402, Monetization Gateway, and Kubona

Cloudflare's AI Crawl Control / Pay Per Crawl / Monetization Gateway launch (full breakdown in the companion insights file) is, on inspection, a near-exact external validation of what Kubona is already building — and it fills the one gap this plan has been carrying since §9: a sustainability model for a genuinely open-source project. It doesn't change Kubona's architecture so much as give it a payment rail and a business model that don't compromise the Apache-2.0, dependency-free posture established in §2.2.

### 12.1 The core reframe, applied to Kubona

The transcript's central question — *what resource does an agent need badly enough, often enough, and reliably enough to pay for* — has an unusually direct answer inside Kubona: **quantum shot-time, and validated algorithm knowledge.** Both are scarce, metered, and exactly the kind of resource the "agent web monetizes useful resources" thesis describes. Kubona doesn't need to go looking for a monetizable resource; it's already building two of them (hardware access via the provider abstraction layer, and the algorithm library via the Autoresearch Engine, §11.4).

### 12.2 Adopt x402 as an optional payment rail on the Cost-Governance MCP server (extends §4.5)

x402 — built on HTTP 402 "Payment Required," agent requests, server quotes a price, agent pays and retries with proof of payment, verified before the job reaches the real queue — maps onto Kubona's Execution/Cost-Governance servers almost without modification:

- **This is additive, not a replacement.** The mandatory simulator pre-flight and the two-stage human approval gate for large spends (§4.5) remain the hard safety rail for anything above a configurable threshold — that doesn't change. x402 fills the gap *underneath* that threshold: many small, sub-cent-to-sub-dollar transactions (a cheap simulator-tier run, a single algorithm-library lookup) don't need a human in the loop at all, and forcing them through the approval queue today is exactly the kind of friction x402 is designed to remove.
- **Standard rail beats bespoke rail.** Speaking x402 rather than a Kubona-specific payment protocol means any agent or framework that already understands x402 (a growing standard, not a Cloudflare-proprietary one) can transact with Kubona with zero custom integration work — the same "request becomes the transaction" frictionlessness the source material describes.

### 12.3 The algorithm library is a niche data refinery (extends §11.4)

Run the transcript's five-part filter for a good data-refinery niche against Kubona's accumulating algorithm library, and it passes cleanly:

| Filter | Does the Kubona algorithm library qualify? |
|---|---|
| Valuable | Yes — a validated ansatz/mitigation config directly changes how much a VQE/QAOA run costs and how good the result is |
| Repeated | Yes — every new problem instance on a given backend needs this lookup again |
| Changing | Yes — hardware calibration drifts, new devices launch, better configs get discovered |
| Fragmented | Yes — currently scattered across papers, provider docs, and individual researchers' private notebooks |
| Annoying | Yes — collecting it requires real simulator/hardware time, which is exactly why it's worth paying for instead of re-deriving |

That's a strong signal the library is a real monetizable resource, not just an internal convenience. Treat it accordingly: free, low-resolution answers ("yes, a known-good config exists for this problem class") stay open, and a small x402 microtransaction unlocks the full validated config with provenance — mirroring the "some things stay free for distribution, some things are paid because they're valuable" split from §4 of the resource stack.

### 12.4 Who pays first (extends §11.5 / roadmap thinking)

The "sell to whoever already sells into the niche first" pattern suggests Kubona's first paying users of a metered library/hardware-routing tier are less likely to be individual quantum researchers and more likely to be: agent-orchestration-framework vendors (LangGraph/CrewAI-adjacent ecosystems) wanting a ready-made quantum tool for their users, quantum-consulting shops, and other MCP-native platforms buying access rather than doing the research themselves. This should shape early go-to-market conversations once Kubona has a working core (Phase 4+), not the build itself.

### 12.5 "Agent readiness" as a service Kubona could offer providers (new idea, not scoped into a phase)

The "agent readiness for businesses" pattern — audit how AI currently describes/prices/recommends a company, then sell the fix — has a direct analog: most quantum hardware providers' current docs, pricing, and APIs are *not* agent-readable (scattered PDFs, inconsistent SDKs, no MCP surface). Kubona's provider capability schema (§4.4) already builds the "fix" for Kubona's own users. Worth keeping on the radar as a distinct, sellable offering once Kubona has traction — a "quantum agent-readiness audit" for hardware providers, showing them what an agent gets wrong trying to use their platform today, and selling the fix as a Kubona-maintained connector. Not roadmapped yet; flagged for later consideration rather than built now, consistent with the "functional product first" decision in §9.

### 12.6 Existing design gets external validation, plus one addition

- The transcript's "trust and analytics layer" (data freshness, source reliability, which requests are worth money) is already covered by Kubona's provider capability schema and provenance ledger (§4.4, §4.6) — this is confirmation the design is pointed the right way, not a new gap.
- **One addition worth making:** a simple per-provider and per-library-entry freshness/trust score, surfaced in the provenance browser (§5.3) — the same scoring primitive as the Context Score pattern already proven out in Mainza's AfrEval project, giving both platforms a consistent "certify before you trust it" vocabulary.
- "Agents will have wallets and agent-addressable identities" reinforces, rather than changes, the OAuth 2.1 + per-call identity-aware authorization design already planned (§2.3, §4.7) — x402's payment-proof-per-request model is a natural extension of that same identity layer, and Kubona's audit ledger can double as the payment-proof ledger with no structural rework.

### 12.7 Roadmap placement

- **Phase 1** (§7): when building the Cost-Governance MCP server, implement the budget-ceiling/pre-flight/approval-gate mechanism as planned, and design the payment interface to be x402-compatible from the start (cheap now, expensive to retrofit — same logic already applied to PQC transport in Phase 0).
- **Phase 5+**: actual x402 activation for sub-threshold transactions, algorithm-library metering, and any provider agent-readiness-audit offering are business-model exploration once Phase 4 has produced a working engine and real library entries worth metering — this is explicitly deferred alongside the open-source governance work in §9, for the same reason: sequence a working product before monetization mechanics.

---

## 13. Closing the Loop: Meta-Search and True Self-Evolution

§11 gives Kubona a working Karpathy loop, but it's honest to name what it actually does: it improves *artifacts* — circuits, mitigation configs, compiler passes. The orchestrator, the search strategy, and the instructions guiding each harness stay fixed. That's autonomous optimization, not self-evolution. For the system itself to get better over time — not just its outputs — the loop needs a second, higher tier: one that can revise *how Kubona searches*, not just what it searches for.

### 13.1 Two tiers, not one

| | **Object-level loop (§11, already designed)** | **Meta-level loop (new, this section)** |
|---|---|---|
| What gets mutated | The artifact — `ansatz.py`, a mitigation config, a pass sequence | The search itself — mutation/proposal strategy, harness instructions, harness registry |
| Frequency | Every iteration (minutes) | Periodic reflection over accumulated results (e.g., weekly, or after N object-level runs) |
| Output | A better circuit/config for one problem class | A better *searcher* — improvements that compound across every future object-level run, every harness, every problem class |
| Risk profile | Bounded — worst case is a wasted simulator run | Higher leverage — a bad meta-change degrades search quality system-wide until caught |

This is the actual mechanism of self-improvement: the object-level loop produces results, the meta-level loop studies which strategies produced the best results, and updates the strategy for next time. Each cycle should make the *next* cycle better, not just add one more data point.

### 13.2 What the meta-loop is allowed to touch

Three concrete surfaces, each mapped to something already in this plan:

1. **Mutation/proposal strategy per harness.** Right now §11.2's harness contract treats "how do we propose the next candidate" as fixed (random mutation, evolutionary step, or eventually GQE's transformer). The meta-loop reviews the provenance ledger's keep/discard rate and convergence speed per harness and adjusts the proposal strategy's own parameters — e.g., "random mutation on the ansatz harness has stalled for 200 iterations, widen the mutation radius" or "this backend's mitigation harness converges fastest when new candidates are seeded from the top-3 historical library entries instead of pure random search."
2. **Harness instruction files (`search.md`, per §11.4's analog to `program.md`).** These are human-authored today. The meta-loop can propose edits to them based on outcome data — e.g., a harness that keeps proposing invalid candidates gets a revised instruction file with tighter validity guidance. Every proposed edit is versioned and diff-reviewable, exactly like the object-level artifacts it governs.
3. **The harness registry itself.** When the correctness-regression or provenance data reveals a recurring problem class none of the existing harnesses cover well (e.g., a new error-mitigation family, a new backend modality from §2.1's expanding provider list), the meta-loop can propose a *new* harness — implementing the same four-part contract from §11.2 — as a draft for human review, rather than Kubona staying limited to whatever harnesses were hand-built at launch.

### 13.3 The flywheel

This is where GQE (already slated as a Phase 4 harness upgrade, §11.5) stops being a one-time model and becomes the clearest instance of the meta-loop in action: the provenance-backed algorithm library (§11.4) is training data → periodically retrain/fine-tune the GQE model on the accumulated library → the improved model becomes the proposal strategy for the ansatz-search harness → it produces better candidates → those candidates enrich the library → repeat. Object-level search produces data; the meta-loop turns that data into a better searcher; the better searcher produces better data. That closed loop — not any single search run — is what "self-evolve" actually means here.

### 13.4 The safety gate has to be stricter at this tier, not the same one

Everything in §11.3's sandboxing logic still applies (structural write-scope, simulator-only by default, no access to real-hardware submission), but a meta-level change needs additional guardrails because its blast radius is every future object-level run, not one run:

- **Every meta-change is versioned and reversible.** A revised `search.md`, a new proposal-strategy parameter set, or a new harness draft is a diffable, rollback-able artifact in its own right — never an in-place mutation of live orchestrator state.
- **Staged rollout, not global activation.** A meta-change is trialed on a subset of future object-level runs (shadow mode: run both the old and new strategy, compare outcomes) before it replaces the incumbent strategy system-wide.
- **Human sign-off required for anything that touches the harness registry or a proposal strategy already in production use** — this is a capability-level approval, distinct from and in addition to the cost-governance dollar-threshold gate in §4.5. A meta-change costs nothing in QPU spend and can still be high-risk, so the existing cost gate alone doesn't cover it.
- **The meta-loop never edits Kubona's own platform code** (compiler, MCP servers, security layer) — its write scope stops at search strategy, instruction files, and harness definitions, which is a deliberately narrower boundary than "the system can rewrite itself" would imply. Self-evolution here means Kubona gets better at searching and testing quantum algorithms over time, not that it has open-ended write access to its own codebase.

### 13.5 Roadmap placement

Add as a **Phase 4 stretch goal, gated on §11's object-level engine actually shipping first** (§7, §9's "functional product first" discipline applies here too — a meta-loop with nothing to reflect on is premature): once the three initial harnesses (VQE ansatz, mitigation-strategy, correctness-regression) have produced enough provenance data to be worth reflecting on, add the meta-loop as a scheduled reflection job that proposes — but does not auto-apply — strategy and harness-registry changes for human review. Auto-application of meta-changes (via the shadow-mode staged rollout in §13.4) is a Phase 5+ capability, added only after the propose-and-review version has run safely for a meaningful period.

- **Exit criteria:** the meta-loop has proposed at least one strategy change that, when shadow-tested, measurably improved convergence speed or candidate quality on an existing harness — demonstrating the flywheel actually compounds rather than just adding noise.


---

## 14. Science Discovery & Collaboration — From Quantum Toolset to Discovery Platform

This revision extends Kubona's mission (rev 3, noted in §1): beyond authoring and executing
quantum algorithms, the platform becomes an **AI-enhanced science discovery platform** —
where users and agents design, run, and archive experiments across quantum hardware and
classical/data resources, where users and agents can **create their own workflows and
agents**, and where the platform itself can **conduct experiments autonomously and return
results in an organized, systematic, reproducible form**. Nothing in Phases 0–5 changes;
this layer reuses the governance (cost + capability gates), provenance, and safety
machinery already designed rather than inventing parallel rails.

### 14.1 The expanded mission and what stays fixed

- **Expanded mission:** quantum processors are the differentiating instrument, but one
  instrument family inside a full discovery loop — ground → hypothesize → design →
  experiment → analyze → report → learn (§14.4). Multi-user labs, user/agent-created
  workflows and agents, and structured, reproducible results are first-class products.
- **What stays fixed:** open-first posture (§2.10), "functional product first" sequencing
  (§9), the cost-governance dollar gate (§4.5), and the capability gate for anything that
  changes system behavior (§13.4). Every new surface below must sit behind these rails.

### 14.2 Identity & collaboration model (users, agents, labs)

Design reference: the open-source Buzz workspace ([block/buzz](https://github.com/block/buzz);
fork at [mainza-ai/buzz](https://github.com/mainza-ai/buzz) — same operator as the Milimo
foundation referenced in §8) — patterns only, not code (same single-source caveat as §8's
Mainza/Milimo claims).

- **One identity model for people and processes.** Every actor — human, platform agent, or
  user-created agent — is a keypair identity with a profile, memberships, budget envelope,
  and signed audit trail. Scoped by identity, not by coarse permission flags (consistent
  with the per-call identity-aware authorization of §4.7).
- **Agents are members, not bots.** An agent joins a lab the way a person does: own keys,
  own memberships, own provenance footprint, own budget.
- **Delegated authority, instantly revocable.** An agent inherits its owner's access via a
  delegation attestation (the NIP-OA pattern): add a user and all their authorized agents
  gain access; remove the user and all their agents lose access immediately — no per-agent
  permission surgery. This is the single most important safety property of the
  collaboration layer.
- **Community-local state.** A lab/team scopes profiles, presence, jobs, memberships, and
  audit chains by host/URL; a hosted operator can run many labs on shared infrastructure
  without cross-lab leakage.
- **Phase 2 groundwork:** keypair identities for users and agents plus owner-delegated
  authorization beneath the existing OAuth/PQC layer (cheap now, expensive to retrofit —
  same logic as PQC swappability in Phase 0).

### 14.3 Workflows & agent authoring (user-created and agent-created)

- **Workflows:** versioned, YAML-declared sequences of steps, each step calling an MCP tool
  (transpile, estimate, submit, analyze, fetch-literature, …). Triggers: manual, schedule,
  event (new result / new data), or webhook (the Buzz trigger set). **Gates between steps:**
  cost approval (§4.5) and/or capability approval (§13.4). **Dry-run mode:** every workflow
  passes through the simulator tier before anything reaches a paid queue.
- **User-created agents:** a manifest combining persona + workflow + tool scope + budget
  envelope, registered in the agent registry, bound by §14.2 identity rules.
- **Agent-created agents:** a workflow or agent may spawn another agent (e.g., a specialist
  reviewer for a new experiment class) — gated exactly like §13 meta-changes: proposed,
  versioned, shadow-tested, human-approved before independent action. **No privilege
  escalation through creation:** a created agent can never hold capabilities its creator
  does not hold.
- **Compound asset:** workflows and agents are indexed, reusable, creator-identity-traceable
  library entries — the collaboration analog of the §11.4 algorithm library.

### 14.4 The discovery loop & systematic results

- **Experiment lifecycle:** every experiment is a first-class object: hypothesis → protocol
  (with controls and preregistration where appropriate) → data sources fetched (with
  license/TOU flags) → execution on QPU/simulator/data API → analysis (error mitigation,
  statistical validation) → structured report → archive into the library → meta-learning
  (§13). The §4.6 ledger record extends to the full lifecycle, not just the hardware run.
- **Self-experimentation:** the platform can run whole campaigns on its own — hypothesis
  sweeps across problem classes, backends, ansätze, and mitigation strategies — inside the
  same gates. §11 is the object-level machinery, §13 the reflection tier; both are
  prerequisites before the platform proposes full experiments independently.
- **The results contract ("organized and systematic"):** every experiment yields two
  artifacts keyed by the same content address — (a) the immutable, agent-queryable ledger
  record (§4.6), and (b) a human-facing structured report in the experiment notebook:
  objectives · methods · protocol · results · analysis · conclusions · citations ·
  *how to reproduce* — with credential-safe sharing (the QFlow pattern) and a unified
  search index over conversations, runs, approvals, and reports (the Buzz single-index idea).

### 14.5 Science data integrations (literature + domain data)

Shortlist from the public-apis catalog ([public-apis/public-apis](https://github.com/public-apis/public-apis)),
ranked by leverage for the discovery loop:

- **Tier 1 — literature & scholarly grounding:** arXiv (preprints), OpenAlex (open scholarly
  graph: works, authors, institutions, citations), CORE (open-access full text), NASA ADS
  (astrophysics), Open Science Framework (preregistration/study designs), SHARE (research
  activity). **Ship arXiv + OpenAlex first.**
- **Tier 2 — domain data:** Materials Platform for Data Science (curated experimental
  materials data — pairs with VQE/molecular workflows), GBIF / iDigBio / ITIS (biodiversity),
  NASA/ISRO/Launch Library (space), Purple Air (environmental time series).
- **Integration pattern:** each integration is an MCP tool/resource behind the same rail —
  rate limits, license/TOU flags, cost metering where paid, fetch provenance (source URL,
  timestamp, license) written into the ledger, and freshness/trust scores ("certify before
  you trust it", §12.6's vocabulary). Inference/grounding operations may use Hugging Face,
  Jina (embeddings/rerank), or Groq — operational choices, not dependencies.

### 14.6 Model training infrastructure — Unsloth

[unslothai/unsloth](https://github.com/unslothai/unsloth) (Apache-2.0, Python) — local LLM
run/train platform with ~2× faster fine-tuning at ~70% less VRAM (LoRA/QLoRA/full/RWKV, RL),
GGUF export, OpenAI-compatible serving, MCP tooling, and dataset "recipes". Integration
points:

- **GQE flywheel (§13.3):** the periodic retraining of the GQE proposal model from the
  algorithm library becomes affordable on Apple Silicon / consumer GPUs instead of rented
  clusters — making the meta-loop a practical lab capability, not an enterprise one.
- **Personas & domain models:** fine-tune small specialized models per lab/workflow class
  locally and privately — pharma/finance data never leaves the lab (consistent with the
  sovereign/self-host posture).
- **RL for agent behavior:** RL fine-tuning is a fit for the §2.7/Phase-4 agent-behavior
  evals (e.g., reward budget-compliance and simulator-first behavior).
- **Dataset recipes:** turn the §4.6 ledger and §11.4 library into training sets.
- Adopted as an operational integration, not a dependency; verify current product surface
  (Desktop/Studio) before committing.

### 14.7 Roadmap placement (rev 3 additions)

- **Phase 2:** identity groundwork — keypair identities + delegated authorization (§14.2).
- **Phase 4:** GQE retraining via Unsloth (§14.6); object-level engine and §13 meta-loop as
  already specified.
- **Phase 6 — Science Discovery & Collaboration (Weeks 37+)**, gated on a working,
  governed core (Phase 4 release): profiles & labs (§14.2); workflow & agent authoring with
  the agent registry (§14.3); experiment notebook & results studio (two new control-plane
  surfaces on top of §5.3's six); Tier-1 scholarly integrations (§14.5); full autonomous
  discovery campaigns (§14.4); personas & fine-tuned domain models (§14.6).
- **Exit criteria (Phase 6):** a lab runs the full discovery loop — literature ground →
  experiment → structured report → archived library entry — using user- and agent-created
  workflows, with every claim tracing to a ledger record or citation.

### 14.8 New risks (additions to §9)

- **User/agent-created workflows and agents widen the attack surface** — an agent-created
  agent could escalate privileges or burn budget. Mitigation: delegated authority with
  instant revocation (§14.2), no privilege escalation through creation (§14.3), per-agent
  budget envelopes, capability sign-off + shadow mode for every new workflow/agent (§13.4).
- **Scholarly/data integrations carry licensing and data-quality risk.** Mitigation:
  license/TOU flags + freshness/trust scores on every fetched source, citation provenance
  written into the ledger, Tier-1 curation first (§14.5).
- **Scope creep risk:** Phase 6 must not pull staffing or attention off the Phase-0–4 core;
  it is explicitly staffed with optional/part-time roles (§8) and gated on the core
  shipping first.
