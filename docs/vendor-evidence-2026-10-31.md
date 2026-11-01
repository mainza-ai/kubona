# Vendor evidence and reconciliation — 2026-10-31

## Runtime rate card

- Active local-private runtime card: `/contracts/rate-cards.json`.
- Source: `vendor-list-prices-2026-10-30`; Atlas `marble-1.1` is
  1,500 credits ≈ `$1.20` per dispatch.
- This is a reviewed **list-price** card, not an invoice or discounted
  contract. It is functional for local-private estimates and production
  pricing gates. List prices are sufficient for local-private list estimates;
  invoice/discount replacement is required only before quoting a discounted
  actual cloud bill.
- Atlas credit balance can be read from the documented
  `GET /marble/v1/credits` endpoint. The current `WLT_API_KEY` is an Atlas
  beta key and returns 401 against the Marble credits endpoint, so the
  platform dashboard remains the balance authority for that key.

## New paid validation evidence

- Atlas operation `8edf2aaa6d6144468e7961534b6c3555`: `SUCCEEDED`, one image,
  `$1.20` reserved; 1,500 credits consumed.
- IBM vendor job `dbj4o0gnceqs73aj8n9g`: `ibm_marrakesh`, `DONE`, 100 shots,
  2 QPU seconds, counts `00:61`, `01:4`, `11:35`.
- Local job `09912676ae2c436c`: completed, local usage 2 s. The authenticated
  instance is `plan=open`, `pricing_type=free`; actual charge is `$0.00`. The
  pre-fix `$3.20` local settlement was a list-price estimate and is not a
  vendor charge. The provider now settles free-plan usage at zero.
- Final vendor evidence file (local only): `/tmp/kubona-paid-e2e/final2-status.json`.

## Historical IBM row — resolved by vendor API

- Vendor job `dbha28pq260s73busu70`: `DONE`, 2 QPU seconds.
- Local job `39b5e6b832cd4ae3`: cancelled, settled `$0.00`, estimate
  `$8.000568`.
- Authenticated runtime API reports the instance:
  - `plan: open`
  - `pricing_type: free`
- **Disposition:** the vendor plan is **free**, so the local `$0.00`
  monetary result is correct. No invoice or ledger rewrite is required. The
  local cancelled status and stale estimate remain historical metadata; the
  vendor evidence is attached here.
- Code fix: the Qiskit provider now detects `open`/`free` plans and reports
  `actual_usd: 0` for measured usage, so future free-plan runs do not
  overcharge the local ledger with list-price dollars.

## Required operator decision

1. **Resolved for local-private:** accept the reviewed vendor list prices.
   Invoice/discount evidence is needed only for a discounted actual-cost
   claim, not for local estimates.
2. **Resolved by vendor API:** the historical IBM instance is Open Plan/free;
   its `$0.00` settlement stands. No historical ledger rewrite is performed.

Remaining operator fact: the Atlas beta key’s credit balance is dashboard-only
with the current key.
