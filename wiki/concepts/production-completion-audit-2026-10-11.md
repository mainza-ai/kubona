---
type: concept
name: Production Completion Audit 2026-10-11
date: 2026-10-11
updated: 2026-10-11
tags: [audit, production, agents, console, science, hardening, plan]
sources: [source-gap-analysis-plan]
status: tracked
---

# Production Completion Audit (2026-10-11) — B/C/D/E verification + build plan

**Method:** four parallel code investigations (agentic core, console UX, science
depth, hardening) with `file:line` evidence, plus spot-confirmation of the
three most serious claims. Standard: production, not demo. Verdict: **not
100%** — B ~55%, C ~40% live, D deep but with one critical honesty bug, E far
from P4. This page is the build plan; checkboxes flip as slices land (each
with tests + live proof + green CI, per project rule).

## Claim corrections (what is actually true)

| Claimed | Actual |
|---|---|
| Tool protocol + dispatcher | PARTIAL — copilot path real; tribunal/stream second-class |
| Single registry, dead allow-lists replaced | PARTIAL — registry enforces; both dead allow-lists still standing |
| Context budgeting | PARTIAL — real bounds, hardcoded constants, recall uncounted |
| Hindsight over HTTP | ABSENT — in-process only, zero routes, zero console callers |
| Planner loop replacing tribunal | ABSENT — fixed 4 turns, hardcoded pass |
| Versioned/event-sourced campaigns | Versioned DONE; event-sourced NO (snapshots, no replay) |
| Compile/validate gates on launch | Validate DONE; compile NO (QIR never on launch path) |
| Memory write-back | DONE on chat paths; tribunal unattributed, non-chat paths open |
| Multi-geometry scans | Classical DONE (`pes_sweep`); quantum multi-geometry ABSENT |

## P0 blockers (fix before any production claim)

- [ ] **Silent anchor fallback.** Bridge down/slow → hardcoded energies labeled
  `PySCF STO-3G FCI`, `COLLAPSED_VERIFIED`, chemical-accuracy pass by
  arithmetic (`vqe = fci + 0.47mHa`). Fix: `anchor_computed` +
  `anchor_source` on `ClassicalAnchor`, fallback never verified, UI badge.
  (`platform/core/pkg/agents/campaign_synth.go:336-351,354,518`)
- [ ] **Unauthenticated `/api/mandates/sign`.** No bearer middleware on any
  `/api/*`; arbitrary tenant/job/budget signed with reviewer key.
  (`platform/core/pkg/api/server.go:258,573-616`)
- [ ] **Hardening trio.** CORS `*` (`server.go:224`, SSE `:1867`,
  `console_api.py:853-854`); `AllowHostFallback=true` (`server.go:142`);
  zero rate limits on auth/mandate/dispatch.
- [ ] **Proposal fallback fabrication.** No-model-proposal → server-synthesized
  Bell-ladder QASM in the proposal slot, fallback anchor string unlabeled.
  (`server.go:1443-1499`)
- [ ] **Launch silent defaults.** Empty QASM → Bell, empty backend →
  local_simulator, `BudgetUSD≤0` → 1.0 on a spend path. (`server.go:1563-1571`)

## Phase B0 — correctness before capability

- [x] B0-1. Anchor fallback flag (above). — landed 2026-10-11: `anchor_computed`/`anchor_source`, fallback never verifies (UNVERIFIED + pending weights + caveats at every surface), replan promotes to computed-grade.
- [x] B0-2. Auth on sign + bearer skeleton + CORS allowlist + sandbox default
  false + sign-path rate limits. — landed 2026-10-11: tenant-admin envelope
  on sign, Bearer fail-closed 503, CORS allowlist (`*` needs opt-in), host
  fallback opt-in, per-IP buckets on sign/verify.
- [x] B0-3. Proposal `designed_by:"server-fallback"` + labeled fallback
  strings; launch refuses empty QASM/backend (or explicit `allow_defaults`).
- [x] B0-4. Stream `withToolMessages` routing (`pkg/llm/client.go:184`);
  `tool_called` from `observeTurn`; tribunal `toolNamesSince` (not `nil`);
  delete/wire dead stream-tool helpers; delete dead allow-lists (or
  registry-resolution test). — landed 2026-10-11: stream wire shape fixed
  + pinned, observeTurn returns auditable calls (turn-level `observed_tools`,
  ledger `tool_called` via helper), dead stream helpers + RoleManifest
  allow-list deleted, registry contract test replaces it.
- [x] B0-5. `qir.Analyze` in `SubmitJob` circuit_run branch; record
  `compiler_used`/`fallback_estimate`; launch inherits.
- [ ] B0-6. Secrets purge: `.env` discipline, lease HMAC + PG DSN to
  env-required, rotation doc.

## Phase B1 — real agency

- [ ] B1-1. Planner early-exit: Turn-2 anchor/gradient check → 2-turn
  CONTESTED; `ConsensusPassed` wired to evidence.
- [ ] B1-2. Hindsight HTTP (`GET recall`/`POST retain`, tenant-scoped) + one
  console caller — or formally downgrade the claim.
- [ ] B1-3. Drop "event-sourced" language or add `ReplayCampaign` tested
  against the live row; budgets into one config counted against history;
  failure breaker + cost accumulator on tool loop.

## Phase C — wire what exists (no new viz systems)

- [ ] C-1. DAG-b: persist `circuit_cut` fragments into `wire_cuts`, render
  chips, relabel cut node `"planned (not executed)"`; original-vs-transpiled
  + receipt hash via existing `/api/qir/compile`; fix QASM-as-`qirCode`.
- [ ] C-2. ResultsStudio energy table + FlightRecorder tooltips from latest
  completed jobs; tribunal/stuck badges on DAG nodes; fix SpatialCanvas
  verified-chrome, WidgetStage LIVE badges, mandate demo defaults,
  A2UI→Preview suffixes.

## Phase D — by anchor family, honesty first

- [ ] D-1. Milestone-fiction badges (`synthesized-projection` vs `measured`)
  or fresh-empty default; synth `ChemicalAccuracyPassed=false` until a fold;
  QKSD `classical-emulation` badge; curvature decisional-or-diagnostic;
  mechanical contradiction check at claim_eval; literature offline label; DAG
  partition relabels. Then per-family bridge verification (large molecules
  first).

## Phase E — P4 gate

- [ ] E-1. FIPS-203 KAT + transport decision; Bao credential into one job
  path + AppRole; leases doc + single-writer guard; policy parity test;
  low-trust CI split; review engagement + sidecar grading; can-agent-act CI
  job; Q-27 flip sequence.

## Evidence notes (kept, not re-verified here)

- Tribunal constants (`collective.go:248-249,304,336,368,400`), non-H2/LiH/BeH2
  silently H2; `recent_events` pays `{type,id}` only; schemas validate scalars
  only; `maxToolRounds=3` unbounded. Console honest-patterns to copy:
  ProofDagView/FlightRecorder/Manifold/TribunalRoom empty states, LIVE/REF
  split, UNVERIFIED banners. Science: 14 synth families (bridge covers 10);
  syndrome "no producer" verified accurate; advantage language clean (keep the
  gate; consider renaming `COLLAPSED_VERIFIED` → `WITHIN_CHEMICAL_ACCURACY`).
  Hardening: mandate-envelope ML-KEM-768 real; transport plain HTTP; PG/HNSW
  staged not wired; CI runs PR code privileged; dev-token fallback; no PII
  path; Ed25519-only auth.
