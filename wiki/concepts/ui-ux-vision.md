---
type: concept
name: Kubona UI/UX Vision & Console Specification
date: 2026-08-16
updated: 2026-10-11
tags: [ux, ui, control-plane, console, report]
sources: [source-gap-analysis-plan, source-architecture-paper, source-openai-math-corpus]
status: adopted
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
   distinct; credibility wavefunctions over `{corroborated, contested, unverified}` and
   their variance costs are shown inline, not in a footnote.
5. **Systematic results & negative results.** Every experiment produces a structured report
   (objectives · methods · protocol · results · analysis · conclusions · citations · *how to reproduce*)
   — with refutations, barren plateaus, and non-advantaged runs recorded as first-class citizens.
6. **Provenance everywhere.** Any number, plot, or claim can be clicked through to its ledger
   record and citation. Nothing in Kubona floats.
7. **Identity-aware (Buzz pattern).** Who (human, agent, lab) did what is always visible; agents
   are first-class members with keypairs and bounded delegation envelopes ([[user-identity-and-profiles]]).
8. **Recovery as guidance.** Failures surface an actionable next step (switch backend, request
   budget, fall back to simulator) — never a raw error dump.
9. **Anomaly first, telemetry second.** The console surfaces drift/fidelity-drop/burn-rate
   spikes, Ring 0 reflex watchdog events, and epistemic stagnation ($\kappa > 0.85$) instead of
   requiring a human to watch every trace.
10. **Calm instrumentation.** Mobile-first "check-in" presence: approve via biometric AP2 signing,
    glance, alert — the heavy authoring stays on desktop.

## 4. The 4-Tier Classical-Quantum Continuum

The interface unifies four distinct execution tiers:
- **Tier 0 · Classical Physics Anchors:** Mandatory baselines (DMRG, Matrix Product States, RHF/FCI)
  that must be computed before and beside any quantum experiment ([[quantum-advantage-realism]]).
- **Tier 1 · Credential-Free Local Emulation:** Deterministic statevector simulation ($\le 24$ qubits),
  noise-aware depolarizing channels with Zero-Noise Extrapolation (ZNE), and LOCC wire-cutting.
- **Tier 2 · Managed Cloud Simulators:** Qiskit Aer (CUDA/CPU), AWS Braket SV1/DM1/TN1 for pre-flight
  Clifford canary probes.
- **Tier 3 · Cloud Physical QPUs:** Superconducting (IBM Heron, Rigetti Aspen), Trapped Ion
  (Quantinuum, IonQ), Neutral Atom (QuEra Aquila), Photonic (Xanadu), Annealing (D-Wave), gated by
  jurisdiction routing (`policy.py`), AP2 spend mandates, and Qiskit Serverless co-location.

## 5. The seven core observatory surfaces — detailed specification

### 5.1 Spatial Research Canvas & "The Living Lab" (Surface 1)
- **Views:** Infinite zoomable canvas from Galaxy view ($10^{-2}\times$, Ring 0–4 loops) to Surface
  view ($10^2\times$, physical QPU coupling lattices, gate error heatmaps, $T_1/T_2$ decay).
- **Agent Co-Presence:** First-class agent personas (`ansatz-scout`, `noise-tuner`, `literature-critic`)
  with spatial presence cursors, visibly linking literature nodes to candidate Hamiltonians.
- **Post-QIR Glyphs & Ghost Circuits:** Universal post-QIR gate glyphs independent of SDK; translucent
  ghost circuits displaying parallel simulator trials and LOCC wire-cut boundaries.

### 5.2 4D Flight Recorder & Scrubbable Cognitive Timeline (Surface 2)
- **Views:** Time-travel scrub bar spanning multi-hour execution trajectories; animated parameter
  update traces, fidelity gradient trajectories, and agent thought streams.
- **Cognitive Bifurcation & Cavity Replay:** Renders extended-horizon search dynamics derived from
  [[source-openai-math-corpus]]: isolates cavity expansions (Family 221), tree depth shifts (Family 271),
  backtracking from non-constructive dead ends, and Lean 4 `comparator` validation steps.
- **Interactions:** Pause, counterfactual branching ("what-if" forks with altered optimizers or
  regularization parameters), snapshot restoration, and speculative branch diffing.
- **Ring 0 Telemetry:** Non-LLM watchdog triggers (divergence, vanishing gradient, barren plateau)
  pinned directly to the scrub timeline.

### 5.3 Epistemic Manifold & Analytical Anchor Horizon (Surface 3)
- **Views:** Tripartite credibility wavefunction rings ($\alpha |\text{corr}\rangle + \beta |\text{cont}\rangle + \gamma |\text{unver}\rangle$);
  soft amber halos for uncollapsed hypotheses.
- **Deductive Boundary:** Visualizes the epistemic spectrum from machine-checked formal proof
  ($\rho = 1.0000$ via Lean 4) through exact analytical anchors ([[mathematical-physics-anchors]] Families 271, 281, 268, 278)
  down to empirical QPU runs.
- **Riemannian Belief Geometry:** Renders the 3D Fisher–Rao curvature landscape $R_{\mu\nu}$; regions of high
  curvature highlight volatile scientific claims where autonomous curiosity yields maximal entropy reduction.
- **Classical Anchor Baseline:** Inline DMRG / tensor network benchmarks displayed beside any quantum
  expectation value $\langle H \rangle$. Unadvantaged runs visibly badged as "No Advantage Claimed".
- **Stagnation Meters:** Live telemetry for hypothesis concentration $\kappa$, literature drift
  $d(E_{\text{canon}}, E_{\text{live}})$, and refusal ratio.

### 5.4 Autonomy Cockpit & Calibrated Friction Spend Cards (Surface 4)
- **Views:** Active AP2 spend mandate envelopes (`{tenant_id, job_hash, max_budget_usd, expiry}`),
  burn-rate speedometer, and A2A delegation hierarchy tree.
- **Calibrated Friction Architecture:** Low-friction autonomous execution for local simulation, Lean 4
  typechecking, and literature retrieval; high-friction cryptographic gates for expensive QPU jobs
  (neutral-atom / superconducting) or ledger state commits.
- **Action Gate Cards:** One-glance pre-flight cards showing why the backend was chosen, predicted
  Clifford fidelity, and estimated cost with one-click actions: [Authorize (WebCrypto Ed25519)],
  [Downgrade to Aer Sim], [Re-Scope Ansatz], [Abort].

### 5.5 Execution Tier & Hardware Fleet Radar (Surface 5)
- **Views:** Real-time multi-modality fleet status across local simulators and cloud QPUs (IBM,
  Quantinuum, QuEra, D-Wave, Xanadu) with live queue depths, $T_1/T_2$, and pricing.
- **Jurisdiction & Export-Control Shield:** Direct enforcement of `policy.py` (`ROUTE_ELIGIBLE` vs.
  `ROUTE_INELIGIBLE`) based on caller region and ITAR/EAR classifications.
- **Fidelity Predictor:** Compares predicted fidelity $F_{\text{pred}}$ from device noise models
  against Clifford canary probes to gate QPU execution.

### 5.6 Cognitive LLM Mesh & Sovereign Model Control Deck (Surface 6)
- **Views:** Zero-config local discovery radar scanning host ports (`11434`, `8787`, `1234`, `8080`),
  displaying loaded weights, context size, and unified memory usage.
- **Model Assignment Matrix:** Drag-and-drop routing assigning fast local models (Qwen 2.5 Coder on MLX/Ollama)
  to rapid tool calls and transpile loops, while reserving frontier models (Claude 3.7 Sonnet) for literature synthesis.
- **Sovereign Air-Gap Switch:** Hardware-level toggle guaranteeing 0 bytes leave the local host or VPC.
- **Argument Normalization Inspector:** Live stream auditing tool schema mapping and parameter synonyms.

### 5.7 Citable Results Studio & Frame-Anchored Notebook (Surface 7)
- **Views:** Auto-generated structured scientific notebooks (Objectives · Methods · Protocol ·
  Results · Epistemic Analysis · Conclusions · Citations · Reproduction Instructions).
- **Provenance Everywhere:** Every number and plot is a clickable link to an immutable content-addressed
  ledger record.
- **Frame-Anchored Annotations:** Pinned, threaded discussions attached directly to figure regions,
  circuit subgraphs, or convergence plots.
- **Negative Results Repository:** Immutable archival of refuted hypotheses and non-advantaged experiments.

### 5.8 Epistemic Tribunal Room & Swarm Deliberation Surface (Surface 8)
- **Views:** Dedicated adversarial deliberation chamber where specialized agent personas
  (`theorist`, `proof_engineer`, `anchor_solver`, `quantum_compiler`, `falsification_critic`, `sentinel_governor`)
  debate candidate discoveries.
- **Quorum Gauges:** Real-time visual consensus bar showing corroboration amplitudes ($\rho$), dissenting
  rebuttals, and formal AST verification hashes.
- **Adversarial Red-Team Feed:** Side-by-side stream of the Critic attempting to construct counterexamples or
  detect barren plateaus while the Prover isolates Lean 4 lemmas.

### 5.9 Generative Scientific UI / A2UI Component Specification (Surface 9)
- **Dynamic Widget Injection:** Agents stream typed, interactive React components directly into the view:
  1. *3D Molecular Active Space Viewer:* Interactive Mol*/Three.js visualization highlighting active orbitals
     vs. frozen core electrons.
  2. *Quantum Circuit & QIR Editor:* Interactive schematic with gate depth, $T$-count, and wire-cut boundaries.
  3. *Lean 4 Goal Tree Inspector:* Visual proof tree displaying open hypotheses, active goals, and tactic state.
  4. *Classical Shadow Tomography Radar:* 3D Bloch sphere projections and randomized Clifford shadow convergence bounds.

## 6. Visual language

- **Color semantics:** Deep obsidian background (`#0A0D14`), cyan accents for coherent states,
  violet for entanglement, amber for uncollapsed hypotheses / warnings, red for Ring 0 watchdog aborts.
- **Quantum glyphs:** SDK-independent post-QIR universal glyphs, physical QPU lattice mappings with
  gate crosstalk heatmaps.
- **Typography & density:** High semantic density with progressive disclosure, monospace for
  circuits/code/hashes, proportional for epistemic narrative.
- **Motion:** Subtle and functional; reserved for state transitions and scrubbable timeline playback.
- **Accessibility:** WCAG 2.1 AA, keyboard-first approval shortcuts, dual-encoded indicators (never color alone).

## 7. Frontend Technology Architecture: TypeScript + React/Next.js

> ⚑ Architectural Decision (2026-10-04): The console runtime is standardized on **TypeScript + React/Next.js**,
> retiring the earlier Dart/Flutter prototype scaffold.

### Why Dart/Flutter was retired for the Scientific Control Plane
- **Canvas Rendering vs. Scientific DOM:** Flutter for Web renders via CanvasKit/HTML canvas. This cripples standard web browser developer tools, deep DOM accessibility, native browser text selection of dense scientific papers/equations (LaTeX/KaTeX), and embedding heavy DOM/WebGL-based visualizers (such as Mol* / 3Dmol.js for molecular structures and complex vector diagrams).
- **Interactive Multi-Agent Canvas Ecosystem:** An autonomous science discovery platform requires interactive, node-based visual workflows (agent-authored experiment loops, Hamiltonian wiring). The TypeScript ecosystem provides mature, production-grade node canvas engines (**React Flow / xyflow**, Rete.js) that have no equivalent in Dart.
- **Real-Time Multiplayer Presence (CRDTs):** Implementing multiplayer human-agent collaboration (shared lab spaces, co-presence cursors, live concurrent workflow mutation) is solved in TypeScript via CRDT frameworks (**Yjs**, Automerge) over WebSockets.
- **Ecosystem Depth:** TypeScript provides native, actively maintained client libraries for gRPC-Web, streaming WebSockets, LLM token streaming, and scientific data manipulation.

### Responsive Web & Mobile Presence
- **Responsive Next.js Interface:** Desktop-first high-density scientific cockpit; mobile-responsive layout for on-the-go review.
- **Calm Check-in Mode:** Glanceable burn-rate meter, active ring status, and anomaly notifications.
- **WebAuthn / Passkey Biometric Action Cards:** Push notifications for spend gates exceeding autonomy ceilings, signed via browser-native WebAuthn / Passkeys for AP2 mandate authorizations.
- **No Heavy CAD on Mobile:** Deep circuit topology editing and spatial canvas manipulation remain desktop-exclusive.

## 8. The agent-facing "interface" (MCP & A2A, not console)

Agents interact via MCP Tools/Resources and A2A Agent Cards. The console and the agent surface read
the **same immutable ledger** — human annotations and agent run records share the identical content-addressed
substrate.

## 9. Prior art borrowed (and where)

| Source | Pattern adopted |
|---|---|
| IBM Quantum Composer | SDK-independent circuit diagrams, transpiled-vs-original side-by-side |
| QFlow Studio | Credential-safe evidence sharing and reproducible run packages |
| AgentOps / Confident AI | 4D flight recorder replay, HITL approval module, anomaly surfacing |
| Buzz (Block/Mainza) | First-class agent identities, frame-anchored media comments, unified search, YAML triggers |
| ComfyUI / Miro | Multi-scale spatial canvas for branching research campaigns |
| AP2 & A2A Protocols | Cryptographic spend mandates and bounded autonomy delegation boundaries |

## 10. Phase mapping

| Surface | Phase |
|---|---|
| Shell, Top Bar, Surface 4 (AP2 Cockpit), Surface 6 (Cognitive LLM Mesh) | 1–2 |
| Surface 1 (Post-QIR Circuit Topology), Surface 2 (4D Flight Recorder), Ring 0 Watchdog | 2 |
| Surface 3 (Epistemic Manifold), Surface 5 (Execution Fleet Radar & Export Control) | 3 |
| Mobile Companion (Biometric AP2 Signer) | 4 |
| Spatial Multi-Scale Canvas, Surface 7 (Results Studio & Negative Results Repo) | 5–6 |

## 11. Open design problems (tracked, not solved)

1. Platform-agnostic post-QIR glyph specification across exotic modalities (photonic continuous-variable, neutral-atom analog).
2. Where agent-materialized UI (MCP Apps / A2UI) is permitted — strictly restricted to read-only auxiliary widgets.
3. Standardizing epistemic uncertainty visualization across interdisciplinary scientific domains.
4. Notification fatigue across hundreds of concurrent autonomous agent campaigns.
5. Sovereign multi-lab theming and cryptographic key distribution for enterprise air-gapped clusters.

## 12. UX acceptance criteria (gates)

- **P2 gate companion:** a reviewer can watch, understand, approve, and interrupt a live
  loop in under 60 seconds without training.
- **P3:** an operator can answer "has any provider degraded?" from the Home feed, not by
  watching dashboards.
- **P4:** a contributor registers a provider and reads pass/fail without maintainer help.
- **P6:** a researcher runs the full discovery loop and shares a citable, credential-safe
  report — with every number one click from its ledger record.

## 13. Sovereign redesign (built 2026-10-11, prototype register)

The console was rebuilt around the operator-supplied concept (obsidian `#07080B`,
cyan `#00E5FF` / amber `#FFB300` / emerald `#00E676` / rose accents, no purple
gradients, Plus Jakarta Sans + JetBrains Mono self-hosted via `next/font`):
64 px icon-only dock (arena / recorder / manifold / fleet / proofs / mesh + air-gap
lock, settings, avatar campaign menu) with hover tooltips; center arena with HUD
(target pill, orbital / proof-DAG / hardware-QPU mode pill, live spend + session
clock), data-driven probe universe (Theorist / Formalist / Compiler / Critic fed
by `quantum_factors`, anchors, and epistemics — placeholders, never invented
numbers), and a floating Action Gate that appears only while authorization is
outstanding (real AP2 Ed25519 mandate sign + backend verify, deny-with-reason).
The co-pilot keeps all chat/tribunal logic in the new visual language (cyan user
bubbles, synthesis eyebrows, ✦ chips, composer card with real file attach).
All thirteen prior surfaces remain reachable (proofs/manuscripts+tribunal and
mesh/mesh+guardrails+models+labs+settings sub-toggles, legacy Living Canvas
overlay, ⌘K palette, ⌘1–6 shortcuts). Verified: `pnpm build` + 22 vitest green +
eslint clean + served-200 smoke test.

## 14. On-demand canvas widgets (built 2026-10-11, prototype register)

The vision's core interaction — *"agents show, don't tell"* — is now working:
the co-pilot materializes exactly one widget at a time onto the arena backdrop
(`WidgetStage`, empty by default), fed with live campaign props. Tribunal
syntheses carry ✦ dispatch pills (Inspect circuit → live QIR widget with the
campaign's qubits/depth/QASM; Shadow estimates; Active space with live
e/m counts; Spend card with live cost/backend/budget). Trust rules enforced in
code and tested: the spend card is read-only and defers to the Action Gate
(the widget with its own signing display was deliberately *not* reused — it
verifies nothing and would display "Authorized" the gate never granted);
materializing switches to the arena; widget state clears on session switch.
Shell de-worded to icon-only affordances (avatar menu, icon canvas entry).
Verified: `pnpm build` + 26 vitest green + eslint zero-warning + served-200
smoke (zero widget markup in default SSR — the stage is truly empty).

Related: [[agent-ux-control-plane]] · [[science-discovery]] · [[cost-governance]] ·
[[provenance-and-reproducibility]] · [[user-identity-and-profiles]] ·
[[workflows-and-agent-authoring]] · [[buzz]] · [[ibm-quantum-composer]] · [[qflow-studio]]
