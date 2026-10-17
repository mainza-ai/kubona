---
name: kubona
description: Operate the Kubona quantum-computing platform (tribunals, mandates, ledger, providers) with plain local commands. Covers first-move inventory, the fastest path per target, and the fail-closed traps. Use to answer how a job/tribunal/mandate stands, with evidence.
---

# kubona: fast path

Version 2026-10-17. Guidance only, never guarantee: every trust
property (mandates, budgets, admissions, ledger appends) is enforced in
code regardless of what this file claims. A stale line here costs turns,
never safety. Run from the repo root unless noted.

## Rules
- Never touch live state while exploring. Override the state root for
  experiments: `KUBONA_STATE_DIR=$(mktemp -d)` — the daemon's
  `~/.kubona` is not your scratch dir.
- Ledger tail first. Before reading code, ask what already happened:
  Python rail `tail -n 20 ~/.kubona/ledger.jsonl`, Go rail
  `sqlite3 ~/.kubona/kubona.db "SELECT ... FROM ledger_events ..."`.
  History beats speculation.
- `rg` skips hidden and ignored files. Search extracted/generated trees
  with `rg -uu` (`platform/.venv`, `platform/console/node_modules` hide
  real code the same way bundlers hide theirs).
- Never `cat` an append-only file. Slice it: `tail -n 50 FILE`,
  `rg -o '.{0,200}NEEDLE.{0,200}' FILE | head`. `jobs.jsonl` and the
  event tables only grow — read the tail.
- Batch independent probes into ONE shell call with `;` and
  `echo '--- name'` markers. Cap every output with `head`.
- Answer from evidence. Mark each claim as **seen** (cite file:offset,
  a ledger entry hash, or a record id) or **inferred**. A fact you did
  not find is **unknown**, not "absent". A template fallback is not an
  observation.
- Turns cost more than bytes. Aim for 2–4 tool calls: one batched
  inventory call, one or two targeted slices, then answer. Stop when
  every item asked has evidence. Do not re-verify what you already saw.

## Router
| Target (detect) | Fastest path |
|---|---|
| Job full state | Python tail for `workflow_*` rows by `job_id`, then Go `SELECT id,status,reserved_tenant,reserved_amount FROM jobs WHERE id='<id>'` on `~/.kubona/kubona.db`. Params live in `params_json`; results in `result_json`. |
| Mandate validity (offline shape check) | `verify_mandate` snippet in First moves. Shape-valid is NOT admitted: admission mints a ledger receipt whose `entry_hash` the job cites as `mandate_ref` — check `mandate_verified_client` events for the receipt. |
| Provider inventory | `list_providers()` — 8 providers; `local-simulator` and `qiskit-aer-simulator` need no credentials. Anything else needs a vendor key you do not have. |
| Tribunal transcript | `SELECT turn_index,role,verdict FROM ledger_events WHERE session_id='<sid>' AND event_type='tribunal_turn' ORDER BY id`. Turn content is sealed per turn; `authored=0` means template fallback, never evidence. |
| Session spend | `session_burn_rate` (MCP) or `SELECT id,current_spend_usd,spend_limit_usd FROM sessions WHERE id='<sid>'` on `~/.kubona/kubona.db`. |
| Ledger tail, Python rail | `tail -n 20 ~/.kubona/ledger.jsonl \| jq -c '{kind, record_id, label}'`. Kinds are a closed taxonomy — a kind you do not recognize is a typo, not a discovery. |
| Ledger tail, Go rail | `sqlite3 ~/.kubona/kubona.db "SELECT event_type,COUNT(*) FROM ledger_events GROUP BY 1 ORDER BY 2 DESC LIMIT 10;"` |
| Cost estimate before dispatch | `estimate_cost` with the exact QASM, provider, and shots. Estimates are heuristic (`heuristic: true` on the record) — ceilings bind, estimates inform. |

## First move: one call each, paste and run
Config and state sanity (from `platform/` with the venv on `PATH`):
```sh
.venv/bin/python -m kubona.run doctor
```
Python-rail tail, last 20 records as kinds:
```sh
tail -n 20 ~/.kubona/ledger.jsonl | jq -c '{kind, record_id, label}'
```
Tribunal turns for one session (ordered, with verdicts):
```sh
sqlite3 ~/.kubona/kubona.db "SELECT payload FROM ledger_events WHERE session_id='<sid>' AND event_type='tribunal_turn' ORDER BY id;" | jq -c '{turn_index, role, verdict}'
```
Provider inventory, no credentials needed:
```sh
.venv/bin/python -c "from kubona.providers import list_providers; print(list_providers())"
```
Offline mandate shape check (ephemeral key; proves binding/expiry/signature logic, NOT ledger admission):
```sh
.venv/bin/python -c "
import secrets
from kubona.cost.mandate import create_mandate, verify_mandate, utc_now_plus
m = create_mandate(secrets.token_bytes(32), 't', 'job-sha256-test', 25.0, utc_now_plus(3600))
print(verify_mandate(m, 't', 'job-sha256-test', 10.0))
print(verify_mandate(m, 't', 'job-sha256-test', 99.0))"
```

## Traps
- Fail-closed arithmetic: NaN, Inf, and negatives refuse everywhere — amounts, budgets, noise rates. `platform/core/pkg/session/session.go:15`, `platform/kubona/cost/mandate.py:78`, `platform/kubona/cost/payment.py:20`. A zero or missing number never means "free" or "unset"; it refuses.
- Cancel graph is terminal-immutable: `QUEUED` never executes on cancel, `RUNNING` is fenced, terminal states replay instead of transitioning. `platform/kubona/jobs.py:43`, `platform/core/pkg/jobs/jobs.go:639`. Never re-drive a settled job.
- Mandate binding order is tenant, job, expiry, budget, signature — verified in that order, empty expectations refuse. `platform/kubona/cost/mandate.py:37`, `platform/core/pkg/jobs/admission.go:220`. A receipt for another program refuses even when tenant and budget cover.
- Ledger kinds are a closed taxonomy: unregistered `event_type` refuses at the append choke, both rails. `platform/core/pkg/ledger/events.go:15`, `platform/contracts/event-types.json`. Unknown kinds are typos, not discoveries.
- Idempotency replays byte-identical receipts; same ID with different content 409s (`request_conflict` — mint a new id, never retry the same bytes). `platform/core/pkg/api/idempotency.go:9`. Ghost sessions 404 before any key is stored.
- The dispatcher does not coerce: unknown server/tool is `KeyError`, arguments are never guessed. `platform/kubona/mcp/gateway.py:105`. A misspelled tool name is a refusal, not a fuzzy match.
- Provenance reads are tenant-scoped: `requester=None` sees public rows only, never another tenant's. `platform/kubona/provenance/ledger.py:220`. Absence of rows is not evidence of absence — query the right tenant first.

## Speed patterns
- Tail before code: the answer is usually in the last 20 ledger rows, not in the source.
- Diff digests before rows: compare section/content hashes before reading payloads (migration manifests, idempotency receipts).
- Session-scope every query: unscoped reads across tenants refuse by design — always carry the session or tenant.
- From the native/JS boundary inward applies here as: from the ledger event inward to the handler, then to the test that pins it.
