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
(Agent/AgentRegistry definitions), `user_defined.py`.

## Overlap map

- Agent DEFINITIONS: Go `custom.go` registry vs Python `registry.py` —
  real overlap (two homes for "registerable agent").
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
2. Single verdict schema (Python models ← Go verdict strings, tested
   both directions).
3. One identity home (decision point: Go vs Python lead).
4. Deprecate the loser behind the winner's API (adapter, not flag day).
5. Delete only after two green releases with zero fallback traffic.

## Explicit non-goals

- Merging the tribunal with LLMAgentLoop (different jobs).
- Rewriting Python agents in Go or vice versa (stack ruling stands).

Related: [[rea-adoption-plan]] · [[protocol-strategy]] · [[multi-agent-research-collective]]
