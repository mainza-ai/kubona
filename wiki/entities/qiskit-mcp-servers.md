---
type: entity
name: Qiskit MCP Servers
date: 2026-10-03
updated: 2026-10-03
tags: [ibm, qiskit, mcp, vendor-tooling, execution]
sources: [source-agent-protocol-landscape-2026]
status: tracked
---

# Qiskit MCP Servers

IBM's official collection of Model Context Protocol servers (Apache-2.0, released September 2026
at `Qiskit/mcp-servers`) providing LLMs and AI agents with direct access to Qiskit libraries and
IBM Quantum hardware (source-agent-protocol-landscape-2026).

## Server Suite

The repository contains five independently deployable servers:
1. `qiskit-mcp-server`: Local circuit optimization, analysis, and OpenQASM 3 / QPY transpilation.
2. `qiskit-ibm-runtime-mcp-server`: Backend selection, job submission, status polling, and result retrieval on IBM Quantum hardware.
3. `qiskit-ibm-transpiler-mcp-server`: AI-powered cloud transpilation service.
4. `qiskit-docs-mcp-server`: Semantic documentation search.
5. `qiskit-gym-mcp-server`: Reinforcement learning quantum environments.

## Comparison & Topology for Kubona

`Qiskit/mcp-servers` provides useful vendor drivers for IBM Quantum, but exhibits fundamental
architectural differences from Kubona's [[protocol-strategy|Capability Core]]:
- **Credential Exposure:** Relies on ambient environment variables (`QISKIT_IBM_TOKEN`) or raw tokens passed in tool parameters (`setup_ibm_quantum_account`). Kubona isolates credentials in an [[openbao]] sidecar vault.
- **Unbounded Spend:** Contains zero spend estimation, cost ceilings, or approval gates; runaway agent loops can burn cloud credits unchecked. Kubona enforces mandatory pre-flight cost gates.
- **Vendor Lock:** Targets IBM Quantum exclusively. Kubona federates IBM, IonQ, Quantinuum, Rigetti, and neutral-atom hardware.
- **Epistemic Anchors:** Returns unverified raw execution dictionaries without cryptographic circuit hashes or classical comparison baselines.

**Kubona Integration Posture:** Tracked as an optional downstream execution target or provider plugin.
Kubona sits above vendor MCP suites as the multi-provider governance and provenance control plane.

Related: [[qiskit]] · [[mcp-and-context-flooding]] · [[protocol-strategy]] · [[cost-governance]] · [[quantum-advantage-realism]]
