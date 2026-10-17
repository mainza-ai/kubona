---
type: concept
name: Agent subsystem consolidation
date: 2026-10-17
updated: 2026-10-17
tags: [plan, agents, consolidation, production]
sources: [source-frontier-repo-audit-2026]
status: candidate
---

# Agent subsystem consolidation (P1-4) — design record

> The refactor itself is a queued program, not a slice. This page records
> the inventory, the target shape, and the sequenced migration so the work
> starts from a decision, not from exploration. Status stays `candidate`
> until an operator schedules the program.

## Inventory (2026-10-17)

Go `platform/core/pkg/agents/` (production tribunal/runtime): `a2a.go`
(A2A server), `campaign_synth.go`, `collective.go` + R-series
(`evidence.go`, `obligation.go`, `scoring.go`), `custom.go` (custom agent
registry), `molecule_table.go`, `replan.go`.

Python `platform/kubona/agents/` (bridge/agent runtimes): `collective.py`
(DisconfirmationVerdict/Officer pydantic models only), `loop.py`
(LLMAgentLoop gateway tool loop + arg normalization), `registry.py`
(Agent/AgentRegistry definitions — no-escalation creation path),
`user_defined.py` (UserDefinedAgentManifest + CustomAgentRegistry with
STANDARD_SYSTEM_AGENTS seeds + JSON persistence — a THIRD home for
registerable agents, found during step-1 inventory; the overlap map
below is corrected accordingly).

## Overlap map

- Agent DEFINITIONS: Go `custom.go` registry vs Python `registry.py`
  vs Python `user_defined.py` CustomAgentRegistry — THREE homes for
  "registerable agent", not two (the third found in step-1 inventory):
  Go seeds + user manifests (DB-persisted, seed-immutable, shape
  validation); Python `registry.py` creator-scoped agents (subset Rule,
  ceiling inheritance, JSON file); Python `user_defined.py` manifests
  (seed reservation, JSON file). Three stores, three validation
  dialects, one concept.
- Loops: Go tribunal/collective + autoresearch LoopEngine vs Python
  LLMAgentLoop — different purposes (verdict debate + autonomous
  research vs gateway tool loop), shared vocabulary only.
- Collective: Go full tribunal vs Python disconfirmation models —
  complementary (models could TYPE the Go verdicts, not replace them).

## Target shape (proposed, not decided)

- Agent definitions converge on ONE registry (home TBD — the Q-22 stack
  ruling puts orchestration in Go, but the Python registry serves the
  bridge runtimes that Go cannot import; likely outcome: Go owns
  identity/lifecycle, Python keeps a read-through cache, not a rewrite).
- LLMAgentLoop stays Python (gateway-bound); tribunal stays Go. No merge
  — the "consolidation" is definitions + verdict types, not loops.
- Python disconfirmation models become the cross-rail verdict schema
  (they already mirror the Go roles).

## Sequenced migration (when scheduled)

1. Freeze both registries' APIs with conformance tests (no behavior
   change — pin first).
> ✅ STEP 1 LANDED 2026-10-17 (all three homes, not two):
> `custom_contract_test.go` (Go seeds/validation/delete contract),
> `test_agent_registry_contract.py` (creation-path refusal edges +
> corrupt/drift store behavior), `test_user_defined_agents.py`
> extended (empty-field/system-delete/forgery edges). Live finding
> fixed in-slice: non-dict rows in `agents.json` crashed registry
> boot (fail-open, against the file's own never-fatal contract) —
> now skip, negative-proven (saw red pre-fix). Steps 2–5 still await
> operator scheduling + the identity-home decision (step 3).
2. Single verdict schema (Python models ← Go verdict strings, tested
   both directions).
> ⏸️ STEP 2 SCOPED 2026-10-17, freeze landed, decision open: Go has
> THREE verdict vocabularies, not one — turn-lifecycle
> (`PROPOSED/ANCHORED/AUDITED/COLLAPSED`), deliberation outcome
> (`pass/fail`, `REFUTED` in scoring), finding-falsification (Python
> `DisconfirmationVerdict`). Minting the canonical mapping now would
> pre-empt the step-3 identity-home decision — so step 2 lands as
> wire-shape freezes both sides (`verdict_wire_test.go`: turn JSON
> round-trip + lifecycle vocabulary; `test_verdict_wire_shape.py`:
> finding field names + JSON round-trip both polarities) and STOPS.
> The mapping choice stays an operator decision.
3. One identity home (decision point: Go vs Python lead).
4. Deprecate the loser behind the winner's API (adapter, not flag day).
5. Delete only after two green releases with zero fallback traffic.

## Explicit non-goals

- Merging the tribunal with LLMAgentLoop (different jobs).
- Rewriting Python agents in Go or vice versa (stack ruling stands).

Related: [[rea-adoption-plan]] · [[protocol-strategy]] · [[multi-agent-research-collective]]
