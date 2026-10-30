# LLM circuit proposer

`NvidiaProposer` is now a real OpenAI-compatible client:

- `provider="strata"` uses `STRATA_BASE_URL` / `STRATA_MODEL` (local default).
- `provider="nvidia"` uses NVIDIA NIM and requires `NVIDIA_API_KEY`.
- `propose()` POSTs to `<base_url>/chat/completions` with a bounded timeout,
  one retry on transport/5xx, and no retry on 4xx. 4xx is a refusal.
- Tests use a local stub HTTP server; CI never calls a real model.

`make_gate_proposer` adapts the text proposer to `IndependentRerun`'s
`(current, step) -> ops` contract. The parser accepts only the closed
Kickmix gate vocabulary and arity, checks qubit range and record ids, and
raises on any doubt. The rerun loop records a proposer failure as an invalid
trial instead of aborting the run.

## Governed Go rail (C5b)

`POST /api/codesign/rerun` is the production rail. It is **session-scoped**
(not open-write), requires a logged-in session or operator bearer (anonymous
callers are refused even in single-lab mode), rate-limited, idempotent
(`request_id` required), and bounded by steps, tokens, timeout, and a per-run
USD guardrail. The Go core
owns the LLM call and role (`codesign`); the bridge stays a stateless
evaluator (`spec` + `evaluate` ops only). No LLM key moves into the bridge.

Billing is BYOK: the provider bills the operator's key. Kubona records an
**estimate** (`billing:"provider"`, `payer:"user-key"`) and never debits the
QPU session/tenant cap. Local providers are free (`billing:"none"`,
`payer:"local"`). Cloud is refused unless `KUBONA_LLM_CLOUD=1` and the
Sovereign Air-Gap is off. The rate card must be valid; stale/unknown models
refuse before egress. See `docs/local-deployment.md` for the operator flow.

The Python `IndependentRerun`/`make_gate_proposer` remain the reference and
test path. The console Codesign Cockpit now has a **Governed proposer rerun**
panel (session picker, request_id retry, budget fields, sealed receipt), and
the MCP `cost/codesign_rerun` tool forwards to this route with the operator
bearer (`KUBONA_API_URL` + `KUBONA_API_TOKEN`). The MCP server holds no LLM
key; it only carries the scoped budget envelope.

## GQE

`build_gqe_harness` is exported from `kubona.autoresearch`, but it is still
the n-gram + random stand-in and is **not** registered in the Go harness
registry. Treat it as experimental; a trained transformer adapter is the
drop-in replacement.
