---
name: kubona-governance
description: Manage Kubona session budgets, cost estimates, Ed25519 spend mandates, MRTR approvals, stage autonomy policies, receipts/undo, and supervised control-plane loops.
---

# kubona-governance: Budgets, Mandates, Autonomy & Loops

Version 2026-11-20. Use this skill when opening sessions, checking spend
ceilings, handling approvals, respecting operator halts, or supervising
long-running agent loops.

## Budget & Spend Governance
1. **Every spend is ceilinged:** Open sessions with `create_session` and a
   positive finite budget ceiling. Zero, negative, `NaN`, and `Inf` fail closed
   (`platform/core/pkg/session/session.go:15`, `platform/kubona/cost/payment.py:20`).
2. **Estimate before dispatch:** Call `estimate_cost` and `session_burn_rate`
   before submitting paid jobs. `estimate_cost` carries `heuristic: true` —
   estimates inform, ceilings bind.
3. **Staged approvals & x402 quotes:**
   - Review workflow: `request_approval` → `hold_approval` → `approve_approval`
     or `reject_approval`.
   - x402 payment rail: `quote_payment` binds the amount to the session;
     `verify_payment` settles strictly against the stored quote.
4. **Ed25519 spend mandates:** Verification order is strictly **tenant → job →
   expiry → budget → signature** (`platform/kubona/cost/mandate.py:37`).
   A shape-valid mandate is **not** admitted until a `mandate_verified_client`
   receipt is recorded in the ledger. Verify offline + admission with:
   ```sh
   platform/.venv/bin/python -m kubona.run mandate-check --mandate M.json --tenant t --job-hash h --cost 10
   ```

## Session Autonomy & Operator Oversight (`AG-17..24`)
Always inspect `get_session_autonomy` (`GET /api/sessions/{sessionId}/autonomy`)
before autonomous actions:
- **`agentic_enabled: false`** → returns `403` `agentic_disabled`. Pause agent
  execution; the manual experiment rail stays open for the operator.
- **`agentic_locked: true`** → returns `409` `agentic_locked` (operator halt).
  Only an operator can unlock it; never retry in a loop.
- **Stage policies (`suggest` / `approve` / `notify` / `act`):**
  - `suggest` returns `403` `autonomy_suggest_only` on execute.
  - High-risk plans or plans exceeding the act budget return `409`
    `risk_approval_required` in **every** mode; poll
    `GET /api/sessions/{sessionId}/approvals` until decided.
- **Action receipts & undo:** Reversible decisions (agentic mode flip, unrun
  plan decision, deferral) appear at `GET /api/sessions/{sessionId}/receipts`
  and can be reversed via `POST /api/sessions/{sessionId}/receipts/{receiptId}/undo`
  (irreversible actions return `409` `undo_not_reversible`).

## Supervised Control-Plane Loops (`control` server)
Long-running agent loops MUST announce themselves and obey operator overrides:
1. `register_loop` — binds loop to `session_id` + tenant.
2. `step_loop` — report progress each iteration; **stop immediately** if
   `should_run` is `false`.
3. `list_loops` / `loop_status` — inspect active, paused, or `sleeping` loops.
4. Operator overrides: `pause_loop`, `resume_loop`, `kill_loop` (require
   `control:override` scope).

## Traps
- `ceiling_exceeded` (`403`) or `budget_exceeded` (`200` `valid:false`) means
  shrink the workload or ask the operator to raise the ceiling — never split
  a job to evade a ceiling.
- `proposal_stale` (`409`) means campaign state or calibration moved since
  pricing; re-read the campaign and mint a fresh proposal.
- `close_session` releases holds and is terminal; never submit to a closed
  session (`ghost_session` / `not_found`).
