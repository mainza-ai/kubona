---
type: concept
name: Observatory plan-vs-implementation gap audit (Oct 2026)
date: 2026-10-23
updated: 2026-10-25
status: tracked
tags: [observatory, audit, gaps, plan-conformance, atlas]
sources:
  - source-world-labs-atlas-2026
---

# Observatory plan-vs-implementation gap audit (2026-10-23)

Full re-verification of the master plan (`dev-docs/kubona_observatory_master_plan.md`)
against the shipped tree (post-`e62dc1c`). Method: every claim below was
checked against code, tests, or live Atlas responses on 2026-10-23 — not
against prior audit pages. Verdicts: **Done** (implemented + tested),
**Partial** (present but incomplete or unproven), **Absent** (not built),
**Vision** (plan frames it as research/future — not a defect).

Related: [[observatory-lens-audit-2026-10-23]] (findings F-01–F-13, P0–P6
execution record), [[observatory-spatial-representation-layer]] (design),
[[world-labs-atlas]] (vendor contract).

## 1. Phase gates (plan §13)

| Phase | Gate | Verdict | Evidence / gap |
|---|---|---|---|
| 1 — Local truth layer | Walkable landscape, $0.00, zero network, 60 FPS, raycast click-to-ledger-ID | **Partial** | Terrain + probe + VSUP real (`SpatialCanvas3D.tsx`); zero-network true unless lens triggered; clicks pin R/V **and mint ledger IDs** (`lib/pin.ts`, R-24); **60 FPS metered but never measured in a browser** (R-23) |
| 2 — Atlas lens adapter | `atlasChisel` client, planar depth encoder (EXR/log-PNG), `dev_inspect`, one-click coherent view | **Partial** | Encoder + chisel dispatch + upload + history + promptUsed all land + tested; warp consumption **proven live** (flip-pair NCC 0.14→0.62); backend-lane **proven live 2026-10-23** (pose + purge + T2I through kubonad, R-02/R-03/R-20); but **no browser click-through ever ran** (R-01) and depth→chisel via kubonad hit Atlas `EXECUTION_FAILED` on a PNG gradient depth (R-31) |
| 3 — Vault + surprise | `prediction_sealed/evaluated` in taxonomy, ghost surface, z-gap volume, seal→sim→read | **Partial** | Seal/evaluate/**assert**/calibration/history + ledger events all land + tested; ghost renders the **μ±σ band** (R-07) with amber/rose **gap tint** (R-06); z-badge + replay scrubber (R-15) land; **Brier stored but never feeds budgets** (R-08); `vault_assert` mints a ledger `claim_record_id` — full `ScientificClaim` minting stays human-gated (R-09 partial) |
| 4 — Director + calm check-in | Alert routing, AP2 batching, L0–L3 | **Done** | Routing + **gate batching, duplicate collapse, precision tracking** (R-11) + **Hands/Eyes/Ear/Away dial** (R-10) + ambient audio + CheckInStrip pill all land + tested; mobile push absent (R-27, operator-gated) |
| 5 — Agent presence + 4D replay | `world.*` tools, gaze cones + trails, replay via camera path | **Partial** | All 7 tools mounted, governed, catalogued, tested (10 tests); **gaze cones render** (cap-8 `ArrowHelper` trails, R-14) and **replay scrubs** through ghost/gap/badge (R-15); `world.look` carries a **validated lens-request draft** that never dispatches without approval (R-29); gaze is call-driven, not a stream (R-30) |

## 2. Cross-cutting plan requirements

| Requirement (plan §) | Verdict | Evidence / gap |
|---|---|---|
| Truth contract: skin watermarked, non-evidential | **Done** | `[SYNTHETIC LENS — WORLD LABS ATLAS]` badge + promptUsed display; hit-testing resolves against local terrain, never Atlas depth |
| Truth contract: annotations ledger-bound | **Done** | Sealed μ±σ + z + promptUsed all read from ledger/API, never invented |
| Two-tier + air-gap ($0.00 local always works) | **Done** | Pavilion + terrain + vault render with zero egress; lens refuses 403 under air-gap (tested) |
| Data sovereignty: retention modes, ephemeral purge | **Partial** | `dev_inspect` default (Q-36); purge flag plumbed + mock-tested, **fired live 2026-10-23** (R-03: `purged=['asset_e269…']` in resolve record); **explicit asset DELETE** lands as defense-in-depth (R-17); deletion-after-~1h re-check not yet done (Atlas 30-day retention clock is theirs) |
| Data sovereignty: zero-PII egress filter | **Done** | `detectSecretsInPrompt` fail-closes on API keys, private-key blocks, and cloud credential prefixes before dispatch (R-16, F-11 closed); scientific prose passes untouched |
| Cost governance: $0.50 reservation, idempotency, error codes | **Done** | Allowlist, pre-spend validation, derived asset keys, 402/404/429 mapping + tests |
| Cost governance: free lens over cached depth (plan §3) | **Absent** | Every dispatch bills; design fork → Q-39 |
| Provenance stain on human touch (§3) | **Done** | Every probe pin appends a `touched:true` ledger record and renders the amber stain halo; pin click resolves to `ledger_hash` (R-12, R-24) |
| Webhooks (§5 contract) | **Partial** | Verifier + resolve-on-delivery + e2e tests land; **never observed live** (no public URL; poll remains the path, R-04) |
| Poll lifecycle (bounded, resolve, history) | **Done** | 40×3 s cap, resolve-on-done, reservation ledger UI |
| MCP catalog discipline | **Done** | 36 tools, verdicts filled, scope mapping enforced, budget test green |
| Secrets hygiene | **Done** | `.env` + `dev-docs/` + `.obsidian/` gitignored; no live keys in tree (only `wl_live_mock_key` fixtures); key rotation verified 200 |
| T2I A/B + burn metering (Q-38) | **Partial** | T2I outage **cleared 2026-10-23** (R-20: `done:true` + `asset_urls` via kubonad); burn needs the billing dashboard (no balance API) |
| Rooms 1:1 (§9, 11 rooms) | **Vision** | Terrain ≈ Instrument Hall; CheckInStrip ≈ Power Room glance; rest unbuilt — plan-framed, not MVP defects (R-28) |
| Relativistic time / volumetric history / inferred architecture | **Vision** | Research-grade; not defects (noted for completeness) |

## 3. Concrete remaining work (ranked)

**Prove-it-live (nothing here is proven until it runs against the real API):**
- **R-01 Browser E2E.** Click encode→chisel→preview + snapshot→pose in the real console; assert promptUsed + history entry. Nothing in P1c ever executed outside mocks. (~$1–2, operator browser session)
- **R-02 Backend-lane live proof.** ~~Snapshot→pose and depth→chisel *through kubonad* (all Q-38 live ops bypassed it via direct curl). (~$1–2)~~ **Done live 2026-10-23** ($2.00, cap $2.50): pose dispatched through kubonad → `done:true` → resolve 200 with `asset_urls` → history lists the op `resolved:true`; depth PNG uploaded via backend assets endpoint (real Atlas `asset_id`) → chisel dispatched through kubonad with `depth_asset_id` (no validation refusal) — lane verified to Atlas execution (see R-31 for the Atlas-side failure).
- **R-03 Ephemeral-purge live path.** ~~Flagged dispatch → local artifacts + cloud purge; verify deletion after ~1h. (~$0.50 + patience)~~ **Done live 2026-10-23**: flagged pose through kubonad → resolve record carries `purged=['asset_e269374e2f55e90470f505a2cfaa5d86']`, history flags `ephemeral_purge:true`. Deletion-after-~1h re-check not yet done (Atlas 30-day retention clock is theirs).
- **R-31 Depth-input fidelity for backend-lane chisel.** ~~PNG gradient failed with Atlas `code 13 EXECUTION_FAILED`~~ **Done live 2026-10-23** ($0.50): genuine encoder-pipeline EXR (65×65 float from production `buildTerrainHeightField` + `encodeDepthEXR`, LiH, ground −1.137 — GL view-render step excluded, headless has no GL) uploaded via backend assets endpoint → Atlas `asset_4b0408cf…` → depth-conditioned chisel `b25c8180…` → **SUCCEEDED** → resolve 200 with `asset_urls`. The R-02 failure was the PNG input, not the lane or the encoder shape.
- **R-04 Webhook live delivery.** Needs a public deployment; until then the poll rail is the path and the receiver is tested-but-unproven code.
- **R-05 Browser depth-encoder output live.** Encoder bytes have only met mocks + hermetic tests; run one chisel off real encoder output (folds into R-01).
- **R-20 T2I re-probe.** ~~Outage status unknown since 2026-10-23; one `enhancePrompt:false` call settles it.~~ **Done live 2026-10-23**: `atlasTextToImage` through kubonad → `done:true` → resolve 200 with `asset_urls`. The `TRANSIENT_UPSTREAM` outage has cleared.
- **R-21 Burn numbers.** Dashboard-only; no API exists.

**Product gaps (design-conformant, unblocked):**
- **R-08 Brier→budget coupling.** Feed author calibration into autonomous spend envelopes (plan §8.3). Open — the code defers it as a human-gated question (`vault_predictions.go`, calibration comment).
- **R-23 60 FPS measurement.** The meter (`lib/fps.ts`) is wired to the rAF loop and unit-tested, but no browser session has ever reported a number. Folds into R-01.
- **R-30 Gaze stream.** Gaze is call-driven (`ArrowHelper` trails over recent `world.gaze` calls); the plan's live gaze stream is unbuilt. Open, low rank.
- **R-24 Click-to-ledger-ID.** ~~Clicks pin coordinates; plan asked for ledger identity.~~ **Done 2026-10-23**: `lib/pin.ts` (`pinValuesFromHit` + `buildPinPayload`, single-source `touched:true` payload) wired into the `SpatialCanvas3D` click handler, 4 unit tests green; headless raycast click still needs the operator browser (folds into R-01).
- **R-06 Surprise gap volume.** ~~Amber/rose divergence mesh~~ **Done `a624d3c`**: gap tint shader (`uShowGap`) between the μ-plane and the terrain.
- **R-07 Ghost μ±σ band.** ~~Two translucent wireframes~~ **Done `a624d3c`**: `ghostUpper`/`ghostLower` μ±σ shells.
- **R-09 Formal assertion minting.** ~~Mint `ScientificClaim` on verified evaluation~~ **Done in part `a624d3c`**: `vault_assert` mints a ledger `claim_record_id` (rho∈(0,1] + evidence + deterministic hash); full tribunal `ScientificClaim` minting stays human-gated.
- **R-10 Presence control UI.** ~~Hands/Eyes/Ear/Away dial~~ **Done `a624d3c`**: dial in `CheckInStrip`, provider + gating live.
- **R-11 Director completeness.** ~~Gate batching, duplicate collapse, alarm-precision tracking~~ **Done `a624d3c`**: `batchGateRequests`, `shouldCollapse`, `createPrecisionTracker` + auto-tighten, all tested.
- **R-12 Provenance stain.** ~~Mark human-touched regions~~ **Done `a624d3c`**: `touched:true` ledger records + amber stain halo.
- **R-14/R-15 Gaze cones + replay scrub UI.** ~~Tools exist; nothing renders them~~ **Done `a624d3c`**: gaze `ArrowHelper` trails (cap 8) + replay scrubber through ghost/gap/badge.
- **R-16 Prompt egress sanitizer.** ~~Controlled-vocabulary filter~~ **Done `a624d3c`**: `detectSecretsInPrompt` fail-closes pre-spend, tested both directions.
- **R-17 Explicit purge DELETE.** ~~Per-request flag only~~ **Done `a624d3c`**: `deleteAtlasAsset` defense-in-depth + mock-tested.
- **R-29 `world.look` lens-request path.** ~~Read-only~~ **Done `a624d3c`**: validated `request_lens` draft, never auto-dispatches (governed, tested).

**Test gaps (all pinned `a624d3c`):**
- **R-18a** ~~CheckInStrip pill rendering~~ **Done**: pill + duplicate-collapse test (`CheckInStrip.test.tsx`, R-11/R-18).
- **R-18b** ~~Resolve path for failed Atlas operations~~ **Done**: `done+error` resolve test (`observatory_assets_test.go:462`).
- **R-18c** ~~Webhook JWKS-rotation refetch path~~ **Done**: rotation-branch tests (`observatory_webhooks_internal_test.go`).
- Ambient-audio engine + GL render paths: headless-untestable by construction (documented in code, acceptable).

**Hygiene (minutes each):**
- **R-19** ~~Remove dead `DepthData` field~~ **Done**: no readers, field removed.
- **R-25** ~~`scratch/` sits untracked~~ **Done 2026-10-25**: added `scratch/` to `.gitignore` under `# Local-only working files — never pushed`.
- **R-26** ~~Temper README roadmap~~ **Done 2026-10-23**: roadmap line now names the live-proof ledger and the prove-it-live remainders (R-01/R-04/R-05).
- **Q-39** (new, P2): free cached-depth lens vs bill-everything — plan §3 says lenses are free, implementation bills $0.50 per dispatch.

**Operator actions (no code):** training opt-out toggle; dedicated dev project + monthly limit; key-scope review (`operations.cancel`, `assets.delete` only if P6 needs them); CLI `setup` for probes; mobile-push endpoint if Away is ever real (R-27).

## 5. Quantum billing completeness + human dispatch (2026-10-24 audit; implemented hermetic 2026-10-24, live verification pending)

Mechanism is built and hermetically tested (no live keys exist); the
remaining gates are operator rulings (Q-40, Q-41) and live runs.

- **R-32 Audited refund primitive.** ~~No refund path exists~~ **Mechanism done**: Go session `RefundSpend` + `POST /api/sessions/refund` (admin envelope, cross-tenant refusal, no-negative-books, receipted refusals — mirrors tenant `RefundSpend`); Python `SessionStore.release_spend` + governance wrapper (the MCP book had no refund at all). Open policy → Q-41 (signer scope beyond tenant admin, window, partials for partially-executed jobs).
- **R-33 Provider-side cancel.** ~~In-flight hardware runs cannot be recalled~~ **Done where verifiable**: trait-level `cancel()` (fail-closed default); IBM Runtime cancel + qnexus cancel implemented against current vendor docs and injected-tested; `on_hardware_job` pins vendor ids into job meta at dispatch; `request_provider_recall` + bridge `QuantumCancel` + Go/console wiring. Rigetti/D-Wave/pulser/photonic/IonQ/Braket refuse with exact reasons (submit unwired or unverified — no fabricated SDK calls). Cancel never refunds by itself — pairs with R-32.
- **R-34 Human quantum dispatch.** **Done (hermetic)**: `QuantumSubmit/JobStatus/Cancel/RunCanary` bridge RPCs calling the SAME `submit_job` rail (idempotency via persisted job meta, dry-run estimates, tenant attribution); Go `/api/quantum/jobs` proxy (bearer + autonomy + allowlist, no Go-side spend reservation — Python book stays single); `QuantumDispatch` console surface (estimate→mandate card→dispatch→poll→cancel). `mandate_ref` recorded, not enforced, pending the Q-10 conformance ruling (new Q-40).

## 4. Live-proof ledger (what the API actually answered)

Proven live: pose SUCCEEDED (recovered camera within 1 mm of ground truth);
chisel SUCCEEDED prompt-only + depth-conditioned (flip-pair: NCC 0.14 →
0.62, depth causally drives output); generate SUCCEEDED (exact requested
camera); inline asset upload 201 READY incl. 7.4 MiB RGBA EXR.
Backend-lane live 2026-10-23 through kubonad (session spend $2.00, cap
$2.50): pose `f82741b9…` → `done:true` → `asset_urls` + history
`resolved:true`; purge-flagged pose `fd821cad…` → `purged_assets:
[asset_e269374e…]`, history `ephemeral_purge:true`; depth PNG upload →
Atlas `asset_id asset_8e6e3d9d…` → chisel `0636eba1…` reached Atlas
execution, which returned `code 13 EXECUTION_FAILED` (PNG gradient is not
valid chisel depth input — R-31); T2I `940752e3…` → `done:true` +
`asset_urls` (outage cleared). Live since: backend dispatch lane, resolve,
purge download + `purged_assets`, lens history, T2I. Still mock-only:
canvas snapshots (backend lane used synthetic PNGs — real snapshot bytes
need R-01), encoder output, all `world.*` writes, webhooks. Never:
browser click-through, webhook delivery, burn totals, purge
deletion-after-~1h re-check.
