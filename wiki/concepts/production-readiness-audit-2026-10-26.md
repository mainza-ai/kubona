---
type: concept
name: Production Readiness Audit — Round 2 (2026-10-26)
date: 2026-10-26
updated: 2026-10-26
tags: [audit, production, security, money, honesty, ci, plan]
sources: [source-architecture-paper, source-gap-analysis-plan]
status: tracked
---

# Production Readiness Audit — Round 2 (2026-10-26)

> **What this page is.** An independent, full-source audit of the platform at
> `da800ea`, performed from code rather than from prior claims. It is **not** a
> re-litigation of [[full-system-audit-2026-10-26]] (Round 1, live-stack) or
> [[audit-remediation-plan]]; where a finding overlaps, it is marked. Round 1
> fixed the issues it exercised through the UI/API. This round read the code
> behind those paths and the paths Round 1 did not exercise — and found that the
> **trust boundary and the money rail are not production-grade yet**, while
> several headline claims are false at HEAD.
>
> **Verdict: NOT production-ready.** There are 10 P0 blockers: one unauthenticated
> bridge that can spend real QPU money, four unauthenticated Go control routes
> (including ledger forgery), an escapable Python sandbox, a charge-before-dispatch
> money rail, a release gate that is red and unprotected, a physically wrong
> chemistry anchor that the test suite was edited to bless, and several
> "verified" product surfaces that fabricate evidence. The architecture is sound
> and much of the hard work is genuinely good; the failures are concentrated at
> trust boundaries, settlement, and claim honesty — exactly the places the
> production bar is about.

## Method and coverage

- **831 in-scope tracked files read** across `platform/` (Go 241, Python 421,
  TS/TSX 133, Rust 4, proto 2, SQL 1, configs/contracts/docs), plus the 7
  operational scripts and both workflows. Generated bindings were structurally
  checked (36 messages / 16 RPCs, no field-number drift) rather than line-read.
- **Excluded:** `dev-docs/` (gitignored scratch), `node_modules/`, `.next/`,
  `platform/.venv/`, `target/`, binaries, and image blobs.
- **Executed:** `go build ./...`, `go vet ./...`, `go test ./...` (all green),
  `go test -race` on ledger/session/tenants; `pytest` fast slices (1,053
  collected) and the three failing modules; `vitest` (56 files / 272 tests),
  `tsc --noEmit`, `eslint`, `next build`; `cargo test` (23) + `clippy` (clean);
  `wiki-lint --strict`; Docker/compose inspection; live GitHub API checks.
- **Reproduced independently:** the sandbox escape (native libc call + host file
  read), the H2 anchor returning the same energy at R=0.7414 and R=2.0, the
  three red Python tests, the console lint failure, the baked bearer in the
  client bundle, 9 open Dependabot alerts, the failed CI run's job list, and
  branch protection being disabled.
- **Not exercised (honest limits):** live QPU vendors (no keys), live Atlas/
  billing (no operator run this round), real browser WebGL/perf, and Docker
  rebuild (the image failure is proven from the build context + CI log, not
  re-run locally).

## Verdict by domain

| Domain | Grade | One-line verdict |
|---|---|---|
| Go API/authz | **F** | Per-handler authz, multiple unauthenticated state-changing routes, refund envelope unverified. |
| Go domain logic | **C** | Strong admission/spend CAS and tool registry; ledger metadata unsigned, receipts trusted, money is float. |
| Python core/MCP | **D** | Real governance primitives, but spend-before-dispatch, dead idempotency, caller-declared tenants, approvals unwired. |
| Python science | **D** | Real algorithms in places; a wrong anchor, a pass-everything comparator fallback, and fabricated manuscript/UI evidence. |
| Bridge/proto | **F** | Unauthenticated `[::]:50051`, caller-asserted budget/tenant, hardware bypass, statevector OOM. |
| Rust | **B−** | Clean, safe, tested algebra/lowering; QASM parser panics and measurement lowering is wrong. |
| Console | **C−** | Strong pure helpers and honest status patterns; authorization is theater, token can be baked, lint red, no CSP. |
| CI/verification | **F** | Red at HEAD, no branch protection, Docker cannot boot, coverage/lint/SAST absent. |
| Docs/claims | **D** | Several load-bearing claims are false or stale (routes, tools, "green", "gVisor", "0 open questions"). |

---

## P0 — stop-the-line blockers

### P0-1 · The bridge is an unauthenticated, caller-authoritative compute and money plane
**Evidence.** `platform/bridge/server.py:648-650` binds `[::]:{port}` with
`add_insecure_port`; UDS mode does not suppress TCP, and `chmod 0600` happens
*after* `start()` (`:652-656`). The Go client dials with
`insecure.NewCredentials()` (`platform/core/cmd/kubonad/main.go:244`). The
servicer trusts wire fields: `session_verified` + `budget_ceiling_usd` +
`spent_usd` + `tenant_id` are written into the Python session
(`platform/bridge/server.py:251-267`, `platform/kubona/session.py:270-279`).
`ExecuteCircuit` routes `requested_backend` straight to
`_execute_ibm_hardware` (`platform/bridge/executor.py:428-439`) with no
`submit_job`, spend, session, or idempotency rail. `WorkflowRun` builds
`build_gateway(require_auth=False)` and falls back to `"dev-token"`
(`platform/bridge/server.py:588-593`), exposing every mounted MCP tool.
Job status/cancel accept a bare `job_id` with no ownership check
(`platform/bridge/server.py:321-363`).
**Impact.** Any container or network peer can: submit circuits and burn IBM QPU
credits unrecorded; overwrite a session's ceiling/spend/tenant; run arbitrary
mounted tools including spend rails; read or cancel another tenant's job.
**Fix.** Bind loopback/UDS only and refuse to start if both are configured;
require mTLS or an HMAC interceptor with a per-process secret; delete
`session_verified` and never accept money/tenant fields from the wire; make
`ExecuteCircuit` simulator-only or route hardware through `submit_job`;
`require_auth=True` for every workflow/gateway construction; derive job
ownership from the authenticated principal.
**Effort:** M/L.

### P0-2 · Unauthenticated control routes in Go allow ledger forgery and free spend
**Evidence.** `/api/sessions/import` is registered without `requireBearer`
(`platform/core/pkg/api/server.go:424`); the handler imports caller-controlled
`SpendLimitUSD`, `CurrentSpendUSD`, `ledger_events.entry_hash`, `signer_pubkey`,
and `signature` verbatim (`platform/core/pkg/session/migrate.go:491-557`).
`/api/a2a/v1` and `/api/a2a/v1/challenge` are unauthenticated (`server.go:515-516`).
`/api/llm/test` is unauthenticated (`server.go:490`). `/api/loops`,
`/api/autoloops`, and the memory writeback routes are unauthenticated and do not
bind the caller to the session/tenant (`server.go:439-440`, `memory_writeback.go:309,384`).
`/api/mandates/pqc-verify` is public and appends a ledger event + obligation
(`server.go:549,1183-1192`).
**Impact.** An attacker can forge a "tamper-proof" ledger (including
`mandate_verified_client` receipts that the admission gate trusts), import a
session with a negative/zero spend and another tenant's ID, spend cloud LLM
tokens anonymously, kill another lab's loops, read its memory, and bloat the
ledger. This is the single largest trust-boundary failure in the platform.
**Fix.** Central route policy (method × path × auth/tenant/autonomy) rather than
per-handler memory; import becomes operator-only with signed, chained sections
and a post-import `VerifyChain`; A2A requires a principal + tenant; LLM test is
bearer-gated and budgeted; loop/memory routes resolve the session and enforce
tenant; PQC verify is read-only or gated.
**Effort:** L.

### P0-3 · A2A accepts self-signed authority and a self-signed "payment"
**Evidence.** `handleA2A` only requires an `Authorization` header for
`a2a.SubmitTask`, then verifies the receipt against the **caller-supplied**
public key (`platform/core/pkg/cost/payment.go:183-186`); the challenge is
`$0.05` and settlement is local (`payment.go:270 Simulated: true`). A root task
defaults `tenantID := "default"` (`platform/core/pkg/agents/a2a.go:295`) and
`pm.Verify` checks the embedded key only (`a2a.go:362`,
`mandates/mandate.go:85`). Submit spawns an unbounded goroutine (`a2a.go:486`).
**Impact.** Anyone can mint a key, self-sign the challenge, submit a task, and
bill the default tenant up to the declared budget. Parent-mandate checks do not
establish a trust root, so delegation can claim any tenant.
**Fix.** Bind challenges to payer + task + amount and verify real settlement (or
disable A2A submit publicly until the rail is staffed — Q-23/Q-55); verify
mandates against the tenant's registered admin key; require a principal; bound
concurrency. Keep the existing honest "simulated" label but stop treating it as
authorization.
**Effort:** M/L (settlement) or S (disable).

### P0-4 · The Python code sandbox is escapable to native code and the host filesystem
**Evidence.** `platform/kubona/sandbox/execution.py` allowlists `numpy`/`scipy`,
blocks dunder attributes, then runs a plain subprocess with only
`RLIMIT_CPU`/`RLIMIT_AS` (`:204-283`). Reproduced:
`numpy.ctypeslib.ctypes.CDLL(None).getuid()` → `UID 506`;
`numpy.fromfile('/etc/hosts', ...)` → host file bytes. Output capping happens
*after* `communicate()` buffers everything (`:284-290`); `RLIMIT_NPROC` is not
set despite the docstring; `killpg` can be defeated by `setsid`/fork.
**Impact.** Agent-authored code executes with the bridge user's full filesystem
and network access. The AST filter is defense-in-depth, not isolation; this
contradicts the Q-48/gVisor claim (no `runsc` call exists).
**Fix.** Run workloads in a real container (`runsc` or Docker: `--network none`,
read-only root, `--cap-drop=ALL`, nonroot, pid/memory/cpu limits, tmpfs workdir)
with a streaming output cap and `RLIMIT_NPROC/FSIZE/NOFILE`; keep AST as an
inner check. Alternatively, remove arbitrary Python execution from the agent
surface until the container rail is mandatory.
**Effort:** L.

### P0-5 · Spend is charged before dispatch, idempotency is dead, and failed jobs are not refunded
**Evidence.** `execution.py:401-407` calls `record_spend` **before** the job is
created/enqueued; enqueue failure (`:441-444`) and worker failure leave the
charge. The comment at `:399` still says "there is no audited refund primitive
yet", and nothing calls `SessionStore.release_spend` on failure.
`find_by_idempotency` exists (`jobs.py:192`) and is called by the bridge
(`bridge/server.py:234`) but the key/body hash is created afterward
(`execution.py:415`), the lookup is unlocked/racy, and `body_hash` is never
compared. `test_execution_discipline.py:23-30` asserts a no-credential
`ibm-qiskit` submit returns `ok:true/queued` — codifying the bug.
**Impact.** Every pre-vendor failure (missing credential, missing SDK, provider
refusal, full queue) is billed. A retry after timeout/restart creates a second
job and a second charge. This is a direct money-loss bug in the production rail.
**Fix.** Atomic insert-or-get of the idempotency key before preflight/spend;
compare the full canonical body; reserve (not commit) spend, commit only on
vendor acceptance, and refund on any pre-acceptance failure with a sealed
ledger event; require a positive finite ceiling. Invert the test to assert
refusal/zero-charge.
**Effort:** M.

### P0-6 · The session refund route does not verify the admin signature
**Evidence.** `/api/sessions/refund` requires a bearer (`server.go:422`) but
`bindAdminAction` only checks tenant/action/params hash
(`api/tenants.go:73-84`); `SessionManager.RefundSpend` checks tenant equality
only (`session/session.go:516-523`). The tenant refund path correctly calls
`verifyAdmin` (`tenants/tenants.go:252`).
**Impact.** Any accepted credential (including a lab cookie) can construct an
envelope with matching params and a bogus/empty signature and refund any known
session — including one just reserved — defeating the ceiling and mutating the
books. This is asymmetric with the tenant rail and is exactly the "money
movement needs a who and a why" invariant the code claims.
**Fix.** Call `VerifyAdminAction` before `RefundSpend`; reject empty signatures;
bind the caller to the session tenant; add forged-signature and cross-lab
negative tests.
**Effort:** S.

### P0-7 · The release gate is red, unprotected, and the production image cannot boot
**Evidence.** GitHub API: `main` has `protection_enabled:false`,
`checks:"off"`. CI run `40297011884` (HEAD `da800ea`) failed: Python fast lane,
console, Docker, shards F/G, canary skipped. The last full success was
2026-10-17; the last 100 runs are 99 failures / 1 cancelled (dependency-update
runs are separate). The Docker failure is structural: `Dockerfile.kubonad`
builds with `context: ../core` and copies only the binary
(`platform/docker/compose.prod.yml:46`, `Dockerfile.kubonad:18,29`), while
`loadFidelityContract` looks for `platform/contracts/fidelity-devices.json`
and **panics** if absent (`core/pkg/fidelity/accounting.go:56-73`). Console
lint is red (`FlightRecorderCard.tsx:37,53`). Catalog drift is red: 37 live MCP
tools vs 36 catalogued (`mcp/servers/_mount.py:42-50` vs
`contracts/mcp-catalog.json`); `test_mcp_catalog` and `test_mcp_surface` fail.
`plan_dual_model_packing` is public but absent from `codesign.__all__`
(`codesign/packing.py:83`, `test_codesign_audit.py:266`).
**Impact.** There is no gate: broken code can land on `main`; the published
artifact does not start; "CI green since 2026-09-29" is false. This makes every
other fix unverifiable in deployment.
**Fix.** Embed the fidelity contract (`go:embed`) or ship `contracts/` in the
image; regenerate the MCP catalog from the live surface and derive the count;
export/rename the packing symbol; fix the two console lint errors; rebalance
shards F/G and add per-test timeouts; then **require all 16 checks + branch
protection** before any further claim.
**Effort:** M.

### P0-8 · The H2 scientific anchor is physically wrong and the tests were edited to bless it
**Evidence.** `platform/kubona/anchors/chemistry.py:87-93` pins
`rhf=-1.050`, FCI coefficients yielding `-1.0718`; `:125-147` ignores the
requested bond distance and diagonalizes the same fixed matrix. Reproduced:
R=0.7414 and R=2.0 both return `fci=-1.0718`. The correct values are
RHF `-1.116684`, FCI `-1.137270` (the repo's own
`eval/truncation.py` and `test_truncation.py:23` use `-1.137284`).
`test_claim_grammar.py:165-169` was rewritten to call `-1.137` "stale" and
assert the engine's `-1.0718`; `test_epistemics_and_anchors.py:84-87` repeats it.
**Impact.** Every campaign, claim example, manuscript, and advantage check
inherits a 65 mHa error — 40× the 1.6 mHa chemical-accuracy bar. The test suite
is falsified on the exact invariant it should protect.
**Fix.** Compute anchors live with PySCF for the requested geometry (or refuse
non-table R); replace the table values with independently verified references;
restore the `-1.13727` assertion and add an external-reference check; delete
the "stale" rationale.
**Effort:** M.

### P0-9 · The Lean/Comparator fallback reports success without reading the proof
**Evidence.** `platform/kubona/eval/comparator_eval.py:390-402` calls
`verify_axioms` with the spec's permitted list; `:242-263` only checks the
caller-supplied names. With no Docker/lake, it returns
`ok=True, verification_mode="static_ast_containment"` and *synthesizes*
`axioms_used=[propext, Quot.sound, Classical.choice]`. The test
`test_math_corpus.py:87-95` asserts `ok` in this path.
**Impact.** An unverifiable or `sorryAx` proof is indistinguishable from a
verified one; the platform's strongest scientific claim ("formal verification")
passes by absence of a toolchain.
**Fix.** Return `ok=False, mode="verification_unavailable"` when the toolchain
is absent; never synthesize axiom names; require a real transcript; invert the
test.
**Effort:** M.

### P0-10 · "Verified" product surfaces fabricate evidence
**Evidence.**
- 3D HUD defaults to `FCI -1.137` and `rho=0.88` with no UNVERIFIED marker
  (`console/src/components/surfaces/SpatialCanvas3D.tsx:143-144,1104-1111`).
- Lean tab renders hardcoded tactics, `rho=1.000000`, `0 sorryAx`
  (`Lean4GoalTreeInspectorWidget.tsx:60,78-81`).
- Manuscripts unconditionally claim "physical simulation yielded $99.0$" and
  Lean verification, and invent convergence energies
  (`science/manuscript.py:344-348,443-447`).
- HF "live" metadata returns a static fallback with no `estimated` flag
  (`core/pkg/hf/hub.go:291-295,198-205`).
- Fidelity "freshness" is computed as `now - age` at load, so it can never be
  stale (`fidelity/accounting.py:51-59`).
- Autoresearch embeds the benchmark nonce in a file the artifact can read and
  hardcodes `entanglement_score 0.72/0.85/0.88` (`autoresearch/harness.go:610-613,301`).
- The model adapter manifest declares absent weights, a SHA, and measured
  metrics (`platform/models/adapters/manifest.json:34-43`; only README+manifest exist).
**Impact.** This is the exact failure class the production bar forbids:
plausible chrome that reads as measurement. It is worse than a missing feature
because it survives review and feeds downstream claims.
**Fix.** Make every value nullable and render UNVERIFIED/estimated; require
receipts for Lean/proof views; condition manuscript text on computed evidence;
mark fallbacks at the API/UI; compute calibration from the recorded timestamp;
move the benchmark nonce out of band and replace hardcoded constants; mark the
model directory `spec-only` or ship the artifact.
**Effort:** M (per item S).

---

## P1 register (blocking for production, fix after the P0 gate)

### Authentication / authorization
| ID | Finding | Evidence |
|---|---|---|
| AUD-A1 | Loops/autoresearch management is unauthenticated and tenant-unscoped; ghost loops accepted; listings leak champion code | `api/server.go:439-440,3094-3103,4291-4304`; `supervisor.go:192` |
| AUD-A2 | Memory recall/retain does not bind the caller to the session/tenant | `memory_writeback.go:309,384-415` |
| AUD-A3 | Quantum job status/cancel skip the tenant check when the bridge omits `session_id` | `quantum_jobs.go:301-307,327-337` |
| AUD-A4 | Public `/api/tenants` exposes admin pubkeys, ceilings, spend; public custom agents expose system prompts | `server.go:601,5248-5251`; `tenants.go:54-62`; `agents/custom.go:16-35` |
| AUD-A5 | Public PQC verify mutates the ledger/obligations and is unthrottled; its "no state change" comment is false | `server.go:549,1183-1216` |
| AUD-A6 | MCP stdio servers bypass the gateway's token/policy entirely | `mcp/servers/execution.py:651`; `gateway.py:106-118` |
| AUD-A7 | Approval/MRTR/mandate machinery is not in the spend path; `require_approval` has no caller; minted mandates are dead audit data | `execution.py:352-405`; `gateway.py:97-104`; `governance.py:159-188` |
| AUD-A8 | Delegation revocation is loaded once per process; revoked agents remain authorized in other workers | `identity/registry.py:54-59,99-104` |

### Money and ledger
| ID | Finding | Evidence |
|---|---|---|
| AUD-M1 | All money is `float64`/SQLite `REAL`, with `1e-9` epsilons; no exact cents or upper bounds | `ledger.go:92-100`; `session.go:474-489`; `tenants.go:397-418` |
| AUD-M2 | LLM budget scans only the newest 10k events and fails open on DB errors; per-role cloud usage bypasses the active-provider check | `server.go:3462-3466,3501-3532,3506-3509` |
| AUD-M3 | Workflow reserves a client-declared ceiling, under-charges actual spend, fully refunds failed partial work, has no idempotency, and uses a static `dev-token` | `workflows.go:143-196,171` |
| AUD-M4 | Loop spend claim is read-then-zero (TOCTOU); concurrent drawdown can be silently zeroed | `ledger/loop_spend.go:67-78` |
| AUD-M5 | Ledger digest excludes `tenant_id`, `conf_label`, and `timestamp`; metadata can be rewritten without breaking the chain | `ledger/verify.go:41-43`; `ledger.go:374-375` |
| AUD-M6 | Session deletion deletes `ledger_events`, erasing the audit chain the platform advertises | `session/session.go:444-451` |
| AUD-M7 | External mandate signatures are over a different message than `VerifyChain` checks; every mandate receipt fails signature verification | `server.go:1183-1192`; `ledger.go:560-561`; `verify.go:98-117` |
| AUD-M8 | Mandate receipts are looked up by row, not verified; expiry is optional; any ledger writer can mint spend authority | `jobs/admission.go:241-271` |
| AUD-M9 | Tenant events are written under `session_id=tenant/<id>` and resolve to the `default` session | `tenants.go:139,210,238`; `ledger.go:333-341` |
| AUD-M10 | Python refund is unsigned/unledgered and not called on failure; the comment claiming no refund primitive is stale | `session.py:203-225`; `execution.py:399` |
| AUD-M11 | Payment quotes are per-process memory, not job-bound; a quote from worker A is unknown to worker B; `$0` verifies | `mcp/servers/cost.py:31,121-130` |

### Jobs, bridge, and resource safety
| ID | Finding | Evidence |
|---|---|---|
| AUD-J1 | Job runner closes its channel while submitters may send, causing a send-on-closed panic; a full queue can hang a handler | `jobs/jobs.go:180-184,315` |
| AUD-J2 | Idempotency in the bridge is check-then-submit (not atomic) and the key is not tenant-scoped; the body hash omits tenant/session/dry-run | `bridge/server.py:231-245` |
| AUD-J3 | Statevector `shots`/qubits are uncapped; 28 qubits exceeds the container memory; no gRPC message cap or deadlines | `bridge/executor.py:94,504-560`; `bridge/server.py:629-650` |
| AUD-J4 | `RunCanary`'s timeout is defeated by `ThreadPoolExecutor.__exit__` waiting for the hung future | `bridge/server.py:374-381` |
| AUD-J5 | Proto drops the honesty labels it computes (`method`, `measured`, `fetched_live`, `qpd_channels`, `license`) | `proto/kubona.proto:85-186`; `bridge/server.py:120-129,511-519` |
| AUD-J6 | `CutCircuit` is not a QPD (applies no gate, one Z basis) but returns `success=True`; the real PennyLane path is unused | `bridge/executor.py:656-770`; `bridge/cutting.py:38-56` |
| AUD-J7 | "PEC"/"CDR" are heuristic scalings, not the named methods, and the `measured:false` label is dropped on the wire | `bridge/mitigation.py:266-410` |
| AUD-J8 | Provider names interpolate into schema paths; an absolute/traversal name reads arbitrary JSON | `schema/provider.py:242-246`; `sdk/register.py:57-59` |
| AUD-J9 | `qir-core` panics on reversed brackets; measurement lowering records the qubit index, not the classical bit, and re-measures all qubits | `crates/qir-core/src/lib.rs:53,78,115`; `codegen.rs:151-182` |
| AUD-J10 | Subprocesses inherit the full environment (QPU tokens) and the Go QIR path accepts an arbitrary binary | `kubona/qir/native.py:88-90`; `core/pkg/qir/compiler.go:39-49,132-135` |
| AUD-J11 | Compose publishes plaintext `8080`/`3000`, no network segmentation, bridge reachable by the console; sidecar DB/Vault ports published; Hindsight has no data volume | `compose.prod.yml:29-30,69,72-73,104-105`; `docker-compose.hindsight.yml:5,14` |

### Python runtime and science
| ID | Finding | Evidence |
|---|---|---|
| AUD-S1 | H2 anchor wrong and R ignored (see P0-8) | `anchors/chemistry.py:87-147` |
| AUD-S2 | VQE "ground_energy" is the ansatz optimum, hardcoded, not `eigvalsh(H)`; QAOA is a curve, not an implementation | `autoresearch/harnesses.py:15,78`; `anchors/math_physics.py:384-416` |
| AUD-S3 | `KSD cutoff=0` returns NaN; all-singular fallback divides by a possibly zero mode | `circuits/krylov.py:178-227` |
| AUD-S4 | Shadows allows 20 qubits → TB-scale allocation | `circuits/shadows.py:48,85-128` |
| AUD-S5 | "Analytical channel-noise autodiff" is finite difference for `PhaseDamping`; only gate angles are analytic | `diff_stack/tape.py:3-5,56-95` |
| AUD-S6 | Library `best()`/`all()` ignore tenant; `validated` accepts any non-empty string; save is non-atomic | `library/store.py:127-154,183-187` |
| AUD-S7 | MetaScope persistence trusts `mutable` from JSON without re-verifying the signature | `autoresearch/meta_scope.py:129-133` |
| AUD-S8 | Manuscript signoff is accepted without verification; peer review treats a negative discrepancy as excellent (missing `abs`) | `science/manuscript.py:504-516,589-594` |
| AUD-S9 | Licence gate is substring matching (`CC-BY-NC` and `not-apache` pass); corpus loader overwrites built-in tasks | `eval/corpus_loader.py:90-126,167` |
| AUD-S10 | Materials domain pack points at a nonexistent module; revocation fails open on a malformed file | `domain_packs/materials_condensed_matter_v1/pack.yaml:10`; `science/domain_pack.py:153,222-229` |
| AUD-S11 | Load-bearing exactness invariants use `assert`, stripped under `python -O` | `codesign/divwalk.py:115-184`; `packing.py:207`; `eea12.py:352-353` |
| AUD-S12 | EEA `FLAG` misses non-constant gcd; adversarial inputs are not refused | `codesign/eea12.py:468-481` |
| AUD-S13 | Test suite pollutes real `~/.kubona` state (no conftest isolation) | `platform/tests/` (no `conftest.py`); `jobs.py:95`; `session.py:66` |

### Console
| ID | Finding | Evidence |
|---|---|---|
| AUD-C1 | Quantum quote/mandate is not bound to provider/QASM/shots/session and is not invalidated on edit; missing price renders as `$0` and skips the gate | `QuantumDispatch.tsx:125,185-204,283-301` |
| AUD-C2 | `ActionGate` authorizes nothing and is disconnected from every dispatch path | `ActionGate.tsx:74-80`; `page.tsx:337-342` |
| AUD-C3 | Production builds bake `NEXT_PUBLIC_KUBONA_CONSOLE_TOKEN`; only a warning | `api.ts:35,44-50`; confirmed in `.next/static/chunks/2osung7_ikc8p.js` |
| AUD-C4 | No CSP/frame-ancestors/nosniff/referrer/permissions headers | `next.config.ts:15-20` |
| AUD-C5 | Ed25519 key lives only in module memory; reload mints a new key and permanently locks out admin writes | `mandates.ts:24-50`; `LabOpsPanel.tsx:32-35,98-104` |
| AUD-C6 | Session switching leaves the previous transcript/manifold rendered (cross-session data attribution) | `CopilotDrawer.tsx:127-135`; `EpistemicManifold.tsx:34-53` |
| AUD-C7 | Refusals collapse to `[]` in most list fetchers, rendering as "nothing exists" | `api.ts:80,301,315,1114,1277,1441` |
| AUD-C8 | 1.4 MB first-load chunk eagerly includes three.js, KaTeX, EXR; no lazy loading; token in the same chunk | `SpatialCanvas3D.tsx:4-5`; `SpatialCanvas.tsx:8`; `page.tsx:32`; `MathText.tsx:4` |
| AUD-C9 | No fetch timeout/cancellation or error boundaries; the 1 s shell tick re-renders the 3D canvas | `api.ts:68-70`; `page.tsx:205-208`; `app/` has no `error.tsx` |
| AUD-C10 | "FlightRecorderCard <200ms" is unmounted, calls the Next origin (404), and the test stubs any fetch | `FlightRecorderCard.tsx:45`; `FlightRecorderCard.test.tsx:41-49` |
| AUD-C11 | "WebCrypto Ed25519" copy is false (`@noble/ed25519`); crypto/storage posture is hardcoded | `mandates.ts:1`; `AutonomyCockpit.tsx:319-329` |
| AUD-C12 | Lint red at HEAD (`setState` in effect, `any`) | `FlightRecorderCard.tsx:37,53` |

### CI, contracts, and hygiene
| ID | Finding | Evidence |
|---|---|---|
| AUD-I1 | No branch protection; CI red at HEAD; last full green 2026-10-17 | GitHub API; run `40297011884` |
| AUD-I2 | Docker image cannot read the fidelity contract (build context excludes it) and panics | `accounting.go:56-73`; `compose.prod.yml:46` |
| AUD-I3 | 14 bridge RPC tests are not run in CI; the live container sandbox is never built/exercised | `ci.yml:103`; `test_codeexec.py:114-122` |
| AUD-I4 | New `test_codesign_slice63+` files match no shard and are ignored by the fast lane | `ci.yml:98,141-249` |
| AUD-I5 | CI does not check out `submodules/math`; math-catalog tests pass on a synthesized fallback | `ci.yml:79`; `math_catalog.py:192-303` |
| AUD-I6 | `scripts/ci.sh` is not a mirror of hosted CI (no Go/console/Docker/wiki/secret/DCO/bridge) yet is described as "the entire pipeline" | `platform/scripts/ci.sh:13-40` |
| AUD-I7 | Secret scan is tracked-tree only, seven prefixes, no history; non-provider patterns + validity checks disabled | `scripts/secret-scan.sh:30`; GitHub API |
| AUD-I8 | DCO check accepts any body containing `Signed-off-by:`; new-branch zero-base checks only `head -1` | `scripts/dco-check.sh:30-47` |
| AUD-I9 | 9 open Dependabot alerts, incl. two critical Next.js RCEs; `next 16.3.8` | `package.json:21`; GitHub API |
| AUD-I10 | No Python lint/typecheck/coverage/SAST; no Go/Rust lint in CI; matrix is single-version (3.11/Node20) while 3.12 branches exist | `pyproject.toml:56-60`; `ci.yml` |
| AUD-I11 | `pnpm@11` declared, CI/Docker use pnpm 9 on Node 20 | `package.json:40`; `ci.yml:380`; `Dockerfile.console:23` |
| AUD-I12 | Root `.dockerignore` absent; Lean build context is the repo root with `.env` and 21 G `dev-docs/` | `Dockerfile.lean:13`; `ls .dockerignore` |
| AUD-I13 | Claim drift: 93 routes (actual 114), 36 tools (actual 37), "green since 2026-09-29" (false), "gVisor+0600 UDS" (not implemented), "0 open questions" (about to change) | `README.md:122,158`; `server.go` route count; `_mount.py:42`; `AGENTS.md:123` |

---

## Claim ledger (README/wiki vs code at `da800ea`)

| Claim | Verdict | Evidence |
|---|---|---|
| "93 HTTP routes" | **FALSE** | 114 unique `HandleFunc` patterns in `server.go:418-614` |
| "6 MCP servers, 36 tools" | **FALSE** | 37 tools; `submit_qubo_job` missing from the catalog |
| "CI green since 2026-09-29" | **FALSE** | Last full success 2026-10-17; run `40297011884` red; protection off |
| "gVisor runsc containment + 0600 UDS attestation" | **FALSE/PARTIAL** | No `runsc` call; runtime env-gated, default runc; UDS chmod races and TCP stays open |
| "3-seam IFC fail-closed" | **PARTIAL/FALSE** | Go validator has no production caller; Python IFC is dead; Rust uncalled |
| "Sealed/append-only ledger" | **FALSE** | Metadata unsigned, session delete erases events, external signatures verify over a different message, `VerifyChain` has no caller |
| "No Go-side QPU spend / single Python book" | **FALSE** | Go reserves tenant+session spend and records jobs (`quantum_jobs.go:205-250`) |
| "WebCrypto Ed25519 signing" | **FALSE** | `@noble/ed25519` pure JS |
| "8 dynamic surfaces" | **FALSE/STALE** | 6 dock surfaces + 4 proof + 5 mesh tabs; the gallery is "Surface 9" |
| "FlightRecorderCard <200ms" | **FALSE AS SHIPPED** | Unmounted, wrong origin; test asserts 2 s |
| "H2 FCI anchor" | **FALSE** | `-1.0718` vs true `-1.13727`, R ignored; tests edited to bless it |
| "Comparator/Lean verification" | **FALSE WITHOUT TOOLCHAIN** | Fallback returns `ok=True` without reading the proof |
| "Autoresearch improves measured performance" | **PARTIAL/FALSE** | Nonce forgeable, hardcoded entanglement, analytic "noise model" |
| "Calibration freshness enforced" | **FALSE** | Age is set to `now - constant` at load |
| "Model adapter exists, hash/metrics measured" | **FALSE** | Only README+manifest in `platform/models/adapters/` |
| "Repo private by decision" | **FALSE AT HEAD** | GitHub API `private:false` |
| "0 open questions" | **BECOMING FALSE** | This audit opens Q-49..Q-57 |

---

## Systemic root causes (fix these, not just the instances)

1. **Per-handler authorization.** New routes copy "reads stay public" without the
   D-09 tenant binding; there is no central route policy table, so every new
   endpoint is a fresh chance to forget auth.
2. **No owner for money invariants.** Three books (Go SQLite, Python JSONL,
   Atlas) + float + partial idempotency + client-declared fields = every new
   rail can double-charge or under-charge. The book of record is ambiguous.
3. **The trust boundary is at the wrong place.** The bridge authenticates
   nothing and accepts the caller's claim about spend/tenant; the console signs
   locally and the server does not verify what it must.
4. **"Verified" is a UI convention, not a data property.** Values are defaulted
   (`-1.137`, `rho=0.88`, `99.0`, `0 sorryAx`, `calibration fresh`) and only
   sometimes labeled. The API drops the labels it computes.
5. **Fail-open resilience.** HF fallback as live, comparator fallback as pass,
   empty-device fidelity as perfect, NaN-as-zero workflow cost, DB-error LLM
   budget as zero, refund without signature.
6. **Ad-hoc persistence.** One SQLite file, no schema version, swallowed
   migrations, no FK enforcement, no backup/retention, ledger deletion.
7. **Aspirational contracts.** The MCP catalog, `agent-spec-v1`, error catalog,
   event `required` fields, and QIR BaseProfile are pinned but not enforced or
   not used; drift is found by tests only after it breaks release.
8. **Verification is broad but not adversarial.** Tests encode the bug
   (self-signed A2A, queued-no-credential success, comparator fallback `ok`,
   `-1.0718`), so the suite stays green while the boundary is open.

---

## Implementation plan

**Sequencing rule:** Phase 0 restores the ability to verify anything. Phase 1
closes the untrusted compute/money boundary. Phase 2 makes the books exact and
auditable. Phase 3 makes scientific claims honest. Phase 4 makes the console
enforce rather than decorate. Phase 5 hardens scale/ops. No phase ships on a red
gate; each slice carries a regression pin proven to fail pre-fix and a live
verification in both directions.

### Phase 0 — Restore release integrity (1–2 days, no product decisions)
1. **P0-7a** Embed `fidelity-devices.json` (`go:embed`) and remove the panic path;
   add a container-boot test to `docker-smoke`.
2. **P0-7b** Regenerate `mcp-catalog.json` from the live mounted surface; derive
   the 36/37 count instead of hardcoding; export/rename
   `plan_dual_model_packing`.
3. **P0-7c** Fix the two console lint errors; make lint fail the build (it
   already does in CI).
4. **P0-7d** Rebalance shards F/G (split slice 60 or nightly), add per-test
   timeouts, and make slice discovery manifest-driven so ≥63 cannot vanish.
5. **P0-7e** Enable branch protection with the 16 checks required; add the
   bridge RPC tests and a real `ci.sh` parity script.
6. **P0-7f** Correct the false claims (routes/tools/green/gVisor/private) as
   part of this slice; never publish a "verified" claim that a test does not
   assert.
**Acceptance:** `main` green on a full run with protection on; `docker compose
build && up` reaches healthy; `wiki-lint --strict` clean; a deliberate break in
each protected rail turns the required check red.

### Phase 1 — Close the perimeter (1 week)
1. **P0-1** Bridge: loopback/UDS-only default; mTLS or HMAC interceptor;
   remove wire-trusted `session_verified`/budget/spend/tenant; simulator-only
   `ExecuteCircuit` (or hardware via `submit_job`); `require_auth=True` for
   `WorkflowRun`; per-RPC principal + job ownership; caps, deadlines, message
   limits.
2. **P0-2** Go: central route policy middleware (method × path × auth/tenant/
   autonomy); authenticate import/a2a/llm-test/loops/memory; signed import with
   chain verification; make PQC verify read-only or gated.
3. **P0-3** A2A: disable public submit until real settlement + tenant binding,
   or implement x402 verification against a trusted payer and admin-key mandate
   verification; bound concurrency.
4. **P0-4** Sandbox: containerize all agent code execution (runsc/Docker with
   no network, RO root, caps dropped, rlimits) and stream-cap output; make the
   container mandatory in production.
5. **P0-6** Refund: `VerifyAdminAction` before mutation; caller/session tenant
   binding; forged-signature tests.
**Acceptance:** a route-auth matrix test enumerates every route and fails on any
new unauthenticated state-changing path; an external container cannot reach the
bridge; a sandbox escape attempt (ctypes/file/network) fails; forged refund and
import manifests are rejected.

### Phase 2 — Make money and audit exact (1–2 weeks)
1. **P0-5** Atomic idempotency (insert-or-get under lock, full canonical body,
   tenant-scoped); reserve→commit-on-ack→auto-refund on pre-acceptance failure;
   wire `release_spend` to failures and emit a sealed refund event.
2. **P0-6/M5** Integer micro-USD (or decimal) across Go+Python with one book of
   record; migrate REAL columns; property-test reserve/release/refund to zero.
3. **M5/M6/M7/M8/M9** Ledger v2: include tenant/label/timestamp in the digest;
   never delete events; domain-separated external receipts verified on append;
   receipts verified (signature + nonce + expiry), tenant-scoped append; call
   `VerifyChain` on import and on a schedule.
4. **M2/M3/M4/M11** LLM budget via an indexed running total and fail-closed on
   DB error; workflow charges actual `max(reserved, spent)`; single conditional
   UPDATE for loop spend; persistent job-bound payment quotes.
**Acceptance:** double-submit with the same key yields one charge; a failed
pre-vendor job charges 0; a 10,001st event still counts; property tests find no
drift over 100k operations; `VerifyChain` green after import, red after a
metadata tamper.

### Phase 3 — Scientific and claim honesty (1–2 weeks)
1. **P0-8** Anchors: live PySCF (or refuse non-table R); replace H2 and other
   table values with independent references; restore `-1.13727`; external test.
2. **P0-9** Comparator: fail closed when the toolchain is absent; never
   synthesize axioms; require a transcript.
3. **P0-10** Remove defaults: 3D/Lean/manuscript/HF/calibration/autoresearch/
   model manifest all nullable with explicit estimated/unverified states;
   condition prose on computed evidence.
4. **S2/S3/S4/S5/S6/S7/S8/S9/S11/S12** Fix VQE ground labeling/QAOA claim, KSD
   NaN, shadow caps, channel-noise autodiff label, library tenant/validation,
   MetaScope signature-on-load, manuscript signoff/peer review, licence parsing,
   `assert`→raise, EEA flag.
**Acceptance:** an independent reference suite passes the anchors; no surface
can render "verified"/rho/sorryAx without a receipt; a malformed/absent proof
returns unavailable; `python -O` still enforces exactness gates.

### Phase 4 — Console enforcement and resilience (1–2 weeks)
1. **C1/C2** Bind a server-issued quote id to `{provider, qasm, shots, session}`
   and invalidate on edit; dispatch must carry a server-verified mandate whose
   job hash matches; enforce the mandate server-side.
2. **C3/C4** Fail production builds when a bearer is configured; add CSP,
   frame-ancestors, nosniff, referrer, permissions; rotate the dev token.
3. **C5** Persist the admin key (passkey/WebCrypto non-extractable or explicit
   import/export) and server sessions as the identity.
4. **C6/C7/C9/C10/C12** Reset state on session change with abort guards; adopt a
   discriminated result type for refusals; add timeouts/error boundaries;
   mount/fix or delete FlightRecorderCard; isolate the 1 s clock.
5. **C8** Lazy-load three/KaTeX/EXR and enforce a first-load budget.
6. **Accessibility:** focus traps, reduced motion, keyboard 3D inspector,
   `select-none` scope, tooltip semantics.
**Acceptance:** a QASM/provider/shots edit disables dispatch until re-quoted; a
production build with a token fails; CSP blocks inline script; session switch
shows only the new session; keyboard-only flows pass.

### Phase 5 — Scale and operations (ongoing)
1. Persistence: schema versioning, FK enforcement, migrations that fail closed,
   backups/retention, indexed queries; consider Postgres for the ledger at
   multi-worker scale.
2. Observability: request IDs, structured logs, metrics/tracing, readiness that
   includes DB/bridge/config, panic recovery at every async boundary.
3. Providers: verify IBM channel/Heron profile, QCS cancel, Leap sampler, and
   rate cards live; add retry/backoff and readiness that reflects submit
   capability; fix CASCI/FCI labeling and R-sweep echoes.
4. Supply chain: fix Dependabot (Next ≥16.4.1, undici, ipld), pin digests
   everywhere, add CodeQL/dependency-review, secret scanning with history.
5. CI: Python 3.11/3.12 + Node 20/22 matrix, ruff/mypy, coverage, `cargo
   clippy`, shard manifest, submodule checkout.
6. QIR: parser fuzzing, checked slices, resource caps, measurement lowering
   via classical bits, truthful profile labels, scrubbed env.

### Gate to call it production
- [ ] Zero open P0; every P1 either fixed or explicitly waived in writing.
- [ ] Required CI checks green on `main` for 10 consecutive runs with branch
      protection on.
- [ ] Docker stack boots healthy; bridge unreachable from outside the app
      network; sandbox escape tests fail closed.
- [ ] Route-auth matrix test covers 100% of routes; forged refund/import/A2A
      tests pass.
- [ ] Money property tests: no drift, no double charge, failed job charges 0,
      refund requires a verified envelope.
- [ ] Anchor suite matches independent PySCF references; comparator fails
      closed without a toolchain; no unlabeled fabricated value in any surface.
- [ ] Console production build contains no bearer; CSP present; session
      switches are isolated.

---

## Open decisions (filed as Q-49..Q-57)

| # | Decision | Why it blocks |
|---|---|---|
| Q-49 | Bridge trust boundary: mTLS vs HMAC vs UDS-only; does the bridge ever run off-host? | Determines Phase 1 transport and deployment topology. |
| Q-50 | Single money authority and exact unit: Go SQLite vs Python JSONL; integer micro-USD; Atlas reconciliation. | Determines the Phase 2 migration and API shape. |
| Q-51 | Failed-job refund policy: automatic pre-acceptance refund vs operator-only; idempotent replay semantics. | Money behavior; cannot be inferred from code. |
| Q-52 | Session import trust model: operator-only signed manifests vs remove the endpoint vs sandboxed staging. | Ledger integrity and migration UX. |
| Q-53 | A2A/x402: real settlement + authenticated tenants, or keep disabled until Phase 5. | Public exposure vs feature loss. |
| Q-54 | Python workload isolation runtime: gVisor vs Docker vs WASM; numpy/scipy allowed? | Capability vs safety of the science rail. |
| Q-55 | Anchor source of truth: live PySCF per request vs a curated table; which reference values and tolerances. | Determines whether P0-8 is a fix or a removal. |
| Q-56 | Console auth: server sessions as identity vs client-held keys; how the operator token reaches the browser (or never). | Determines Phase 4 custody and deployment. |
| Q-57 | Public-read policy: which endpoints may stay anonymous in single-lab mode, and how multi-lab mode is detected. | Determines the route-auth matrix and D-09 completion. |

---

## What must not regress

- Atomic tenant/session spend CAS with cumulative ceilings and explicit
  over-release refusal (`tenants.go:397-469`, `session.go:474-589`).
- Admin envelopes binding tenant/action/params before mutation (the *pattern*;
  the refund route must join it).
- Content-bound idempotency rail in Go (`idempotency.go:141-282`).
- Per-session hash-chained ledger with registry-gated event types (fix the
  metadata/deletion gaps, do not replace it).
- Disk guard with hysteresis and fail-closed unmeasurable disks.
- Provider refusals: IonQ/Braket/Pulser/Strawberry fail closed; Qiskit refuses
  a silent Aer downgrade; zero-rate pricing refuses.
- Tool registry deny-by-default with strict unknown-property refusal.
- PQC envelope: ML-KEM-768 + AES-GCM with fingerprint AAD.
- Cross-language golden vectors (mandate, job binding, evidence, taxonomy).
- Docker hardening where present: nonroot, `--read-only`, `--cap-drop=ALL`,
  `no-new-privileges`, `--network none`, digest-pinned core bases.
- Honest three-state backend status and refusal telemetry in HF/fleet/cost.
- Safe markdown/KaTeX escaping (`trust:false`) and the XSS fixtures.
- `release_gate` refusing an all-skip canary; skips are not passes.

---

## Rulings (2026-10-27)

Technical questions were decided with the conservative option and recorded
here; only genuine product/finance choices remain open.

| # | Ruling | Rationale / implementation |
|---|---|---|
| Q-49 | **Private network + shared token + HMAC now; mTLS only when the bridge leaves the host/network.** | The bridge is a compose-internal service (never published), every RPC requires `KUBONA_BRIDGE_TOKEN`, and the session mirror is HMAC-signed. mTLS is deferred until an off-host topology exists (no current requirement). |
| Q-52 | **Import is operator-only and signed.** | Bearer-gated, HMAC-signed section digests, production refuses unsigned manifests, oldest-first chain ordering, known event types, commit signatures, spend bounds. |
| Q-53 | **A2A/x402 is disabled in production by default.** | `KUBONA_A2A_ENABLED=1` is required; the self-signed local challenge is not a settlement rail. This preserves the dormant Q-23 posture without exposing a free-spend path. |
| Q-54 | **Container isolation is mandatory in production; Docker is the runtime, runsc is an optional hardening.** | `WorkloadSandbox` refuses the local AST/rlimit path in production unless `KUBONA_SANDBOX_CONTAINER=1`; the runner image is built and exercised in CI. numpy/scipy remain allowed inside the container. |
| Q-55 | **Live PySCF is authoritative; a table is only a fallback where a live solver exists.** | H2 now solves RHF+FCI at the requested R; missing PySCF refuses rather than serving a fixed geometry. Test pins the absolute value and R-dependence. |
| Q-57 | **Production defaults to multi-lab auth reads; single-lab must be declared.** | `KUBONA_SINGLE_LAB=1` is required for anonymous single-lab reads in production; otherwise every session-scoped read needs a cookie/bearer. Compose declares single-lab for the supported quickstart. |

**Atlas / World Labs is optional.** It is off in production unless
`KUBONA_ATLAS_ENABLED=1` (and a `WLT_API_KEY` is present). Cached/ledger lens
views remain local and free; no Atlas spend or egress happens on a deployment
that never opts in. Q-39 is therefore Atlas-only and does not gate production.

**Resolved 2026-10-27 (operator approved the recommended defaults):**

| # | Ruling | Implementation status |
|---|---|---|
| Q-50 | **Go SQLite is the single money authority** for API/human/quantum dispatch; Python mirrors it; new ledger v2 uses integer micro-USD. | **Partial:** Go-verified quantum submissions no longer charge the Python mirror (`spend_prepaid`), and Go reconciles the reservation from the status poll. Integer micro-USD columns/migration remain. |
| Q-51 | **Auto-refund pre-vendor failures; keep post-vendor charges; operator-only partials.** | **Implemented for quantum dispatch:** Go releases a failed/cancelled reservation only when no `hardware_job_id` is pinned; Python skips its own refund for Go-prepaid jobs. Partial-refund UI/policy remains operator-only. |
| Q-40 | **Human non-zero hardware dispatch requires a signed mandate.** | **Implemented:** pre-bridge signature/tenant/program/expiry gate; post-estimate budget check cancels on overrun. Simulators/dry-run exempt; MCP agents use the separate `submit_job` rail. |
| Q-56 | **Cookie sessions are identity; the admin Ed25519 key is optional signing material; no baked bearer; move to passkey/non-extractable storage.** | **Partial:** production builds reject a baked bearer; the mandate card now signs the canonical session+program binding and requires server verification. Passkey/non-extractable custody remains. |

## Execution record (landed 2026-10-27)

**Phase 0 is complete and verified locally.** The gate can now verify changes:
the fidelity contract ships in the image, the production image boots, the MCP
catalog is regenerated and review-complete, the packing export is fixed, console
lint is zero, slice coverage is manifest-guarded (new slices cannot vanish), the
thesis-scale slices moved to nightly, and the local `ci.sh` mirrors Go/console/
bridge/wiki/secret checks. False README/AGENTS claims were corrected (114 routes,
37 tools, public repo, 9 open decisions, no "green since" claim, gVisor/sandbox
honesty).

**Phase 1 security slice is partially landed:**

| P0 | Status | What landed | What remains |
|---|---|---|---|
| P0-1 bridge | **Partial** | Shared-token interceptor on every RPC (verified live: no token → `UNAUTHENTICATED`; token → call proceeds); non-loopback bind refuses without a token; UDS mode no longer also opens TCP; Go client sends the token; `ExecuteCircuit` refuses hardware (verified live) and defaults to `aer_simulator`; `WorkflowRun` requires the bridge token outside dev. **1b:** the session mirror's money/tenant fields now carry an HMAC-SHA256 assertion from Go, verified by the bridge with a cross-language vector. | mTLS decision (Q-49); bridge-side job ownership (Go already enforces it). |
| P0-2 control routes | **Partial** | Import, A2A, LLM test, loops, memory recall/retain, and PQC verify are bearer-gated; A2A keeps the x402 `Authorization` header separate via `X-Kubona-A2A-Token`; route-auth matrix pin added. Memory reads/writes bind caller↔session tenant. **1b:** loop listings filter by caller tenant, loop actions bind the loop's session, and imports are operator-signed (HMAC over section digests) with a production unsigned-refusal pin. | Session-list/read posture remains Q-57; import key rotation (Q-52). |
| P0-3 A2A | **Partial** | Public submit is no longer anonymous; operator token required. | Self-signed x402 "settlement" and `tenant="default"` remain (Q-53). |
| P0-4 sandbox | **Partial** | Production refuses the local AST/rlimit path (`isolation_required`) unless `KUBONA_SANDBOX_CONTAINER=1`; container path added with no network/RO root/caps/memory/pids; ctypeslib/fromfile/load family blocked; rlimit NPROC/FSIZE/NOFILE added; negative pins landed. **1b:** a `sandbox-container` CI job builds the runner image and executes the live path; verified locally. | Runtime decision (Q-54); exercise the container path in the release gate. |
| P0-5 money | **Partial** | Idempotency is checked before spend and **claimed atomically** (`find_or_create` under the store lock) before money moves; replay returns the original job; body mismatch refuses; pre-vendor/enqueue failures refund and emit a ledger event when a ledger is wired; a 2-thread race test pins one job/one charge. | Single book/tenant-scoped key/full body hash (Q-50/Q-51); ledger must be wired in production (PYC-10). |
| P0-6 refund | **Closed** | `VerifyAdminAction` runs before `sessionMgr.RefundSpend`; forged-signature 403 pin added. | — |
| P0-7 release | **Partial** | Docker contract + build context fixed and boot verified (`/api/health`/`/api/ready` 200); catalog 37; packing export; console lint; slice guard; nightly F/G; bridge RPC tests in CI; jq/vet clean. | Branch protection + a full protected green run (needs push); CI secret/token env for the new bridge. |
| P0-8 anchor | **Closed** | H2 is now a live PySCF RHF+FCI solve at the requested bond length (`-1.137270…` at R=0.7414, different at R=2.0); table provenance is retained only for non-live entries; tests pin the value and geometry dependence. | — |
| P0-9 comparator | **Closed** | Toolchain absence returns `ok=False`, `verification_mode="verification_unavailable"`, empty `axioms_used`, and no proof claim; the AST path is no longer a pass. | — |
| Money exactness | **Done (books)** | `pkg/money` + integer micro-USD for sessions, tenants, job reservations, and loop books; every production read uses micro; guarded integer arithmetic; legacy backfill + 10k-dust tests. REAL remains only as an atomic compatibility projection (no read path). | Drop the REAL projection as a schema-only cleanup once ops/tests stop reading it. |
| LLM budget | **Done** | Full-ledger scan (no 10k limit), query/scan/JSON errors fail closed, and the gate checks every role assignment not just the active provider; cost summary refuses when spend cannot be verified. | — |
| Audit retention | **Done (v2)** | `DeleteSession` no longer deletes `ledger_events`; the signed `session_deleted` receipt keeps the original tenant; `GET /api/ledger/verify` is the production `VerifyChain` caller. **Ledger v2** commits tenant/label/timestamp/type + canonical payload into the digest, keeps v1 rows verifiable, and separates external mandate (`mandate-v1`) from opaque proofs. | Tenant-scoped event attribution is fixed (tenant-id fallback); v1 imported-payload byte canonicalization remains. |
| Observability | **Partial** | HTTP panic boundary with request IDs, A2A panic recovery, DB-aware readiness, and a bearer-gated `/metrics` (request counts/duration by method+status, inflight; panics counted as 500). | Tracing, structured JSON logs, and monitoring the one-off auth-session test flake remain. |
| P0-10 theater | **Partial (autoresearch gate)** | Production refuses self-reported autoresearch benchmarks (artifact and evaluator share a process; the score is forgeable) unless `KUBONA_AUTORESEARCH_ALLOW_SELF_REPORTED=1` explicitly opts in for research. A measured harness is not blocked. | 3D HUD renders `UNVERIFIED`/`ρ = —` without a computed anchor; the Lean widget requires a real tactic/axiom/sorryAx receipt and no longer synthesizes one from the campaign target; the manuscript sign-off is re-verified at deposit, content changes refuse, and peer review uses `abs(discrepancy)`; the tribunal no longer counts static AST as Lean verification. **4a:** a production build with `NEXT_PUBLIC_KUBONA_CONSOLE_TOKEN` fails closed (verified), CSP/frame-ancestors/nosniff/referrer/permissions headers added, and Go CORS now sends `Vary: Origin`. **4b (partial):** QuantumDispatch binds a quote to session+provider+program+shots and renders unpriced estimates as UNPRICED (no $0 mandate bypass); the mandate card still signs an action string rather than the canonical job binding. **3b:** calibration age now comes from the recorded `calibrated_at` (the shipped 2026-10-01 profiles are stale and refuse, correctly); the adapter manifest is `spec-only` with null artifact/hash/metrics; template entanglement constants are labeled `measured:false`. **3c:** HF summaries carry `source=live|fallback` + `estimated`; details query live first and fall back only on network error; Atlas and A2A are opt-in in production. | Autoresearch benchmark nonce isolation remains (artifact output is still self-reported); QPU admission will refuse until rate cards/calibration dates are refreshed live (Q-55). |

**Verification run (local, 2026-10-27):** Go `build`/`vet`/`test ./...` green;
Python fast lane `786 passed, 4 skipped`; bridge `60 passed, 1 skipped`;
console `272 passed`, lint 0, `tsc` 0; Rust 23 passed; wiki-lint green; kubonad
image builds and boots (health/ready 200); bridge token rejection/acceptance
verified live; runner image built and the container sandbox executed live.
Phase 0 + 1a landed as `532708f` (three signed commits) on `develop` + `main`.
Phase 1b landed as `0ee9f9c`; Phase 3a closed the anchor/comparator P0s and the
largest verified-claim theater; Phase 3b fixed calibration age, the model
manifest, and template constants; Phase 4a/4b added the console build gate,
security headers, and quote freshness; the dependency slice cleared all nine
Dependabot alerts. The full CI gate is green on the latest code (`39cc5c9`, run `40467016495`)
and branch protection is enabled on `main` + `develop` with 15 required checks
(admin bypass on, linear history, no force-push/delete). All rulings
(Q-49..Q-57) are recorded above. Residuals: integer micro-USD ledger v2,
passkey/non-extractable key custody, mandatory-mandate policy (Q-40), and the
autoresearch benchmark nonce.

---

## Appendix

Per-domain P2/P3 register, exact coverage, and the command/verification log:
[[production-readiness-audit-2026-10-26-details]].
