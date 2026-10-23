---
type: concept
name: Observatory lens audit & completion plan (Oct 2026)
date: 2026-10-23
updated: 2026-10-23
status: tracked
tags: [observatory, atlas, audit, plan, honesty, ui]
sources:
  - source-world-labs-atlas-2026
---

# Observatory lens audit & completion plan (Oct 2026)

Deep-dive audit of the Observatory lens implementation against
[[observatory-spatial-representation-layer]] and the live Atlas beta contract
([[source-world-labs-atlas-2026]]), superseding the 2026-10-22 handover notes
(where they were shallow or wrong). Code anchors verified 2026-10-23
pre-remediation; remediated items name the landing change.

**Verdict: the mission is ~15–20% complete.** Tier-1 truth rendering is real;
the Atlas sidecar is a dispatch proxy with no depth pipeline, no 3-D
reconstruction, and one active fiction (stock-photo input). Master-plan
Phase 1 ≈ 70%, Phase 2 ≈ 15%, Phase 3 ≈ 10% (taxonomy only), Phases 4–5 at 0%.

## 1. What is real (verified)

- PES terrain + hover/click energy probe bound to the campaign FCI anchor:
  `platform/console/src/components/surfaces/SpatialCanvas3D.tsx:86-101,389-458`.
- Air-gap refusal, ghost-session refusal, spend-ceiling enforcement, refund on
  missing key / upstream rejection, Merkle `annotation` commit:
  `platform/core/pkg/api/observatory_lens.go:118-149,227-270`.
- Route bearer gating (`platform/core/pkg/api/server.go:497` via
  `requireBearer`, `platform/core/pkg/api/edge_auth.go:35-49`); key
  server-side only, never in the bundle.
- 6 behavioral Go tests (`observatory_lens_test.go`, 257 lines); auth on the
  live API verified 2026-10-23 (`GET operations` → 200, free read, no spend).

## 2. Handover corrections (the notes were wrong twice)

- **C-01 — no error-triggered fallback exists.** The notes blame a
  `TRANSIENT_UPSTREAM` failure for routing to `images2PosedRGBD` with a stock
  photo. The code has no such error string and no routing logic: `taskType`
  defaults to `images2PosedRGBD` when empty
  (`observatory_lens.go:151-154`, frontend default `api.ts:1866`), and the
  Pexels URL fires on **empty `ImageURL`** — i.e. on every default call. The
  defect is the happy path, not a fallback.
- **C-02 — the chisel branch would not survive contact with the API.**
  `targetCameras[]` sends `{camera:{position,orientation,coordinateSystem},
  width,height}`; the live schema requires full `PinholeCamera`s
  (`extrinsics{position,quaternion,coordinateSystem}` +
  `intrinsics{fx,fy,cx,cy,width,height}`) — expect HTTP 422. The mock test
  pins the wrong shape (`observatory_lens_test.go:235-243`).

## 3. Findings the handover missed (pre-remediation)

- **F-01 Pexels stock photo as default input** (`observatory_lens.go:161`):
a cleanroom/telescope stock photo is submitted as the scene. Violates the
plan's zero-PII/controlled-vocabulary egress rule and the docs'
world-first-inputs guidance. → *Remediated P0a: fail-closed `image_required`,
no substitution ever.* (Nuance, 2026-10-23: URL-input *mechanism* is
docs-blessed — the web quickstart itself poses a Pexels hotlink. The fiction
was unrelated *content* as the silent default, not the transport.)
- **F-02 No depth encoder anywhere**: no EXR/log-PNG writer, no RUB camera
  builder with intrinsics, no `contextFrames` — Phase 2's core artifact is
  absent, not merely unwired.
- **F-03 No idempotency**: no `Idempotency-Key` sent
  (`observatory_lens.go:226-234`); double-click = double $0.50.
  → *Remediated P0a: client-or-UUID key forwarded, echoed in response,
  sealed in the ledger annotation.*
- **F-04 No error-code branching**: every upstream failure collapses to
  `bridge_unavailable`; 402/404-entitlement/429+`Retry-After` are
  indistinguishable. → *Remediated P0b: distinct codes
  (`credits_unavailable`, `spend_limit_exceeded`, `task_not_entitled`,
  `rate_limited` with `Retry-After` surfaced, `bridge_unavailable`
  retryable flag).*
- **F-05 Retention gap**: no `purgeContentOnOperationCompletion`, no asset
  DELETE, no local artifact persistence — plan §6 `ephemeral_purge`
  unimplemented, and output `url`s expire in 24 h (docs). Today's lens card
  is a dead link tomorrow. → *Tracked: Q-36.*
- **F-06 Hardcoded price**: `$0.50` in Go + console copy vs docs "never embed
  model prices" and plan §14 credit-burn metering. → *Priced constant kept
  behind one named const with a live-rate TODO; metering tracked in plan P6.*
- **F-07 Divergent physics**: workbench Morse (`a=1.44`, `De=|FCI|·0.15`,
  `SpatialCanvas.tsx:207-211`) vs observatory Morse (`a=1.6`, physical De
  table, `SpatialCanvas3D.tsx:86-92`) — same molecule, two energies.
  → *Remediated P0c: one shared `lib/morse.ts` (physical parameters).*
- **F-08 Poll-loop gaps** (`SpatialCanvas.tsx:132-167`): fixed 3 s interval,
  no backoff/deadline, swallowed errors, `images2PosedRGBD`-shaped parser
  only. → *Remediated P0b (bounded attempts, surfaced errors); chisel/generate
  shapes tracked in P1.*
- **F-09 Silent task-type coercion**: unknown `task_type` falls into
  `default:` and spends. → *Remediated P0a: allowlist validated before
  `ReserveSpend`.*
- **F-10 Op-ID amnesia**: operation ID lives in React state only.
  → *Partial P0a: idempotency key + op ID sealed in the ledger annotation;
  full job-record persistence tracked in P6.*
- **F-11 Egress sanitizer absent**: free-text prompt egress with no
  controlled-vocabulary filter (plan §6). No SMILES leak today
  (`target_system` only) — the control still doesn't exist. → *P1.*
- **F-12 Theater-adjacent ghost**: the "Vault Ghost" toggle renders a static
  +0.12 wireframe unbound to any sealed prediction, reading as Phase-3
  progress. → *Remediated P0c (relabeled illustrative) and P3 (bound to
  sealed μ + z-badge + seal/evaluate loop).*
- **F-13 Missing edge auth on the spend route (found 2026-10-23 during P3
  wiring):** `/api/observatory/lens` was the only spend-authority route
  without `requireBearer` and with no in-handler `checkAuth` — a bare
  session UUID could bill the lab $0.50/click (compounded by the known
  session-enumeration hole). Every sibling spend route (mandates,
  metasearch, vault, HF, ledger, grants, tenants) is wrapped. →
  *Remediated with the P3 routes: lens + all three prediction routes
  wrapped; 401 pin proves no spend moves unauthed.*

## 4. Wiki drift corrected alongside (register discipline)

The blueprint was describing code that never existed:

- `[[observatory-spatial-representation-layer]]` §10: backend claimed as
  `atlasChisel`-specific proxy to `api.worldlabs.ai` with error code
  `airgap_blocked`, `llm_spend_estimated` ledger recording, `three ^0.170.0`.
  Verified: default `images2PosedRGBD` to `api.atlas-beta.worldlabs.ai`,
  code `forbidden`, spend via `sessionMgr` + text annotation, `three ^0.186.1`.
  → ⚑ Correction blocks on the page.
- `[[world-labs-atlas]]`: webhooks claimed as "HMAC-signed (`WLT-Signature`)";
  the live contract is Ed25519 `v1a` Standard Webhooks — no HMAC, no such
  header. Air-gap refusal claimed as `AirGapEnforcedError`; the code writes
  `forbidden`. → ⚑ Correction blocks on the page.

## 5. Completion plan (status 2026-10-23)

- **P0 landed:** F-01, F-03, F-04, F-07, F-08 (bounded), F-09, F-12
  (honesty), F-10 (partial), F-13 (edge auth). Verified: Go lens tests
  incl. new pins, console suite, `wiki-lint --strict` clean.
- **P3 landed 2026-10-23:** seal/evaluate/latest endpoints
  (`/api/vault/predictions/*`, `prediction_sealed`/`prediction_evaluated`
  ledger events, σ floor 1e-4, deterministic hash, z + sealed-3σ-Brier +
  level math pinned); console Vault panel (seal form gated on FCI anchor,
  ghost bound to sealed μ, z-badge, evaluate-pinned-point loop); F-12/F-13
  closed. CheckInStrip director levels stay P4 work.
- **Q-37 landed 2026-10-23:** snapshot capture (preserveDrawingBuffer +
  16:9 cover-crop to exact 1280×720, null-safe fail-closed) + Atlas inline
  asset-upload flow (`data:` URLs → project asset → assetId frame refs,
  9 MiB cap, derived idempotency key) — all mock-tested, zero live spend.
- **P2 zero-cost prep landed 2026-10-23:** `src/lib/depth.ts`
  (half/float EXR decode, bottom-up→top-down flip, planar unproject,
  finite-stats) with hermetic EXR round-trip tests proving the row flip;
  interop pin: three r186 EXRExporter ZIP default misdecodes in its own
  EXRLoader — P1 must encode uncompressed (or ZIPS).
- **P1 — real chisel lens:** planar-depth encoder (offscreen 1280×720
EXR + log-PNG, CPU row-flip per docs), RUB cameras with computed
intrinsics, `contextFrames` 1:1 with targets, `enhancePrompt` control +
`promptUsed` persistence, retention flag (`dev_inspect` per Q-36),
download artifacts to `~/.kubona/artifacts/`. Gate: unchecked plan §14
boxes — **entitlement proof DONE 2026-10-23 for pose/chisel/generate**
(Q-38 probe: pose SUCCEEDED, chisel SUCCEEDED, generate SUCCEEDED with
exact requested camera; text-to-image 0/4 with sustained
`TRANSIENT_UPSTREAM`, re-probe another day). Live corrections to the
plan: chisel requires ≥1 `targetCameras` even prompt-only (422
otherwise); asset-create returns `{"id": "asset_..."}` (not the
tutorial's resource-name shape — backend fixed + pinned).
  - Workflow-derived rules ([[source-world-labs-atlas-2026]],
  generated-world-edit-workflow): posed RGB output is **never passed
  through verbatim** — strip to depth-only + camera (omit RGB, mask,
  confidence) when building chisel `contextFrames`; **never rescale
  pixels after posing** (capture at exactly 1280×720 —
  a client intrinsics scale misses the center-crop offset); snapshot
  PNGs <10 MiB ride inline base64 (`POST /api/v2/assets`),
  upload flow only above that.
  - Route awareness (atlasGenerate schema): the Depth-consuming path is the
  **reference-context warp route**; the hero base checkpoint conditions on
  posed images only and ignores depth. Routing is automatic (omit `model`),
  so P2 must prove depth was consumed empirically (view-consistency check).
  Keep ≤19 context frames with centered principal points or frames silently
  skip reconstruction; `base64Outputs` + `?wait=true` is the fast path
  (skips storage) with 60 s fallback to poll.
- **P2 — depth backprojection + posed loop:** fetch `depthAsset` EXR,
row-flip decode, unproject against **Kubona's own input depth** (never
Atlas-returned depth for measurement), `atlasGenerate` context loop;
`splats2Mesh` spike only if `.spz` path materializes. Upgrade path from
the workflow tutorial: **N orbit snapshots are real overlapping
captures** (unlike independent text-to-image variations, which must
never be treated as multi-view) — one multi-view `images2PosedRGBD`
over them yields a consistent rig for `atlasGenerate`; `returnDepth`
only with ≥2 translated views. Reference implementation:
`worldlabsai/atlas-camera-studio` (MIT, reference class — decode EXR,
colored point cloud, path smoothing, 48-camera interpolation) and its
`public/example/` bundle (image + EXR + cameras) as a **zero-spend local
fixture** for the decode/unproject path; geometry helpers in
`worldlabsai/marble-multiview-inpainting` (MIT: lift/project/occlusion,
same pinhole schema) as method reference.
- **P3 — Vault + surprise:** emit `prediction_sealed` (μ, σ ≥ 1e-4 floor)
  from the lens action; ghost renders the ledger μ±σ band; on run completion
  emit `prediction_evaluated`, compute z, render gap volume
  (cyan/amber/rose), wire to `CheckInStrip` levels.
- **P4 — Director + audio:** L0–L3 routing, gate batching, duplicate
  collapse, presence gating, WebAudio hum/clicks/chime + mute.
- **P5 — Agents + rooms:** `world.{look,probe,anchor,gaze,summon,handoff,
  replay}` MCP tools with the plan's governance gates; gaze cones; 4D
  replay; procedural pavilion fallback so air-gap keeps a Tier-1 world.
- **P6 — Harden:** webhook receiver (Ed25519 `v1a`, JWKS cache, `410`
handling), live-schema CI check, credit-burn dashboard, op job-record
persistence. Billing-derived rules: per-project monthly limits (dedicated
dev project + key), Usage-page grouping by key for attribution, spend
limits as guardrails with observation lag (dashboard shows ranges, never
exact estimates), 402 recovery = check limits → add credits → retry the
*same* idempotency key (already P0b). Q-38 probes run via the official
`worldlabs` CLI (`tasks list`, asset upload, `--idempotency-key`,
`operations wait`) — no custom code; operator runs `setup` (key stays
out of chats, `WLT_API_KEY` env takes precedence).

## 6. Decisions (Q-36/Q-37 closed 2026-10-23, Q-38 open)

Q-36 → `dev_inspect` for beta (operator ruling; production gets its own
review). Q-37 → snapshot-accepting with fail-closed fallback (landed;
cover-crop capture + inline asset upload, mock-tested). Q-38 probe
EXECUTED 2026-10-23 (7 task ops + 4 asset creates; pose/chisel/generate
SUCCEEDED, text-to-image 0/4 `TRANSIENT_UPSTREAM` outage; burn totals
need the billing dashboard — no API for balances). Probe shaped as the
tutorial chain (snapshot→pose→chisel→generate); first submit also
disambiguates 403 scope vs 404 entitlement gaps. Fixtures: real Atlas
depth EXR + posed frame + chisel/generate views in gitignored
`dev-docs/kubona-probe-2026-10-23/` with op/asset IDs for re-fetch
(operations 30 d, library assets persist). Account hygiene (operator):
confirm the **training opt-out** (Settings → Account → "Use my data to
improve models"), a dedicated dev project with its own monthly limit,
and key scopes. Official TS client `@worldlabsai/client` evaluated and
declined (Node server-side; the Go proxy stays the governed rail). No
further live spend occurs without explicit approval.
