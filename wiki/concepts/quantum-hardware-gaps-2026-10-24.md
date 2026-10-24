---
type: concept
name: Quantum hardware completeness audit (Oct 2026)
date: 2026-10-24
updated: 2026-10-24
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
| G2 | Billing hole: zero-rate hardware estimated $0.00 | **Absent** (fail-open) | **Done**: `per_shot`/`per_task_plus_per_shot` with zero rates now refuse like the existing `per_second` guard; pins in `test_schema.py` + `test_qpu_providers.py` |
| G3 | Pricing notes unverified/undated (except IBM 2026-10-10) | **Partial** | **Done in part**: `verified`/`source` fields on all 8 JSONs (null except IBM); `scripts/qpu-provision-check.py` flags unverified/stale (>90d) advisory; live re-verification of each rate card still open |
| G4 | Fleet `/api/fleet` hardcodes queue/T1/status; "Live Telemetry" banner | **Partial** (theater) | **Done in part**: per-card `telemetry: measured\|declared` + `catalog_verified`; console CATALOG badges (`FleetRadar`, `QuantumSettings`); Go test pins provenance. Live canary proxy still open |
| G5 | No QPU provider settings surface | **Absent** | **Done**: `GET/PUT /api/quantum/settings` (bearer-gated PUT, validated) + `GET /api/quantum/backends` + `QuantumSettings.tsx` in mesh→settings (picker, harness overrides, credential badges, no key entry by design) |
| G6 | No user-facing job submission | **Absent** | **Open (R-34)**: picker shipped; dispatch needs a `QuantumSubmit` bridge RPC reusing the MCP execution rail + threat analysis + live keys |
| G7 | D-Wave Leap: local-only despite "Leap cloud" notes | **Absent** | **Done**: `backend="leap"` branch (`LeapHybridSampler`, token + SDK fail-closed, injected-factory tests); never live-verified (keys don't exist) |
| G8 | IonQ / Braket: no JSON, no plugin | **Absent** | **Done**: `ionq.json` + `aws-braket.json` (`status: planned`, zero rates refuse), `IonQProvider` (credential plumbing real, submit gated on `LIVE_VERIFIED`), `BraketProvider` (no fake credential var — SigV4 unimplemented by design) |
| G9 | pulser / strawberry silently unwired | **Partial** | **Done**: `wiring: not-wired` in health + surfaced in settings UI |
| G10 | No refund primitive; no provider cancel | **Absent** | **Open (R-32/R-33)**: proposals recorded, implementation gated on operator ruling (money movement + hardware recall) |
| G11 | Provision-time validation/test connection | **Absent** | **Done**: `qpu-provision-check` (shape checks, no values; advisory exit 0); live canary wiring still open |

## What was deliberately NOT built

- No live QPU calls (no keys — user constraint; all new paths are mock-first with refusal pins).
- No human dispatch route (R-34): a console→Go→bridge spend path is a new trust boundary needing its own threat analysis + live keys to verify against.
- No refund/cancel implementation (R-32/R-33): needs the operator's policy ruling first (who signs, windows, partials).
- No estimated `verified` dates beyond IBM: inventing verification dates would be fabrication; nulls are honest.

## Live-proof ledger (this audit)

Hermetic only, by constraint: Go full suite green (29 pkgs), console 53 files/262 tests + lint + tsc clean, Python schema/qpu/catalog/taxonomy/conformance green (34), wiki-lint clean, provision-check exit 0 advisory. Live re-verification checklist for the day keys exist: IBM Runtime submit, QCS submit, Quantinuum submit, Leap hybrid sample, per-vendor rate cards, fleet canary proxy.
