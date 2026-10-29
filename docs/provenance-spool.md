# Provenance spool

The ledger is the receipt of record. Some actions have an irreversible side
effect before the receipt can be written (the Atlas lens dispatch is the
current case). `Ledger.AppendEventWithSpool` handles that window:

1. It tries `AppendEvent`.
2. On failure, it writes the exact event to
   `<state>/provenance-spool/provenance-*.json` atomically (temp + fsync +
   rename).
3. It returns `sealed=false`, and the API reports
   `provenance_status: "pending"` rather than claiming a sealed receipt.
4. If the ledger **and** the spool both fail, the API returns
   `provenance_unavailable` with the operation id. Nothing is silently
   reported as recorded.

Money-critical append sites use the spooled helper: `llm_spend_estimated`,
`tenant_spend_reserved`, `session_spend_reserved`, `job_queued`, `job_settled`,
`settlement_skipped_unknown_usage`, and `loop_spend_released`. The money
movement itself is already atomic in SQLite; the spool protects the receipt,
not the balance.

On the next writer start, `replayProvenanceSpool` appends each spooled event
and removes the file only after the ledger accepts it. A malformed or
unreplayable file is left in place and logged.

## Operator notes

- Back up `<state>/provenance-spool` together with the ledger. It is on the
  same disk and is not an off-host backup.
- Do not delete spool files by hand. If replay keeps failing, fix the ledger
  first, then restart the daemon; the replay is idempotent at the file level
  (a file is removed only after a successful append).
- A `pending` response means the science/dispatch result is real but the
  receipt is not yet sealed. Do not present it as sealed in downstream
  reports.
