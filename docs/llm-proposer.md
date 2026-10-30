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

## Remaining edge

The bridge does not expose an LLM rerun route yet. A public stateless route
would be an ungoverned model-egress and public-write surface. The intended
wiring is at the governed search-loop edge with auth, a spend/egress policy,
and ledger sealing. Until then the deterministic mutator remains the default
and the live path is library-only.

## GQE

`build_gqe_harness` is exported from `kubona.autoresearch`, but it is still
the n-gram + random stand-in and is **not** registered in the Go harness
registry. Treat it as experimental; a trained transformer adapter is the
drop-in replacement.
