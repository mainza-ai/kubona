# Production gap audit and implementation plan — 2026-11-09

**Scope:** fully functional local-private deployment on one trusted host.
Not a public SaaS, not a multi-tenant product, not a demo.
**Audited head:** `0820b85` (2026-11-09 slices through `746ddca`, then
CEO/hardware-loop/mitigation/claimable/sandbox/result-gate work).
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

1. **Hardware-in-the-loop VQE/QAOA is live-validated.** The bridge's SPSA
   VQE loop and QAOA loop ran on `ibm_fez` through the governed parent rail.
   VQE: 4 SPSA iterations / 9 batches, `-1.1253618969 Ha`, `11.908 mHa` delta,
   `12.11 mHa` uncertainty, not claimable. QAOA: 4 batches, cut `5.953125`
   vs optimum `10` (ratio `0.5953`), not an advantage claim. Both are
   `simulated_optimizer:false` with signed receipts. Evidence:
   `docs/live-qpu-2026-11-09.md`.
2. **Molecular ZNE is live-validated.** A governed H2 run folded every
   Hamiltonian term at scales `[1,3,5]`, extrapolated the total energy, and
   completed as vendor `dbos2u44148s739th03g`: mitigated `-1.1441740297 Ha`,
   `6.904 mHa` delta, `12.04 mHa` uncertainty, `measured:false` and
   `claimable:false`. PES still does not request mitigation per point.
3. **BeH2/CEO full-space chemistry is now validated.** The earlier CEO stall
   was a spin-layout bug: the pool assumed even/odd interleaving while the
   pinned Qiskit Nature layout is alpha-block then beta-block. With the
   corrected pool, LiH reaches `0.0399 mHa` exact / `0.4499 mHa` measured and
   BeH2 reaches `0.3933 mHa` exact / `0.0146 mHa` measured, both at 262144
   shots. CEO is the default full-space pool; UCCSD remains selectable.
4. **Sandbox is live on a Linux VM; macOS remains unsupported natively.**
   `sandboxd` + gVisor `runsc` was proven on a Lima Ubuntu arm64 VM: health
   returned available, and a hostile benchmark ran as UID 1000 with no network,
   read-only rootfs, and `cap-drop=ALL` (`docs/sandbox-live-2026-11-09.md`).
   macOS Docker Desktop still has no `runsc`; autoresearch stays unavailable
   there instead of falling back to a weaker runtime.
5. **Calibration/queue operations are partially closed.** A live
   `POST /api/fleet/calibration/refresh` succeeded against `ibm_marrakesh`
   (in-process profile update), and `/api/fleet` exposes age plus local queue
   depth. There is still no scheduled provider refresh worker, vendor queue
   position/ETA, or UI refresh button; QPU dispatch can go stale until the
   operator refreshes.
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
| PG-01 | **Live-validated on IBM hardware.** | SPSA VQE loop `da0b1703...`: 9 batches, `-1.1253618969 Ha`, delta `11.908 mHa`, uncertainty `12.110 mHa`, not claimable. QAOA loop `7014cd0c...`: cut `5.953125`/`10`, ratio `0.5953`, not an advantage claim. Both signed, `simulated_optimizer:false`. | No code gap; continue only with higher-shot/optimizer tuning for an accuracy claim. |
| PG-02 | **Live-validated on IBM hardware.** | ZNE job `5f0064eb...`: scales 1/3/5, mitigated `-1.1441740297 Ha`, delta `6.904 mHa`, uncertainty `12.036 mHa`, `measured:false`, not claimable. | Live mitigation works; PES opt-in remains a product choice, not a correctness gate. |
| PG-03 | **Closed 2026-11-09.** BeH2 full-space reaches `0.3933 mHa` exact / `0.0146 mHa` measured at 40 CEO operators, 262144 shots. | Corrected spin-block pool; measured gate still requires exact delta. |
| PG-04 | **Closed 2026-11-09.** CEO-ADAPT reaches LiH `0.0399 mHa` exact / `0.4499 mHa` measured at 20 operators. | Spin layout derived and asserted; CEO is the default full-space pool. |
| PG-05 | **Sandbox live on Linux VM; macOS native remains unsupported.** | Lima Ubuntu arm64 + rootful Docker + gVisor: health available; benchmark UID 1000, network blocked, read-only, cap-drop=ALL. macOS Docker Desktop still has no `runsc`; API reports unavailable. | Keep the Linux-host requirement in the deployment runbook; no macOS fallback. |
| PG-06 | **Calibration/queue operations are partial.** | Stale alert, local Go queue depth, authenticated live refresh, and opt-in scheduled refresh exist; vendor queue/ETA and UI action remain absent. | Vendor queue/ETA diagnostics and dispatch UX that names exact age. |
| PG-07 | **Versioned schema and signed receipt exist; campaign/UI adoption remains.** | Shared schema + TS interface + Go version gate; completed jobs embed a ledger-backed `result_receipt` (signed when a result/reviewer key is configured). Campaign/PES/UI projections still use separate structs. | Move campaign/export/UI onto the shared type; commission the signing key in production. |
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

1. **ExperimentResult v1:** shared schema + TS interface + Go version gate are
   implemented. Remaining: make campaign/export/UI consume it directly.
2. Add `pes_sweep` job/campaign fold with typed spec and per-point evidence.
3. **Hardware loop:** implemented in code (Option B); live bounded QPU
   acceptance remains.
4. Add per-result signed receipt (backend, layout, calibration, usage, cost,
   counts, seed, shots, method, CI).

### Phase 2 — mitigation and scientific completeness

1. **Energy-level ZNE:** implemented for hardware molecular VQE with
   measured/inferred labels and fit refusal. Remaining: live run and optional
   PES opt-in.
2. Preserve all mitigation/Krylov truth fields across proto and API.
3. **Full-space BeH2:** CEO pool now reaches the target; UCCSD remains the
   reference.
4. **CEO:** corrected spin-block layout; CEO is the default and measured on
   LiH/BeH2.

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

- Corrected CEO-ADAPT spin-block pool; LiH/BeH2 reach the 1.6 mHa gate.
- Added hardware-in-the-loop VQE/QAOA loops and molecular hardware ZNE.
- Added `result_version` enforcement, schema, and TS result interface.
- Added local Go queue depth to `/api/fleet`.
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
- Live QPU: governed SPSA VQE loop, QAOA loop, and ZNE documented in
  `docs/live-qpu-2026-11-09.md`; all three completed with signed receipts.
- Commits: `3252180` (CEO/hardware loops/ZNE/result gate), `4c4ea51`
  (claimable PES + live gVisor proof), `956deab` (claimable/measured campaign
  and claim guard), `5872c6c` (live calibration refresh), `4d8830d` (signed
  ledger result receipts), `526674a` (receipt publication ordering).
  CI/E2E green at `526674a`.
