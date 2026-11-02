# Placeholder and Fake-Data Audit — 2026-11-02

**Status:** audit complete; Phase 0-3 implemented
**Scope:** console UI, Go campaign synthesis, AI/auth interaction, branding
**Production bar:** real logic or fail-closed refusal; no invented `success:true`, no
plausible-looking fallback presented as measurement.

## Verdict

The console is materially better than the state that produced the "UNVERIFIED DRAFT"
report, but it is **not yet placeholder-free**.

- The exact placeholder report path in `ResultsStudio` is closed: a citable studio now
  requires a recorded notebook, an evaluated claim, and a real provenance hash.
- The console no longer offers "Synthesize Reference Baseline" when creating a campaign.
- The AI dead-end shown in the screenshot is fixed: the AuthPanel button now routes to
  Laboratories; anonymous users get sign-in copy instead of "Session expired".
- The campaign menu no longer renders a raw `HTTP 401` as "Campaigns unavailable".
- The Kubona logo is now in the navigation rail and About dialog.
- Quantitative surfaces now carry a `data-provenance` badge in the controlled
  vocabulary: `measured`, `proposed`, `catalog`, or `model`. The Workbench and
  widget metrics are covered by a test that rejects metrics without a provenance
  ancestor, and CI runs a literal gate over production console code.

The campaign data model is now split by truth: `BuildCampaignProposal` emits only target,
proposed circuit, and a bridge-computed anchor. A fresh campaign stores `trajectory: []`,
`branches: []`, `epistemics: null`, and `notebook: null`; no VQE energy, canary fidelity,
or cut overhead is manufactured. The old 30-step trajectory, projected amplitudes, and
template notebook are gone from the creation path.

The secondary surfaces no longer ship illustrative numbers. Shadow tomography, QIR,
molecular active space, manifold geometry, the 3D ghost, and the gallery now render
explicit empty states until a measured input exists. A production-component grep for the
old literals (14.8x, 99.2%, 4.2x, LiH defaults, 0.042, 4.218, 0.64, -1.842) is clean, and
that grep is now a CI step. The audited console surfaces are placeholder-free; explicitly
labeled server fallback proposals and demo seeds remain outside measured surfaces.

## Fixed in this pass

| Area | Change | Verification |
|---|---|---|
| Results | `ResultsStudio` now renders a citable-empty state unless the campaign has `COLLAPSED_VERIFIED`, notebook energies, and a non-empty provenance hash. Removed the fake `sha256:pyscf-anchor` fallback and the unreachable `UNVERIFIED DRAFT` branches. | `ResultsStudio.test.tsx`, `ResultsStudioHeading.test.tsx` pass. |
| Campaign creation | Removed the "Synthesize Reference Baseline" option from `NewCampaignModal`; all console-created campaigns now request `fresh: true`. | `NewCampaignModal.test.tsx` passes (7 tests). |
| Campaign state | `BuildCampaignProposal` replaces result-shaped synthesis. Fresh campaigns store empty trajectory/branches and `null` notebook/epistemics; no VQE, canary, cut, or template notebook is written. Chat/tribunal no longer overwrite campaign state; `/campaign/synthesize` refuses to overwrite measured evidence. | Full Go suite; `TestFreshCampaignIsProposalOnly`, `TestBuildProposalCarriesNoMeasuredFields`, `TestCampaignSynthesizeRefusesToOverwriteMeasurement`; live API returned `epistemics:null`, `notebook:null`, no VQE. |
| Widgets / 3D / DAG | Shadow tomography, QIR, molecular active space, manifold geometry, gallery, 3D ghost, and canvas schematic defaults now use empty/absent states. The 3D ghost is hidden until a sealed μ±σ prediction exists. ProofDag labels proposal vs measured and proposed cuts. | New empty-path widget tests; `SpatialCanvas`/`SpatialCanvas3D`/`GenerativeUIStudio`/`ProofDagView` tests; literal grep clean. |
| Provenance / gate | Added `ProvenanceBadge` and `data-provenance`/`data-metric` coverage across Results, MeasuredOutputs, Workbench cards/list, FlightRecorder, EpistemicManifold, SwarmUniverse, SpatialCanvas/3D, ProofDag, widgets, Inspector, and the model hub. Added `scripts/check-console-placeholders.py` and a CI step. | `ProvenanceBadge.test.tsx`, `provenance-coverage.test.tsx`, Workbench metric-ancestor test; literal gate clean (119 files). |
| AI/auth | `Open Labs AuthPanel` now maps to `surface='mesh'`, `meshTab='labs'` instead of the non-existent `'labs'` surface. Anonymous vs expired copy is distinguished. The no-spend guard remains in place. | `CopilotDrawer.sessionguard.test.tsx` (6 tests) passes; live browser verified Labs opens and the console stays interactive. |
| Campaigns 401 | Multi-lab `GET /api/sessions` 401 now shows "Sign in to view campaigns in this multi-lab store" with an AuthPanel link, not the raw refusal. | Shared `handleCopilotSurface` path; live browser verified. |
| Branding | Navigation rail uses `/assets/milimo_kubona_logo.png` (210x180, alpha) with `alt="Kubona"`; About dialog uses the same asset. | Live browser: one visible `img[alt="Kubona"]`; production build passes. |

## Findings

### P0-1 — Campaign synthesis manufactured result-shaped state (closed in Phase 1)

**Before:** `campaign_synth.go` built a template QASM circuit, `vqeVal := fciVal + 0.00047`,
a 30-step convergence trajectory, branch outcomes, projected epistemics, a notebook, a
98.4% canary, and a 4.0x cut estimate. The state was labelled synthesized/projected but
still populated the product as if it were campaign state.

**Now:** the creation path is `BuildCampaignProposal`:

- `trajectory: []`, `branches: []`
- `epistemics: null`, `notebook: null`
- `ClassicalAnchor` carries RHF/FCI only when the live PySCF bridge answered;
  `anchor_computed:false` + `anchor_source:"uncomputed"` otherwise, with no borrowed
  table energies.
- `vqe_expectation_energy`, `discrepancy_mha`, `clifford_fidelity`, and
  `chemical_accuracy_passed` are omitted until a measured fold sets them.
- `wire_cuts: []`.
- A proposal circuit remains, marked `synthesized:true`; it is a proposal, not a result.
- Chat/regenerate/tribunal no longer re-synthesize the campaign on every turn.
- `GET /api/results/notebooks` returns `[]` instead of an H2 template; `/api/epistemics`
  returns no active claim instead of H2 fallback amplitudes.

**Acceptance evidence:** `TestFreshCampaignIsProposalOnly` asserts the API response has no
notebook, epistemics, trajectory, VQE, fidelity, or cut overhead; the live stack returned
exactly that for a new session. Replan tests assert measurement fields appear only after
job/claim folds.

### P0-2 — Secondary surfaces rendered illustrative defaults (closed in Phase 2)

**Before:** several widgets shipped specific numbers with no measured input: shadow
tomography `14.8x`/`99.2%`, QIR `4.2x`/`12 : 288`, molecular LiH/4e/4o/`0.042 Ha`,
manifold `4.218`/`0.64`/`-1.842`, a 3D `+0.12` ghost plane, and a gallery that showed
"synthetic demo" widget bodies.

**Now:**

- Shadow tomography renders an empty measurement state.
- QIR renders only a real compiler receipt; T-count derives from the QIR when present.
- Molecular active space shows proposal counts and only a recorded gap; per-orbital
  energies are not invented.
- Manifold geometry renders only supplied curvature/Fisher/entropy values.
- The gallery shows an empty state until a measured notebook/claim/trajectory exists.
- `WidgetStage` labels feeds `LIVE`, `PROPOSAL`, or `EMPTY`; no `MIX`/`REF` bodies.
- The 3D ghost is hidden unless a sealed/replayed μ±σ expectation exists; the canvas no
  longer synthesizes a schematic QASM, Hamiltonian, or H2 target.
- `ProofDag` labels a proposal DAG as proposed, and proposed cuts as not executed.

**Acceptance evidence:** new widget empty-path tests, updated SpatialCanvas/3D tests,
GenerativeUIStudio tests, WidgetStage tests, ProofDag proposal label, and a clean grep for
the old literal set in production component paths.

### P0-3 — ResultsStudio placeholder (closed, with residual cleanup)

The original screenshot's `UNVERIFIED DRAFT` report is no longer reachable for a campaign
without a verified claim and notebook. The citable gate is now:

```
COLLAPSED_VERIFIED
&& notebook quantum_energy
&& notebook classical_energy
&& notebook delta_mha
&& provenance_hash not empty and not "uncomputed"
```

Residual: `fmtEnergy`/`fmtDelta` still return the literal `UNVERIFIED` when a value is
absent, and the export status says `UNVERIFIED DRAFT` when a claim exists but no Lean proof
receipt does. Those are absence markers, not invented data; keep them, but make the export
copy say "verified claim; no proof receipt" rather than "draft" in the next cleanup.

### P0-4 — AI dead-end and misleading "Session expired" (fixed)

**Root cause chain:**

1. The live local stack is multi-lab because repeated E2E runs created labs. Anonymous
   `GET /api/sessions` therefore returns 401.
2. `CopilotDrawer` correctly refuses to mint a spend-bearing session without an
   authenticated identity (`authWhoami` + `wasRecentAuthFailure` guard). This is intended.
3. The AuthPanel button called `onSelectSurface('labs')`, but the page had no `'labs'`
   surface; it is a tab under `mesh`. The button therefore did nothing.
4. The drawer interpreted the list 401 as "Session expired" even for an anonymous user.

**Fix:** map `'labs'` to `mesh/labs`, pass `requiresSignIn` when the session list is
multi-lab/401, and use anonymous copy ("Sign in to start a campaign session…") unless an
authenticated session actually expired. The guard stays: no anonymous spend.

**Live verification:** campaign menu sign-in prompt, Open Labs AuthPanel, Operator session
panel, Copilot sign-in prompt, and the Labs transition all work in a browser on
`localhost:3000`.

### P1-5 — Campaign menu raw 401 (fixed)

`Campaigns unavailable: GET /api/sessions refused (HTTP 401)` was a raw transport error in
the menu. Multi-lab/unauthorized errors now render an AuthPanel link; unrelated errors
retain the retry/error path.

### P1-6 — Synthesis trajectory consumers (closed by P0-1)

New campaigns carry no synthesized trajectory, and chat/tribunal no longer overwrite state.
`FlightRecorder` still filters legacy `source === 'synthesized'` rows (`FlightRecorder.tsx:83`)
for pre-Phase-1 stored campaigns. A migration may discard or relabel those old rows; no new
row is written.

## Remediation plan

### Phase 0 — Truth at the interaction boundary (done in this pass)

- Remove the exact placeholder report.
- Remove the console "Reference Demo" campaign path.
- Fix AI/AuthPanel routing and anonymous/expired copy.
- Add the logo.
- Keep tests and build green.

### Phase 1 — Campaign data model split (done)

1. `CampaignState` now uses nullable `EpistemicState`/`SystematicNotebook`; a proposal is
   not a measurement object with zero values.
2. `GET /api/campaign` returns `notebook: null`, `epistemics: null`, `trajectory: []`
   until evidence folds.
3. Template arithmetic is deleted from the scaffold: no `fci + 0.00047`, no 98.4%,
   no 4.0x, no hardcoded fallback energies.
4. Contract/regression tests cover the empty campaign and the no-overwrite rule; console
   types and empty states are updated.
5. `KUBONA_ALLOW_ANCHOR_FALLBACK=1` now permits only an explicit uncomputed scaffold;
   production still refuses a missing anchor. The flag cannot reintroduce borrowed values.

### Phase 2 — Surface-by-surface de-placeholdering (done)

1. Widgets gate on real input; numeric defaults are removed.
2. Illustrative bodies are empty states; no preview/mock surface ships.
3. Canvas and 3D use "No target" until a campaign exists.
4. The `-1.137` FCI fallback and +0.12 ghost offset are removed; the ghost renders only
   for a sealed/replayed μ±σ prediction.
5. `ProofDag` labels proposal vs measured and proposed cuts.

### Phase 3 — Provenance UI and regression gate (done)

1. Added `ProvenanceBadge` (`measured`/`proposed`/`catalog`/`model`) and
   `data-provenance` coverage across the result-bearing surfaces. FleetRadar keeps its
   existing per-row provenance chips rather than a misleading global badge.
2. Added `data-metric` markers plus `provenance-coverage.test.tsx`; metrics without a
   provenance ancestor fail. Workbench canvas and list cards are checked.
3. Added `scripts/check-console-placeholders.py`, wired into the Next.js CI job; it
   rejects the old illustrative literals in production component/app/lib files.
4. Full console tests, `tsc`, lint, production build, and real-stack E2E smoke pass.
   CI and nightly still need the final run.

## What is not a finding

- `KUBONA_ALLOW_ANCHOR_FALLBACK=1` is an explicit opt-in demo escape hatch; production
  refuses a missing anchor. It is acceptable only if it never ships in a production image.
- `FleetRadar`, `HuggingFaceModelHub`, periodic-table data, and backend catalogues are
  reference data, not measurements, provided they are labeled as catalogues.
- Unit-test fixtures containing H2/LiH energies are test data, not product claims.
- Empty-state `UNVERIFIED` markers are honest absence markers, not fabricated values.
- `KUBONA_CONSOLE_WORKBENCH=0` remains the tested rollback to the legacy orbital surface.

## Verification log

- Full Go suite passes (`go test ./...`).
- Full console suite: 114 files / 570 tests pass.
- `tsc --noEmit`, ESLint, and the production build pass.
- Phase-1 live check on the real stack: a fresh session returned
  `trajectory: []`, `branches: []`, `epistemics: null`, `notebook: null`, no
  `vqe_expectation_energy`, no `clifford_fidelity`, and `wire_cuts: []`; the
  notebooks endpoint returned `[]`; the epistemics endpoint returned no active claim.
- Live browser: logo visible; campaign 401 sign-in prompt visible; Copilot anonymous
  copy is "Sign in…" (not "Session expired"); Open Labs AuthPanel reaches Operator
  session; the Reference Demo option is absent.
- Real-stack Console E2E smoke (boot, anonymous quiet, deep links): 3/3 pass after the Phase-2 rebuild.
- Live gallery check: no campaign renders the empty gallery state, and none of
  `14.8x`, `99.2%`, `4.2x`, `4.218`, `0.64`, or `synthetic demo` appears.
- Phase-2/3 literal gate: `scripts/check-console-placeholders.py` clean over 119
  production files; it rejects `14.8x`, `99.2%`, `4.2x`, `4.218`, `0.64`, `-1.842`,
  `-1.137`, `0.042`, `12 : 288`, `98.4`, `0.00047`, `4.0x`, `synthetic demo`, and the
  old draft labels. CI runs it in the console job.
- Full CI and nightly still need the final gate run on the exact commit.

## Relevant files

- `platform/core/pkg/agents/campaign_synth.go` — proposal-only scaffold (P0-1).
- `platform/core/pkg/agents/replan.go` — creates notebook/epistemics only on measured folds.
- `platform/core/pkg/api/server.go` — null/empty endpoints, no chat re-synthesis, fail-closed create.
- `platform/core/pkg/api/agent_tools.go` — null measurement fields for proposals.
- `platform/core/pkg/api/replan_hook_test.go` — proposal, notebook, epistemics, and no-overwrite tests.
- `platform/console/src/types/index.ts` — nullable measurement types.
- `platform/console/src/components/surfaces/SpatialCanvas.tsx` / `SpatialCanvas3D.tsx` — no schematic QASM/Hamiltonian; hidden unsealed ghost.
- `platform/console/src/components/widgets/*` — receipt/measurement empty states.
- `platform/console/src/components/sovereign/WidgetStage.tsx` — LIVE/PROPOSAL/EMPTY feed labels.
- `platform/console/src/components/sovereign/ProofDagView.tsx` — proposed vs measured DAG/cuts.
- `platform/console/src/components/common/ProvenanceBadge.tsx` — controlled provenance UI.
- `platform/console/src/components/common/provenance-coverage.test.tsx` — metric/provenance gate.
- `scripts/check-console-placeholders.py` — CI literal gate.
- `.github/workflows/ci.yml` — runs the gate in the console job.
- `platform/console/src/components/surfaces/ResultsStudio.tsx` — fixed citable gate.
- `platform/console/src/components/modals/NewCampaignModal.tsx` — removed reference demo.
- `platform/console/src/components/CopilotDrawer.tsx` — no-spend guard and copy.
- `platform/console/src/app/page.tsx` — Labs routing and campaign 401 menu.
- `platform/console/src/components/shell/ModeRail.tsx` — logo.
- `platform/console/src/components/AboutLabModal.tsx` — logo.
- `platform/console/src/components/surfaces/EpistemicManifold.tsx` — removed H2 fallback.
- `platform/console/src/components/surfaces/GenerativeUIStudio.tsx` — remaining gallery demo.
- `platform/console/src/components/widgets/*` — remaining illustrative widgets.
- `platform/console/src/components/surfaces/SpatialCanvas*.tsx` — remaining H2/ghost defaults.
