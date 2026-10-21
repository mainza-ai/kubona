---
type: concept
name: Audit Remediation Plan (findings F-R1-09 through F-R1-16, F-001, F-006)
date: 2026-10-26
updated: 2026-10-26
tags: [audit, remediation, planning, gaps, production]
sources: [source-gap-analysis-plan, source-architecture-paper]
status: tracked
---

# Audit Remediation Plan

> **What this page is.** The fix plan for every *open* finding in
> [[full-system-audit-2026-10-26]] plus the two standing items (F-001,
> F-006). **Nothing below is implemented.** Each item names the
> decision to make first, the recommended path, and the acceptance test
> that closes it. Rule of the house: a slice lands green or not at all —
> findings fix with a regression pin proven to fail pre-fix, verified
> live where the finding was found (browser/Docker/rail, not just unit).
> Sequencing: **Now** (small, safe, same-bar as round-1 fixes) →
> **Next** (needs a design decision first) → **Gated** (needs an
> operator product call). Do not start Next/Gated items without recording
> the decision on the linked concept page.

## Now (approved pattern — small, safe, directly testable)

### R-09 · Cross-tenant session reads (P1 — from F-R1-09)
- **Decision first:** which identity scopes reads — bearer-token lab,
  session cookie, or explicit "single-lab deployment" posture?
- **Recommended path:** bearer-gate `GET /api/sessions` (list) and add
  a tenant check on session-scoped reads (epistemics/campaign/events/
  messages/ledger-export) against the caller's tenant; keep the public
  shape but scope it. Console already sends bearer-or-cookie on every
  call, so no surface breaks — verify per surface.
- **Alternative (if multi-tenancy is not a deployment target):** declare
  single-lab posture and record it here; the check becomes "list shows
  only the caller's lab" once identity exists.
- **Acceptance:** the round-1 repro (list → replay UUID into another
  tenant's reads) 401s/403s with a named reason; own-tenant reads
  unaffected; console surfaces re-verified (toasts, not blanks).

### R-10 · Restart-orphan loops (P1 — from F-R1-10)
- **Decision first:** rehydrate sleeping DB loops into the supervisor
  on boot, or settle-and-archive them (terminal receipt + event)?
- **Recommended path:** settle-and-archive — a loop whose engine died
  with its daemon cannot safely resume mid-step; write a terminal
  record, release reservations, unblock session delete. Rehydration
  risks double-execution of non-idempotent steps.
- **Acceptance:** kill daemon mid-loop → restart → reconciler logs the
  disposition; API shows a terminal record (never `[]` while the DB
  says `running`); session delete proceeds after settle.

### R-11 · ModelHub download safety (P2 — from F-R1-11)
- **Path:** (a) size-confirmation gate before any fetch over a threshold
  (e.g. 100 MB — "4.36 GB — download?"); (b) a cancel control outside
  the progress re-render path (stable target, tested by click).
- **Acceptance:** one click never starts a multi-GB fetch; cancel is
  clickable mid-download in automation (Playwright actionability,
  no `force:`); partial file cleaned on cancel (already true
  server-side — keep it).

### R-13 · Import confirmation (P2 — from F-R1-13)
- **Path:** toast naming what imported (session title, event count)
  plus an explicit session switch or "view" affordance.
- **Acceptance:** import round-trip ends with a visible, accurate
  receipt; silent success is gone.

## Next (needs a design decision recorded first)

### R-12 · Metasearch proposer gap (P2 — from F-R1-12)
- **Decision first:** which engine event mints proposals (loop
  stagnation? anomaly detector? manual console entry?), and what
  quorum approves them.
- **Path:** wire the chosen producer, then the existing
  approve/reject/apply rails become testable end-to-end (the round-1
  ghost-refusals become the negative half of that battery).
- **Acceptance:** propose → approve → apply → loop behavior changes,
  all ledger-sealed; ghost paths still refuse.

### R-14 · ifc-core test coverage (P3 — from F-R1-14)
- **Path:** unit tests for the reader-set lattice (join/meet,
  `CanFlowTo` transitivity, tenant-boundary cases mirroring M-2).
- **Acceptance:** `cargo test` covers the lattice, not just compiles it.

### R-15 · Tribunal turn-2 flakiness (P3 observation — from F-R1-15)
- **Decision first:** is officer-turn reliability a prompt problem
  (shorten/anchor the turn-2 prompt), a sampling problem (retry once
  on empty, still bounded), or accepted variance (coverage already
  says unobserved)?
- **Path:** measure first — log reply lengths per turn for a week of
  runs, then pick the smallest intervention the data supports.
- **Acceptance:** empty/timeout turns stay contested (already true);
  only the *rate* may move, with evidence.

## Gated (operator product calls — do not touch without sign-off)

### R-01 · Python exec rails in Docker (standing F-001)
- **Options:** (a) route codesign/chemistry/discovery through the
  bridge gRPC service like krylov (consistent, larger); (b) ship
  python+venv in the kubonad image (simpler, heavier, supply-chain
  surface grows); (c) document codesign as local-rail-only and make
  the console degrade explicitly (smallest, weakest product).
- **Acceptance:** whichever is chosen, the Docker-stack codesign
  battery from [[full-system-test-plan]] goes green on the shipped
  image — or the docs state the limitation where the user meets it.

### R-02 · Fidelity default probe (standing F-006)
- **Options:** (a) refuse without explicit probe parameters;
  (b) label default assessments uncalibrated-demo in the response.
- **Acceptance:** no caller can read `ADMIT` + `0.94 measured` from
  a zero-input call; `/api/fidelity` contract test pins it.

### R-03 · Public session list posture (extends R-09)
- If multi-tenant consoles ever ship, the list endpoint needs the
  tenant identity the R-09 decision introduces. Until then, R-09's
  single-lab posture (if chosen) must be written down, not assumed.

## Explicitly not in this plan

- Paid/QPU execution paths, x402 live rail, third-party review,
  load/chaos, public-repo flows — same exclusions as the test plan.
- Re-litigation of settled constraints ([[overview]] §Settled
  constraints) — propose a new decision with evidence instead.

Related: [[full-system-audit-2026-10-26]] ·
[[full-system-test-plan]] · [[frontend-backend-ui-ux-audit]] ·
[[implementation-gaps]] · [[risks]]
