# Vendor evidence and reconciliation — 2026-10-31

## Runtime rate card

- Active local-private runtime card: `/contracts/rate-cards.json`.
- Source: `vendor-list-prices-2026-10-30`; Atlas `marble-1.1` is
  1,500 credits ≈ `$1.20` per dispatch.
- This is a reviewed **list-price** card, not an invoice or discounted
  contract. It is functional for local-private estimates and production
  pricing gates.
- **Still operator-supplied:** vendor invoice/discount rates, if any. Replace
  the runtime card before quoting an actual cloud bill; do not derive prices
  from model IDs or the embedded bootstrap.

## New paid validation evidence

- Atlas operation `8edf2aaa6d6144468e7961534b6c3555`: `SUCCEEDED`, one image,
  `$1.20` reserved; 1,500 credits consumed.
- IBM vendor job `dbj4o0gnceqs73aj8n9g`: `ibm_marrakesh`, `DONE`, 100 shots,
  2 QPU seconds, counts `00:61`, `01:4`, `11:35`.
- Local job `09912676ae2c436c`: completed, local usage 2 s, settlement
  `$3.20`; session/tenant spend `$3.20`.
- Final vendor evidence file (local only): `/tmp/kubona-paid-e2e/final2-status.json`.

## Historical IBM row

- Vendor job `dbha28pq260s73busu70`: `DONE`, 2 QPU seconds.
- Local job `39b5e6b832cd4ae3`: cancelled, settled `$0.00`, estimate
  `$8.000568`.
- Current IBM instance: Open Plan; the repo cannot prove a paid invoice.
- **Disposition:** preserve the row; do not rewrite it. The vendor status and
  usage are now independently re-queried and recorded here.
- If the operator has a paid invoice for this run, the row needs an approved
  monetary correction by the operator. If it was Open Plan/free, `$0` is the
  correct monetary result and only the local status/evidence is stale.

## Required operator decision

1. Attach invoice/discount evidence for the runtime rate card, or explicitly
   accept list prices for local-private estimates.
2. Decide the historical row: free/Open Plan (`$0` stands) or paid (approved
   correction amount and evidence).

No code rewrite was performed for the historical row.
