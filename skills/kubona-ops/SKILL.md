---
name: kubona-ops
description: Operate, inspect, and troubleshoot a local-private Kubona deployment (Docker Compose, kubona.run doctor, SQLite/JSONL ledger forensics, sleeping loop settlement, backups, and error-catalog recovery).
---

# kubona-ops: Local-Private Stack & Forensics

Version 2026-11-20. Use this skill when checking stack health, diagnosing
failed jobs or blocked deletes, inspecting the SQLite/JSONL ledgers, or
running backup/restore procedures.

## Stack Health & Preflight
```sh
# 1. Local Python runtime & ledger sanity
platform/.venv/bin/python -m kubona.run doctor

# 2. Live production Compose stack (loopback-only: :8080, :3000, :8123)
curl -sf http://127.0.0.1:8080/health && echo
curl -sf http://127.0.0.1:8080/api/ready && echo
curl -sf http://127.0.0.1:8123/health && echo
```

## Ledger Forensics (Tail First, Never `cat`)
- **Python rail (last 20 records):**
  ```sh
  tail -n 20 ~/.kubona/ledger.jsonl | jq -c '{kind, record_id, label}'
  ```
- **Go SQLite rail (event distribution):**
  ```sh
  sqlite3 ~/.kubona/kubona.db "SELECT event_type, COUNT(*) FROM ledger_events GROUP BY 1 ORDER BY 2 DESC LIMIT 10;"
  ```
- **Job state by ID:**
  ```sh
  sqlite3 ~/.kubona/kubona.db "SELECT id, status, reserved_tenant, reserved_amount FROM jobs WHERE id='<job_id>';"
  ```
- **Tribunal transcript (`authored=0` is template fallback, never evidence):**
  ```sh
  sqlite3 ~/.kubona/kubona.db "SELECT payload FROM ledger_events WHERE session_id='<sid>' AND event_type='tribunal_turn' ORDER BY id;" | jq -c '{turn_index, role, verdict}'
  ```

## High-Frequency Failure Recovery Playbook
Always consult `kubona://contracts/error-catalog` (`platform/contracts/error-catalog.csv`)
for the full closed vocabulary. Common incidents:

| Error Code | HTTP | Retryable? | Action |
|---|---|---|---|
| `delete_blocked` | 409 | No | Check `list_loops` / `GET /api/autoresearch/loops` for `running`, `paused`, or restart-orphan `sleeping` loops. Stop them (`POST /api/autoresearch/loops/{id}/stop` or `kill_loop`), then retry delete. |
| `publication_evidence_present` | 409 | No | Export manuscripts first (`kubonad manuscript-export`), then pass `?acknowledge_publication_loss=1`. |
| `request_conflict` | 409 | No | Same `request_id` was sent with a different payload. Mint a fresh `request_id`; never retry the same ID. |
| `request_in_progress` | 409 | Yes | Poll the same `request_id` until settled. |
| `revision_conflict` / `precondition_required` | 409 / 428 | No | Re-read the record and send the current `expected_revision`. |
| `bridge_unavailable` | 502 | Yes | Check `bridge` container health and `unix:///run/kubona/bridge.sock`; sim-only paths stay open. |
| `storage_blocked` | 503 | Yes | Free disk space above the resume threshold; reads stay available. |
| `agentic_disabled` / `agentic_locked` | 403 / 409 | No | Operator must toggle Auto Agent on or clear the halt lock in the console. |

## Backup & Restore
- Pre-migration snapshots are taken automatically before ledger schema bumps.
- Restore and verify via `kubonad restore` (see `docs/local-deployment.md`).
- Never edit `raw/` or mutate `~/.kubona/kubona.db` directly; use API/CLI rails so hash chains remain intact.
