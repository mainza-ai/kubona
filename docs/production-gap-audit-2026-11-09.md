# Production gap audit and implementation plan — 2026-11-09

**Scope:** fully functional local-private deployment on one trusted host.
Not a public SaaS, not a multi-tenant product, not a demo.
**Audited head:** `d016e41` plus the committed 2026-11-09 PES/fleet/CI95/
calibration slice through `746ddca` (PES/fleet/CI95/hardware-active/calibration/JSON receipts).
**Method:** re-read the current Go/Python/console code and tests, not the prior
status prose. The earlier completion report contained stale entries; this page
corrects them explicitly.

## 1. Executive verdict

The local-private **core simulator stack is operational**: compose boots,
loopback ports are enforced, the ledger/jobs/claims rails work, H2 and
active-space LiH/BeH2 VQE run, full-space LiH ADAPT reaches the target, governed
H2/QAOA hardware parent runs exist with contested results, workflow approvals
are signed/single-use, and CI + Console E2E are green.

It is **not yet the complete product** promised by the research/experiment
surface. The remaining blockers are functional, not cosmetic:

1. **No iterative hardware VQE/QAOA experiment loop.** The hardware path
   optimizes locally and measures the final fixed ansatz on one Runtime batch.
   That is a real hybrid result, but it is not a hardware-in-the-loop VQE or a
   native hardware QAOA optimizer. Results are labelled `simulated_optimizer`.
   The final-state path now also accepts active-space LiH/BeH2 ansatz QASM and
   Pauli terms (not only H2); full-space has no single hardware QASM.
2. **Error mitigation is not part of VQE/PES.** The separate mitigation route
   demonstrates one observable (Z0) on a circuit; it is not energy mitigation
   for a molecular result. The direct Krylov/mitigation routes now preserve the
   complete JSON receipt (measured/inferred labels, fit diagnostics), so the
   remaining gap is scientific integration, not label loss.
3. **BeH2 full-space chemistry remains contested.** LiH full-space is validated;
   BeH2 stayed above 1.6 mHa through a 60-operator UCCSD test. CEO-ADAPT is
   implemented but stalls on LiH. It must not be advertised as chemically
   accurate.
4. **The production sandbox is not runnable on this host.** The `sandboxd`
   + gVisor `runsc` topology is implemented and fail-closed, but this macOS
   Docker Desktop host has no `runsc`; autoresearch stays unavailable. A Linux
   host/VM is required for a hostile-code boundary.
5. **Calibration/queue operations are partial.** Live refresh exists through the
   bridge image; `/api/fleet` exposes calibration age and a boot/hourly
   stale/resumed alert is now sealed; there is still no automated refresh
   worker, queue-position/ETA surface, or UI refresh action. QPU dispatch can
   still go stale until the operator refreshes.
6. **Result truth is split across contracts.** Job JSON, campaign `measurement`,
   PES `points`, notebook text, and UI projections are not one typed
   `ExperimentResult`. `result_version` exists on new results, but there is no
   signed per-result receipt shared by every rail.
7. **Browser coverage is uneven.** The workflow approval test is real-stack; the
   claim-paging/full-space test is a deterministic API-contract fixture, not a
   real claim/measurement flow. PES, error mitigation, calibration/queue, and
   claim review lack full browser acceptance.
8. **Manual accessibility is not run.** Automated axe/keyboard/reduced-motion/
   forced-colors/200% zoom evidence is green; VoiceOver/NVDA requires a human.
9. **The host ledger is forensic, not a verified restore source.** The local
   `~/.kubona/kubona.db` has 606 pre-existing chain faults from unsigned legacy
   deletes; secrets are now sealed, but rewriting the chain would launder
   history. A fresh volume is the supported release source.

**Verdict:** suitable for a controlled local-private **simulator/H2** run and
for governed, contested QPU measurements by an operator who accepts the listed
limitations. Not yet suitable to call the complete scientific product.

## 2. Corrections to the previous status

| Previous claim | Verified current state |
|---|---|
| Approval/agent workflows still use Python | **Closed.** `POST /api/workflows/approve` mints a signed, single-use, session+workflow-SHA-bound token; `runGoMixedWorkflow` consumes it atomically and runs the Go step rail. |
| Launch hardcodes shots and has no seed | **Closed.** `handleChatLaunch` accepts both `shots` and `seed` and persists them in job params (`server.go` launch block). |
| Krylov/mitigation/QIR bypass the ledger | **Closed for Krylov/mitigation.** `runDirectComputeJob` creates a ledger-bound job and waits for terminal state. QIR is a stateless compiler analysis, not a measurement. |
| Claims have no review decision | **Closed.** `claim_reviews` stores append-only `accepted|contested|reopened` decisions; API and Claim History expose them. |
| PES is H2-only | **Closed for active-space LiH/BeH2.** `run_pes` now delegates each point to `run_vqe`, supports full-space LiH/BeH2, and returns per-point uncertainty/FCI/delta/method. |
| Fleet has no calibration age | **Fixed in this slice.** `/api/fleet` returns `calibration_at`, `calibration_age_hours`, `calibration_fresh`, `fidelity_source`, `fidelity_version`; the console table renders age/stale. |
| Local simulator says “noisy density matrix” | **Fixed.** The bridge now labels the un-noised Aer path as shot simulation, not a noisy density-matrix run. |

These corrections do not close the items in §3.

## 3. Verified gap register

### P0 — blocks complete local-private functionality

| ID | Gap | Evidence | Required closure |
|---|---|---|---|
| PG-01 | **No hardware-in-the-loop VQE/QAOA.** | `run_vqe_hardware` accepts fixed `theta`, one batch, `simulated_optimizer:true`; `run_qaoa_hardware` measures a fixed QASM. | A governed iterative hardware worker with per-iteration budget/admission, observable grouping, vendor polling, settlement, and a claim; or an explicit product decision that hardware is final-measurement only. |
| PG-02 | **Mitigation is not part of molecular VQE/PES.** | `mitigate_circuit` returns one `<Z0>` value; `handleMitigationApply` is a separate circuit route. | Integrate per-term/energy mitigation with measured-vs-inferred labels, or remove it from the experiment promise. |
| PG-03 | **BeH2 full-space is not target-accurate.** | 16 ops ≈4.16 mHa, 40 ≈3.04 mHa, 60 ≈3.23 mHa exact; LiH succeeds. | Improve pool/optimizer or declare BeH2 full-space unsupported. Do not claim accuracy. |
| PG-04 | **CEO-ADAPT does not reach the LiH target.** | CEO pool 750 ops stalls ≈4.66 mHa; UCCSD default reaches 0.706 mHa measured. | Keep CEO experimental or improve pool/optimizer; never make it the default until measured. |
| PG-05 | **Sandbox cannot run on this host.** | `compose.sandbox.yml` requires `runsc`; macOS Docker Desktop has none; API reports unavailable. | Linux host/VM with gVisor; prove live execution. macOS remains unavailable. |
| PG-06 | **Calibration/queue operations are not closed.** | Refresh is a bridge CLI; no scheduler/alert; fleet queue is `UNKNOWN`; no calibration-age panel before this slice. | Scheduled refresh, stale alert, queue/plan diagnostics, and dispatch UX that names exact age. |
| PG-07 | **Result contract is not single-source.** | `result_version` exists, but campaign/notebook/UI/job JSON are separate; no signed per-result receipt. | One typed ExperimentResult schema shared by bridge, Go, campaign, export, UI, with CI95/usage/layout/calibration/refs and a receipt. |
| PG-08 | **Browser E2E is not complete.** | paging/full-space test intercepts `/api/**`; PES/mitigation/calibration/review have no full-stack browser flow. | Real-stack fixtures for PES table, claim review, full-space status, calibration/queue, and hardware opt-in. |
| PG-09 | **Manual VoiceOver/NVDA not run.** | `docs/manual-accessibility-2026-11-01.md` says NOT RUN. | Human run per surface or signed residual-risk waiver. |
| PG-10 | **Host ledger is forensic, not verified.** | `kubonad preflight` reports 606 pre-existing chain faults on `~/.kubona/kubona.db`; secrets now sealed. | Fresh-volume release source; do not rewrite/launder the forensic history. Off-host drill is implemented. |

### P1 — required for a complete researcher workflow

| ID | Gap | Improvement |
|---|---|---|
| PG-11 | PES is not a first-class campaign/experiment lifecycle. | Store typed PES spec (r_min/r_max/steps/full_space/pool/shots/seed) and fold a series into campaign state. |
| PG-12 | No repeat-run variance / error bars / convergence diagnostics in the campaign. | Repeat runs, CI95, optimizer trajectory, below-FCI guard, and claim linkage. |
| PG-13 | Claim review UI exists but no review decision drives campaign/epistemic state. | Project review state; keep measurement immutable; link review receipt to proof card. |
| PG-14 | Result UI lacks complete PES comparison and full job/result detail. | FCI curve + error bars, per-point job/claim links, export bundle. |
| PG-15 | Non-executable providers remain visible. | Mark `executable_now`; hide/gate planned providers in the default picker. |
| PG-16 | Legacy/demo surfaces remain in the shipped console. | Gate behind explicit proposal-only/dev flags; no measured-looking demo. |
| PG-17 | Multi-lab roles/first-run are incomplete. | Document single-operator as the only complete mode or finish role separation. |
| PG-18 | Calibration refresh/queue is not scheduled or alerted. | Worker + stale alert + UI. |
| PG-19 | Model assignment does not validate price/capability. | Save-time pricing/structured-output capability gate. |
| PG-20 | Workflow job compensation is unsupported. | Explicit inverse/job cancel compensation or refuse the workflow at plan time. |

### P2 — enhancements, not local-private blockers

- H2O/other molecular families; broader PES.
- Circuit cutting as a first-class experiment with reconstruction error.
- Hardware batching/parameter sweeps beyond one final state.
- OTel/Sentry exporters; perf/chaos baselines.
- Public plugin marketplace/A2A/x402; out of scope.

## 4. Execution plan (updated)

### Phase 0 — finish the current functional slice (status: implemented/tested)

1. **PES LiH/BeH2 + UI** (implemented in this slice).
   - Bridge delegates points to `run_vqe`; active and full-space; per-point
     uncertainty/FCI/delta/method.
   - Console exposes r_min/r_max and full-space/pool controls.
2. **Fleet calibration age** (implemented).
   - Go contract exposes max age; `/api/fleet` returns provenance/age/fresh;
     console renders it.
3. **Accurate simulator label** (implemented).
4. Tests for all of the above.

### Phase 1 — experiment truth and hardware loop

1. Define `ExperimentResult` v1 in one generated contract (Python/Go/TS).
2. Add `pes_sweep` job/campaign fold with typed spec and per-point evidence.
3. Decide hardware scope:
   - **Option A (recommended for local-private):** final-state hardware
     measurement only, explicitly labeled; no hardware optimizer claim.
   - **Option B:** governed iterative hardware worker (costly; needs
     per-iteration admission/settlement).
4. Add per-result receipt (backend, layout, calibration, usage, cost, counts,
   seed, shots, method, CI).

### Phase 2 — mitigation and scientific completeness

1. Add energy-level mitigation with measured/inferred labels; refuse when the
   mitigation cannot be computed for the full observable.
2. Preserve all mitigation/Krylov truth fields across proto and API.
3. Improve full-space BeH2 or declare it unsupported in API/UI.
4. Keep CEO experimental; require measured target before default.

### Phase 3 — operations and accessibility

1. Scheduled calibration refresh + stale alert + queue/plan surface.
2. Real-stack browser E2E for PES, claim review, full-space, calibration/queue.
3. Manual VoiceOver/NVDA run or explicit waiver.
4. Fresh-volume release acceptance + encrypted off-host restore.

### Phase 4 — product boundary

1. Gate demo/legacy surfaces.
2. Provider picker shows executable state.
3. Document single-operator scope and multi-lab gaps.
4. Keep A2A/x402/pgvector dormant unless explicitly activated.

## 5. This slice

Implemented after the audit:

- `run_pes` supports H2, LiH, BeH2; active-space default; optional full-space;
  rejects H2 full-space; returns per-point measured energy, uncertainty, FCI,
  exact delta, method, and claimability.
- `server.py` forwards `full_space` and `full_space_pool` for PES.
- Console PES range inputs and full-space controls for LiH/BeH2.
- `/api/fleet` calibration provenance/age/freshness from the fidelity contract.
- Console fleet table renders calibration age and STALE.
- `kubonad` boots a calibration monitor that seals `calibration_stale` /
  `calibration_resumed` events and alerts on transitions.
- Bridge Aer label corrected from “noisy density matrix” to shot simulation.
- Tests: LiH active-space PES; existing PES points; fleet/fidelity; console tsc.

Uncommitted at report time; commit and full CI follow.
