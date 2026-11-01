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
- Dense workbench: axe WCAG 2 A/AA is green on both the 361-object canvas and
  its list equivalent. This run caught and fixed the composer footer's
  low-contrast 11 px caption.

## Manual scripts (pending)

Run each surface with VoiceOver (macOS) and NVDA (Windows), at 100% and 200%
zoom, in normal and forced-colors modes. Record the exact commit, OS, AT
version, and pass/fail; do not mark a surface passed without running it.

### Shared checks

1. Load `/`; confirm the skip link is the first Tab stop and moves focus to
   `#kubona-main`.
2. Tab through the primary rail; every mode announces its name, pressed state,
   and destination.
3. Turn on forced colors; confirm boundaries, focus rings, and chart axes stay
   visible with no horizontal overflow.
4. Set the 200%-zoom equivalent (720×450); confirm the shell never scrolls
   horizontally and the composer remains reachable.
5. Trigger one backend refusal; confirm the reason is announced once, not per
   poll or stream frame.

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

### Per-surface script

- **Arena / Workbench**: arrow to a card, press Space (selected announced once),
  Enter (object view opens), Escape (returns), Delete then Undo (removal and
  restoration announced). Switch to List; confirm the same object is reachable
  and its facts read in order.
- **Copilot / Inspector**: open from the rail, confirm focus enters the
  composer, send a short prompt, confirm one completion announcement (not per
  token), close, and confirm focus returns to the opener.
- **Flight Recorder / Evidence**: confirm the figure exposes its summary, the
  exact-values table follows it, and CSV copy announces success.
- **Fleet / Operations**: confirm queue/status labels are not color-only and
  the credentialless badge is announced with the backend name.
- **Settings / Codesign / Quantum Dispatch**: confirm form errors are tied to
  fields, refusal copy is verbatim, and a settled write announces applied,
  refused, or unknown exactly once.

Record one row per pass with the exact commit, OS, assistive technology
version, and pass/fail. Do not mark a surface passed without running it.
