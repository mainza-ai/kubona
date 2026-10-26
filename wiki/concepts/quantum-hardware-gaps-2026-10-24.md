---
type: concept
name: Quantum hardware completeness audit (Oct 2026)
date: 2026-10-24
updated: 2026-10-25
status: tracked
tags: [qpu, providers, provisioning, audit, gaps, billing, frontend]
sources:
  - source-frontier-repo-audit-2026
---

# Quantum hardware completeness audit (2026-10-24)

Provisioning, provider wiring, and frontend QPU configuration measured
against the tree — what a user needs to choose and run real quantum
hardware, and what was missing. Companion to
[[observatory-plan-gap-audit-2026-10-23]] (lens) and
[[full-system-audit-2026-10-26]] (round-1 findings register).

## Method

Every claim checked against code, tests, or live API responses on
2026-10-24 — not against prior pages. Verdicts: **Done** (implemented +
tested), **Partial** (present but incomplete), **Absent** (not built).
No live QPU calls were made (no operator keys exist yet) — everything
below is hermetic plus one honest rule: unwired/unverified paths refuse
with the exact missing piece named.

## Findings and dispositions

| # | Gap | Verdict at audit | Disposition |
|---|---|---|---|
| G1 | `.env.example` documents zero quantum credentials | **Absent** | **Done**: quantum section added (4 `KUBONA_*` keys + `BAO_*` vars + acquisition links). Convention is `KUBONA_{PROVIDER}_{KEY}` per `secrets.py` |
| G2 | Billing hole: zero-rate hardware estimated $0.00 | **Absent** (fail-open) | **Done**: `per_shot`/`per_task_plus_per_shot` with zero rates now refuse like the existing `per_second` guard; `CostGovernanceService.preflight` converts estimation failures into governed `ok:False` refusals (was: uncaught exception); pins in `test_schema.py` + `test_qpu_providers.py` + `test_fidelity_admission.py` |
| G3 | Pricing notes unverified/undated (except IBM 2026-10-10) | **Partial** | **Done in part**: `verified`/`source` fields on all 8 JSONs (null except IBM); `scripts/qpu-provision-check.py` flags unverified/stale (>90d) advisory; live re-verification of each rate card still open |
| G4 | Fleet `/api/fleet` hardcodes queue/T1/status; "Live Telemetry" banner | **Partial** (theater) | **Done**: per-card `telemetry: measured\|declared` + `catalog_verified`; console CATALOG badges; Go test pins provenance; live canary proxy shipped (bridge `RunCanary`, Go 5-min TTL cache, console trigger) — live *runs* still need keys |
| G5 | No QPU provider settings surface | **Absent** | **Done**: `GET/PUT /api/quantum/settings` (bearer-gated PUT, validated) + `GET /api/quantum/backends` + `QuantumSettings.tsx` in mesh→settings (picker, harness overrides, credential badges, no key entry by design) |
| G6 | No user-facing job submission | **Absent** | **Done (hermetic; live verification pending)**: `QuantumSubmit/JobStatus` bridge RPCs calling the SAME `submit_job` rail (idempotency via persisted meta, dry-run estimates, tenant attribution); Go `/api/quantum/jobs` proxy (bearer + autonomy + allowlist, no Go-side spend reservation); `QuantumDispatch` console surface (estimate→mandate card→dispatch→poll→cancel). Live end-to-end run is the remaining proof (R-34) |
| G7 | D-Wave Leap: local-only despite "Leap cloud" notes | **Absent** | **Done**: `backend="leap"` branch (`LeapHybridSampler`, token + SDK fail-closed, injected-factory tests); never live-verified (keys don't exist) |
| G8 | IonQ / Braket: no JSON, no plugin | **Absent** | **Done**: `ionq.json` + `aws-braket.json` (`status: planned`, zero rates refuse), `IonQProvider` (credential plumbing real, submit gated on `LIVE_VERIFIED`), `BraketProvider` (no fake credential var — SigV4 unimplemented by design). Correction to the audit report: **Rigetti QPU submit was never wired** (`_submit_via_qcs` is an explicit stub) — only IBM Runtime and Quantinuum (legacy blocking API) execute hardware today |
| G9 | pulser / strawberry silently unwired | **Partial** | **Done**: `wiring: not-wired` in health + surfaced in settings UI |
| G10 | No refund primitive; no provider cancel | **Absent** | **Done (mechanism; policy open → Q-41)**: Go session `RefundSpend` + `POST /api/sessions/refund` (mirrors tenant RefundSpend); Python `SessionStore.release_spend` + governance wrapper; provider `cancel()` on the trait (IBM Runtime cancel + qnexus cancel implemented + injected-tested; Rigetti/D-Wave/pulser/photonic/ionq/braket refuse with exact reasons — no fabricated SDK calls); `request_provider_recall` + `on_hardware_job` pinning + bridge `QuantumCancel` + Go/console wiring |
| G11 | Provision-time validation/test connection | **Absent** | **Done**: `qpu-provision-check` (shape checks, no values; advisory exit 0); canary proxy shipped (G4); live runs pending keys |

## What was deliberately NOT built (2026-10-24 implementation pass)

- No live QPU calls (no keys — operator constraint; all new paths are mock-first with refusal pins).
- No mandate enforcement on dispatch: `mandate_ref` is recorded, not enforced, pending the Q-10 conformance ruling (new Q-40 — session ceilings may already be the mechanism).
- No refund policy beyond the conservative mechanism (admin-only, reason-required, no-negative-books): signer scope, window, and partials are open questions (new Q-41).
- No fabricated SDK calls or verification dates: Rigetti cancel, D-Wave cancel, IonQ/Braket/Pulser/photonic execution refuse with the exact missing piece; unverified pricing stays null.
- No Go-side QPU spend reservation (would double-count the Python book of record); no Go-side refund of Python-book spend.

> ⚑ Correction (2026-10-25 investigative audit, F-NEW-01..F-NEW-07): leaving Go's SQLite ledger out of `/api/quantum/jobs` split session/spend authority across two non-synced stores (`kubonad.db` vs `~/.kubona/sessions.jsonl`), making `POST /api/sessions/refund`, `DeleteBlockers`, and console spend bars blind to HTTP-dispatched quantum spend — and `session_spend_refunded` / `session_spend_refund_refused` were missing from `KnownEventTypes` (`platform/contracts/event-types.json` and `events.go`), so `AppendSignedEvent` silently dropped every session refund receipt. Converged on 2026-10-25 (per [[source-architecture-paper]] single-ledger discipline): Go mirrors verified SQLite session ceilings + current spend into `QuantumSubmitRequest` (`sync_external_session`), threads `caller_jurisdictions` / `caller_export_class` / `require_review`, enforces Sovereign Air-Gap + sessionless tenant autonomy + T-09c cross-tenant isolation, reserves non-replay `estimated_usd` in Go's SQLite (`tenants` + `sessions`), registers the session refund event types, maps `"quantinuum"`/`"quantinuum-tket"` to `"quantinuum-h1"` in both Python and Go fidelity catalogs + wires `on_hardware_job` in `_submit_via_quantinuum`, and indexes `/api/quantum/canary` results per provider key.

## Live-proof ledger (this audit)

Hermetic, by constraint: Go full suite green (29 pkgs), console 54 files/266 tests + lint + tsc clean, Python fast lane 767 + bridge 40 green, wiki-lint clean, provision-check exit 0 advisory. Live re-verification checklist for the day keys exist: IBM Runtime submit + cancel, Quantinuum submit + qnexus cancel, Leap hybrid sample, per-vendor rate cards, fleet canary proxy against live backends, end-to-end human dispatch (estimate→mandate→dispatch→status→cancel) with session caps.
