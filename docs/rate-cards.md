# Rate cards (runtime, vendor-derived)

Kubona prices cloud LLM calls and the Atlas lens from a **rate card**, never
from model IDs hardcoded in Go. The embedded card is a local/dev bootstrap;
production cloud pricing requires an operator-supplied runtime card.

- Canonical bootstrap: `platform/contracts/rate-cards.json`
- Embedded copy: `platform/core/pkg/rates/rate-cards.json` (parity-tested)
- Runtime source: `KUBONA_RATE_CARD_PATH` (absolute file) or
  `KUBONA_RATE_CARD_URL` (HTTPS; loopback HTTP allowed). They are mutually
  exclusive; neither set keeps the embedded bootstrap.
- The daemon reloads a configured file/URL every 5 minutes. A failed refresh
  keeps the last good card, which then becomes stale on its own schedule.
  A stale card is a hard refusal, never an estimate that silently becomes a
  bill.

## Current bootstrap snapshot (2026-10-30)

The embedded card was refreshed from current vendor lists; it is **not** a
substitute for the operator's runtime card. Prices are USD per 1M tokens
(the engine converts to per-1K):

| Provider / model | Input | Output | Source | Confidence |
|---|---:|---:|---|---|
| `opencode/deepseek-v4.1-flash` | $0.30 | $1.20 | OpenCode Go peak | vendor |
| `opencode/deepseek-v4-pro` | $1.32 | $3.96 | OpenCode Go peak | vendor |
| `nvidia/nemotron-3.5-super-vl-120b-a12b` | $0.06 | $0.35 | CostGoat/Nemotron | aggregator |
| `openai/gpt-6-astra` | $10.00 | $50.00 | OpenAI pricing | vendor |
| `openai/gpt-6.1-sol` | $2.00 | $10.00 | OpenAI pricing | vendor |
| `openai/gpt-6.1-terra` | $1.00 | $5.00 | OpenAI pricing | vendor |
| `openai/gpt-6-luna` | $0.10 | $0.50 | OpenAI pricing | vendor |
| `anthropic/claude-opus-5-5` | $4.00 | $20.00 | Anthropic list 2026-10-22 | vendor |
| `anthropic/claude-sonnet-5-5` | $2.00 | $10.00 | Anthropic pricing | vendor |
| `anthropic/claude-haiku-5-5` | $0.10 | $0.50 | OpenCode/Anthropic list | aggregator |
| `atlas-lens` (`marble-1.1`) | 1,500 credits | ≈ $1.20/dispatch | World Labs API pricing | vendor |

Retired/legacy entries (`deepseek-chat`, Claude 3.5/3.7, GPT-4o, o3, old
Meta Llama rows, etc.) were removed. The provider's live `/models` endpoint is
the authority for availability, not this table.

DeepSeek/OpenCode use **peak** rates as the ceiling; off-peak is half. The
provider dashboard/invoice is authoritative for actual billing.

## Dynamic audit / capture of what is used

`kubonad rates-audit --db /abs/path/kubona.db` reads the persisted role
assignments and, for every non-local role:

1. Verifies the provider is enabled.
2. Fetches the provider's live `/models` and checks the assigned model still
   exists (a retired model fails the audit).
3. Looks up a **provider-qualified** rate in the active card; an unpriced
   model fails the audit.
4. Checks the active card is not stale.

Exit 0 is clean; 1 means migrate the model and/or install a current runtime
card. `--json` emits the machine-readable report. Local providers
(Strata/oMLX/Ollama/LM Studio) are free and skipped.

This is the "capture what is actually used" rail: only assigned cloud models
are audited, and no model list or price is compiled into the daemon.

## Stale and production behavior

- `kubonad` loads/validates the active card at boot and refuses a stale or
  invalid configured source.
- Cloud pricing checks at call time, so a long-running daemon cannot keep
  pricing from an expired card.
- In `KUBONA_ENV=production`, cloud LLM pricing **refuses the embedded
  bootstrap** and the Atlas lens refuses without a runtime card. Local
  providers remain free and unaffected. This is deliberate: production rates
  must be operator-supplied data, not a repo snapshot.
- BYOK: the provider bills the operator's key. The card produces a local
  estimate for `KUBONA_CODESIGN_RUN_BUDGET_USD` / `KUBONA_LLM_BUDGET_USD`;
  it is not a Kubona charge and never debits the QPU session/tenant cap.

## Install / refresh a runtime card

```bash
# Preferred: host a vendor-derived JSON card and point the daemon at it.
export KUBONA_RATE_CARD_URL=https://internal.example/kubona/rate-cards.json
# or a mounted file:
export KUBONA_RATE_CARD_PATH=/etc/kubona/rate-cards.json

kubonad rates-audit --db /home/kubona/.kubona/kubona.db
```

For Docker, mount the file read-only and set the container path (for example
`-v /host/rate-cards.json:/etc/kubona/rate-cards.json:ro` with
`KUBONA_RATE_CARD_PATH=/etc/kubona/rate-cards.json`), or use a URL. The
compose file passes both env vars through.

## Reconciliation checklist (before 2026-11-11)

This is an operator task. The embedded bootstrap is not vendor proof; the
runtime card is.

- [ ] Inventory the cloud models actually assigned (console LLM Settings /
      `kubonad rates-audit`).
- [ ] Obtain current vendor list/invoice rates: input/output USD per 1M,
      model ID, region, discounts. Record URL + retrieval date.
- [ ] Obtain World Labs Marble pricing (1,500 credits for `marble-1.1`;
      credits are $1.00 per 1,250 → $1.20 per generation).
- [ ] Build the runtime JSON with `source`, `effective_date`,
      `stale_after_days: 30`, provider-qualified `models.*` entries, aliases,
      `status`, and per-row `source`/`confidence`.
- [ ] Install it via `KUBONA_RATE_CARD_PATH`/`URL`; do not edit only the
      embedded copy.
- [ ] Run `kubonad rates-audit`; confirm every assigned model is live and
      priced, and the card is not stale.
- [ ] Run `go test ./pkg/rates ./pkg/llm ./pkg/api` and boot `kubonad`.
- [ ] Record the vendor evidence and the update in the operator log/wiki.

If no runtime card is installed by 2026-11-11, cloud pricing/Atlas dispatch
refuses; local providers still work. Do not disable the gate or invent rates.
