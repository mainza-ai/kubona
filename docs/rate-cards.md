# Rate cards

`platform/contracts/rate-cards.json` is the single source for the Atlas lens
per-dispatch cost and the LLM token rates. `platform/core/pkg/rates/rate-cards.json`
is the build-time copy embedded by `pkg/rates`; a parity test fails if the two
drift. Copy the canonical file, never hand-edit the duplicate.

## What the card is

- `version`: contract version.
- `source`: where the numbers came from. The current card is
  `internal-heuristic-2026-10-12`, not a vendor price sheet.
- `effective_date` + `stale_after_days`: the validity window.
- `heuristic`: `true` until a real vendor sheet replaces the values.
- `lens.atlas-lens.cost_usd`: per-dispatch reservation.
- `models.*`: input/output USD per 1K tokens.

## Stale is a refusal

`kubonad` calls `rates.Validate` at startup and refuses to serve if the card
is stale. LLM pricing and Atlas lens dispatch also check at call time, so a
long-running daemon cannot keep billing from an expired heuristic. Unknown
models still return `ok=false`; the code never invents a rate.

Local providers (Strata/oMLX/Ollama/LM Studio) are free and never consult the
card. Cloud LLM is BYOK in the supported local deployment: the provider bills
the operator's key; the card only produces a **local estimate** for the
`KUBONA_CODESIGN_RUN_BUDGET_USD` / `KUBONA_LLM_BUDGET_USD` guardrails. It is
never a Kubona charge and never debits the QPU session/tenant cap. The provider
dashboard is authoritative.

## Updating

1. Edit `platform/contracts/rate-cards.json` (source, effective date, rates).
2. Copy it to `platform/core/pkg/rates/rate-cards.json`.
3. Run `go test ./pkg/rates ./pkg/llm ./pkg/api`.
4. Raise `version`; do not shorten `stale_after_days` to make a red gate pass.

Vendor reconciliation (an actual provider invoice/price sheet with
`heuristic:false`) remains a release-time task. The current card makes the
heuristic explicit and stops it from silently outliving its window.

## Reconciliation checklist (before 2026-11-11)

This is an operator task, not a code-only change. The current card is
`heuristic:true`; the values must be replaced from authoritative vendor
documents, not guessed.

- [ ] Inventory the models actually routable in this deployment
      (`/api/llm/settings` role assignments and the console LLM Settings UI).
      At minimum: NVIDIA NIM, OpenCode/DeepSeek, Anthropic, OpenAI, and any
      other enabled provider. Remove or explicitly leave unpriced models that
      are not used (unknown models refuse by design).
- [ ] Obtain the current vendor price sheet or account invoice for each:
      input USD per 1K tokens, output USD per 1K tokens, model ID, region,
      and contracted/enterprise discount if any. Record the URL and retrieval
      date (or keep the PDF in `raw/` only if redistribution is allowed).
- [ ] Obtain the World Labs Marble 2 Atlas-lens price (credits or USD per
      dispatch) and confirm what one dispatch includes.
- [ ] Decide the effective date and keep `stale_after_days: 30`; do not
      shorten it to make a red gate pass.
- [ ] Update `platform/contracts/rate-cards.json`:
      `version` bump, `source` = vendor document + date, `effective_date`,
      `heuristic:false`, exact `models.*` rates, and `lens.atlas-lens.cost_usd`.
- [ ] Copy the canonical file to `platform/core/pkg/rates/rate-cards.json`
      (the embedded copy; never hand-edit only the duplicate).
- [ ] Run `go test ./pkg/rates ./pkg/llm ./pkg/api` and boot `kubonad`; confirm
      no stale-card refusal and that `/api/cost/summary` labels estimates.
- [ ] Record the vendor evidence and the update in the operator log/wiki.
- [ ] Re-check BYOK semantics: the provider bills the user's key; Kubona's
      estimate is a guardrail, never a charge or a QPU-cap debit.

If the vendor evidence is unavailable by 2026-11-11, the fail-closed startup
check refuses to boot. That is intentional; do not disable it or invent rates.
