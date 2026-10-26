---
type: concept
name: Agent subsystem consolidation
date: 2026-10-17
updated: 2026-10-25
tags: [plan, agents, consolidation, production]
sources: [source-frontier-repo-audit-2026]
status: adopted
---

# Agent subsystem consolidation (P1-4) — design record

> Steps 1–5 landed (2026-10-25): Go SQLite `custom_agents` is the single authoritative
> store, backed by the canonical cross-language contract `platform/contracts/agent-spec-v1.json`.
> Python registries use read/write adapters to this authority. Verdict spaces are mapped
> via explicit typed transition functors (`verdicts.go`), and `WorkflowEngine` is wired
> end-to-end via gRPC and Go HTTP API with SQLite budget reservation.

## Inventory (2026-10-17 / updated 2026-10-25)

Go `platform/core/pkg/agents/` (production tribunal/runtime): `a2a.go`
(A2A server), `campaign_synth.go`, `collective.go` + R-series
(`evidence.go`, `obligation.go`, `scoring.go`), `custom.go` (custom agent
registry backed by authoritative SQLite `custom_agents`), `verdicts.go` (typed
cross-rail verdict functors), `molecule_table.go`, `replan.go`.

Python `platform/kubona/agents/` (bridge/agent runtimes): `collective.py`
(DisconfirmationVerdict/Officer pydantic models only), `loop.py`
(LLMAgentLoop gateway tool loop + arg normalization), `registry.py`
(Agent/AgentRegistry definitions — no-escalation creation path),
`user_defined.py` (UserDefinedAgentManifest + CustomAgentRegistry backed by
SQLite `custom_agents` or JSON with STANDARD_SYSTEM_AGENTS seeds).

## Overlap map

- Agent DEFINITIONS: Go `custom.go` registry vs Python `registry.py`
  vs Python `user_defined.py` CustomAgentRegistry — converged onto
  authoritative Go SQLite `custom_agents` table obeying canonical
  `platform/contracts/agent-spec-v1.json`.
- Loops: Go tribunal/collective + autoresearch LoopEngine vs Python
  LLMAgentLoop — different purposes (verdict debate + autonomous
  research vs gateway tool loop), shared vocabulary only.
- Collective: Go full tribunal vs Python disconfirmation models —
  typed via `platform/core/pkg/agents/verdicts.go`.

## Target shape (adopted 2026-10-25)

- Go owns agent identity and lifecycle in SQLite `custom_agents`.
- Python `user_defined.py` and `registry.py` operate as adapters reading
  and writing to this authoritative store.
- Both runtimes obey canonical `platform/contracts/agent-spec-v1.json` supporting
  both conversational prompt personas and deterministic pipeline execution.
- LLMAgentLoop stays Python (gateway-bound); tribunal stays Go.
- `WorkflowEngine` is wired via gRPC `WorkflowPlan` / `WorkflowRun` to Go Core
  `POST /api/workflows/plan` and `POST /api/workflows/run` with SQLite budget
  reservations.

## Sequenced migration (completed 2026-10-25)

1. Freeze both registries' APIs with conformance tests (`custom_contract_test.go`,
   `test_agent_registry_contract.py`, `test_user_defined_agents.py`). (✅ Landed 2026-10-17)
2. Single verdict schema: Go `platform/core/pkg/agents/verdicts.go` defines explicit
   typed functors mapping Lifecycle (`PROPOSED/ANCHORED/AUDITED/COLLAPSED`), Deliberation
   (`pass/fail/REFUTED`), and Disconfirmation Finding (`SURVIVES/FALSIFIED/INCONCLUSIVE`).
   (✅ Landed 2026-10-25)
3. One identity home: Go SQLite `custom_agents` table is the authoritative store. (✅ Landed 2026-10-25)
4. Adapters in Python `user_defined.py` and `registry.py` reading and writing directly
   to SQLite `custom_agents` while preserving existing test interfaces. (✅ Landed 2026-10-25)
5. End-to-end `WorkflowEngine` wiring with session budget reservation and failure
   compensations. (✅ Landed 2026-10-25)

## Explicit non-goals

- Merging the tribunal with LLMAgentLoop (different jobs).
- Rewriting Python agents in Go or vice versa (stack ruling stands).

Related: [[rea-adoption-plan]] · [[protocol-strategy]] · [[multi-agent-research-collective]]
