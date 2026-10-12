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
> ⚑ Correction (2026-10-12, Q-33 closed): the gap is landed —
> `POST /api/tenants/status` flips active ↔ suspended (admin-signed,
> bearer + limiter); suspension freezes spend-affecting actions while the
> admin keeps reactivation + key rotation. See [[user-identity-and-profiles]].
- [x] E-1c. Bao credential into one job path + AppRole. — landed 2026-10-11:
  `OpenBaoSecretSource` (AppRole login, KV reads, fail-closed outages,
  404-absent passthrough, lease revoke) + `SecretStore.from_env`
  (Bao authoritative when configured, env otherwise); Qiskit submit
  path consults Bao first. 9 tests; runbook covers Bao rotation.
- [x] E-1d. Low-trust CI split + can-agent-act job. — landed 2026-10-11:
  `contents: read` on CI + nightly (nothing publishes; SHA-pinning
  needs a bot first, logged); can-agent-act test (reach/refuse/budget
  against production paths) fails CI if any leg is assumed. Also fixed
  a real CI flake found en route (A2A revoke race, blocking executor).
- [x] E-1e. FIPS-203 KAT + transport decision doc; review engagement +
  sidecar grading; Q-27 flip (operator-gated). — landed 2026-10-12 as
  code+docs where executable, operator actions where not: KEM parameter
  contract pinned (sizes, nonce, fingerprint, wrong-key) with the NIST-KAT
  limitation documented (seedless stdlib API admits no vector seam);
  transport decision recorded in [[security-and-pqc]] (TLS at edge +
  ML-KEM envelopes, KEMTLS deferred, plain-HTTP LAN declared).
  Operator-gated, in order: (1) revoke the `store`-helper `gho_…` grant;
  (2) commission the third-party review (brief exists); (3) run one real
  vendor through the conformance gate; (4) Q-27 flip sequence
  (filter-repo → force-push → re-clone → rotate). None of these can be
  executed from inside the build — this checkbox tracks the docs, not
  the deeds.

## Phase F — audit findings (2026-10-12 adversarial re-audit)

Three falsification passes plus independent re-verification found the plan
~85% truly complete: 5 refutations confirmed real, 2 rejected with evidence
(autonomy bypass is false — `admitJob` enforces for all types; chemistry
coverage exists in `platform/tests/test_bridge_chemistry.py`; the Analyze
refuse branch stays as documented defense). Each slice ships with tests +
live proof + green CI, checkboxes flip on landing.

- [x] F-1. Python CORS allowlist (P0). — landed 2026-10-12:
  `_cors_allow_origins()` in `console_api.py` mirrors the Go rail
  (localhost:3000 + 127.0.0.1:3000 + `KUBONA_CORS_ORIGINS` extras;
  `*` only under `KUBONA_CORS_ANY=1`); helper + live middleware tests
  prove allowlisted origins echoed, foreign origins headerless. Line
  audit: methods/headers tightened from `*` to the actually-used
  `GET/POST/PUT/DELETE` + `Authorization/Content-Type` (verified
  against console usage) + preflight test.
- [x] F-2. Epistemic state coherence (P1). — landed 2026-10-12: R1
  `synthSources()` attests the PySCF pair only for computed anchors
  (fallback → unattested `[]`, via shared `fallbackCaveat` const so
  synth and fold cannot drift). R3a: claim PASS on an uncomputed
  anchor folds amplitudes but withholds collapse → `CONTESTED` +
  "UNCOMPUTED" note + `PASS WITHHELD` measure line. R3b: a recomputed
  anchor strips the caveat, re-attests sources, and restores
  `PROJECTED` + live weights only for pristine `UNVERIFIED`
  (fresh `HYPOTHESIS_PENDING` keeps pending weights; decided states
  never rewritten; notebook sections stay historical). Line audit:
  live weights hoisted to shared `liveAlpha/liveBeta/liveGamma`
  consts (synth + repair can no longer rot apart). Live: computed
  session → pair + `PROJECTED`; bridge-paused session → `[]` +
  `UNVERIFIED` + caveat; validation sessions deleted. Full Go suite
  green; 3 tests updated to the corrected behavior + 1 new
  (fresh-fold-keeps-pending).
- [x] F-3. Display/handler honesty (P1). — landed 2026-10-12: R2
  `handleNotebooks` no-campaign path tracks `anchorComputed` — bridge
  absent/failed → `UNVERIFIED_FALLBACK`, accuracy false, fallback named
  in anchor + sections. Line audit caught the success path still
  claiming an un-run VQE execution + verification (`fci + 0.47mHa`
  arithmetic labeled "execution"/"VERIFIED_ACCURATE") — rewritten:
  template path is now always `PROJECTION` (real anchor) /
  `UNVERIFIED_FALLBACK` (assumed), accuracy never passes, corridor
  estimate labeled "(not measured)", execution language removed; only
  job folds verify. R7 `MeasuredOutputs`
  gates on `status === JOB_STATUS_COMPLETED` (FAILED carries
  `completed_at` too) + regression fixture with a newer FAILED job.
  R16 tribunal badge reads `CONTESTED` + `XCircle` on false
  (a rendered tribunal is finished — never `DELIBERATING`) + render
  tests both ways. Live: template path both directions via API
  (campaign rows removed to reach it); validation sessions deleted.
  Go `pkg/api` green, console 73 tests green, `pnpm build` clean.
- [x] F-4. Auth/hardening leftovers (P1). — landed 2026-10-12: R4
  bearer rail extended to `grants/register`, `grants/revoke`, and the
  four tenant mutations (envelopes stay the real auth; bearer is the
  edge rail); new 5rps/15-burst limiters on tenants + grants routes.
  Tenant self-provisioning stays open by design (L1 simulator-only,
  first-key-wins — pinned by test) but rate-limited. `mandates/verify`
  was wrapped then deliberately UNWRAPPED in the line audit: it is a
  pure read-only check (no ledger write, nothing secret), and gating it
  would break the console's mandate-display flow (console token ≠
  operator token) for zero security gain — spray protection is the
  limiter's job (pinned by `TestMandateVerifyStaysPublic`). R5
  `ensureRunner` defaults `AllowHostFallback=false`
  + `KUBONA_ALLOW_HOST_FALLBACK=1` opt-in (all existing tests already
  set the flag explicitly). R6 compose password → required-env
  (refuses `config` without the var, resolves with it). Live matrix
  on an isolated daemon+DB: 401s no-token, 201 open creation, 401
  wrong token, 403 token-without-envelope. Live proof also caught a
  real pre-existing bug: `clientKey` keyed on `RemoteAddr` verbatim
  (one bucket per TCP connection — 25 rapid curls all 201, and the
  same hole applied to the B0-2 sign/verify limiters); now keys on
  remote IP, live re-proof 15×201 + 10×429. Six existing tests
  updated with tokens; parked daemon rebuilt to current develop.
- [x] F-5. Backend robustness + dead code (P2). — landed 2026-10-12:
  R8 QIR empty/whitespace QASM → 400 (Analyze is total; empty must not
  look like a result). R9 extended to the class: all four bridge
  handlers with success/error envelopes (execute, cut, krylov,
  mitigate) return structured `success:false` on unexpected
  exceptions; Evaluate/GroundLiterature deliberately still raise —
  their proto messages have no success field, so UNKNOWN is honest
  there (wrapping literature to return empty records would read an
  outage as "no literature"). R10 ledger hygiene: open-failure path
  uses `releaseLock`, `-lock` typo fixed, double-Close test (no panic,
  clean reopen), shared test helpers clean `.lock`, 43 stale test
  lockfiles swept (gitignored, prod DB untouched). R11 parity
  ID-binding: both drivers validate rail labels ∈ {go,py} and assert
  ran == railed count (self-maintaining, no hardcoded numbers).
  R12 qiskit store built per `_credential()` call (injected stores
  honored); test proves a mid-process BAO_ADDR switch bites instead
  of serving stale env (verified to fail on old code). R14 deleted
  `pkg/mcp/gateway.go` + test (zero production callers; unwired
  allow-list invites false confidence) — real enforcement lives in
  the Go tool registry + admission and the Python approval gateway;
  no wiki page claimed it as live. R17 `designed_by` transcribed
  from proposal into job params (row + `job_queued` event carry it);
  absent stays absent, never inferred. Live: dispatched job ran on
  Aer with the stamp in params; krylov XenonX refusal flows through
  the new envelope; QIR empty → 400; bridge + daemon restarted to
  current builds; validation session deleted. Go full + 401 python
  green.
- [x] F-6. Docs/minors (P2). — landed 2026-10-12: R15 measured
  14-row matrix published below (all values computed live on the date,
  PySCF STO-3G; FCI ≤6 orbitals else CASCI cascade; variational check
  holds on every row). B1-2 empty-q inventory isolation test
  (similarity:null pinned). `provenance_verified` renamed
  `fetched_live` with its narrow meaning documented (bytes arrived
  live — nothing about the claims). Analyze refuse branches
  documented as deliberate defense, not dead code. Production-bar
  extras caught en route: synth notebook section 3 claimed un-run Aer
  execution (same class as R2 — now corridor-estimate language), and
  the dead H2-substitution `except` in chemistry.py removed (was
  unreachable behind the refusal raise, but fiction-shaped).
  Rejected refutations stand recorded above. Go full + 401 python
  green.

### R15 measured family matrix (2026-10-12, PySCF STO-3G, live bridge)

Every value below was computed live on the audit date — no carried,
no rounded-from-memory numbers. Solver: exact FCI at ≤6 orbitals,
else the fixed CASCI cascade (6,6)→(6,4)→(4,4)→(4,2)→(2,2), first
success wins (deterministic). Every FCI sits below its RHF
(variational check passes on all 14 rows). `XenonX` refuses over the
same bridge (`unsupported molecule … refusing instead of
substituting`, live-proven in F-5). R = campaign equilibrium bond
distance (Å); large-molecule R seeds the RDKit conformer path.

| Molecule | R | RHF (Ha) | FCI/CASCI (Ha) | Solver |
|---|---|---|---|---|
| H2 | 0.7414 | -1.11668 | -1.13727 | FCI |
| LiH | 1.5957 | -7.86200 | -7.88239 | FCI |
| BeH2 | 1.3300 | -15.56010 | -15.57580 | CASCI* |
| H2O | 0.9575 | -74.96298 | -74.97040 | CASCI* |
| N2 | 1.0977 | -107.49589 | -107.62185 | CASCI* |
| CO2 | 1.1620 | -185.06522 | -185.15720 | CASCI* |
| CO | 1.1283 | -111.22459 | -111.33095 | CASCI* |
| HF | 0.9168 | -98.57076 | -98.59659 | FCI |
| NH3 | 1.0124 | -55.45398 | -55.49467 | CASCI* |
| CH4 | 1.0870 | -39.50989 | -39.55590 | CASCI* |
| Benzene | 1.3970 | -227.88822 | -227.94430 | CASCI* |
| Caffeine | 1.3800 | -667.65428 | -667.68304 | CASCI* |
| Aspirin | 1.4000 | -636.60816 | -636.64195 | CASCI* |
| Paracetamol | 1.4000 | -505.83584 | -505.89628 | CASCI* |

## Evidence notes (kept, not re-verified here)

- Tribunal constants (`collective.go:248-249,304,336,368,400`), non-H2/LiH/BeH2
  silently H2; `recent_events` pays `{type,id}` only; schemas validate scalars
  only; `maxToolRounds=3` unbounded. Console honest-patterns to copy:
  ProofDagView/FlightRecorder/Manifold/TribunalRoom empty states, LIVE/REF
  split, UNVERIFIED banners. Science: 14 synth families (bridge covers 14/14 since D-1c — R15 matrix below);
  syndrome "no producer" verified accurate; advantage language clean (keep the
  gate; consider renaming `COLLAPSED_VERIFIED` → `WITHIN_CHEMICAL_ACCURACY`).
  Hardening: mandate-envelope ML-KEM-768 real; transport plain HTTP; PG/HNSW
  staged not wired; CI runs PR code privileged; dev-token fallback; no PII
  path; Ed25519-only auth.
