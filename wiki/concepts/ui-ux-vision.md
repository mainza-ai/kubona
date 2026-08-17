---
type: concept
name: Kubona UI/UX Vision & Console Specification
date: 2026-08-16
tags: [ux, ui, control-plane, console, report]
sources: [source-gap-analysis-plan, source-architecture-paper]
status: living
---

# Kubona UI/UX Vision — How the Interface Looks, Feels, and Works

A comprehensive report on the future user experience and interface, synthesized from the
source's §5 UX investigation, the [[agent-ux-control-plane]] surface list, the
[[science-discovery]] loop, [[user-identity-and-profiles]], [[workflows-and-agent-authoring]],
and the [[buzz]] collaboration patterns. This page is the detailed specification;
[[agent-ux-control-plane]] is the summary.

## 1. What Kubona's UI must do that nothing else does

Two disciplines that have never been combined in one product:

1. **Quantum circuit & experiment visualization** — a post-QIR, **SDK-independent** rendering
   layer (the 2026 design-space literature calls platform-agnostic quantum circuit
   visualization an *open problem*; Kubona inherits it by being multi-SDK from day one).
2. **Agent UX as an accountability layer** — the interface must always show *what the system
   is doing, why it chose an action, allow override at any point, and recover gracefully* —
   because Kubona's UI is the trust boundary between a human and an autonomous agent spending
   real money on real hardware.

Plus a third, new layer from the discovery vision: **science communication** — the same
surface that narrates agent action must also present *results* in an organized, citable,
reproducible form (the "systematic results" contract).

## 2. Who the interface serves (personas)

| Persona | Goal | Depth of use |
|---|---|---|
| **Researcher / Operator** | Pose a question, launch an experiment, interpret results, iterate | Daily, deep (desktop) |
| **Reviewer / Approver** (PI, ops lead, finance) | Approve spend, watch live loops, respond to anomalies | Frequent but shallow ("one glance") |
| **Contributor** (plugin provider) | Register & test a provider integration against the capability contract | Occasional, Phase 4+ |
| **Lab member / collaborator** | Review reports, annotate figures, share evidence; works under lab identity | Intermittent, mobile-friendly |
| **The agent itself** | Consumes MCP Tools/Resources/Prompts — the console is *not* an agent surface; agents read the same ledger |

Primary journeys: (a) first-run — feel a simulator run within 5 minutes with zero config;
(b) daily — approve 3 spend requests, skim 2 active loops, read one report; (c) incident —
burn-rate spike or barren-plateau detected: the console explains, offers recovery, one click
away.

## 3. Design principles (10)

1. **Read/approve only.** The console is an oversight layer, never a second path to submit
   jobs — no parallel API surface, no credential handling in the browser.
2. **Legibility of autonomy.** Every screen answers: *what is the agent doing now, why, at
   what cost, and how much do we trust the result?*
3. **Progressive disclosure.** Deep circuits and long loops default to summarized stats with
   drill-down; never a firehose.
4. **Confidence & uncertainty are visual.** Raw vs. error-mitigated values are visually
   distinct; the technique and its variance cost are shown inline, not in a footnote.
5. **Systematic results.** Every experiment produces a structured report (objectives ·
   methods · protocol · results · analysis · conclusions · citations · *how to reproduce*)
   — the notebook is a first-class citizen, not an export.
6. **Provenance everywhere.** Any number, plot, or claim can be clicked through to its ledger
   record and citation. Nothing in Kubona floats.
7. **Identity-aware.** Who (human, agent, lab) did what is always visible; approvals show the
   actor and the delegation chain once (Phase 6).
8. **Recovery as guidance.** Failures surface an actionable next step (switch backend, request
   budget, fall back to simulator) — never a raw error dump.
9. **Anomaly first, telemetry second.** The console surfaces drift/fidelity-drop/burn-rate
   spikes instead of requiring a human to watch every trace.
10. **Calm instrumentation.** Mobile-first "check-in" presence: approve, glance, alert — the
    heavy authoring stays on desktop.

## 4. Information architecture & the workspace shell

A lab-scoped workspace (per [[user-identity-and-profiles]]), Flutter (desktop + web + mobile):

- **Left rail (primary navigation):** Lab Home · Experiments · Circuit Lab · Live Loops ·
  Workflows & Agents · Providers · Ledger (Provenance) · Reports · Settings.
- **Top bar (always visible):** active-agent status pill ("searching ansätze — step 3/9"),
  global burn-rate meter (live, color-coded), approval badge with count, lab/identity
  switcher.
- **Home (Lab Home):** today's activity (running loops, pending approvals, anomalies), recent
  reports, saved workflows, budget state — the "one glance" cockpit.
- **Global search** (the Buzz single-index idea): one field searches conversations, runs,
  workflows, approvals, reports, ledger records, and citations together.
- **Canvas support** (Phase 6): workflow builder and experiment boards as shared canvases —
  humans and agents drop steps, evidence, and annotations on the same surface.

## 5. The eight console surfaces — detailed specification

### 5.1 Circuit Visualizer (Phase 2)
- **Views:** schematic (post-QIR, SDK-independent), code pane (OpenQASM/Qiskit) with
  **bidirectional code↔diagram sync** (IBM Composer pattern), transpiled-vs-original
  side-by-side, Q-sphere / statevector / histogram backed by the local simulator.
- **Interactions:** drag-and-drop editing; click-any-gate → inspector (parameters, error
  rate, connectivity); critical-path highlight; collapse-to-stats for deep circuits
  (gate count, depth) with drill-down.
- **States:** empty (template gallery), loading (simulator preflight), error (invalid circuit
  → suggestion where it broke).
- **Mobile:** view + share; no drag-editing (desktop only).

### 5.2 Cost & Approval Console (Phase 1–2)
- **Views:** live burn-rate chart (per agent/session/lab), budget ceilings with
  remaining-time-at-current-rate, **pending-approval queue** (one-click approve/deny with
  optional note), spend history, cost forecast for the next loop iteration.
- **Interactions:** approve/deny from the queue or from a push-style notification;
  "explain this approval" expands the agent's reasoning + cost estimate supplied at
  pre-flight.
- **States:** all-clear (green, no action), pending (attention), breached ceiling (red, loop
  paused automatically by the [[cost-governance]] service).

### 5.3 Live Session / Loop Inspector (Phase 2)
- **Views:** session timeline with step-by-step parameter-update trace; **replay/scrub**
  (AgentOps time-travel pattern); current step card ("what and why"); backend & queue-depth
  indicator; spend ticker; fidelity/entanglement metrics inline.
- **Interactions:** pause / override / kill; bookmark a step; jump from a step to its ledger
  record.
- **Alerts:** barren-plateau/WBP detection fires inline with an explanation and a suggested
  action (restart with adjusted params, switch ansatz, continue).
- **States:** running / waiting-on-approval / paused / errored / completed — each with a
  distinct banner and recovery path.

### 5.4 Provenance / Experiment Browser (Phase 3)
- **Views:** ledger search by circuit hash, backend, mitigation, date, creator identity;
  side-by-side run comparison tables; record detail (full immutable record + inputs +
  outputs + cost).
- **Interactions:** **[Diff]** any two runs; **[Export evidence]** — credential-safe,
  citable artifact (QFlow pattern); one click from a result to the report that cites it.
- **Trust furniture (Phase 6):** freshness/trust scores on every record and data source —
  "certify before you trust it".

### 5.5 Provider / Fleet Status (Phase 3)
- **Views:** per-provider cards — queue depth, fidelity trend (sparkline + canary-CI
  pass/fail badges), modality, current workload, price model, jurisdiction/export-control
  flags ([[provider-capability-schema]]).
- **Interactions:** drill into a provider's canary history; "why is this provider flagged?"
  surfaces the drift evidence.
- **Anomaly:** a sudden fidelity drop or queue spike promotes the provider to the Home
  anomalies feed.

### 5.6 Plugin / Provider Registration Console (Phase 4)
- **Views:** capability-contract form (schema-driven), sandboxed test runner against the
  plugin trait, validation report (gate set, pricing model, modality, jurisdiction).
- **Interactions:** run a test suite in a sandbox; view pass/fail per contract field;
  submit for review → status moves through the contribution/approval flow.

### 5.7 Experiment Notebook & Results Studio (Phase 6)
- **Views:** per-experiment notebook — objectives · methods · protocol (with controls and
  preregistration where applicable) · results (plots, tables) · analysis · conclusions ·
  **citations** (literature + internal ledger refs) · *how to reproduce*.
- **Interactions:** every section links its ledger records; **frame/figure-level
  annotations** (Buzz media-comment pattern) for team discussion pinned to a specific plot;
  report versions; export as PDF/HTML/JSON + credential-safe share; autogenerated draft from
  the discovery loop the agent fills, human edits.
- **States:** draft (agent-generated) → review (human) → published (versioned, cited by
  future work).

### 5.8 Lab, Workflows & Agents View (Phase 6)
- **Views:** members (humans + agents as members, each with keys/memberships/audit),
  delegation & revocation controls (instant), **workflow library** (versioned YAML, triggers,
  gates), **agent registry** (persona + tool scope + budget envelope), approval queue for
  workflow/agent changes.
- **Interactions:** author a workflow in the canvas or YAML editor with dry-run; propose an
  agent-created change → moves through the §13.4-style capability gate (shadow mode →
  sign-off); revoke a user → cascade visible ("3 agents lost access").

## 6. Visual language

- **Color semantics** (consistent across surfaces): spend/risk heat (green→amber→red), trust
  (fresh/verified vs. stale/uncertain), status (running/waiting/approving/errored/mitigated),
  provenance (linked vs. floating). Confidence is always a visual property of a value, not a
  footnote.
- **Quantum glyphs:** Q-sphere/statevector representations reused as thumbnails across
  reports and ledger; transpilation diffs highlighted; native-gate sets color-coded per
  provider.
- **Typography & density:** dense-but-summarized default (stats-first), monospace for code/
  circuits, proportional for narrative; dark theme default (science night-work), light theme
  for reports/sharing.
- **Motion:** subtle; reserved for state change and anomaly escalation; scrubbing is
  scrubbing (time-linked), not decorative.
- **Accessibility:** WCAG 2.1 AA, keyboard-first for all approval actions, value + color
  dual-encoding (never color alone), screen-reader labels on circuit graphs.

## 7. Mobile presence (Flutter)

- **Check-in mode:** burn-rate, approval queue, active-loop status, anomaly alerts,
  report reading/annotating. No circuit editing, no workflow authoring.
- **Push channels:** approval requests, breach warnings, loop completion/failure — each with
  an inline action (approve/deny, pause, fallback) that never re-opens job submission.

## 8. The agent-facing "interface" (MCP, not console)

Agents see the platform through MCP Tools/Resources/Prompts + the code-execution sandbox.
The console and the agent surface read the **same ledger** — a report written in the console
is a resource an agent can cite; an agent's run appears in the console's provenance browser.
This single-substrate design (the Buzz "one event log" idea) is what makes human and agent
work mutually verifiable.

## 9. Prior art borrowed (and where)

| Source | Pattern adopted |
|---|---|
| IBM Quantum Composer | drag-drop editor, code↔diagram sync, transpiled-vs-original results |
| QFlow Studio | credential-safe evidence sharing |
| AgentOps / Confident AI (2026 agent-UX) | session replay, HITL approval module, anomaly surfacing, framework-agnostic traces |
| Buzz (Block/Mainza) | agents as members, frame-anchored annotations, unified search, canvases, workflow triggers + gates |

## 10. Phase mapping

| Surface | Phase |
|---|---|
| Shell, Home, Circuit Visualizer, Cost & Approval, Loop Inspector | 2 |
| Provenance Browser, Fleet Status | 3 |
| Plugin Registration | 4 |
| Notebook & Results Studio, Lab/Workflows/Agents, canvases, mobile check-in | 6 |

## 11. Open design problems (tracked, not solved)

1. Platform-agnostic circuit rendering — candidate: a Kubona-owned post-QIR glyph spec.
2. Where agent-materialized UI (MCP Apps / A2UI) is allowed — fixed to ad-hoc, read-only
   visualizations (v2+); core surfaces stay human-designed and audited.
3. Uncertainty visualization lacks standards — Kubona defines its own raw-vs-mitigated
   encoding and publishes it.
4. Notification fatigue with many concurrent agents — per-lab digest modes and persona-based
   alert routing.
5. Multi-lab theming/branding for hosted operators.

## 12. UX acceptance criteria (gates)

- **P2 gate companion:** a reviewer can watch, understand, approve, and interrupt a live
  loop in under 60 seconds without training.
- **P3:** an operator can answer "has any provider degraded?" from the Home feed, not by
  watching dashboards.
- **P4:** a contributor registers a provider and reads pass/fail without maintainer help.
- **P6:** a researcher runs the full discovery loop and shares a citable, credential-safe
  report — with every number one click from its ledger record.

Related: [[agent-ux-control-plane]] · [[science-discovery]] · [[cost-governance]] ·
[[provenance-and-reproducibility]] · [[user-identity-and-profiles]] ·
[[workflows-and-agent-authoring]] · [[buzz]] · [[ibm-quantum-composer]] · [[qflow-studio]]
