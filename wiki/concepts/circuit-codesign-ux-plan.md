---
type: concept
name: Circuit Co-Design UX Plan
date: 2026-10-18
updated: 2026-10-20
tags: [ux, console, codesign, frontier, observatory]
sources: [source-llm-circuit-codesign-2026, source-gap-analysis-plan]
status: adopted
---

# Circuit Co-Design UX Plan (backend → frontend, inclusive)

## Backend (landed slice 1)

- Python `platform/kubona/codesign/` (evaluator ground truth): `kickmix.py` (trajectory sim Eqs 10-16, `ε` Eqs 20-21), `tq.py` (shared `κ∈[1,3]`, golden `5×10=50`), `frontier.py` (MAP-Elites min-TQ/cell), `harness.py` (four-part contract, forged-artifact invalid), `passes.py` (MBU saving, routing detector — report only, rewrite stays in search loop).
- Python `console_api.py` (Python-owned, pending Go migration): `POST /api/codesign/evaluate` + `POST /api/codesign/frontier` (`and2/cz-phase/copy1`, `≤64` batch, fail-closed, read-only, no submission endpoints).
- Go `platform/core/pkg/codesign/` (serving mirror, not validity): `tq.go` + `frontier.go` (same golden pinned both sides), `pkg/api/codesign.go` `GET /api/codesign/tq` + `POST /api/codesign/frontier` (read-only, no spend, no jobs). Live-verified 2026-10-18 (`50` golden, HHL `1348×115=155020`).
- LLM: Strata local engine for any proposal calls (OpenAI-compat, default `http://127.0.0.1:47831/v1` overridable via `STRATA_BASE_URL`, model via `STRATA_MODEL`); NVIDIA parked (configured, explicitly selectable, not the default). No LLM calls in slice 1 tests (deterministic).

## Frontend (landed slice 1)

- `CodesignCockpit.tsx` (read-only): TQ scorer + MAP-Elites batch (≤64) + claimed-gains disclaimer. Talks only to Go `:8080`. **No job-submission endpoints** (hard rule #1 in [[ui-ux-vision]]).
- Wired as 4th `proofs` tab (`manuscripts/tribunal/gallery/circuits`) — no new dock icon this slice (minimizes SovereignDock/page/CommandPalette blast radius). Full dock surface deferred to slice 2 behind operator sign-off.
- Test `CodesignCockpit.test.tsx` asserts read-only (no `submit job` text).

## Full refactor (slices 2-5, queued)

1. **Slice 2 — Evaluator runs view:** per-trial ledger refs, `ε`-bound display with Wilson/Hoeffding note, failure-budget union-bound table (Table-S4 style), `κ` slider (`1→3`) + depth/volume toggles when factory data present.
> **✅ LANDED 2026-10-20 (2.2a).** Cockpit "Evaluator runs" card: spec select + editable artifact JSON (live-verified presets) + tests/kappa/seed + optional factory trio → `POST /api/codesign/evaluate` → validity badge, T/Q, ε-bound, depths, kappa sweep, factory report (G3 UI half). Console-tested (mocked rail + refusal paths). Remaining slice-2 scope (per-trial ledger refs, depth/volume toggles) stays queued behind pack/family surfaces.
2. **Slice 3 — Families view:** ≤5 ideation tracks, elite-sharing events, widen-nudge button (human nudge, never auto-prompt-injection), lineage graph (parent→child, non-monotonic allowed).
> **✅ LANDED 2026-10-20.** `POST /api/codesign/families` (bounded evaluated search: same spec, ≤64 offers, ≤3 share rounds, fixed tests; invalid recorded never scored) + cockpit Families card (spec, family list, offers JSON with live-verified presets, share rounds; elites/stagnation/widen-nudge/shares rendered, lineage length shown). Console-tested (mocked rail + refusal paths); Go-tested (elites, invalid-record, 4-cross-offer sharing incl. earned-elite case, deterministic stagnation nudge, oversize/unknown/GET refusals). Replaces the old families process-note (D6: replace, never append).
3. **Slice 4 — Packs:** `numerics → inversion (jstore slider 0/5/9) → DQI (BM/Lanczos/Ψ/stripes) → chemistry (frames, fallback intact) → gauge (4³ only)`, each `kubona-pack/v1` with anchor + evidence reqs.
> **✅ LANDED 2026-10-20.** `GET|POST /api/codesign/packs` serves the registry live from `packs.py` (no hand-typed list); cockpit Domain-packs card renders version/status/evidence/title per pack with loaded/refused/empty states + the promotion rule. Promotion itself stays a privileged op (ledger refs) — read-only rail. Console-tested, Go-tested.
4. **Slice 5 — Go migration:** move `console_api` codesign routes into Go via bridge RPC (Python stays evaluator); delete Python duplicates to honor single-surface rule. Needs operator sign-off (contract change).
> **✅ CLOSED 2026-10-20.** Called paths migrated (`evaluate|evaluate-batch|budget|families|packs|verified`); `serve_console` verified callerless with no compose target — exactly one surface served. `console_api.py` stays as tested reference + harness; `science/*` + stateful routes migrate iff a surface ever calls them.

## Threat model (reads public, writes gated)

Scoring reads stay public behind limiters (same as fleet/epistemics); no spend, no QPU, no secret touches. Forged artifacts invalidate, never score. Oversize batches refuse whole. Console never mints mandates or submits jobs from this surface.

See [[evaluator-guided-circuit-codesign]], [[failure-budget-and-frontier]], [[agent-ux-control-plane]], [[ui-ux-vision]].
