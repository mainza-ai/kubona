---
type: roadmap
date: 2026-08-16
tags: [kubona, plan]
sources: [source-gap-analysis-plan]
status: living
---

# Roadmap — Kubona Implementation Plan

The working implementation plan for Kubona. Phase content, exit criteria, and team shape
trace to §§7–8 of [[source-gap-analysis-plan]], with the §11.5 (autoresearch engine) and
§12.7 (x402) roadmap revisions folded in. **Task ordering, dependency chains, gate
structure, and tracking mechanics below are maintainer synthesis** derived from those
sections. Total ~30 weeks to Phase 4 release-readiness; Phase 5+ is continuous scale +
business-model work, and **Phase 6 (Weeks 37+) adds the Science Discovery & Collaboration
layer** (maintainer synthesis).

## How to use this page

- Every phase ends in an **exit criterion = go/no-go gate**. Do not start the next phase
  until the gate is met; record the result (pass / fail + reason) in [[log]].
- This page stays mirrored to the source plan; when the source's plan changes, update this
  page and log the change.
- Details live on the linked pages ([[cost-governance]], [[qir-mlir-compilation]],
  [[karpathy-autoresearch]], …); this page is the sequencing, not the design.

## Sequencing principles (why the phases are ordered this way)

1. **Functional product first** (§9) — build a working core before process, governance, or
   monetization. This single rule drives every "deferred by design" line below.
2. **Cheap now, expensive later** (§4.7, §12.7) — three designed-in-early, deferred
   activations: PQC transport swappability (design P0 → harden P3), x402-compatible payment
   interface (design P1 → activate P5), plugin provider trait (architecture P0 → new
   providers P2).
3. **Open-first, no closed dependencies** (§2.10) — Apache-2.0 from day one; open-source
   components only; [[qbraid]]/[[strangeworks]] are references, never load-bearing.
4. **De-risk with existing assets** (§8) — Milimo Quantum's HAL/agent-mesh and Finance
   Claw's two-stage approval de-risk P0–P1 — *but* those claims are single-source and must
   be validated against the real codebases first ([[mainza-milimo-foundation]]).
5. **Cost governance is non-optional rail** (§2.5) — exists from Phase 1, never deferred.

## Phase gates at a glance

| Gate | When | Verdict asked (from the source's exit criteria) |
|---|---|---|
| **P0 gate** | Week 4 | Simulator job end-to-end via MCP with a provenance record written, open-source deps only |
| **P1 gate** | Week 10 | VQE loop runs on a real QPU with budget enforced + every step provenance-logged |
| **P2 gate** | Week 16 | A human watches *and interrupts* an in-flight multi-hour hybrid loop from the console |
| **P3 gate** | Week 22 | Third-party gateway security review passes; canary CI caught a real regression in staging |
| **P4 gate** | Week 30 | Public repo + documented plugin API; third-party provider integration merged; Search Mode beats a hand-designed baseline (in simulation) |
| **P5 gate** | 31+ | Multi-tenant scale + monetization rail live (business-model exploration, §12.7) |
| **P6 gate** *(synthesis)* | 37+ | A lab runs the full discovery loop — literature ground → experiment → structured report → archived library entry — using user- and agent-created workflows, every claim tracing to a ledger record or citation |

## Phase 0 — Foundations (Weeks 1–4) → gate P0

> **Status (2026-08-16):** skeleton committed under `platform/` — four MCP servers
> (compiler/execution/cost/provenance, MCP stdio), gateway (auth + swappable transport,
> audit), provider trait + pure-numpy local simulator + 3 capability-schema entries,
> content-addressed hash-chained ledger, cost pre-flight, QASM2-subset IR. **P0 gate test
> green end-to-end over stdio MCP** (`platform/tests/test_phase0_gate.py`, 13 tests).
> Apache-2.0 LICENSE added. OAuth 2.1 interface stubbed (dev auth); real IdP wiring,
> Qiskit adapter, and the streamable-HTTP gateway remain.

Ordered execution (dependencies within the phase):

1. **Apache-2.0 license + public repo** — one-time decision, cheap now, expensive later (§7).
   *(Status 2026-08-16: repo created privately at github.com/mainza-ai/kubona; kept private
   until a working core exists — per maintainer decision. Flips public at the Phase-0 gate:
   simulator job end-to-end via MCP with a provenance record.)*
2. **Federated MCP server skeleton** — compiler / execution / cost-governance /
   eval-provenance, stub tool schemas ([[mcp-and-context-flooding]]).
3. **OAuth 2.1 on the gateway**; transport layer designed **swappable from day one**
   (PQC itself deferred to Phase 3 — cryptographic agility, not a retrofit).
4. **Provider abstraction as a plugin trait** — Kubona's own open-source interface with the
   first two implementations: Qiskit (IBM) and local/GPU simulator ([[nvidia-cuda-q|CUDA-Q]]
   open components). Trait from day one so later providers are pure additions, not core
   rewrites.
5. **[[provider-capability-schema]]** with 2–3 initial entries (IBM via Qiskit, local GPU
   simulator, +1).

**Gate P0:** agent submits a simulator job end-to-end through the MCP chain and gets results
back with a provenance record written, **using only open-source dependencies**.

## Phase 1 — Core execution + cost governance (Weeks 5–10) → gate P1

- `job_id` session abstraction threaded through every call (§4.3).
- [[cost-governance]] service: mandatory simulator pre-flight, hard budget ceilings,
  two-stage approval gate, real-time burn-rate telemetry.
- Design the cost-gov payment interface **x402-compatible from day one** (§12.7 — cheap now,
  expensive to retrofit; same logic as PQC in Phase 0).
- [[qir-mlir-compilation]] lowering for OpenQASM 3.0 + Qiskit-generated circuits (Rust core).
- [[mitiq]] integration, **ZNE first** (highest value-to-effort of the three techniques).

**Gate P1:** a VQE loop runs end-to-end on a real QPU via direct provider SDK integration,
budget ceiling enforced, every step provenance-logged.

## Phase 2 — Multi-provider + control plane (Weeks 11–16) → gate P2

- Add [[quantinuum]] (pytket), [[rigetti]] (pyQuil), [[neutral-atom-quera-pasqal]] (Pulser)
  as direct, open-source SDK integrations — validating the P0 plugin trait.
- [[circuit-cutting]] (Qiskit Circuit Knitting Toolbox) for width/depth-constrained circuits.
- Dart/Flutter console, highest-priority surfaces first (§5.3): cost/approval console +
  circuit visualizer; **provenance browser + fleet-status views follow in Phase 3**.
- Compatibility shim for ≥1 orchestration framework — LangGraph recommended (long-running
  stateful graphs map cleanly onto the VQE session model).
- **Identity groundwork** *(synthesis)*: keypair identities for users and agents + owner-
  delegated authority beneath the OAuth/PQC layer ([[user-identity-and-profiles]]) —
  scaffolding for the Phase-6 collaboration layer.

**Gate P2:** a human operator can watch and interrupt an in-flight multi-hour hybrid loop
from the Dart console.

## Phase 3 — Security hardening + provenance depth (Weeks 17–22) → gate P3

- Hybrid [[security-and-pqc]] transport (ML-KEM) on the gateway — the Phase-0
  swappability pays off.
- Per-call identity-aware authorization (replacing coarse RBAC roles).
- Hardware-canary CI — nightly regression against every connected backend.
- [[provenance-and-reproducibility]] server queryable as an **MCP resource**
  ("what worked best historically" queries).
- Console: provenance/experiment browser + provider/fleet status views (the Phase-2
  follow-ons).

**Gate P3:** third-party security review of the gateway passes; canary CI caught at least
one real provider-side regression **in staging** before it reaches an agent.

## Phase 4 — Evaluation, autoresearch engine, release prep (Weeks 23–30) → gate P4

- Eval corpora: QuantumKatas + Qiskit HumanEval + PennyLang; **agent-behavior evals**
  (budget compliance, simulator-first compliance, WBP-detection response)
  ([[evaluation-and-benchmarks]]).
- Build the [[karpathy-autoresearch]] **general engine first** (§11.5): pluggable harness
  interface + orchestrator, shipping with **three harnesses** — VQE ansatz search ·
  mitigation-strategy search · correctness-regression. **QAOA and compilation-pass search
  follow once the engine has proven out** (not in the first ship).
- **Kubona Search Mode** (§11.4): the autonomous ansatz-search MVP, simulator-only by
  default; validates the sandboxed-write-scope pattern before GQE lands.
- **GQE circuit optimization as the 4th harness** (v2) — same four-part contract, not a
  separate system; retraining runs on [[unsloth]] (2× faster LoRA, fits Apple
  Silicon/consumer GPUs) so the §13.3 flywheel stays affordable *(synthesis)*.
- Correctness-regression harness wired into the Phase-3 canary CI — **gates releases**.
- Plugin/provider-registration SDK for third-party backend contributions + contribution
  governance doc for the tool-surface security boundary.
- **Stretch goal — meta-level loop (§13):** a scheduled reflection job that **proposes**
  (never auto-applies) proposal-strategy and harness-registry changes for human review —
  the [[meta-search-and-self-evolution]] tier that makes the engine self-evolving. Gated on
  the object-level engine shipping first (a meta-loop with nothing to reflect on is
  premature); auto-application of meta-changes is Phase 5+.

**Gate P4 (three criteria):** public repo with documented plugin API; ≥1 third-party
provider integration merged (validates the extensibility model); Search Mode produced ≥1
ansatz beating a hand-designed baseline on a benchmark problem, entirely in simulation.
**Plus two §11.5 additions:** the correctness harness has caught ≥1 real regression in
staging; the algorithm library has ≥1 entry through the full **simulator → cost-governance-
approved → real-hardware-validated** pipeline.

## Phase 5 — Scale + breadth (Weeks 31+) → gate P5, then continuous

- [[xanadu-strawberry-fields]] photonic/CV integration; [[dwave]] Ocean SDK deepening.
- Export-control/jurisdiction enforcement at the routing layer (§2.9; the
  [[provider-capability-schema]] jurisdiction field becomes load-bearing).
- Horizontal scaling of the provenance ledger + cost-governance for multi-tenant/enterprise.
- Activate [[x402]] for sub-threshold transactions and algorithm-library metering (§12.7).
- Auto-application of meta-loop changes via the shadow-mode staged rollout (§13.5) — only
  after the Phase-4 propose-and-review tier has run safely.

**Gate P5:** multi-tenant scale + first live monetization rail; thereafter the roadmap
becomes business-model exploration (agent-readiness audits, library metering), not core
build.

## Phase 6 — Science Discovery & Collaboration (Weeks 37+) *(maintainer synthesis)* → gate P6

The human/AI science layer on top of the Phase-0–5 quantum core. Everything here is
scaffolded earlier (identity in P2, engine + meta-loop in P4) and shipped only after a
working, governed core.

- **Profiles & labs:** multi-user identities, lab/team communities with scoped state
  ([[user-identity-and-profiles]]).
- **Workflow & agent authoring:** versioned YAML workflows with triggers + approval gates;
  agent registry with personas & delegated authority; agent-created agents under the §13.4
  safety rail ([[workflows-and-agent-authoring]]).
- **Experiment notebook & results studio:** the two new control-plane surfaces
  ([[agent-ux-control-plane]]) — structured, citable, reproduction-ready reports over the
  provenance ledger ([[science-discovery]]).
- **Scholarly & data integrations:** Tier-1 first (arXiv, OpenAlex), Tier-2 domain data
  (materials, biodiversity) as governed MCP resources ([[science-data-integrations]]).
- **Full discovery loop:** the platform runs autonomous experiment campaigns — literature
  ground → hypothesis → experiment → analysis → report → library → meta-learn —
  inside the existing cost/capability gates.
- **Personas & fine-tuned domain models** via [[unsloth]] (private, local; ties to the
  Milimo Apple-Silicon HAL, [[mainza-milimo-foundation]]).

## Deferred by design (until a working product exists)

- Fit-for-governance process and formal contribution governance (P4 is *targeted* only if
  outsiders appear), x402 activation, provider agent-readiness-audit offering —
  all sequenced after a working product ("functional product first", §9).
- Generative/agent-materialized UI (MCP Apps / A2UI) → v2+, not for core surfaces.

## Critical path & cross-phase dependencies (maintainer synthesis)

The chain that must not slip — and the debt that must not be left unpaid:

1. **P0 plugin trait + P1 QIR/MLIR (Rust core)** → everything downstream assumes the
   provider abstraction and compiler core. Rust is the highest-leverage hire (§8).
2. **P1 cost-gov + x402-compatible interface** → unblocks P5 monetization.
3. **P0 provenance record → P3 ledger-as-MCP-resource → P4 algorithm library** —
   the compounding asset; never let earlier phases drop the record.
4. **P3 canary CI → P4 correctness harness wiring** (release gate) — the §11.5 exit
   additions depend on P3 being real.
5. **P4 Search Mode → GQE harness** — write-scope pattern must be validated first.

## Team loading (§8)

| Role | Phases | Notes |
|---|---|---|
| Rust systems engineer (compiler/WASM/cost-gov) | 0–5 | Highest-leverage single hire; QIR/MLIR + cost-gov correctness is the core defensibility |
| TS/Node MCP platform engineer | 0–3 | Gateway, federated server topology, OAuth/PQC |
| Python quantum SDK integration engineer | 0–5 | Direct Qiskit/PennyLane/Q#/Cirq/pyQuil/tket/Pulser bindings, CUDA-Q open components, eval harnesses |
| Dart/Flutter engineer | 2–5 | Console surfaces; part-time/contract OK given narrower scope |
| Security reviewer (contract) | 3 | PQC + MCP threat-model review — don't skip given the credential/spend blast radius |

Go optional — Rust can absorb the canary/ledger roles. Phase 6 adds *(synthesis)*: a
workflow-DSL/UX engineer (authoring surfaces, experiment notebook) and a
fine-tuning/ML engineer ([[unsloth]] GQE retraining, personas) — both part-time/contract
options given the narrower, post-core scope. **De-risking note (§8):** the Milimo
Quantum foundation (Qiskit v2.2, 14+ agents, VQE, Apple Silicon/CUDA HAL) and Finance Claw
approval pattern make P0–P1 substantially de-risked — validate the claims against the real
codebases before lifting HAL/agent-mesh/approval patterns ([[mainza-milimo-foundation]]).

## Schedule risks to watch

- Recruiting the six-language stack (Rust/TS/Dart/Py/JS/±Go) — Go optional, Rust absorbs
  ([[risks]]).
- Provider queue times/availability during P1–P3 hardware work — off-peak scheduling and
  Sessions ([[quantum-cloud-pricing]]).
- Single-source Milimo reuse claims may not survive validation — plan P0–P1 as if they
  might not.
- CUDA-Q open components depend on NVIDIA's OSS posture — keep the plugin trait honest so
  the dependency stays swappable.

## Living-plan mechanics

- Mirrored to [[source-gap-analysis-plan]] §§7–9, 11.5, 12.7; when that source changes,
  update this page and note it in [[log]].
- Phase transitions are recorded in [[log]] as `roadmap | gate P<n> <pass|fail> — <reason>`.
- This page stays `status: living`; [[log]] is append-only.

---

*Phase content and exit criteria trace to the source; gates, ordering, critical path, and
tracking conventions are maintainer synthesis, kept deliberately explicit so they can be
disagreed with and revised.*
