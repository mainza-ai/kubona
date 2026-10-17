---
type: concept
name: Full-tree implementation roadmap
date: 2026-10-17
updated: 2026-10-17
tags: [plan, roadmap, audit, production, gaps]
sources: [source-frontier-repo-audit-2026]
status: candidate
---

# Full-tree implementation roadmap (code-read, production-grade)

> Read every implementation file in full before writing a line of this plan:
> `cmd/kubonad/main.go`, all of `pkg/api`, `pkg/jobs`, `pkg/ledger`,
> `pkg/session`, `pkg/agents`, `pkg/autoresearch`, `pkg/tenants`,
> `pkg/grants`, `pkg/mandates`, `pkg/llm`, `pkg/fidelity`, `pkg/disk`,
> `pkg/vault`, `pkg/sandbox`, `pkg/memory` (skimmed), all of
> `platform/kubona/{provenance,workflows,secrets.py,config.py,mcp/gateway.py,
> mcp/servers/provenance.py,execution.py}`, all of `platform/bridge/*.py` +
> proto, `platform/console/src/lib/api.ts` + page + CopilotDrawer,
> `platform/crates/*/src/*.rs`, `ci.yml`, both manifests. Kubona read at
> @ `433e7d5` (develop); REA at `e567033` where cited. Items marked
> **[verified]** were re-checked line-by-line for this page; unmarked items
> come from the full-read pass with `file:line` anchors. Nothing below is
> started. Production stance throughout: full logic or fail-closed refusal —
> no stub that must be redone.

## Wave 0 — spend-safety defects (do first; money is irreversible)

- **W0.1 `DeleteBlockers` status mismatch blocks deletion forever [verified].**
  Failed jobs store `JOB_STATUS_FAILED` (`pkg/jobs/jobs.go:458`), but the
  blocker query excludes only `'JOB_STATUS_COMPLETED','FAILED','failed',
  'completed'` (`pkg/session/migrate.go:434`) — a failed job is never
  excluded, so any session with a failed job can never be deleted. Threat:
  labs accumulate undeletable sessions; operators work around deletion and
  rot the cascade. Fix: compare against the protobuf enum set
  (`JOB_STATUS_*` constants, single source), plus a regression test with a
  failed job. *Test:* failed-job session deletes cleanly.
- **W0.2 Non-`circuit_run` jobs bypass all spend/fidelity gates [verified].**
  `admitJob` returns ok before grant/mandate/label/jurisdiction/fidelity/
  reserve checks for any other `jobType`
  (`platform/core/pkg/jobs/admission.go:84`). `pes_sweep`, `krylov_solve`,
  `mitigate`, `classical_anchor`, `claim_eval` spend bridge/sandbox work
  with no reservation. Threat: unattributed, unreserved spend on every
  non-circuit path. Fix: run the autonomy + reservation rail for all types
  (fidelity floor only where a device model exists; record the exemption
  explicitly otherwise — never a silent bypass).
- **W0.3 Tribunal + autoresearch-start skip autonomy checks [verified].**
  `requireDispatchAutonomy` is called only from launch (`pkg/api/server.go:1949`)
  and jobs (`:2242`) — never from `handleChatTribunal` (`:2062`) or
  `handleAutoresearchStart` (`:3112`). Threat: L0/suspended labs burn LLM
  and loop spend the money rail refuses. Fix: call it in both handlers
  (tribunal already 404s ghosts; add the 403).
- **W0.4 Mandate receipts are not bound to the job [verified].**
  `verifyMandateReceipt` checks tenant, budget, and expiry
  (`pkg/jobs/admission.go:171-200`) but never compares any job hash to the
  submitted QASM — any covering mandate admits any circuit. Threat:
  mandate reuse across jobs voids per-job authorization. Fix: bind
  `ComputeJobHash(QASM)` into verification (and make it cross-language
  canonical — today it is Go-`json.Marshal`-only, so Python can never
  match; see R1 in [[rea-adoption-plan]]).
- **W0.5 A2A admission trusts empty credentials.**
  `Verify("","",budget)` skips tenant/job binding
  (`pkg/agents/a2a.go:308`); zero/negative budgets unvalidated (`:331`);
  root tasks without scopes inherit all capabilities (`:262,288`); the
  client drops Mandate/Scopes/ParentTaskID (`:547-550`, client/server
  drift); nil `autonomyChecker` means unchecked (`:447`). Threat: the
  delegation rail — the spend boundary for collectives — admits what the
  HTTP rail refuses. Fix: require tenant+job binding, validate budget > 0,
  default-deny scopes, sync client/server fields (one struct), refuse when
  no autonomy checker is wired.
- **W0.6 Tool `submit_local_sim` bypasses the HTTP rails.**
  It relies on `admitJob` only — no autonomy check, no idempotency key, no
  disk gate (`pkg/api/agent_tools.go:295`). Threat: retried tool calls
  double-spend; also `get_job`/`recent_events`/`recall_memory` resolve
  ghosts to the default tenant (`agent_tools.go:336`, G7-class leak).
  Fix: route tool dispatch through the same guard chain (or document the
  tool rail as unprivileged with tests proving what it cannot do).
- **W0.7 Tenant arithmetic is fail-open.**
  `CheckSpend` passes negatives, `ReserveSpend(neg)` succeeds as a no-op,
  `ReleaseSpend` floors at zero — masking double-release
  (`pkg/tenants/tenants.go:308-331,366`). Threat: negative/zero amounts
  corrupt ceilings silently. Fix: refuse non-positive amounts at all three
  entries; make double-release an error, not a floor.
- **W0.8 Grant revocation is half-built.**
  `RevokedCIDs` stores only roots (`pkg/grants/grants.go:433-456`), so leaf
  revocation is dead although `VerifyChain` checks every CID (`:319`);
  attenuate skips TTL validation and doesn't require the attenuator to be
  the parent audience (`:161-220` vs `:122-124,347` — fail-late chains);
  `scopePath` collides `a:b` with `a/b` (`:403`); depth cap 8 vs A2A's 2
  (two caps, one system). Threat: revocation — the settled guarantee that
  makes delegation safe — doesn't propagate to leaves. Fix: store leaf
  CIDs, validate TTL/audience at attenuate time, one depth cap.

## Wave 1 — science-integrity defects (receipts must not lie)

- **W1.1 Harness stdout is agent-hijackable.**
  `parseHarnessOutput` takes the *last* `{...}` line as the metric
  (`pkg/autoresearch/harness.go:624-633`) — a workload that prints fake
  JSON mints its own score. Threat: self-graded science. Fix: parse only
  a fenced/anchored payload block (origest signed by the runner), refuse
  otherwise; add an adversarial fixture (malicious stdout → refuse).
- **W1.2 Krylov non-H2 numbers are invented.**
  Non-H2 returns `fci+0.05/(k+1)` with hardcoded `meyer_wallach=0.85` and
  `success=True` (`platform/bridge/krylov.py:86-105`); shots ignored,
  `subspace_dim` silently clamped, `time_step` ignored for non-H2.
  Threat: fabricated physics behind a success flag. Fix: fail-closed
  refusal for unimplemented molecules (or label `emulation` end-to-end
  including the receipt — never `success=True` on synthetic data).
- **W1.3 Mitigation math that proves itself.**
  CDR `mitigated==ideal` by construction (`scale=ideal/raw`,
  `mitigation.py:156-164`); PEC gamma ignores depth/qubits with a
  tautological bound (`:139-142`); `err_bound` invented (`:120-124`);
  fidelity probe returns a heuristic labeled measurement (`fidelity.py:97-106`);
  unknown devices silently default to `ibm_heron` (`fidelity.py:91`).
  Threat: each turns a measurement claim into arithmetic. Fix per the bar:
  real logic or fail-closed refusal + `measured:false` flags that survive
  to the receipt.
- **W1.4 Executor placeholders with `success=True`.**
  Pre-flight-partition and no-counts paths return synthetic counts
  (`executor.py:300,411`); unseeded `np.random.choice` (`:404`);
  fallback returns `success=True` *with* a non-empty `error_message`
  (`:427-439`); `kappa=4^K` uncapped (`:550`); >2q cross-partition
  controls silently dropped (`:544-547`). Threat: irreproducible,
  mislabeled results. Fix: seed everything, error iff success is false
  (never both), cap kappa or refuse, record dropped controls.
- **W1.5 QIR lowering emits invalid IR.**
  `measure q->c` drops the classical bit then emits an undeclared
  `@__quantum__qis__measure__body` (`qir-core/src/lib.rs:46-71,129-143`);
  arbitrary gate names emit calls with only fixed declares (`codegen.rs:
  118-133`); 3+q gates miscounted (`codegen.rs:39-64`); `emit_qir` and
  `emit_llvm_ir` diverge while only the latter ships (`main.rs:41`).
  Threat: the compiler core emits what LLVM cannot link. Fix: classical
  bit threading, closed gate-name set (refuse the rest), one emitter.
- **W1.6 Campaign synthesis placeholders.**
  The `h+rz+cx` QASM chain is a placeholder (`campaign_synth.go:427-440`);
  14-family physics duplicates the tribunal's 3-family set and they
  already diverge; milestones canned (`:538-550`); spend/backend always
  `0/local` (`:691-692`). Threat: generated campaigns present fiction as
  plan. Fix: single physics-constants source; mark every synthesized
  (non-measured) field `synthesized:true` through to the receipt.
- **W1.7 Replan folds that invent.**
  `circuit_run` forces `changed=true` for a provenance line alone
  (`replan.go:176`, version spam); anchor substring fallback miscollides
  (`LI`⊂`LIH`, `:236-244`); mitigate defaults print as measured
  (`:381-384`); `%.5f` hashing loses precision (`:562`).
  Fix: exact-match anchors, `measured:false` propagation, full-precision
  hashes.
- **W1.8 Bridge drops proto fields; proto under-specifies.**
  Server ignores `basis_set` and Krylov `requested_backend`
  (`server.py:103,173-179`); proto lacks `sampling_overhead_kappa`,
  `hardware_job_id`, `partition_fragments` (`kubona.proto:40-70`); claim
  responses have no `success/error` (`:99-109`); chemistry ADMET/pipeline
  have no RPCs despite a console path calling them. Fix: wire or refuse
  (fail-closed on unmapped fields), add the missing RPCs or delete the
  console path.

## Wave 2 — robustness + hygiene (outages, leaks, drift)

- **W2.1 Unauthenticated mutation surfaces.**
  `ledger/append` + `supervisor/abort` take no auth and no session check
  (`provenance.go:36,82`) — anyone seals `tribunal_commit` or aborts;
  `metasearch/apply` nil-`prop` deref; HF routes no auth; canvas defaults
  to a shared `default` session with no tenant check; events limit uncapped
  (DoS); SSE streams with no session/auth, polling forever. Threat model
  each as reader-vs-writer per the bar: reads stay public behind limiters,
  writes get edge auth — then implement exactly that.
- **W2.2 Ghost writes and cross-tenant reads.**
  `AddMessage` never checks session existence (FK off — ghost orphans);
  chat GET/stream/regenerate auto-create on the DefaultTenant-150 path,
  bypassing G7 strictness elsewhere; epistemics falls back to `sessions[0]`
  (cross-session leak); `ListJobs("",50)` lists all tenants; job/event
  reads lack tenant checks. Fix: session-existence gate on every write;
  tenant scoping on every read (the G7 rule, completed).
- **W2.3 Network posture gaps.**
  Non-loopback plain HTTP only warns (`main.go:238`); XFF trusted only
  from loopback so proxied deployments share one rate bucket
  (`edge_auth.go:165`); limiter eviction fails open sharing budget
  (`:136`); rates hardcoded, no env/per-tenant (`server.go:256`);
  `KUBONA_CORS_ANY=1` unaudited; empty-Origin takes `allow[0]`.
  Fix behind the threat argument per change (never "consistency").
- **W2.4 CI contradicts its own pins.**
  `ci.yml:100` Go 1.23 vs `go.mod:3` 1.27.1; `ci.yml:106` Qiskit floors
  vs `pyproject ==2.5.2/0.17.2`; chemistry extra not installed in CI
  while pinned; `pennylane` + `scipy` missing from deps entirely;
  `pnpm install` without `--frozen-lockfile`; tag-pinned actions;
  no pip/Go/cargo cache; no `needs:` DAG (compounds R0 in
  [[rea-adoption-plan]] — do once, together).
- **W2.5 Fidelity/gate arithmetic.**
  2q-gate regex misses `cnot/cy/ch/rxx` — undercounting 2q gates
  *over-predicts* fidelity, the unsafe direction; qubits counted but
  unused in the decision; T1/T2 stored never used (tribunal `T2≤2T1`
  claim unchecked); floor logic duplicated in two functions; device
  profiles hardcoded with no date/version/source. Fix the regex first
  (safety), then single-source the floor, then profile provenance.
- **W2.6 Workflows trust holes.**
  `budget_ceiling_usd` stored but never enforced in `run`; missing
  template keys render `""` silently; `SESSION_ID_TOOLS` allowlist means
  new session tools silently miss injection; step failure aborts with no
  compensation; `status` guard racy (no lock); library versions never
  bump, `to_workflow` forks identity (fresh uuid), writes non-atomic.
  Fix per the bar: enforce or refuse (a ceiling that doesn't bind is a
  lie — either wire it to spend checks or delete the field).
- **W2.7 Secrets plumbing.**
  `VaultClient` is memory-only (restart wipes leases *and* revocations —
  a revoked lease resurrects on reboot); Go/Python Bao paths duplicate
  with differing slug rules; gateway `mount` overwrites servers wholesale
  (silent tool loss); audit/ledger failures swallowed; `RevokeLease("")`
  silently nil. Fix: persist revocations (or refuse to start without
  Bao in production — decide explicitly), one slug rule, mount-merge
  with conflict errors.
- **W2.8 Memory semantics.**
  `Recall` is substring search wearing a semantic name; reads mutate
  `LastAccessedAt`; `Decay` touches experiences only (others immortal,
  unexplained); `Reflect` hardcodes importance with no dedup (unbounded);
  per-`Retain` full sort is O(n log n). Fix: rename or reimplement
  Recall honestly; read-only reads; bounded reflect with dedup.
- **W2.9 Console candor.**
  `api.ts` swallows backend reasons into `[]/null/{}` (only launch
  surfaces text); `JOB_STATUS_LABELS` hand-mirrors the proto enum
  (drift risk — generate it); page auto-creates a session + aborts
  without auth headers; abort orphans the backend run (no cancel RPC —
  now exists server-side as `jobs/{id}/cancel`: wire the button);
  transcript export unredacted/uncapped; `includes('CO')` mislabels
  (`CO2`/SCOPE). Fix each with the receipt/test to prove it.
- **W2.10 Guard edge cases (S2/S3 follow-ups).**
  Disk blocked→OK skips warning (hysteresis hole); zero watch paths
  measure MAX (fail-open); sweeper skips unparsable `LastActivity`
  (leak) and sleeping loops keep reservations forever (I3 never landed);
  `persistLoopRow` only `println`s on failure. Fix: warning as
  intermediate state always; refuse empty watch sets; reservations
  release on sleep or block it explicitly.
- **W2.11 Idempotency leftovers (S1 follow-ups).**
  Non-PK claim errors masked as "already claimed"; orphaned leader never
  overwrites + live-serve/409-retry availability gap; unknown rail
  coerced to job; stale windows per-rail but await uses job default for
  legacy `''` rows. Fix each with a targeted test.
- **W2.12 Mandate/PQC hardening.**
  Field checks precede signature (mismatch oracle — verify sig first);
  `%.4f` budget rounding lets near-ceilings share a signature; key→tenant
  unbound; envelope has no TTL; AES-GCM nil AAD; decap errors oracle.
  Fix in the order that breaks attackers first (order, rounding, TTL).
- **W2.13 Dead/orphaned code to delete or wire.**
  `ifc-core` unwired (no callers — integrate or delete); `qir-core`
  `main.rs` panics (`expect`) and ignores unknown flags; `emit_qir`
  dead except tests (one emitter); `cutting.py` never imported by the
  server + `pennylane` not installed; `ansatz_search.py` dead via RPC;
  fidelity `predict/probe` imported but uncalled; ` SESSION_ID_TOOLS`
  drift; `getLatestEntryHashLocked` per-session chains (no global order —
  document as design, not drift).

## Sequencing

- **Wave 0 now** (W0.1–W0.8): money-irreversible, each independently
  landable; W0.2 (admission bypass) and W0.3 (autonomy on tribunal/loops)
  first — they are small diffs on the hottest path.
- **Wave 1 next** (W1.1–W1.8): receipts must not lie before R1 envelopes
  them — fabricated physics would get *sealed* otherwise. W1.2/W1.4 first
  (`success=True` on synthetic data is the sharpest lie).
- **Wave 2 after** (W2.1–W2.13): hardening in any order except W2.4 with
  R0 (same CI file, one pass) and W2.10 with S2/S3 follow-ups.
- **Relation to [[rea-adoption-plan]]:** R0 ⊃ W2.4 (do together); R1
  needs W1.2/W1.4/W2.5 fixed first (don't seal fiction); R2 needs W0.8
  (revocation must propagate to leaves first); R3 needs W2.2 (tenant
  scoping before provenance weighting); R5 needs W2.9-console-catalog
  pieces. P1-4 consolidation still last.
- **Explicitly not here:** new features, new surfaces, new providers.
  Every item above removes a lie, a hole, or a bypass — nothing adds
  capability.

Related: [[rea-adoption-plan]] · [[rea-conflicts-and-remediation]] ·
[[cloudroom-adoption-plan]] · [[implementation-gaps]] ·
[[spend-mandates-and-delegation]] · [[cost-governance]] ·
[[quantum-advantage-realism]] · [[fidelity-accounting]] ·
[[provenance-and-reproducibility]] · [[risks]] · [[questions]]
