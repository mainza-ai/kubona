---
type: concept
name: Copilot Drawer Layout Audit 2026-11-17
date: 2026-11-17
updated: 2026-11-17
tags: [kubona, console, copilot, ux, layout, audit]
sources: [source-gap-analysis-plan, source-architecture-paper]
status: tracked
---

# Copilot Drawer Layout Audit (2026-11-17)

> **What this page is.** A code-read audit of the Copilot drawer after a live
> screenshot showed chat prose and the FCI/RHF table clipped at the right edge.
> It records the layout contract that is missing, the gaps, and the ordered
> implementation plan. It is a prototype/UX audit, not a claim that the design
> is finished.

## Observed failure

The live screenshot showed:

- `Target: Lithium Hydride (LiH) · Δ = UNVE…` truncated in the header.
- The FCI/RHF table cut off on the right; the RHF column was only partly visible.
- Prose lines ending mid-word at the drawer edge instead of wrapping.
- No visible horizontal resizer or table scroll affordance.
- Dense 17-digit numbers forcing the table wider than the available column.

The content is not merely “wide”; it is being clipped rather than contained.

## Findings

| # | Finding | Evidence | Impact |
|---|---|---|---|
| 1 | The Inspector has a normal width ceiling of 420 px. | `platform/console/src/components/shell/Inspector.tsx:18-20`, `:76-88` | A dense Copilot transcript with a 3-column numeric table needs more than 420 px. The expanded state helps only if the user discovers the control. |
| 2 | The resize handle is a 4 px invisible strip at the far left and is hidden below `lg`. | `platform/console/src/components/shell/Inspector.tsx:90-112` | The drawer is technically resizable but not discoverable; mobile users cannot resize it at all. The label says “Resize inspector”, not “Resize Copilot”. |
| 3 | `MathText` renders all text inside an `inline-block` span while emitting block-level `<p>`, `<div>`, and `<table>` content. | `platform/console/src/components/common/MathText.tsx:282-285`; table emission at `:154-171` | An inline-block is shrink-to-fit. A table can set a min-content width larger than the bubble, so the bubble overflows the drawer instead of wrapping/scrolling. |
| 4 | The message bubble constrains width with `max-w-[92%]` but has no `min-w-0`, `max-w-full`, or overflow-wrap policy. | `platform/console/src/components/CopilotDrawer.tsx:761-771`; feed at `:522-523` | Flex/grid min-content sizing and long numeric tokens can defeat the percentage cap; the outer `overflow-hidden` then clips the result. |
| 5 | The table wrapper has `overflow-x-auto`, but no `max-w-full`/`min-w-0` containment and the table is `w-full` without a scroll contract. | `platform/console/src/components/common/MathText.tsx:163-171` | The wrapper can itself be forced wider by the table; when it cannot, the parent clips instead of exposing a scrollbar. |
| 6 | Numeric cells use long unformatted floats and no numeric alignment/nowrap policy. | `MathText.tsx:163-171`; screenshot FCI/RHF values | Columns become unnecessarily wide, hard to scan, and force horizontal overflow. |
| 7 | The target is truncated in both the header and the focus ribbon; only the `title` attribute carries the full value. | `CopilotDrawer.tsx:378-380`, `:459-463` | The operator cannot read the campaign identity or the Δ state at a glance; title tooltips are not available on touch or keyboard-only flows. |
| 8 | There is no Copilot-specific width state, preset, reset, or viewport clamp. | `Inspector.tsx:58-72` uses one `kubona.inspector.width` key for all tabs | Switching tabs or resizing the window can leave a stale/unsuitable width; there is no “fit to content” or “wide” preset. |
| 9 | The table has no caption/`scope` attributes and the resize separator lacks `aria-valuetext`. | `MathText.tsx:163-171`; `Inspector.tsx:90-97` | Table semantics and the resize affordance are weaker than the rest of the console’s WCAG gate. |
| 10 | No test exercises a long table inside a narrow drawer. | `MathText.component.test.tsx` covers table rendering but not width containment | The regression can return without CI noticing. |

## Root cause

The immediate clipping has two layers:

1. **Shell width:** the Copilot is constrained by the Inspector’s 320–420 px
   normal range, with a hidden 4 px splitter.
2. **Content sizing:** `MathText`’s inline-block container lets a table establish
   a min-content width larger than the bubble. The bubble/feed do not establish
   `min-w-0`/`max-w-full`, so the table cannot shrink, and the outer
   `overflow-hidden` clips it.

Fixing only the drawer width will make the symptom less frequent; fixing only
the CSS containment will leave the table cramped at 320 px. Both are required.

## Implementation plan

### C1 — Containment contract (P0)

- Make `MathText` text mode a block-level, width-constrained container
  (`min-w-0 max-w-full`), not an inline-block. Keep inline math inline.
- Add a prose wrap policy (`overflow-wrap: anywhere` only for long tokens,
  normal word wrapping for prose).
- Make the table wrapper `min-w-0 max-w-full overflow-x-auto`; give the table
  an explicit minimum width so it scrolls rather than expands the bubble.
- Add `min-w-0` to the message row, bubble, and feed; make the feed
  `overflow-x-hidden` so only the table wrapper scrolls.
- Acceptance: at 320 px, a long 3-column table produces a scrollable table
  wrapper, not a clipped bubble or page-level horizontal scroll.

### C2 — Copilot splitter and width model (P1)

- Use a Copilot-specific persisted width key, separate from Details/Evidence.
- Normal range: 360–`min(760px, 55vw)`; default 420; expanded default 640–720.
- Make the splitter visible (6–8 px grip), keyboard-operable, double-click to
  reset, and labelled **“Resize Copilot”** with `aria-valuetext`.
- Clamp on viewport resize. Below `lg`, use the full-width overlay/bottom-sheet
  behaviour with no splitter.
- Optional presets: Narrow / Wide / Fit table.

### C3 — Table presentation (P1)

- Add `scope="col"` to headers and a meaningful caption or aria-label.
- Right-align numeric columns with tabular numerals; preserve the raw value in
  `title`/copy while displaying a consistent precision.
- Sticky table header when the table scrolls; visible scrollbar/gradient cue.
- Keep the full numeric string available for provenance; do not silently round
  the underlying claim.

### C4 — Target and focus ribbon (P2)

- Give the campaign target a two-line clamp or wrap instead of a single-line
  ellipsis; expose the full string in an accessible label.
- Separate the `Δ` status into its own badge so target and verification state
  do not compete for the same line.
- Add a copy-target affordance if the target is still visually compressed.

### C5 — Tests and gates (P1)

- `MathText.component.test.tsx`: render a 3-column long-value table inside a
  320 px wrapper; assert the table wrapper scrolls and the document does not
  gain horizontal overflow.
- `CopilotDrawer.component.test.tsx`: render a long target and table; assert
  bubble width ≤ feed width, target has a full accessible label, and the feed
  has no horizontal clipping of prose.
- Console E2E: 390 px mobile and 1440 px desktop screenshots; resize the
  Copilot with pointer and keyboard; assert the persisted width and reset.
- Keep the existing WCAG/axe gate; add `aria-valuetext` and table semantics.

### C6 — Follow-up polish (P3)

- Table “copy as CSV/Markdown” and “open table” actions.
- Optional sticky message header and table header.
- Fit-to-width/table presets if operators use dense numeric transcripts often.

## Acceptance criteria

- No page-level horizontal scroll at 320, 390, 420, 600, or 800 px.
- Long prose wraps; long numeric tokens break/scroll inside a table wrapper.
- The FCI/RHF columns are readable or horizontally scrollable inside the
  Copilot, never clipped by the drawer edge.
- The target and Δ state are readable without hover at the default width.
- The Copilot splitter is visible, focusable, keyboard-operable, persisted per
  tab, and resettable.
- A browser test fails if any of the above regresses.

## Non-goals

- No change to chat/tribunal data contracts.
- No new server endpoint.
- No replacement of the markdown renderer or KaTeX.
- No weakening of the numeric-claim/citation controls.
