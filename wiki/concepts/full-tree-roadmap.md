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
  > ⚑ Resolution 2026-10-17 (W0.8 landed): leaf CIDs now revocable
  > end-to-end (route accepts `leaf_cid`, `RevokedCIDs` reads both
  > fields, `VerifyChain` already checked every envelope); `AttenuateGrant`
  > requires issuer == parent audience + TTL within (0, 90d] + non-empty
  > `/`-free scopes (same at `MintGrant`); the last-50 `grantingTenantFor`
  > limit is gone. The two depth caps stay separate by decision: UCAN
  > attenuation levels (8, DoS bound on presented chains) and A2A delegation
  > trees (2, agent-spawn bound) are different chains with different threat
  > models — merging the numbers would conflate them. Both consts
  > cross-reference each other.

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

## Gap-sweep addendum (2026-10-17)

> Six parallel full-read passes over every file the first pass missed
> (Go: epistemics/qec/qir-compiler/sandbox-output/supervisor/tools/triage/
> watchdog/sweep/validate/ledger-verify/cost-payment/llm-config-client-history-
> tools/branch-manager/hf-hub/ifc-lattice/grants-CLI/verify-CLI/copilot/
> memory-writeback/provenance/task-executor/grants-routes; Python: all of
> security/core-auth/mcp/gateway/session/control/governance/identity/cost/
> jobs/labs/ledger-negative/library/circuits/anchors/mitigation/qir/cutting/
> epistemics/science/fidelity/advantage/autoresearch/eval/agents/canary/chat/
> dialogue/discovery/integrations/intelligence/llm/logging/memory/mesh/mcp-
> servers/orchestration/providers/reports/routing/run/sandbox/schema/sdk;
> console: all sovereign/surfaces/widgets/lib remaining; bridge: ansatz/
> chemistry/epistemics/literature/test-suite + all three domain packs).
> Numbering continues the Waves above.

### Wave 0 continued

- **W0.9 Python mandate path is unsigned fiction.**
  `cost/mandate.py:52-70` sets `signature=""` and `verify_mandate_integrity`
  checks only expiry+positivity while the docstring claims operator signing;
  `governance/delegation.py:43` `_sign` is salted-SHA256 with hardcoded
  source-visible secret and exposes no verify at all; payload/rounding/key
  types all differ from Go, so no Python mandate passes Go `Verify` and
  vice versa. Threat: the Python-side spend gate is open while labeled
  closed. Fix: one canonical mandate (adopt Go bytes), real Ed25519
  sign/verify, refuse empty signatures.
- **W0.10 `cost/governance.py` approve() invents its bindings.**
  Hardcoded `tenant_id="default-tenant"` + synthetic `job_hash=f"job-
  {approval_id}"` (`:94-117`); `REVIEW` directly approvable (HOLD skipped);
  no amount validation (negative/NaN enqueued); `reviewer` free-text;
  approve+reject race (last-line-wins); burn-rate divides by fixed 60s.
  Threat: unbound mandates + nondeterministic money decisions. Fix:
  propagate tenant + `ComputeJobHash` over real params, enforce
  REVIEW→HOLD→APPROVED, lock the state machine, verified principals.
- **W0.11 `jobs.py` cancel is a label.**
  `cancel()` relabels while the worker thread keeps spending (`jobs.py:
  143-151`); `transition()` allows any→any incl. `cancelled→completed`;
  in-process lock only (double-run across workers). Threat: operator
  believes QPU spend stopped while charges accrue. Fix: cooperative
  cancellation + provider cancel, allowlisted transitions, cross-process
  lock.
- **W0.12 Python payment rails fail open.**
  `cost/payment.py:81-83` routes negative amounts to free tier; no
  proof↔quote binding or single-use; `cost/metering.py:59-65` charges a
  constant `$0.99`, and paid entries release on always-true verifiers.
  Threat: negative-price free jobs, replayed proofs, systematic
  undercharge. Fix: reject negatives, bind proofs, require configured
  verifier for paid goods, price from entry policy.
- **W0.13 God-mode defaults across Python auth.**
  `core/auth.py:17-23` Principal defaults to operator/`(*)`; `DevAuthProvider`
  has no in-class prod guard; `security/oauth.py:46` mints `("*",)` with
  huge TTLs on ephemeral keys; `security/policy.py:79-85` wildcard bypass +
  fail-open on missing registry; `config.py:48-60` dev-token whenever env
  isn't exactly `production`; `session.py:97-103` arbitrary `setattr`,
  negative spend accepted, ceiling TOCTOU. Threat: each is a full auth
  bypass by default or by one mis-set var. Fix: least-privilege defaults,
  fail-closed everywhere, allowlisted mutation, locked spend.
- **W0.14 Secure-RPC identity confusion + replay.**
  `secure_rpc.py:50-59` no nonce/expiry (replay re-executes spend);
  request/response share key+AAD (reflection); inner/outer token mismatch
  unconstrained; `http_gateway.py:68-104` authorizes on inner while
  authenticating outer, silent downgrade to plain, missing bearer
  forwarded. Threat: replayed + confused-deputy spend execution. Fix:
  envelope nonce+expiry+single-use, directional AAD, `inner.sub ==
  outer.sub`, explicit `--require-secure`.
- **W0.15 Agent registry + loop bypasses.**
  `agents/registry.py:57-59` `create()` skips the subset proof
  `create_as_agent` requires (god-mode by entrypoint choice); budgets/
  disabled never enforced; `agents/loop.py:85-115` defaults `dev-token`,
  renders allowlist into the prompt but never enforces it, no
  budget/session checks. Threat: model escapes to `execution/submit_job`
  on QPU. Fix: one creation path with proof, server-side allowlist +
  preflight per step.
- **W0.16 MCP cost/execution servers trust callers.**
  `mcp/servers/cost.py:27-95` no auth on approve, `verify_payment`
  re-quotes from caller amount (`$100` verified with `$0.01` proof);
  `mcp/servers/execution.py:121-171` implicit `$1M`-ceiling sessions,
  spend recorded only after 30s `wait()` (multi-hour QPU unrecorded),
  `jurisdiction=="none"` skips the gate for any misconfigured provider.
  Fix: owner+scope on approvals, verify against stored quote, server
  ceiling caps, `wait=False` for non-simulators, local-only allowlist.
- **W0.17 Go `VerifyPayment` never compares to the challenge.**
  `pkg/cost/payment.go:108` ignores `challenge.Token/Amount` (pay $0.01,
  settle as $100 — settlement uses `receipt.Amount` at `:186`); empty/
  negative challenges minteable (`:55`); `SettlePayment` always
  `Simulated:true` (`:194`) while callers may grant real QPU. Fix: exact
  challenge match, refuse empty/negative, gate real spend on
  non-simulated settlement.
- **W0.18 Delegation task mints its own budget.**
  `pkg/api/task_executor.go:40` hardcodes `100.0` on a caller-supplied
  tenant — invent tenants, mint $100 each; metered `zne`/`krylov` default
  `shots=1024` when `<=0`; krylov `dim/tau` and basis defaulted silently;
  `sealReceipt(...,nil)` unsigned. Fix: tenant from authenticated
  identity with central cap, require explicit metered inputs, sign
  receipts.
- **W0.19 LLM posture exfiltrates by default.**
  `pkg/llm/config.go:57` defaults `AirGapped:false` with cloud OpenCode
  active; missing `TotalTokens` bills zero (`client.go:170`);
  `ParseDSMLInvokes` turns model text into executable calls
  (`pkg/llm/tools.go:189`, prompt-injection executes spend tools).
  Fix: default air-gapped, fail-closed/ceiling-estimate on missing usage,
  DSML behind the same allow-list + confirmation.
- **W0.20 Grant CLI + lookup holes.**
  `cmd/kubonad/grants.go:90` empty scopes mint (`Split("",",")→[""]`);
  no TTL cap despite the 90d comment (`:86`); `grantingTenantFor` scans
  last 50 `grant_issued` so old grants become irrevocable
  (`pkg/api/grants.go:142`). Fix: require scopes, enforce TTL cap, index
  by `root_cid`.
- **W0.21 Supervisor/watchdog lifecycle gaps.**
  `supervisor.go:288` `Resume` revives killed/completed; `Pause`
  overwrites any status; transitions never persist to `loops`
  (DB/memory divergence); stall sweep never persists; watchdog
  fingerprint is global not per-session (infinite re-wake) with no
  `ORDER BY`; `FAILED` jobs keep subtrees "unsettled" forever. Fix:
  terminal-state machine + persist every transition + per-session
  fingerprints.

### Wave 1 continued

- **W1.9 Go QIR compiler drops gates silently.**
  `nativeLower` emits only `h`/`cx`, dropping everything else while
  claiming `BaseProfile` (`pkg/qir/compiler.go:183`); `TCount:0`
  hardcoded on the rust path (`:102`); `numQubits==0 → 2` invents qubits
  (`:137`); fallback `Depth=gateCount` mislabeled (`:145`); no QASM
  validation before lowering (`:43`); `exec.Command` without timeout.
  Fix: refuse unhandled gates, parse or omit T-count, validate first.
- **W1.10 Tribunal + wavefunction gates are permeable.**
  `ConcludeTribunal` ignores `Lean4Verified` (`continuum.go:290`); one
  vote reaches 100% quorum (`:259`); `weight<=0 → 0.5` inflates quorum
  (`:269`); no phase-machine enforcement (`:236`); 48-bit session IDs
  collide-silently (`:181`); wavefunction stores caller pointers
  (`wavefunction.go:78`), passes negative deltas (`:76`), accepts
  negative amplitudes (`:64`). Fix: require verification for commit,
  persona-diversity floor, ordered transitions, deep copies, domain
  checks.
- **W1.11 QEC NaN + silent substitutions.**
  NaN passes every claim check (`claim.go:28-40`); `FitLambda` returns
  NaN nil-error (`:74`); `p<=0 → 0.001` invents physics (`estimator.go:
  72`); `TargetLogicalInf` never compared (`:22`); negative acceptance
  means 1.0 (`:79`). Fix: refuse non-finite everywhere, compare the
  target, refuse negatives.
- **W1.12 Ledger verify trusts unsigned history.**
  Unsigned events check linkage+hash only — recompute hashes and history
  rewrites verify clean (`verify.go:73`); `|`-delimited recomputation is
  ambiguous (`:66`); chains from the reported hash after a fault (`:88`).
  Fix: require signatures on commit-class events (with R1), canonical
  bytes matching the signer, chain from recomputed digest on fault.
- **W1.13 Copilot sanitizer censors + drops.**
  `"MUST"` substring match censors legit science (`copilot_response.go:
  48`); `need [a-z]+` opener drops valid prose without digits (`:56`);
  only `"length"` counts as truncation (`:183`); multi-proposal loss
  silent (`:68`). Fix: phrase matching, all truncation reasons,
  extract-all or refuse-multiples.
- **W1.14 Validate gate mis-aris.**
  `ch:{2,1}` and `u0:{1,0}` arities are wrong — valid QASM refused,
  invalid admitted (`validate.go:80,87`); `OPENQASM 3` substring match
  on the whole file; block comments un-stripped. Fix: correct arities
  (`ch:0`, `u0:1`), header-line check, strip both comment forms.
- **W1.15 Python circuit stack fabrications.**
  `model.py:39` `round(p,10)` collapses distinct params to one ledger
  hash; 64-bit content-address; `depth()` returns gate count; `parser.py:
  29` silently drops `measure` (measurement semantics lost);
  `krylov.py:123` `is_converged=True` unconditional incl. degraded;
  `shadows.py:119-120` returns `0.0` on empty; `cutter.py:53-60`
  understates K (one cut per boundary) so `4^K` reads far low.
  Fix each: exact repr + full hash, refuse-on-measure or record,
  computed convergence, raise on empty, circuit-aware cut count.
- **W1.16 Chemistry anchors are wrong, not just heuristic.**
  `bridge/chemistry.py:50` CH4 tetra coordinates are not tetrahedral
  (systematic wrong anchor); `:47` NH3 ignores bond distance (R-sweep
  returns one energy labeled many); `:108-140` CASCI fallback stored as
  `fci_exact_energy` with no method flag (poisons the advantage check);
  no variational-bound guard; unknown molecules invent anchors
  (`anchors/chemistry.py:84-87`); docstring "Solves RHF/FCI" over table
  lookup; `math_physics.py` returns `certified/verified True` + `"rigorous-
  mathematical-proof"` from arithmetic (`:363-386`), synthetic QAOA
  model (`:372`), contradictory Laughlin gaps (`:526` vs `:100`).
  Fix: correct coords + regression test, scale-or-refuse R-sweeps,
  `method:` flags end-to-end, refuse unknowns, evidence tiers not proof
  claims.
- **W1.17 Noise/mitigation math that proves itself.**
  `mitigation/backend.py:58-88` per-qubit depolarizing breaks trace,
  hidden by clip+renormalize; seed not echoed (unreproducible);
  `zne.py:30-39` folding repeats G (different unitary, not `CC†C`);
  same seed every scale (correlated arms); mitigated unclipped to
  physical range. Fix: proper channels + trace asserts, true unitary
  folding, per-scale `SeedSequence`, physicality flags.
- **W1.18 QIR pipeline emits wrong unitaries.**
  `qir/pipeline.py:22-25` u3 decomposition order reversed under
  list-order application (silently wrong unitary, `ok:True`); `native.py:
  54` degrades real compiler failures to the Python reference silently;
  `cutting/wire.py:48-49` drops all CX involving the control + spanning
  multi-qubit gates ("exact" claim false); `seed=7` never threaded
  (`:106`); empty counts succeed (`:85`). Fix: unitary-verified orders +
  tests, degrade only when binary absent, proper partitions or refuse.
- **W1.19 Credibility arithmetic as wavefunction.**
  `bridge/epistemics.py:30-38` weights/caps invent α/β/γ; advantage
  assumes the anchor is FCI-exact (`:68-74`, cross-breaks with W1.16);
  `"advantage" in s` substring gate bypassable (`:65`); NaN propagates
  as success (`:48-61`); `claim_grammar.py:122` substring verb check
  (`reached`⊂`breached`), caller-supplied delta never recomputed
  (`:90-94`), 0-shot `QPU_MEASURED` passes (`:77`). Fix: heuristic
  labels + consts, `anchor_method==FCI-exact` requirement, word-boundary
  matching, computed deltas, cross-field shot requirements.
- **W1.20 Heuristic spend-gates + fiction as control.**
  `bridge/ansatz_search.py:76-129` mints Meyer-Wallach/barren-plateau/
  MPS verdicts from closed-form arithmetic that gate QPU spend;
  `campaign.py:97-98` runs ZNE on an unrelated 1-qubit circuit as
  campaign "analysis"; `chat/dialogue.py:70-90` returns canned
  "FCI/RHF verified" on exception; `library/store.py:41-52` defaults
  `validated=True` and `best()` takes `max()` for minimization
  harnesses. Fix: measured-or-labeled gates, ZNE on the real artifact,
  `ok:False` offline, `validated=False` default + per-harness direction.
- **W1.21 "Immutable ledger" is plain JSONL.**
  `ledger/negative_results.py:1-5,38-74` claims immutable/cryptographic
  with no chain or signatures (refutations forgeable/erasable);
  exact-string Hamiltonian match misses variants (re-spend on known
  failures); defaults to `/tmp/kubona-ledger` bypassing `config`.
  Fix: hash-chained signed records or drop the claim; normalized
  matching; route through `config.state_path`.
- **W1.22 Tests that reward fabrication.**
  `bridge/test_bridge.py:92-150` requires live net (`fetched_live`)
  and pins heuristic outputs as physics; never asserts
  `method==FCI-exact` or unknown-molecule refusal; `canary.py:60`
  masks zero counts; `cost.py:88-95` verifies against caller amount.
  Fix: mock net + live-marked tests, assert labels not values, refusal
  tests, verify against stored quotes.

### Wave 2 continued

- **W2.14 `serve-gateway` never boots.**
  `run.py:123` uses `os` with no import — the production network
  entrypoint always raises `NameError`; `make_gateway` duplicates the
  mount table (already diverged, both omit `compile_circuit`).
  Fix: import + boot-proof for both paths, single builder.
- **W2.15 Sandbox escape routes.**
  `ALLOWED_MODULES` includes `kubona` (transitive `os`/`subprocess`);
  AST checks only `Import/Call(Name)` (importlib/getattr/dunder
  bypasses); child inherits full env (token exfil); Unix-only rlimits,
  no wall-clock/disk/net containment; uncapped stdout. Fix:
  deny-by-default import hook, scrubbed env, caps + containment for
  prod (or gVisor/Firecracker).
- **W2.16 Model-hub path traversal.**
  `mesh/hf_hub.py:242` `target = dest / filename` with unsanitized
  filename (`../../.ssh/authorized_keys` escapes); no size cap/quota;
  SHA-256 computed but never verified. Fix: reject separators +
  containment check, max-bytes gate, verify-against-pin before rename.
- **W2.17 Silent resets + fail-open inference.**
  `settings.py:18-22` corrupt settings swallowed (backend switches
  unlabeled); `public()` denylists instead of allowlists;
  `llm/factory.py:22-32` unknown provider silently returns ScriptedLLM
  (fake reasoning as model output); `session.py:68-75` one corrupt line
  bricks all sessions. Fix: quarantine + fail-closed loudly, allowlist
  public keys, refuse unknown providers, skip-and-count corrupt lines.
- **W2.18 Provider silent downgrades + billing defaults.**
  `qiskit_provider.py:81-91` token+no-backend silently runs Aer
  (quantum-washing); `tket_provider.py:50-52` defaults to `H1-1`
  (bills real hardware unchosen); pyquil/tket cache secrets (stale
  tokens) vs qiskit fresh; `local_simulator.py:99-108` per-shot linear
  scan is O(shots·2ⁿ) DoS. Fix: refuse unless explicit fallback flags,
  fresh secrets everywhere, vectorized sampling.
- **W2.19 Console mislabels + unauthenticated writes.**
  `AutonomyCockpit.tsx:17-68` invents a `$0.12` cap, authorizes as
  `tenant-default` with no session, invents spend locally, reports
  verify-only as "Dispatched"; `CryptographicMandateCardWidget:53-62`
  displays "Authorized & Signed" without ever verifying;
  `EpistemicTribunalRoom.tsx:180` raw ledger POST without auth headers;
  `useTelemetry.ts:49` EventSource can never send Bearer (perpetual
  refused loop); seven gallery widgets render static demo constants
  (FeMoco, Bloch, 99.2%) as session content; `mandates.ts:53-55`
  fail-open tenant/job defaults; `approvalPacket.ts:28-61` substring
  backend classification + hardcoded shots + clamped overspend.
  Fix: one `authorizeSpend()` helper with session binding + priced
  packets, verify-or-label-local, shared API helpers with auth,
  demo watermarks or live props, explicit tiers.
- **W2.20 Go remainder hardening.**
  `branch_manager.go:160` `clean -fd` wipes shared repoDir + unsanitized
  `..` traversal; `hub.go:287` hardcoded download counts;
  `client.go:152` unbounded reads; `memory_writeback.go:445` unknown→
  `default` tenant pollution; `tools.go:26` Capability/ReadOnly never
  enforced; HF cancel TOCTOU; `lattice.go:121` multi-principal labels
  unparseable (deny-all); `verify.go:18` relative default DB path.
  Fix per item: isolate+sanitize, label estimates, bound reads,
  fail-closed writes, enforce or delete fields, round-trippable labels.
- **W2.21 Drift-proof shared values.**
  Gate-modality sets (`canary:183`⇔`conformance:36`), 2q sets
  (`provider:96`⇔`execution:40`), `Modality` enums (canonical
  `schema/provider:19` reused by taxonomy vs rogue `model_router:26`),
  gateway mount tables (`_mount`⇔`run`), `AgentRegistry`×3 paths,
  LLM port maps×3, secret policies (fresh vs cached), price/threshold
  consts (approval `$5`, estimator epsilon, watchdog WBP doc-vs-code
  10× drift). Fix: single const per value, canonical imports.

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

## Landed 2026-10-17 (Waves 0–2, all items)

> Every item below landed with tests + suite-green on all four toolchains
> (Go `go test ./...`, Python 636 passed, bridge 41, console 103 + build,
> Rust qir-core 13). Commit SHAs are the evidence; wiki pages updated where
> the design record needed it.

- **Wave 0 (21 commits):** W0.1 single-source lifecycle sets (`052f9fe`) ·
  W0.2 spend rail for all job types (`79fd0ce`) · W0.3 tribunal/autoresearch
  autonomy (`9a41bb9`) · W0.4 canonical v1 job binding, Go+console
  (`ca47271`) · W0.5 A2A delegation rail (`8ed604d`) · W0.6 tool-rail
  session gates (`9a8fb58`) · W0.7 fail-closed spend arithmetic (`a69dc27`) ·
  W0.8 grant leaf revocation + mint checks (`6bd9d0a`, depth caps stay
  separate by decision — different chains) · W0.9 real Ed25519 mandates,
  cross-language vector on all three sides (`f4ce972`) · W0.10 approval
  gate stages/lock (`c9713ac`) · W0.11 job farm (`ece9dc6`) · W0.12 payment
  rails (`60aaa54`) · W0.13 auth defaults (`0913a3e`) · W0.14 secure-RPC
  (`2ac84e1`) · W0.15 agent creation (`5481e4d`) · W0.16 execution
  discipline (`0ed5c64`) · W0.17 payment match (`22a5f85`) · W0.18
  delegation binding (`0a51edc`) · W0.19 LLM metering (`e579e1d`+`2eea081`)
  · W0.20 grants CLI (`8e60380`) · W0.21 loop machine (`35d9ea6`).
- **Wave 1:** W1.9 Go QIR closed set (`ae95718`) · W1.10–W1.14 Go science
  gates (`67178fd`) · W1.1 harness fences (`575e1d3`) · W1.5 Rust valid-IR
  (`4bf751d`) · W1.6 molecule table (`cca1764`) · W1.8 proto fields +
  bridge-success honored (`608f582`) · bridge W1.2–W1.4, Python W1.15–W1.22
  science fixes, and console candor — implemented and verified in parallel
  batches, landed inside the Wave-2 slice commits below (one tree, no
  uncommitted residue; suite-green at every step).
- **Wave 2:** Go infra batch + Python security batch + console/CI
  (`9bd9ada`, `7179bad`, `f69a037`). Deliberate deviations recorded:
  UCAN vs A2A depth caps stay separate (different threat models); steering
  verbs need no scope map (not MCP tools); asymmetric-QUBO labeled not
  refused (upper-triangular is convention); local-sim t1/t2 kept
  (schema-declared, canary-pinned); ADMET RPCs absent with no callers
  (nothing to add); test_qir lower-case intrinsic updated to the canonical
  cnot name.
- **Residuals for later (stated, not hidden):** X402 persistent
  single-use needs ledger backing (in-memory now); failed-job spend has no
  audited refund primitive; loop reservations are I3-future (guarded, not
  implemented); MCP control tools need principal threading for
  tenant-isolation; Python mandate interop with Go verification is
  byte-compatible but unwired end-to-end; emulator thresholds are v1
  heuristics throughout.

Related: [[rea-adoption-plan]] · [[rea-conflicts-and-remediation]] ·
[[cloudroom-adoption-plan]] · [[implementation-gaps]] ·
[[spend-mandates-and-delegation]] · [[cost-governance]] ·
[[quantum-advantage-realism]] · [[fidelity-accounting]] ·
[[provenance-and-reproducibility]] · [[risks]] · [[questions]]
