# Manual accessibility record — 2026-11-01

Status: **NOT RUN**. This file records the script and the automated evidence;
it does not claim a manual VoiceOver/NVDA pass.

## Automated evidence (current)

- Axe WCAG 2 A/AA on Arena, Labs, Settings, and Circuit Codesign: green in the
  `a11y` E2E job.
- Skip link is the first keyboard stop and moves focus to `#kubona-main`.
- Forced-colors mode: no horizontal overflow; workbench/chart boundaries remain
  visible.
- 200%-zoom equivalent (720×450): no horizontal overflow; skip link remains
  reachable.
- Reduced-motion E2E: ambient animation utilities resolve to `none`.
- Workbench keyboard: Space selects, Enter opens, arrows nudge, and Delete
  removes with Undo (unit coverage in `Workbench.test.tsx`).

## Manual scripts (pending)

Run each surface with VoiceOver (macOS) and NVDA (Windows), at 100% and 200%
zoom, in normal and forced-colors modes.

| Surface | VoiceOver | NVDA | Notes |
|---|---|---|---|
| Arena / Workbench | PENDING | PENDING | object selection, list equivalent, area rename/assign |
| Copilot / Inspector | PENDING | PENDING | open/close, focus restore, approvals |
| Flight Recorder | PENDING | PENDING | chart summary, values table, CSV |
| Evidence / Manifold | PENDING | PENDING | chart summary, values table |
| Fleet / Operations | PENDING | PENDING | queue chart, provenance labels |
| Settings | PENDING | PENDING | section nav, form errors |
| Codesign | PENDING | PENDING | TQ/frontier chart, tables |
| Quantum Dispatch | PENDING | PENDING | settlement announcements, refusal copy |

Record one row per pass with the exact commit, OS, assistive technology
version, and pass/fail. Do not mark a surface passed without running it.
