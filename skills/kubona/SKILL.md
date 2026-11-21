---
name: kubona
description: Router skill for AI agents connecting to Kubona over MCP, HTTP, A2A, or local CLI. Enforces Kubona's three invariants (classical anchor, budget ceiling, cryptographic receipt) and routes to domain skills.
---

# Kubona: Agent Router & Fast Path

Version 2026-11-20. Guidance only, never guarantee: every trust property
(mandates, budgets, admissions, autonomy locks, ledger appends) is enforced in
code regardless of what this file claims.

## Three Non-Negotiable Invariants
1. **No claim without a classical anchor:** Call `get_classical_anchor` before
   reporting any quantum energy, gap, or advantage. Never claim quantum
   advantage from sub-FCI shot noise.
2. **No execution without a budget hold:** Open a ceilinged session
   (`create_session`), call `estimate_cost` first (`heuristic: true` — estimates
   inform, ceilings bind), and never bypass human/mandate approval for paid QPU
   runs.
3. **No result without a cryptographic receipt:** Every job, hypothesis,
   approval, and observation is hash-chained in the ledger. Cite exact IDs and
   mark unobserved facts **unknown**.

## Rules
- Prefer **MCP Resources** first when connected over MCP: read
  `kubona://skills` for this skill suite and `kubona://contracts/error-catalog`
  for exact error recovery.
- Use **profiled MCP connections** (`?profile=science|read-only|governance|ops|world`)
  to avoid context flooding across all 49 tools.
- Never touch live `~/.kubona` state during local experiments: override with
  `KUBONA_STATE_DIR=$(mktemp -d)`.
- Never `cat` append-only ledgers (`ledger.jsonl`, `jobs.jsonl`); slice the
  tail (`tail -n 20`) or query SQLite with `LIMIT`.
- Answer from evidence: mark every claim **seen** (record/job/event ID or
  `file:line`) or **unknown**. A template fallback (`authored=0`) is never an
  observation.

## Router
| Intent / Target | Load Skill | Primary Surface |
|---|---|---|
| Connect via MCP (`stdio`/HTTP `/mcp`), HTTP `:8080`, A2A `/api/a2a/v1`, auth, client configs | `kubona-connect` | `POST /mcp`, `kubona.run mcp-config`, `/.well-known/agent.json` |
| Compute classical anchors, compile QASM, run VQE/PES/QAOA/ZNE/QUBO/discovery | `kubona-experiments` | `science`, `compiler`, `execution` MCP servers; `POST /api/experiments` |
| Budgets, cost estimates, Ed25519 mandates, MRTR approvals, session autonomy, loop control | `kubona-governance` | `cost`, `control` MCP servers; `/api/sessions/{sessionId}/autonomy` |
| Hypotheses, Elo tournaments, research graph, memory, manuscripts, RO-Crate/PROV, Lean 4 | `kubona-research` | `science`, `provenance` MCP servers; `/api/sessions/{sessionId}/hypotheses` |
| Local Docker stack, `kubona.run doctor`, ledger forensics, `delete_blocked`, error triage | `kubona-ops` | Local CLI (`kubona.run`, `kubonad`, `sqlite3`), `kubona://contracts/error-catalog` |

## First move
- **Over MCP (Streamable HTTP or stdio):**
  1. Read resource `kubona://skills` and `kubona://contracts/error-catalog`.
  2. Call `list_provider_registry` (zero spend) and `get_session_autonomy` for your session.
- **Over HTTP (`http://127.0.0.1:8080`):**
  1. Fetch `GET /llms.txt`, `GET /api/openapi.json`, or `GET /.well-known/agent.json`.
- **On a local checkout:**
  ```sh
  platform/.venv/bin/python -m kubona.run doctor
  platform/.venv/bin/python -m kubona.run mcp-config --client claude-code --transport http --profile science
  ```

## Speed patterns
- Read `kubona://contracts/error-catalog` before retrying any 4xx/5xx error.
- Use `get_research_graph` with `since_seq` to resume long sessions cheaply.
- Scope every query to a `session_id` / tenant; unscoped cross-tenant reads refuse.
- On local debugging, inspect the last 20 ledger events before reading handlers.

## Traps
- Fail-closed arithmetic: NaN, Inf, and negatives refuse everywhere — amounts, budgets, noise rates. `platform/core/pkg/session/session.go:15`, `platform/kubona/cost/mandate.py:78`, `platform/kubona/cost/payment.py:20`.
- Cancel graph is terminal-immutable: `QUEUED` never runs on cancel, `RUNNING` is fenced, terminal states replay. `platform/kubona/jobs.py:43`, `platform/core/pkg/jobs/jobs.go:639`.
- Mandate binding order is tenant, job, expiry, budget, signature. `platform/kubona/cost/mandate.py:37`, `platform/core/pkg/jobs/admission.go:220`.
- Ledger event kinds are a closed taxonomy; unknown kinds refuse. `platform/core/pkg/ledger/events.go:15`, `platform/contracts/event-types.json:1`.
- Idempotency replays byte-identical receipts; same ID with different body returns `409` `request_conflict` — mint a new `request_id`. `platform/core/pkg/api/idempotency.go:9`.
- Gateway dispatcher never coerces unknown servers or tools (`KeyError`). `platform/kubona/mcp/gateway.py:173`.
- Provenance reads are tenant-scoped: `requester=None` sees public rows only. `platform/kubona/provenance/ledger.py:220`.
