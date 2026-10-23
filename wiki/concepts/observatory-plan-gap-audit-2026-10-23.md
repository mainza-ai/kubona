---
type: concept
name: Observatory plan-vs-implementation gap audit (Oct 2026)
date: 2026-10-23
updated: 2026-10-23
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
| 1 — Local truth layer | Walkable landscape, $0.00, zero network, 60 FPS, raycast click-to-ledger-ID | **Partial** | Terrain + probe + VSUP real (`SpatialCanvas3D.tsx`); zero-network true unless lens triggered; **60 FPS never measured** (R-23); clicks pin R/V, **not ledger IDs** (R-24) |
| 2 — Atlas lens adapter | `atlasChisel` client, planar depth encoder (EXR/log-PNG), `dev_inspect`, one-click coherent view | **Partial** | Encoder + chisel dispatch + upload + history + promptUsed all land + tested; warp consumption **proven live** (flip-pair NCC 0.14→0.62); backend-lane **proven live 2026-10-23** (pose + purge + T2I through kubonad, R-02/R-03/R-20); but **no browser click-through ever ran** (R-01) and depth→chisel via kubonad hit Atlas `EXECUTION_FAILED` on a PNG gradient depth (R-31) |
| 3 — Vault + surprise | `prediction_sealed/evaluated` in taxonomy, ghost surface, z-gap volume, seal→sim→read | **Partial** | Seal/evaluate/latest + ledger events + ghost-at-μ + z-badge all land + tested; **no volumetric gap mesh** (badge only, R-06); ghost is a flat μ plane, **not the μ±σ band** (R-07); Brier stored but **never feeds budgets** (R-08); **no ScientificClaim minting** (R-09) |
| 4 — Director + calm check-in | Alert routing, AP2 batching, L0–L3 | **Partial** | Routing + presence gating + audio + CheckInStrip pill land + tested; **no gate batching, no duplicate collapse, no precision tracking** (R-11); **no presence-switch UI** (hardcoded `hands`, R-10); mobile push absent (R-27) |
| 5 — Agent presence + 4D replay | `world.*` tools, gaze cones + trails, replay via camera path | **Partial** | All 7 tools mounted, governed, catalogued, tested (9 tests); **no gaze-cone rendering, no replay scrub UI** (R-14/R-15); `world.look` has **no lens-request path** the plan promises (R-29); gaze is call-driven, not a stream (R-30) |

## 2. Cross-cutting plan requirements

| Requirement (plan §) | Verdict | Evidence / gap |
|---|---|---|
| Truth contract: skin watermarked, non-evidential | **Done** | `[SYNTHETIC LENS — WORLD LABS ATLAS]` badge + promptUsed display; hit-testing resolves against local terrain, never Atlas depth |
| Truth contract: annotations ledger-bound | **Done** | Sealed μ±σ + z + promptUsed all read from ledger/API, never invented |
| Two-tier + air-gap ($0.00 local always works) | **Done** | Pavilion + terrain + vault render with zero egress; lens refuses 403 under air-gap (tested) |
| Data sovereignty: retention modes, ephemeral purge | **Partial** | `dev_inspect` default (Q-36); purge flag plumbed + mock-tested, **fired live 2026-10-23** (R-03: `purged=['asset_e269…']` in resolve record); purge relies on per-request flag, **no explicit DELETE call** (R-17) |
| Data sovereignty: zero-PII egress filter | **Absent** | Free-text `prompt` egresses unsanitized (F-11 carried) → R-16 |
| Cost governance: $0.50 reservation, idempotency, error codes | **Done** | Allowlist, pre-spend validation, derived asset keys, 402/404/429 mapping + tests |
| Cost governance: free lens over cached depth (plan §3) | **Absent** | Every dispatch bills; design fork → Q-39 |
| Provenance stain on human touch (§3) | **Absent** | No stain marking anywhere → R-12 |
| Webhooks (§5 contract) | **Partial** | Verifier + resolve-on-delivery + e2e tests land; **never observed live** (no public URL; poll remains the path, R-04) |
| Poll lifecycle (bounded, resolve, history) | **Done** | 40×3 s cap, resolve-on-done, reservation ledger UI |
| MCP catalog discipline | **Done** | 36 tools, verdicts filled, scope mapping enforced, budget test green |
| Secrets hygiene | **Done** | `.env` + `dev-docs/` + `.obsidian/` gitignored; no live keys in tree (only `wl_live_mock_key` fixtures); key rotation verified 200 |
| T2I A/B + burn metering (Q-38) | **Partial** | 4/4 `TRANSIENT_UPSTREAM` outage; burn needs dashboard (no balance API) |
| Rooms 1:1 (§9, 11 rooms) | **Vision** | Terrain ≈ Instrument Hall; CheckInStrip ≈ Power Room glance; rest unbuilt — plan-framed, not MVP defects (R-28) |
| Relativistic time / volumetric history / inferred architecture | **Vision** | Research-grade; not defects (noted for completeness) |

## 3. Concrete remaining work (ranked)

**Prove-it-live (nothing here is proven until it runs against the real API):**
- **R-01 Browser E2E.** Click encode→chisel→preview + snapshot→pose in the real console; assert promptUsed + history entry. Nothing in P1c ever executed outside mocks. (~$1–2, operator browser session)
- **R-02 Backend-lane live proof.** ~~Snapshot→pose and depth→chisel *through kubonad* (all Q-38 live ops bypassed it via direct curl). (~$1–2)~~ **Done live 2026-10-23** ($2.00, cap $2.50): pose dispatched through kubonad → `done:true` → resolve 200 with `asset_urls` → history lists the op `resolved:true`; depth PNG uploaded via backend assets endpoint (real Atlas `asset_id`) → chisel dispatched through kubonad with `depth_asset_id` (no validation refusal) — lane verified to Atlas execution (see R-31 for the Atlas-side failure).
- **R-03 Ephemeral-purge live path.** ~~Flagged dispatch → local artifacts + cloud purge; verify deletion after ~1h. (~$0.50 + patience)~~ **Done live 2026-10-23**: flagged pose through kubonad → resolve record carries `purged=['asset_e269374e2f55e90470f505a2cfaa5d86']`, history flags `ephemeral_purge:true`. Deletion-after-~1h re-check not yet done (Atlas 30-day retention clock is theirs).
- **R-31 Depth-input fidelity for backend-lane chisel (new).** The R-02 depth→chisel dispatch reached Atlas execution, which returned `code 13 EXECUTION_FAILED` on a 65×65 PNG gradient depth. The flip-pair proof used EXR depth via direct curl; repeat with real encoder EXR bytes through the backend lane (~$0.50). Open.
- **R-04 Webhook live delivery.** Needs a public deployment; until then the poll rail is the path and the receiver is tested-but-unproven code.
- **R-05 Browser depth-encoder output live.** Encoder bytes have only met mocks + hermetic tests; run one chisel off real encoder output (folds into R-01).
- **R-20 T2I re-probe.** ~~Outage status unknown since 2026-10-23; one `enhancePrompt:false` call settles it.~~ **Done live 2026-10-23**: `atlasTextToImage` through kubonad → `done:true` → resolve 200 with `asset_urls`. The `TRANSIENT_UPSTREAM` outage has cleared.
- **R-21 Burn numbers.** Dashboard-only; no API exists.

**Product gaps (design-conformant, unblocked):**
- **R-06 Surprise gap volume.** Amber/rose divergence mesh between the μ-plane and the terrain (today: z-badge only).
- **R-07 Ghost μ±σ band.** Two translucent wireframes (or a band shader) instead of the flat μ plane.
- **R-08 Brier→budget coupling.** Feed author calibration into autonomous spend envelopes (plan §8.3).
- **R-09 Formal assertion minting.** Mint `ScientificClaim` on verified evaluation (plan §8.1 stage 3).
- **R-10 Presence control UI.** Hands/Eyes/Ear/Away dial; provider + gating exist, default is hardcoded `hands`.
- **R-11 Director completeness.** Gate batching, duplicate collapse, alarm-precision tracking.
- **R-12 Provenance stain.** Mark human-touched regions in the ledger + visually.
- **R-14/R-15 Gaze cones + replay scrub UI.** Tools exist; nothing renders them.
- **R-16 Prompt egress sanitizer.** Controlled-vocabulary filter before cloud egress (F-11).
- **R-17 Explicit purge DELETE.** Today purge relies on the per-request flag; add asset DELETE for defense in depth.
- **R-29 `world.look` lens-request path.** Plan promises it; currently read-only.
- **R-24 Click-to-ledger-ID.** ~~Clicks pin coordinates; plan asked for ledger identity.~~ **Done 2026-10-23**: `lib/pin.ts` (`pinValuesFromHit` + `buildPinPayload`, single-source `touched:true` payload) wired into the `SpatialCanvas3D` click handler, 4 unit tests green; headless raycast click still needs the operator browser (folds into R-01).
- **R-23 60 FPS measurement.** Gate 1's performance clause was never measured.

**Test gaps (shipped code, unpinned behavior):**
- **R-18a** CheckInStrip pill rendering (STUB hides it in tests; no explicit test).
- **R-18b** Resolve path for failed Atlas operations (pending + success tested; `done+error` branch not).
- **R-18c** Webhook JWKS-rotation refetch path (happy + tamper + expiry tested; rotation branch not).
- Ambient-audio engine + GL render paths: headless-untestable by construction (documented in code, acceptable).

**Hygiene (minutes each):**
- **R-19** Remove dead `DepthData` field (`observatory_lens.go:24`, no readers).
- **R-25** `scratch/` sits untracked (operator docx + atlas_quickstart): move into `dev-docs/` (ignored) or delete.
- **R-26** Temper README roadmap ("shipped P0–P6" overstates R-01–R-05/R-11/R-14/R-15/R-24); link this page instead.
- **Q-39** (new, P2): free cached-depth lens vs bill-everything — plan §3 says lenses are free, implementation bills $0.50 per dispatch.

**Operator actions (no code):** training opt-out toggle; dedicated dev project + monthly limit; key-scope review (`operations.cancel`, `assets.delete` only if P6 needs them); CLI `setup` for probes; mobile-push endpoint if Away is ever real (R-27).

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
`asset_urls` (outage cleared). Mock-only:
backend dispatch lane, resolve, purge download, webhooks, snapshots,
encoder output, all `world.*` writes, lens history. Never: browser
click-through, purge-flagged op, webhook delivery, T2I success, burn totals.
