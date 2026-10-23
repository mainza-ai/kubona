---
type: concept
name: Observatory Spatial Representation Layer
date: 2026-10-22
updated: 2026-10-23
status: adopted
tags: [ux, spatial-canvas, observatory, world-model, atlas, representation-layer, calm-technology]
sources:
  - source-architecture-paper
  - source-gap-analysis-plan
  - source-frontier-repo-audit-2026
---

# Observatory Spatial Representation Layer

An optional, game-like spatial representation layer for Kubona that translates autonomous multi-agent scientific exploration into a physical, observable virtual laboratory. The original high-density Next.js 16 console ([[ui-ux-vision]]) remains primary and undisturbed; the Observatory implements **Surface 1 (Spatial Research Canvas)** and **Surface 9 (Generative Scientific UI)** of the [[scientific-agentic-experience-triad|AX Triad]], mounting into the console's `WidgetStage` or expanding into a full-canvas view.

```
┌──────────────────────────────────────────────────────────────────────────────────┐
│                              THE CORE METAPHOR                                   │
├──────────────────────────────────────────────────────────────────────────────────┤
│ • ROLE IS A STATE OF AGENCY: Scientist, lab director, and visitor are the same  │
│   human traversing different degrees of presence (Hands, Eyes, Ear, Away).       │
│ • HUMAN ATTENTION IS SCARCE: Autonomous swarms are fast and parallel; the world │
│   must answer only: "what deserves a human right now?"                           │
│ • THE FANTASY IS "THE ONE WHO SEES": ku-bona (to see, observe, discover, know)   │
│   is the game progression, not a slogan.                                         │
│ • KUBONA OWNS THE WORLD; ATLAS IS A LENS: Real-time spatial navigation is local  │
│   and deterministic; World Labs Atlas provides an illustrative generative skin.  │
└──────────────────────────────────────────────────────────────────────────────────┘
```

## 1. The Core Scientific Loop

The platform's Chitonga name (*ku-bona*: to see, observe, discover, then know) forms the gameplay loop:

1. **Wonder:** Frame a question, select candidate Hamiltonians, or sketch a circuit in the Workshop.
2. **Commit:** Pre-register and seal an immutable prediction in the Vault *before* compute executes. This is real scientific pre-registration made physical and cryptographic.
3. **Watch:** Agents execute variational and compilation passes while you observe through a calm, ambient director camera.
4. **Surprise:** The world visualizes the exact mathematical gap between pre-registered expectation and measured physical reality ($z$-score).
5. **Know:** Deliberate in the Tribunal (like *Return of the Obra Dinn*), assembling evidence until a claim collapses its credibility wavefunction and earns solid epistemic standing in the ledger.

## 2. Presence & Interaction Mechanics

### Presence is a Dial, Not a Mode
Rather than toggling between disjoint "Operate" and "Explore" modes, the user adjusts a continuous **Presence Dial**:
* **Hands (Direct Authoring):** Active intervention; the Workshop bench is live, circuits can be manually partitioned, and parameters tuned.
* **Eyes (Spectator / Director):** Autonomous execution; the esports camera follows agent reasoning, cavity expansions, and gradient descents.
* **Ear (Ambient Calm):** Minimized or background window; the laboratory hums, translating execution health, convergence cadence, and QPU pulse rhythms into diegetic sound.
* **Away (Lighthouse / Mobile):** The desktop lab goes dark; the system projects a single "lit window" to the user's mobile device via biometric WebAuthn/Passkey push cards for [[cost-governance|AP2 spend authorization]].

### The Observer Effect as a Design Rule
Looking is free; touching is costly. Navigating the world and viewing existing records consumes zero budget. However, intervening in an active experiment re-seeds the random state, alters the optimization path, or incurs hardware costs. Any region touched by a human carries an immutable **provenance stain** in the Merkle ledger, ensuring reproducibility remains completely honest.

### Two Kinds of Looking
* **Lens (Free Observation):** Inspecting existing ledger records, reading pre-computed analytical anchors, or generating illustrative Atlas perspectives over cached depth.
* **Probe (Costly Measurement):** Submitting a new quantum execution pass (Clifford canary, ZNE Richardson extrapolation, or QPU hardware shots). Probes require spend pre-flight estimates and AP2 authorization.

## 3. The Two-Tier Architecture & Data Sovereignty

Kubona strictly enforces a **Two-Tier Decoupled Architecture** to preserve platform sovereignty and satisfy the Sovereign Air-Gap invariant (`air_gapped: true`, 0 outbound bytes egress):

```
                                  KUBONA PLATFORM CORE
                ┌─────────────────────────────────────────────────────────┐
                │  • Go Orchestrator (:8080)                              │
                │  • Immutable Provenance Ledger (SQLite / Merkle)        │
                │  • Epistemic Continuum & Wavefunctions (ρ ∈ [0, 1])      │
                │  • Cost Governance & AP2 Ed25519 Spend Mandates         │
                └────────────────────────────┬────────────────────────────┘
                                             │ Poses, Depth, State
                                             ▼
                                    RENDERER ADAPTER SEAM
                ┌─────────────────────────────────────────────────────────┐
                │  • Normalizes to RUB Camera Convention (1280 × 720)      │
                │  • Encodes Planar Depth (EXR / Log-PNG)                 │
                │  • Data Sovereignty Filter: Sanitizes Prompts & Assets  │
                └────────────┬───────────────────────────────┬────────────┘
                             │                               │
         [ALWAYS-ON: $0.00 LOCAL]                [OPT-IN: ACTIVE BETA]
                             ▼                               ▼
        ┌───────────────────────────────┐     ┌───────────────────────────────┐
        │   TIER 1: THE WALKABLE LAB    │     │      TIER 2: ATLAS LENSES     │
        │  (Three.js / WebGL in Browser)│     │  (World Labs Atlas Sidecar)   │
        │ • 60 FPS Local Navigation     │     │ • Asynchronous View Generation│
        │ • Procedural Scientific GLSL  │     │ • atlasChisel / atlasGenerate │
        │ • Ground-Truth Depth Hit-Test │     │ • Derived Depth Maps Only     │
        │ • VSUP Uncertainty Palettes   │     │ • Non-Evidential Skin Badge   │
        │ • 100% Air-Gap Safe           │     │ • Configurable Retention      │
        └───────────────────────────────┘     └───────────────────────────────┘
```

* **Tier 1 (The Foundational World — Always On, $0.00, 100% Air-Gap Safe):** Built directly in WebGL/Three.js inside the browser client. Renders deterministic ground-truth geometry (e.g. [[vqa-vqe|VQE]] energy landscape heightfields, molecular orbital surfaces, circuit topological DAGs) using procedural GLSL scientific shaders.
* **Tier 2 (Atlas Beta API Lenses — Opt-In Sidecar):** When an Atlas API key is configured and cloud egress is permitted, `atlasChisel` renders an illustrative, material-rich architectural skin over the exact depth layout via [[world-labs-atlas]].
* **Data Sovereignty & Asset Retention Policy:** The adapter enforces two retention modes:
  * `dev_inspect`: Retains uploaded depth assets in the project library for auditing camera alignments and EXR encodings during beta development.
  * `ephemeral_purge`: Issues `DELETE /api/v2/assets/{id}` immediately upon view retrieval to prevent cloud persistence of proprietary laboratory geometries.
  * **Zero-PII Egress:** Egress payloads carry only derived, abstract dimensionless heightfields and controlled-vocabulary architectural prompts. Proprietary chemical structures, SMILES strings, and patient identifiers are strictly forbidden from leaving the local host.

## 4. The Truth Contract & The Epistemic Ladder

### The Three-Layer Truth Contract
Every entity rendered in the Observatory belongs to one of three strictly separated layers:

| Layer | Source | Can It Mislead? | Rule |
|---|---|---|---|
| **Geometry (Truth)** | Computed from ledger records (VQE energy landscape heightfields, orbital isosurfaces, circuit layouts) and encoded as planar depth. | **No.** Fully reproducible and deterministic. | **Hit-testing, raycasting, and measurement use this layer only.** |
| **Skin (Appearance)** | Generated by Atlas (or local shader) from depth, prompt, and seed. | **Yes.** It is illustrative. | **Never carries a value, label, or scientific claim.** Always watermarked `[ILLUSTRATIVE_SKIN]`. |
| **Annotations (Credibility)** | Rendered by Kubona as crisp SVG/DOM overlays (plaques, HUD gauges, wavefunction badges). | **No.** Directly bound to ledger content hashes. | **Model-generated text is never used for data.** Displays verified status `[ρ=0.950 L3 refs=3]`. |

### The Epistemic Continuum Ladder
Cognitive and cartographic research demonstrates that voxel blockiness (`voxelSize`) is perceived as a rendering bug rather than uncertainty. The Observatory instead employs **Value-Suppressing Uncertainty Palettes (VSUP)** and **focus/blur** to map directly onto Kubona's [[epistemic-continuum-and-ontology|5 Epistemic Tiers]]:

```
                                THE EPISTEMIC CONTINUUM LADDER
   RUNG          EPISTEMIC TIER    WAVEFUNCTION STATE    OBSERVATORY VISUAL REPRESENTATION
┌─────────────┬─────────────────┬─────────────────────┬──────────────────────────────────────┐
│ Unknown     │ Void            │ No record           │ Fog of ignorance; zero rendering cost│
├─────────────┼─────────────────┼─────────────────────┼──────────────────────────────────────┤
│ Hypothesis  │ Tier 0 (ρ<0.70) │ |unverified⟩        │ Desaturated VSUP gray, edge blur     │
├─────────────┼─────────────────┼─────────────────────┼──────────────────────────────────────┤
│ Sealed Pred │ Pre-registered  │ Sealed in Vault     │ Translucent ghost contour (μ ± σ)    │
├─────────────┼─────────────────┼─────────────────────┼──────────────────────────────────────┤
│ Observed    │ Tier 1 (QPU/ZNE)│ Partially collapsed │ Solidifying mesh, variance halos     │
├─────────────┼─────────────────┼─────────────────────┼──────────────────────────────────────┤
│ Replicated  │ Tier 2 (FCI sim)│ |corroborated⟩      │ Saturated VSUP hues, sharp shadows   │
├─────────────┼─────────────────┼─────────────────────┼──────────────────────────────────────┤
│ Upheld      │ Tier 3 (Anchor) │ |corroborated⟩ ρ→1- │ Exact analytical curves, anchor line │
├─────────────┼─────────────────┼─────────────────────┼──────────────────────────────────────┤
│ Invariant   │ Tier 4 (Lean 4) │ ρ = 1.000000        │ Indestructible crystalline geometry  │
└─────────────┴─────────────────┴─────────────────────┴──────────────────────────────────────┘
```

## 5. Pre-Registration, Surprise & Author Calibration

To preserve the integrity of Kubona's post-measurement [[quantum-advantage-realism|claim grammar]] and [[claim-credibility-wavefunctions]], pre-registration is modeled as a two-stage cryptographic lifecycle:

1. **Commit (The Vault):** Sealed prior to compute via ledger event `prediction_sealed`. Payload `PreRegisteredPrediction` records system, method, target quantity, expected mean $\mu$, spread $\sigma$ ($\sigma \ge 10^{-4}$ floor), budget cap, author key, and seal hash. In 3D, it renders as a **translucent ghost contour** hovering at $\mu \pm \sigma$.
2. **Evaluation & Surprise:** Upon experiment completion, event `prediction_evaluated` computes standardized surprise:
   $$z = \frac{E_{\text{measured}} - \mu}{\sigma}$$
   * $|z| < 1.0$ (Normal): Measured surface coincides with the ghost contour; the world quietly agrees with itself in cyan/emerald.
   * $1.0 \le |z| \le 3.0$ (Notable): The gap renders as a visible amber divergence volume between the two surfaces.
   * $|z| > 3.0$ (Anomalous): A bold rose volume erupts at the divergence point; the Attention Director summons the camera to investigate.
3. **Formal Assertion & Author Calibration:** If the run passes verification and chemical accuracy gates, a formal `ScientificClaim` is minted citing the prediction. Every evaluated prediction updates the author's historical Brier calibration score, governing future autonomous spend limits.

## 6. The Spatial Laboratory: Room by Room

The Observatory layouts map 1:1 to Kubona's architecture and modules:

| Kubona Subsystem | Spatial Room | Computed Geometry & Player Action |
|---|---|---|
| Go Orchestrator & Federated MCP | Control Hall | Conveyor job queues, server health telemetry. Inspect, pause, re-route. |
| Quantum Bridge & QIR Pipelines | Instrument Hall | Circuits as 3D DAGs, molecular orbital isosurfaces. Probe, run, compare. |
| Merkle Provenance Ledger | The Vault | Shelved immutable specimens by lineage. Seal predictions, audit provenance. |
| Epistemic Tribunal | Courtroom | Sized exhibits by evidence count. Cross-examine personas, cast votes. |
| 4D Flight Recorder | Screening Room | Replayable 4D trajectory camera paths. Scrub history, branch counterfactuals. |
| Experiment Studio & Notebooks | Workshop | Active bench with datasets & circuits. Author, pick up, assemble hypotheses. |
| Autoresearch Engine | Night Shift | Hypothesis constellations & beacons. Spectate autonomous loops, stand down. |
| Meta-Evolution Loop (§13 self-search) | The Annex | Construction scaffold behind gates. Review and sign capability PRs. |
| Cost Governance | Power Room | AP2 fuel gauges and burn-rate meters. Authorize mandates, adjust ceilings. |
| Obsidian Wiki (145+ pages) | Library | Graph-linked aisles and knowledge. Cross-reference concept blueprints. |
| Co-pilot Chat | Walking Companion | Follower avatar pointing at context. Converse, delegate tasks, summon UI. |

## 7. Visual & Sensory Atmosphere Specification

The Observatory is not a cartoonish virtual world or a cluttered game level; it looks like a **high-precision, darkened computational observatory** carved out of architectural obsidian, brushed titanium, and glass, illuminated by diegetic luminescence from quantum states and energy terrains:

* **Tier 1 Procedural Aesthetic (Three.js WebGL):** Grounded in Kubona's adopted sovereign visual language (`#07080B` obsidian, cold carbon fiber, dark matte slate). PES manifolds render as dark topological terrains with fine, glowing isolines. Agent avatars appear as focused survey transits and pencil-thin light beams.
* **Tier 2 Atlas Generative Lens (Marble 2 Beta):** `atlasChisel` applies an illustrative architectural skin: a subterranean research pavilion overlooking a crystalline geological landscape with floor-to-ceiling quartz observation windows, monolithic architectural concrete, dark anodized titanium trim, and cold 6500K recessed lighting. Stamped with `[ILLUSTRATIVE_SKIN · ATLAS CHISEL]`.
* **Diegetic Sound Design (Calm Technology):** Ambient sub-audible 60 Hz electrical transformer hum; rhythmic mechanical watchmaker clicks for completed optimizer iterations; acoustic decay tails reflecting QPU coherence times ($T_1/T_2$); and single warm brass chimes for summons (no siren klaxons).

## 8. The Attention Director & Calm Technology

Grounded in Mark Weiser & John Seely Brown's *Calm Technology* (1995), Amber Case (2015), and ICU alarm fatigue research:

$$\text{Priority} = \text{Cost of Waiting} \times \text{Irreversibility}$$

| Level | Name | Sensory Modality | Trigger Condition |
|---|---|---|---|
| **0** | Ambient | Background sound texture & lighting | Normal convergence cadence, routine tool steps |
| **1** | Glance | Peripheral room brightening | Notable surprise ($1 \le z \le 3$), step completion |
| **2** | Nudge | Companion appears nearby, subtle tone | Optimizer stall, Tribunal deadlock, 80% budget limit |
| **3** | Summon | Lit door, camera focus, mobile push card | Anomaly ($z > 3$), physics violation, 100% budget cap, AP2 mandate gate |

Anti-fatigue rules: **gate batching** (pending approvals presented in single rounds), **duplicate collapse** (identical failures merge into one anomaly object), **presence gating** (Away mode sends only Level 3 to mobile), and **alarm precision tracking** (automatically tightens anomaly thresholds if action rate falls below 80%).

## 9. Agents as Players: The World MCP Tools

Agents navigate the same world as humans using dedicated MCP tools:
* `world.look`: Returns nearby ledger records and camera coordinates. Can request an Atlas lens view.
* `world.probe`: Requests a physical measurement or simulation pass at the active target (cost-gated).
* `world.anchor`: Attaches a prospective hypothesis or finding to a world pose.
* `world.gaze`: Streams the agent's orientation, frustum cone, and intent line (always on).
* `world.summon`: Requests the Attention Director to alert a human.
* `world.handoff`: Passes a camera pose and campaign context to a peer agent.
* `world.replay`: Returns historical trajectory paths for scrubbable playback.

## 10. Platform Implementation Architecture

### 10.1 Frontend: React 19 & Vanilla Three.js (`platform/console`)
- **Runtime:** Next.js 16.3.8 + React 19.2.8.
- **Engine Selection:** Standard `@react-three/fiber` ecosystems exhibit peer-dependency incompatibilities with React 19. The Observatory employs **vanilla Three.js (`three ^0.170.0`)** wrapped inside `SpatialCanvas3D.tsx`.
- **Mounting:** Mounted via `useRef<HTMLCanvasElement>` and `useEffect` with strict cleanup (`cancelAnimationFrame`, geometry/material/texture disposal, `renderer.dispose()`).
- **Surface Integration:** Integrated into `SpatialCanvas.tsx` as a togglable view (`[2D Workbench | 3D Spatial Observatory]`), preserving the 2D Morse potential calculations, OpenQASM schematic, and ADMET profile.

> ⚑ Correction (2026-10-23, settled by [[observatory-lens-audit-2026-10-23]]):
> the engine pin above is stale — `platform/console/package.json` carries
> `three ^0.186.1` / `@types/three ^0.186.0`, not `^0.170.0`. Vanilla-Three.js
> wrapping, mount-once lifecycle, and disposal discipline stand as written.

### 10.2 Backend: Go Air-Gap & Spend Proxy (`platform/core`)
- **Route:** `POST /api/observatory/lens` registered on `s.mux` in `platform/core/pkg/api/server.go`.
- **Sovereign Air-Gap Enforcement:** Checks `s.llmSettings.AirGapped`. If `true`, fails closed immediately with HTTP 403 `airgap_blocked` ("Sovereign Air-Gap active: cloud generative lenses refused"). Zero external network packets leave the security perimeter.
- **Spend Envelope:** Checks session spend cap against recorded spend (`session.spend_limit_usd` vs `session.current_spend_usd`). Debits credit costs (1,000 credits = $1.00 USD) and records `llm_spend_estimated` in the ledger.
- **Marble 2 Client:** Proxies tasks to `https://api.worldlabs.ai/api/v2/tasks:atlasChisel` using the server's `WLT_API_KEY`.

> ⚑ Correction (2026-10-23, settled by [[observatory-lens-audit-2026-10-23]]):
> four claims above misdescribe the implementation
> (`platform/core/pkg/api/observatory_lens.go`). The proxy **defaults to
> `images2PosedRGBD`**, not `atlasChisel` (chisel-shaped payloads only when
> explicitly requested, and in a schema the live API would reject — see audit
> C-02). The default host is `https://api.atlas-beta.worldlabs.ai`, not
> `api.worldlabs.ai`. The air-gap refusal code is `forbidden`, not
> `airgap_blocked`. Spend is reserved through `sessionMgr` ($0.50) with a
> text `annotation` in the ledger — no `llm_spend_estimated` field is
> recorded. Pre-registration events (`prediction_sealed/evaluated`) exist in
> the contract taxonomy but the lens path emits only `annotation`.

### 10.3 Ledger Contract Conformance (`platform/contracts/event-types.json`)
- **Closed Set-Identity:** Both Go and Python rails enforce contract set-identity via `events_contract_test.go`.
- **New Lifecycle Events:**
  - `prediction_sealed`: Emitters `["go"]`, required fields: `["session_id", "prediction_hash", "target_system", "p_correlated"]`.
  - `prediction_evaluated`: Emitters `["go"]`, required fields: `["session_id", "prediction_hash", "brier_score", "surprise_ratio"]`.
- **Go Mirror:** Registered in `KnownEventTypes` in `platform/core/pkg/ledger/events.go`.

## Cross-References

- [[ui-ux-vision]] — full console visual specification and design rules.
- [[agent-ux-control-plane]] — user interaction and flight control surfaces.
- [[scientific-agentic-experience-triad]] — the AX triad (User ↔ Agent, Agent ↔ Agent, Agent ↔ Interface).
- [[epistemic-continuum-and-ontology]] — 5-tier epistemic continuum ($\rho \in [0, 1]$).
- [[claim-credibility-wavefunctions]] — 3-basis claim state vector.
- [[world-labs-atlas]] — World Labs Atlas / Marble 2 developer API specification and Sidecar integration.
- [[cost-governance]] — AP2 spend mandates and autonomy ladder.
- [[provenance-and-reproducibility]] — Merkle provenance ledger and immutable records.
- [[local-simulation-test-scenarios]] — $0.00 local simulation scenarios used to drive initial world geometry.
