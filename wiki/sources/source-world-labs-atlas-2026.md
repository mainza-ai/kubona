---
name: World Labs Atlas / Marble 2 beta documentation (Oct 2026)
type: source
tags: [world-model, atlas, marble, api-contract, generative-lens, source]
date: 2026-10-23
updated: 2026-10-23
status: source
origin: first-party maintainer read of all 23 live Atlas beta documentation pages (https://atlas-beta.worldlabs.ai/docs/*) on 2026-10-23 — concept guides, all task guides, all core-API pages, platform pages, and the four live task schemas in the API reference. No raw/ document (web source, beta contract subject to change).
---

# World Labs Atlas beta docs — Oct 2026 (`source-world-labs-atlas-2026`)

**Evidence class.** This is *not* a `raw/` document. It is a first-party
maintainer read of every page under `https://atlas-beta.worldlabs.ai/docs/`
on 2026-10-23: quickstart, how-to-use-atlas, cameras-and-posed-images,
text-to-image, images-to-posed-images, inpainting, depth-guided-generation,
atlas-generate, atlas-walkthrough, assets, operations, webhooks, errors,
best-practices, billing, rate-limits, authentication, projects-and-api-keys,
generated-world-edit-workflow, api-reference (+ task schemas atlas-chisel,
atlas-text-to-image, images2PosedRGBD, splats2Mesh). Nothing was cloned or
vendored. Beta schemas can change; the live API reference is the source of
truth at implementation time.

**What was read, in full:** all 23 pages above, end to end including request /
response schemas, error-code tables, and the EXR row-order accessor.

**Load-bearing contract facts for [[observatory-spatial-representation-layer]]:**

1. Pinhole cameras only: absolute-pixel intrinsics `fx, fy, cx, cy, width,
   height`; pose is camera-to-world, quaternion **XYZW**, no Euler; `rub`
   matches Three.js; all frames in one request share one world frame + unit.
2. Depth is optical-axis planar view-space *z*, not ray length; `NaN` (or 0)
   means no surface. Three.js `EXRLoader` exposes CPU rows **bottom-up**:
   CPU index must map `y -> height - 1 - y`.
3. `atlasChisel` takes **depth-only** `contextFrames` (no image/mask/
   confidence — rejected, not ignored), exactly one per target, 1280×720,
   plus a required non-empty `prompt`; 1–32 targets; `returnDepth` needs ≥2
   targets; returned depth is *reconstructed*, never copied.
4. `atlasGenerate` needs image + depth + camera per context frame;
   pure-rotation rig + `returnDepth: true` is rejected (422).
5. `images2PosedRGBD` rejects frames carrying camera/depth/mask; default
   grid 1280×720; `?wait=true` submissions die at the 60 s LB cap — poll.
6. Operations: `done` is the only terminal signal; 30-day retention;
   `purgeContentOnOperationCompletion` for ephemeral mode; `Idempotency-Key`
   on every submission; webhooks are Ed25519 `v1a` Standard Webhooks
   (NOT HMAC), `https:443` only, incompatible with `?wait=true`.
7. Errors: branch on status + `code` / `ErrorInfo.reason`
   (`CREDITS_UNAVAILABLE`, `SPEND_LIMIT_EXCEEDED`, `RATE_LIMIT_EXCEEDED`,
   `CONCURRENCY_LIMIT_EXCEEDED`, `TASK_UNAVAILABLE`, `CONTENT_POLICY`,
   `EXECUTION_FAILED`), never on message text; honor `Retry-After`.
8. Billing: 1,000 credits = $1 (beta rate card can change — **never embed
   model prices in client code**); output asset `url`s expire in 24 h
   (persist `assetId`, re-mint reads with `:createReadUrl`).
9. Auth: `WLT-Api-Key` header, project-scoped; 403 = scope, **404 = task
   not entitled on this account**; beta membership is per-account.
10. `atlasMasked` is an internal preview — expect 404 without entitlement.

**Durable analysis:** [[observatory-lens-audit-2026-10-23]] (audit + plan).
**Design:** [[observatory-spatial-representation-layer]].
**Vendor:** [[world-labs-atlas]].
