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
| Versioned/event-sourced campaigns | Versioned DONE + consistency-checked (`VerifyCampaignHistory`: live == newest version, bypass fails loudly). Snapshot trail, NOT event-sourced — the word is retired everywhere it implied replay. |
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
- [x] B0-6. Secrets purge: `.env` discipline, lease HMAC + PG DSN to
  env-required, rotation doc. — landed 2026-10-11: lease key env-required
  (503), PG DSN env-only, `[[secret-rotation-runbook]]`, pre-commit secrets
  guard (value patterns + .env paths, refuse/pass both tested).

## Phase B1 — real agency

- [x] B1-1. Planner early-exit: Turn-2 anchor/gradient check → 2-turn
  CONTESTED; `ConsensusPassed` wired to evidence.
- [x] B1-2. Hindsight HTTP (`GET recall`/`POST retain`, tenant-scoped) + one
  console caller — or formally downgrade the claim.
- [x] B1-3. Drop "event-sourced" language or add `ReplayCampaign` tested
  against the live row; budgets into one config counted against history;
  failure breaker + cost accumulator on tool loop. — landed 2026-10-11:
  `VerifyCampaignHistory` (bypass fails loudly), `llm.BudgetConfig` single
  source (recall sites + HTTP parser + history defaults), observation caps
  (6k) + turn tool-char cap (24k) with ledger events, repeat-failure
  breaker, usage accumulator (usage now parsed, was always nil), all pinned.

## Phase C — wire what exists (no new viz systems)

- [x] C-1. DAG-b: persist `circuit_cut` fragments into `wire_cuts`, render
  chips, relabel cut node `"planned (not executed)"`; original-vs-transpiled
  + receipt hash via existing `/api/qir/compile`; fix QASM-as-`qirCode`.
- [x] C-2. ResultsStudio energy table + FlightRecorder tooltips from latest
  completed jobs; tribunal/stuck badges on DAG nodes; fix SpatialCanvas
  verified-chrome, WidgetStage LIVE badges, mandate demo defaults,
  A2UI→Preview suffixes.

## Phase D — by anchor family, honesty first

- [x] D-1a. Synth honesty: trajectory `source` (synthesized/measured),
  synth `ChemicalAccuracyPassed=false` until a fold, PROJECTED status +
  verdict, DAG partitions relabeled planned. — landed 2026-10-11.
- [x] D-1b. QKSD `classical-emulation` badge; curvature
  decisional-or-diagnostic; mechanical contradiction check at claim_eval;
  literature offline label.
- [x] D-1c. Per-family bridge verification (large molecules first). — landed
  2026-10-11: 14/14 families compute live within budget. Found + fixed:
  silent H2-substitution for unknown molecules (now refuses), H2O
  CASCI(6,6) crash (deterministic candidate cascade), 13 fallback
  defaults that were never STO-3G numbers (Aspirin off 456 Ha) refreshed
  to measured values and pinned by test. Matrix: H2/LiH/BeH2/H2O/N2/CO2/
  CO/HF/NH3/CH4/Benzene/Caffeine/Aspirin/Paracetamol live; XenonX refuses.
  Follow-up (not this slice): CASCI-labeled-as-FCI wording for large
  systems; SMILES-bridge coverage needs the chemistry extra in CI.

## Phase E — P4 gate

- [x] E-1a. Leases doc + single-writer guard. — landed 2026-10-11:
  exclusive flock on `<db>.lock` in `OpenLedger` (second daemon refuses
  with cause, releases on exit, read-only unaffected); invariant
  documented at the struct.
- [x] E-1b. Policy parity test (Go/Python rails). — landed 2026-10-11:
  shared `platform/tests/policy_parity.json` (8 cases) driven by both
  rails; Go covers registry/autonomy/admission incl. grant-admitted QPU
  and L0 refusal; suspended-status has no mutator anywhere (dead state,
  logged gap).
- [ ] E-1c. Bao credential into one job path + AppRole.
- [ ] E-1d. Low-trust CI split + can-agent-act job.
- [ ] E-1e. FIPS-203 KAT + transport decision doc; review engagement +
  sidecar grading; Q-27 flip (operator-gated).

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
