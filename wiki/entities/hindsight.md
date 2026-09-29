---
type: entity
name: Hindsight (vectorize-io)
date: 2026-09-27
updated: 2026-09-27
tags: [memory, mcp, prior-art, sidecar]
sources: [source-frontier-repo-audit-2026]
status: reference
---

# Hindsight — agent memory as a sidecar service

**https://github.com/vectorize-io/hindsight** · MIT · HEAD verified 2026-09-26.
Integration class: **Sidecar** — a separate process with its own storage, reached over MCP, and
graceful-degrade when absent ([[integration-topology-and-licence-discipline]]).

The three operations are `retain` / `recall` / `reflect` over **memory banks**
(`client.retain(bank_id=…)`). Memories are not a vector soup: they are split into
**world-facts vs. experiences** pathways and stored as *entities + relations + time series* with
sparse and dense vectors alongside (`README.md:286`). `observations`, `mental models` and
`knowledge pages` are first-class objects rather than prompt text.

Two design decisions matter more than the algorithm:

- **The bank is part of the endpoint** — the MCP server exposes the three ops at
  `http://localhost:8888/mcp/{bank_id}/`, so *which* memory a caller may touch travels with the URL
  instead of relying on the caller to pass the right filter
  ([[confidentiality-lattice-provenance]]).
- **Recall has a budget** — the LLM wrapper (`wrap_anthropic()`, LiteLLM underneath) takes per-call
  `hindsight_*` kwargs including a **recall budget**, i.e. retrieval cost is a first-class
  parameter ([[agent-resource-economy]]).

Reported result: LongMemEval SOTA, **+41.9% vs RAG** — a **vendor documentation claim**, and
arXiv:2512.12818 is cited by the project; neither is independently verified here
([[evaluation-and-benchmarks]]).

Kubona takes the **shape** (bank-scoped endpoints, budgeted recall, three-op memory API) and the
MIT licence permits more, but memory content in Kubona is a *claim-bearing* object with provenance
([[provenance-and-reproducibility]], [[agentic-memory-and-observability]]) — so the ledger, not the
memory store, remains the source of truth.