# Kubona

**Kubona** — Chitonga (Tonga, Zambia), *ku-bona*, "to see" — is an open-source,
MCP-native **quantum-computing and AI-enhanced science discovery platform** for autonomous AI agents:
multi-user labs, user- and agent-created workflows and agents, and autonomous experiments
with organized, reproducible results.

> To see is the founding act of science: observe, measure, record, understand.
> Kubona surfaces quantum phenomena, runs experiments, and shows their results — systematically.
> The name is the Chitonga infinitive: *ku-* (infinitive) + *-bona* (see, observe, perceive).

## Repository layout

| Path | What it is |
|---|---|
| [`platform/`](platform/) | The code: Python runtime (`kubona/`), Go orchestrator (`core/`), Next.js console (`console/`), PySCF/Qiskit bridge (`bridge/`). Start at [`platform/README.md`](platform/README.md) |
| [`wiki/`](wiki/) | The living knowledge base (Obsidian vault, 136 pages as of 2026-10-19): entities, concepts, sources, overview, roadmap, index, log |
| [`raw/`](raw/) | Immutable ingested source documents (read-only) — incl. the implementation plan (rev 4b) |
| [`scripts/`](scripts/) | Lint + CI helpers (wiki structural lint, codesign re-runs) |
| [`AGENTS.md`](AGENTS.md) | Maintainer operating rules — read before editing the wiki |


## The plan at a glance

- **Phases 0–5** — the quantum core: federated MCP servers, QIR/MLIR, cost governance,
  provenance ledger, autoresearch engine (§11), meta-level self-evolution loop (§13).
- **Phase 6 (synthesis)** — Science Discovery & Collaboration: identities & labs, workflow
  & agent authoring, experiment notebook & results studio, scholarly/data integrations,
  local model training.

See [`wiki/overview.md`](wiki/overview.md) for the synthesis and [`wiki/roadmap.md`](wiki/roadmap.md)
for the implementation plan.

## Run it

```bash
# Backend — Go orchestrator (the API the console was built against)
go build -o /tmp/kubonad ./cmd/kubonad   # from platform/core
/tmp/kubonad -port 8080

# Frontend — sovereign console on :3000 (dev token required locally)
NEXT_PUBLIC_KUBONA_CONSOLE_TOKEN=dev-token pnpm dev --port 3000   # from platform/console
```

Then work the ten adopted local-simulation test scenarios
([`wiki/concepts/local-simulation-test-scenarios.md`](wiki/concepts/local-simulation-test-scenarios.md):
co-pilot chat, tribunal, flight recorder, on-demand widgets — $0.00, no QPU.
Details in [`platform/console/README.md`](platform/console/README.md).

```bash
# Python suite — full verification incl. the circuit co-design program
.venv/bin/python -m pytest platform/tests/ -q   # 961 passed, 3 skipped as of 2026-10-19
```

## License

Apache-2.0
