---
type: concept
name: Runtime Honesty & Commit Policy (outcome-unknown jobs, settings tiers, run allowances, palette provenance)
date: 2026-11-12
updated: 2026-11-12
tags: [governance, settlement, jobs, settings, mandates, ux, licence, decision]
sources: [source-gap-analysis-plan]
status: adopted
---

# Runtime Honesty & Commit Policy

Four operator decisions taken 2026-11-12 after a comparative study of an
external agent-work surface, `zephium-browser/Zephium` @ `1c4846c`
(<https://github.com/zephium-browser/Zephium>, **MPL-2.0**). Per
[[integration-topology-and-licence-discipline]] that repository is
**reference only**: patterns are re-implemented, no file is copied. Each
decision below is written in the design register; what the prototype does
today is marked as such. Extends [[cost-governance]],
[[spend-mandates-and-delegation]] and Q-51.

## 1 · Palette provenance (Q-58, decided and executed)

The prototype console's token literals had been taken over from the reference
repository (121 of 151 literal colour/alpha values identical, plus the radius
and type ladders). Decision: **re-derive**, do not relicense.

- The prototype now derives every literal colour, alpha, shadow, radius and
  type size in `scripts/console-palette.mjs` from Kubona-owned parameters
  (OKLCH seeds on neutral hue 284, a geometric alpha ladder
  `A[n] = 0.044·1.42^(n-1)`, radius `= even(height × 0.3)`, type
  `13·1.2^(k/2)` px). Outputs `palette.css` / `palette.generated.ts` are
  generated, drift-checked in CI and in `tokens.test.ts`.
- The derivation is also a gate: WCAG 2.2 contrast for every text/control
  pairing (text ≥ 7:1, secondary/muted/faint ≥ 4.5:1, focus ring and
  non-text ≥ 3:1, soft-ink on wash ≥ 4.5:1), chart series ≥ 3:1 with
  all-pairs ΔOK ≥ 0.08 and adjacent-pair ΔOK ≥ 0.05 under Machado-2009
  protan/deutan/tritan simulation, and ladder ordering. Negative-tested.
- Only pure white remains shared with the reference set. The chart curve was
  rewritten from Fritsch & Carlson (1980) and its test now asserts no
  overshoot. Code comments no longer name the reference project. Commit
  `e0fa6b2`'s message still does — pushed history is not rewritten.

## 2 · Jobs whose outcome is unknown (Q-59)

**Problem (verified in the prototype, 2026-11-12):** a transport error after
dispatch goes to `failJob`, which seals a bare error with no
`hardware_job_id`, so settlement treats it as pre-vendor and **releases** the
reservation (`platform/core/pkg/jobs/jobs.go` `failJob`). Jobs are not
reconciled at boot (loops and workflows are). Both turn uncertainty into a
refund or a stranded row.

**Decision.** Add `JOB_STATUS_OUTCOME_UNKNOWN`: the effect *may* have reached
a vendor and neither result nor usage can be confirmed.

- **Entry (closed list).**
  - E1: transport or deadline error on a vendor-reachable job after
    `dispatched_at` was set.
  - E2: boot reconciliation finds a vendor-reachable job `RUNNING` under a
    dead owner epoch.
  - E3: cancel of a vendor-reachable `RUNNING` job whose vendor cancellation
    cannot be confirmed.

  A definitive pre-dispatch refusal and host-only job types never enter.
  They settle as `FAILED`/`INTERRUPTED` and release. "Vendor-reachable" is
  one shared constant, never a second list.
- **Classification: neither terminal nor live.** No worker or cancel sweep
  owns it. It blocks session deletion: the existing fail-closed default
  applies, plus a dedicated blocker label. The reservation is **kept in
  full** and counts against tenant and session caps ("missing means unknown,
  never zero", consistent with Q-51). It is **never auto-retried.** A re-run
  needs a new `request_id`, and the console states the double-run risk.
- **Exits (evidence only).** Every exit goes through the single settlement
  funnel and seals `job_outcome_resolved` with an evidence class.
  - X1 **vendor evidence** (automatic). A bounded reconciler polls by the
    pinned vendor reference with backoff, at boot and periodically. A vendor
    terminal state settles with measured usage. The async rail already pins
    the vendor id at submit (`platform/kubona/mcp/servers/execution.py`
    `record_hardware_job`). Whether the synchronous hardware rail pins it
    before acknowledging must be verified, and made true, during
    implementation.
  - X2 **vendor-confirmed absence** (automatic). Used only for vendors whose
    API gives a definitive lookup, such as a tag search. Not enabled until
    verified per vendor.
  - X3 **operator resolution.** A signed `AdminAction`, the same envelope as
    refunds, with one of three outcomes:
    - `vendor_confirmed`: vendor id and/or measured usage; settles measured.
    - `vendor_never_received`: evidence note required; releases.
    - `written_off`: keeps the estimate as spend; terminal `FAILED` with
      reason `outcome_unknown_written_off`.
- **No timer resolves it.** Elapsed time is not evidence. Attention routes it
  to the shell "Needs you" tray with its age, escalating after 1 h.
- **Answer to the original fork:** it is not "terminal for deletion", and not
  "blocking until a button press". It blocks until resolved **by evidence**.
  Operator acknowledgement exists only as a signed resolution that states its
  evidence and preserves the uncertainty in the ledger.
- **Threat argument.** The only money-releasing exit is X3
  `vendor_never_received`. It is writer-only and needs the admin signature.
  A stolen session cookie cannot mint it. It is ledgered with evidence text,
  and it releases at most the job's own reservation. Readers see state and
  age only.
- **Observation (not a defect).** Deletion refuses whenever
  `reserved_amount_micro_usd > 0`. For completed jobs that is *kept spend*,
  so any session that ever paid is undeletable by design (money-trail rule).
  The blocker label "outstanding spend reservations" should read "spend
  records".

## 3 · Run-scoped allowance (Q-60, adopted, gated on review)

The operator adopted "allow for this run": one signature covers many
hardware jobs within bounds. The default remains per-job approval.

- **Shape.** An Ed25519 mandate binds a scope digest over
  `{session/campaign id, backend allow-list, job-type allow-list,
  max_per_job_usd, max_total_usd, max_jobs, shots_cap, expires_at}`.
  Expiry defaults to 8 h, maximum 24 h. Go admission checks scope, then
  atomically decrements the remaining allowance (CAS on the allowance row).
  Tenant and session caps still apply. Each job's receipt cites the
  allowance id and the remaining amount.
- **Suspension, automatic and fail-closed.** The allowance is suspended on
  any of:
  - an explicit revoke (one click; in-flight jobs finish, nothing new is
    admitted);
  - air-gap engaged;
  - a backend or calibration change beyond threshold (stale basis; see
    server-owned proposals);
  - an unreadable allowance store (per-job approval resumes).
- **Threat argument.**
  - *What changes:* the human click per job disappears inside the bounds.
  - *Worst case:* `min(max_total_usd, remaining caps)` before expiry. The
    per-job cap and the backend/type allow-lists stop a pivot to pricier
    hardware.
  - *Who can mint:* only the operator's signing key (non-extractable
    WebCrypto custody, Q-56). *Who can spend:* only that session's own
    admission path. Cross-session use is refused by the scope digest.
    Replay is refused by expiry plus the counter CAS.
  - *Prompt injection:* **a turn that consumed untrusted tool or literature
    content is not covered.** Its jobs fall back to per-job approval
    (structured taint flag; see the untrusted-output rule).
  - *Reader/writer:* reading remaining-allowance state is open to session
    readers; minting and revoking are signed writes.
  - *What breaks, for whom:* nothing for the default flow. Reviewers get
    fewer prompts; finance gets a hard ceiling.
- Implementation must not land before this argument is reviewed against the
  final code, per the Production Bar.

## 4 · Which settings may save automatically (Q-61)

**Problem (verified in the prototype, 2026-11-12).** `PUT /api/llm/settings`
replaces the whole document with no revision. Both `AirGapControl` and the
LLM Settings page write it. A stale LLM Settings save after the shell engaged
air-gap silently **re-enables egress**. Fixing this race is a precondition
for any per-field saving.

**Decision: commit friction follows consequence, not convenience.** Friction
is asymmetric: tightening a control is one click, loosening it confirms.

| Tier | Commit | Fields (prototype today → policy) |
|---|---|---|
| A · device presentation | instant, local, verified by storage read-back | theme, tint, contrast, density, reduce-motion; canvas view toggles (minimap, edge mode, filters); inspector width; panel state; persona/feature switches (v1 local) |
| B · server presentation | instant **only** through a field-level PATCH carrying `expected_revision`, verified by read-back | none today; future: synced UI prefs, keymap overrides, notification and attention presence rules |
| C · behaviour, egress, spend, reproducibility | explicit Save of a staged diff ("what changes"), per-section settlement, revisioned | provider enable/base URL/wire/model/timeout, active provider/model, role assignments and fallbacks, temperature/top-p/seed, max tokens, reasoning effort, DSML-fallback switch, default backend, harness→backend map |
| D · loosening a safety control | explicit + confirm (500 ms click-settle, Cancel focused, Escape decides nothing) + ledger event | air-gap **off**; set or clear a provider key; default backend → metered hardware; enabling a cloud provider. Air-gap **on** (tightening) is a one-click Tier C commit |
| E · authority | signed admin envelope (unchanged) | vault leases, tenant grants, refunds, reviewer key, outcome-unknown resolution, run allowances |

- **Precondition (P0).** Settings documents gain a `revision`. Writes carry
  `expected_revision` and get `409 revision_conflict` with the current state.
  `AirGapControl` becomes a field-level write (`PATCH {air_gapped}`) and stops
  rewriting the whole document. The client verifies the returned snapshot
  equals its intent before reporting `applied`, otherwise `unknown`.
- Nothing server-side auto-saves today, and nothing in Tier C/D ever will:
  those fields change where prompts go, what is spent, or what results mean.

## Related

[[cost-governance]] · [[spend-mandates-and-delegation]] ·
[[integration-topology-and-licence-discipline]] · [[agent-ux-control-plane]] ·
[[ui-ux-vision]] · [[production-readiness-audit-2026-10-26]]
