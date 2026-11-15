---
type: concept
name: Run-Scoped Allowance — Threat Model & Schema Review (C11)
date: 2026-11-14
updated: 2026-11-14
tags: [security, governance, spend, mandates, hitl, threat-model]
sources: [source-gap-analysis-plan]
status: adopted
---

# Run-Scoped Allowance — Threat Model & Schema Review

> **Status: reviewed and adopted 2026-11-14 (recommended answers).** The
> server rail is implemented against this argument: schema, signed mint,
> atomic draw, taint exclusion, calibration/rate-card suspension, air-gap
> suspension, revocation, reader endpoint, and the test matrix. The console
> control ("Allow for this run") is also built: it signs the canonical scope
> with local custody, lists/revokes allowances, and launches under one
> (skipping the per-job mandate; admission still enforces every rule). See
> [[runtime-honesty-and-commit-policy]] §3 for the adopted shape and
> [[spend-mandates-and-delegation]] / [[cost-governance]] for the per-job rail
> this extends.

## 1 · What changes, and what does not

**Default unchanged.** Every hardware job still requires a per-job approval
(AP2 mandate bound to session + exact QASM + budget + expiry) unless a
run-scoped allowance is explicitly minted for that session.

**What the allowance removes.** Inside its bounds, the human click per job
disappears. One signature authorizes up to `max_total_usd` across up to
`max_jobs` hardware jobs on an allow-listed backend, until `expires_at` or an
automatic suspension.

**What it never removes.** Per-job admission still runs in full: fidelity,
jurisdiction/export, mandate/allowance scope, budget reservation, idempotency,
and the taint exclusion below. The allowance changes *who approved*, not *what
is admissible*.

## 2 · Assets and actors

- **Assets:** the tenant's money; the lab's hardware budget; the integrity of
  the approval record; the ability to stop spending quickly.
- **Actor A (operator/reviewer):** holds the signing key (non-extractable
  WebCrypto custody, Q-56). Mints and revokes allowances.
- **Actor B (agent):** proposes and launches jobs; can be influenced by
  untrusted tool/literature bytes.
- **Actor C (stolen session cookie):** can read the session, may attempt to
  spend; not a signer.
- **Actor D (network attacker):** can replay or reorder requests.
- **Non-actor (explicit):** a malicious tenant admin signing its own allowance
  is out of scope — it is the tenant's own money and its own key; cross-tenant
  isolation is enforced elsewhere.

## 3 · Threat tree and invariants

| # | Threat | Invariant that defeats it |
|---|---|---|
| T1 | Stolen cookie spends without a signer | Minting requires an Ed25519 envelope signed by the tenant admin/reviewer key. A cookie alone cannot mint. |
| T2 | Allowance copied to another session/lab | Scope digest binds `{tenant, session, campaign}`; admission recomputes and refuses any mismatch. |
| T3 | Backend pivot to pricier hardware | Closed backend allow-list in the envelope; admission normalizes the requested backend and requires an exact member. |
| T4 | Job-type pivot (e.g. a cheap type used to smuggle a costly one) | Closed job-type allow-list; admission checks the job type before pricing. |
| T5 | One job exceeds the per-job cap | `max_per_job_usd` checked before reservation. |
| T6 | Aggregate overspend under concurrency | Draw is an atomic CAS on `remaining_micro_usd` and `jobs_used` inside the reservation transaction; the loser refuses. |
| T7 | Replay after expiry | `expires_at` checked on every admission; expired rows are never drawn. |
| T8 | Forgotten allowance spends for weeks | Hard max TTL 24 h; default 8 h; expiry is server-checked, never client-reported. |
| T9 | Prompt injection mints/dispatches through the allowance | A turn that consumed untrusted content is **not covered**: the server marks the minted proposal `basis_tainted=true`; admission refuses an allowance draw for a tainted proposal and requires a fresh per-job approval. |
| T10 | Calibration/backend moves after approval | Any calibration digest change or backend config change suspends the allowance; new admissions fall back to per-job approval. |
| T11 | Air-gap engaged mid-run | Engaging air-gap suspends all active allowances (no new hardware admissions). |
| T12 | Allowance store unreadable | Fail closed: no allowance draw; the per-job approval path remains. Never "assume active". |
| T13 | Revocation races with an in-flight launch | Revoke sets status `revoked`; in-flight jobs that already drew finish and settle; nothing new is admitted. |
| T14 | Spending reported as lower than actual | Draws record the reserved amount; settlement reconciles measured usage exactly as per-job mandates do. An unmeasured draw keeps its estimate loudly (existing `settlement_skipped` class). |
| T15 | Reader learns other sessions' allowances | Remaining-allowance reads are session-scoped like every other session read; minting/revoking are signed writes. |

## 4 · Schema (proposed)

```sql
CREATE TABLE run_allowances (
  id                    TEXT PRIMARY KEY,
  tenant_id             TEXT NOT NULL,
  session_id            TEXT NOT NULL,
  campaign_id           TEXT NOT NULL,          -- scope component (session or campaign digest)
  scope_digest          TEXT NOT NULL,          -- sha256 over the canonical scope envelope
  backends_json         TEXT NOT NULL,          -- closed backend ids
  job_types_json        TEXT NOT NULL,          -- closed job types
  max_per_job_micro_usd INTEGER NOT NULL,
  max_total_micro_usd   INTEGER NOT NULL,
  remaining_micro_usd   INTEGER NOT NULL,
  max_jobs              INTEGER NOT NULL,
  jobs_used             INTEGER NOT NULL DEFAULT 0,
  shots_cap             INTEGER NOT NULL,
  status                TEXT NOT NULL,          -- active | suspended | exhausted | expired | revoked
  suspended_reason      TEXT NOT NULL DEFAULT '',
  issued_at             DATETIME NOT NULL,
  expires_at            DATETIME NOT NULL,
  nonce                 TEXT NOT NULL,
  signer_pubkey_hex     TEXT NOT NULL,
  signature_hex         TEXT NOT NULL,
  UNIQUE(nonce)
);

CREATE TABLE run_allowance_draws (
  allowance_id     TEXT NOT NULL,
  job_id           TEXT NOT NULL,
  request_id       TEXT NOT NULL DEFAULT '',
  amount_micro_usd INTEGER NOT NULL,
  created_at       DATETIME NOT NULL,
  PRIMARY KEY (allowance_id, job_id),
  FOREIGN KEY (allowance_id) REFERENCES run_allowances(id)
);
```

Ledger events: `run_allowance_minted`, `run_allowance_drawn`,
`run_allowance_suspended`, `run_allowance_revoked`. Draw events cite the
remaining amount after the draw. Mint/revoke are signed writes; draws are
writer-only server events.

## 5 · Enforcement points (Go)

1. **Mint** (`POST /api/run-allowances`): verify signature over the canonical
   scope bytes; require tenant-admin or reviewer key; clamp TTL to 24 h;
   require `max_total_usd > 0`, `max_per_job_usd <= max_total_usd`,
   `max_jobs >= 1`, non-empty backend/type lists; persist `active`; emit
   `run_allowance_minted`.
2. **Admission** (`SubmitJob` path): if params carry `run_allowance_id`,
   admission loads the row, checks status/expiry/tenant/session/campaign,
   backend + job type membership, shots cap, per-job cap, then in one
   transaction inserts the draw, CAS-updates `remaining` and `jobs_used`, and
   reserves. Any failure refuses before a job row exists.
3. **Taint exclusion:** the console mints proposals server-side (B1). A
   proposal minted on a turn that consumed observations carries
   `basis_tainted=true`; admission refuses an allowance draw for it. This is
   server-derived state, not a client flag.
4. **Suspension:** revoke, air-gap on, calibration/backend change, or store
   error sets `suspended`/`revoked`; admission then requires per-job
   approval. No silent fallback that spends.
5. **Settlement:** unchanged; measured usage settles, unmeasured usage stays
   loud. Draw rows are the allowance's audit trail.

## 6 · Failure semantics

- Allowance present but invalid → **refuse the job** and say why; never fall
  back to spending without approval.
- Allowance absent/expired/suspended → **per-job approval required**; the
  launch refuses if no mandate is presented.
- Draw CAS loses → refuse with `remaining`/`jobs_used` in the detail.
- Store unreadable → refuse allowance draws; per-job path unaffected.

## 7 · Review questions — answered 2026-11-14 (recommended)

**Decisions:** (1) reviewer key with tenant-admin fallback, signer recorded
and returned; (2) UI prefill last-used values, server enforces 1 min–24 h and
positive caps; (3) explicit backend only — exactly one metered backend, no
simulator auto-inclusion; (4) price-card version change suspends the
allowance (implemented at admission); (5) tainted turns fall back to per-job
approval (implemented via the server-minted proposal's `basis_tainted`);
(6) allowances do **not** block session deletion.

Original questions:

1. **Key:** tenant admin key or the reviewer key — or either, with the signer
   recorded on the row? (Recommend: reviewer key with admin fallback, signer
   always recorded and shown.)
2. **Default caps:** what defaults should the UI prefill (total, per job,
   jobs, shots, TTL)? (Recommend: last-used values, never above tenant cap.)
3. **Backend list:** should a simulator be auto-included as a downgrade, or
   must it be listed explicitly? (Recommend explicit only; downgrade already
   exists elsewhere.)
4. **Suspension on price-sheet change:** treat a rate-card version change as
   a calibration-class suspension? (Recommend yes.)
5. **Tainted turns:** is "fall back to per-job approval" the right UX, or
   should a tainted turn be unable to launch hardware at all until the
   reviewer acts? (Adopted text says fall back per-job.)
6. **Deletion:** does an allowance block session deletion like an outstanding
   reservation? (Recommend: no — it is authority, not spend; revocation is
   enough.)

## 8 · Test matrix (must be green before landing)

- Signature: valid/invalid/foreign key, tampered scope, replayed nonce.
- Scope: session, campaign, tenant, backend, job type, shots, per-job cap.
- Concurrent draws: N parallel admissions, exactly the cap succeeds.
- Expiry: just-before/just-after; TTL clamp.
- Taint: proposal marked tainted refuses a draw; per-job path still works.
- Suspension: revoke, air-gap, calibration change, store failure.
- Settlement: measured draw settles, unmeasured stays loud, draw audit rows.
- Reader/writer: session readers see remaining; only signed writes mutate.

## 9 · Non-goals

- No server-held private keys; no auto-renewal; no allowance for non-hardware
  job types; no cross-session sharing; no LLM authority to mint.

Implementation does not land until this page is reviewed against the final
code and the seven questions above are answered. Until then the default
per-job approval remains the only hardware path.
