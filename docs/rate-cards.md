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

## Updating

1. Edit `platform/contracts/rate-cards.json` (source, effective date, rates).
2. Copy it to `platform/core/pkg/rates/rate-cards.json`.
3. Run `go test ./pkg/rates ./pkg/llm ./pkg/api`.
4. Raise `version`; do not shorten `stale_after_days` to make a red gate pass.

Vendor reconciliation (an actual provider invoice/price sheet with
`heuristic:false`) remains a release-time task. The current card makes the
heuristic explicit and stops it from silently outliving its window.
