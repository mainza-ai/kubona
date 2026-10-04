---
type: concept
date: 2026-08-16
updated: 2026-10-04
tags: [kubona, production, gaps]
sources: [source-gap-analysis-plan]
status: tracked
---

# Prototype Gap Register (production vs. scaffold)

> **What this page is.** The honest register of what the [[overview|prototype]] does vs.
> what production requires. It is a *code* register, not a design register: it answers
> "what does the scaffold already do", never "what should Kubona be". Design questions
> live in [[questions]]. This page exists so a skeleton/engine implementation is never
> mistaken for a deployable one, and so each gap is closed with evidence, not intent.

## Status legend
- 🟢 **Closed (M0)** — structural, cheap-now fixes landed.
- 🟠 **Open + required before real deployment** — the "we'd redo it later" items.
- 🔵 **Deferred by design** — matches the plan's own "functional product first" sequencing.
- 🔴 **Regressed / failing now** — verified broken at the stated date; not a future concern.

## Forensic Audit (2026-10-04): Codebase Reality vs. Blueprint

A systematic audit revealed that over 90% of what is documented in the blueprint across the
89 wiki pages, the 7 Observatory Surfaces, and 25 audited dev-docs repositories was either
unimplemented, stubbed, or disguised behind static UI facades:

1. **The 7 Observatory Surfaces are Missing/Stubbed:**
   - Surface 1 (Spatial Living Lab): No infinite zoomable canvas, no React Flow node graph, no post-QIR universal glyphs, no ghost circuits.
   - Surface 2 (4D Flight Recorder): Was reduced to a plain static HTML table; lacked interactive scrub bar, parameter trajectory animation, and counterfactual branching.
   - Surface 3 (Epistemic Manifold): Renders static placeholder cards rather than dynamic SVG/Canvas wavefunction rings ($\alpha|\text{corr}\rangle + \beta|\text{cont}\rangle + \gamma|\text{unver}\rangle$) or stagnation telemetry ($\kappa$).
   - Surface 4 (Autonomy Cockpit & AP2): Unsigned JSON payload without Ed25519 cryptographic spend mandate verification.
   - Surfaces 5, 6, 7 (Fleet Radar, Cognitive LLM Mesh, Results Studio): Completely absent from the frontend.
2. **Multi-Agent Research Collective is a Single Prompt:**
   - The 5 distinct specialist personas (`epistemic-scribe`, `ansatz-scout`, `device-auditor`, `disconfirmation-officer`, `human-reviewer`) with declarative `role.yaml` capability grants and keypair identities are unbuilt.
3. **The Five-Ring Research Loop is Unclosed:**
   - Ring 0 (Reflex): Lacks automated re-scoping and snapshot recovery.
   - Ring 1 (Epistemic): Uncollapsed claims are not blocked from being asserted in agent outputs.
   - Ring 3 (Methodological): No fidelity accounting, no device noise profiles ($T_2 \le 2T_1$), and no Clifford stabilizer probes.
   - Ring 4 (Reflexive): No Karpathy Autoresearch harness runners or §13 meta-search evolution flywheel.
4. **Security, Confidentiality & Isolation:**
   - Buzz IFC `can_flow_to` reader-set lattice is missing from queries.
   - OpenBao secrets vault with dynamic single-use leases is unintegrated.
   - Code execution is unsandboxed on the host instead of container-isolated.

## The 6-Phase Master Implementation Plan

- **Phase 1: Architecture Core, Governance & Isolation:** Ed25519 AP2 spend mandate engine in Go, Buzz IFC reader-set lattice in Rust/Go, Paperclip 4-plane MCP access gateway with `low_trust_review`, and OpenBao secrets vault sidecar.
- **Phase 2: Quantum Physics Engines, Real Anchors & Data Grounding:** PySCF numerical electronic structure solver (RHF, CISD, FCI), hardware noise profiles ($T_2 \le 2T_1$) with `stim` Clifford probes ($\le 64$ shots), PennyLane `qcut` KaHyPar circuit cutting, and live arXiv/OpenAlex connectors.
- **Phase 3: Multi-Agent Collective & Closed Rings 0-1:** 5 specialized agent personas with declarative `role.yaml`, Ring 0 supervisor with automated re-scoping, Ring 1 assertion gating with epistemic curvature ($\kappa$, drift, refusal ratio), and Hindsight tripartite memory banks.
- **Phase 4: Autoresearch Engine, Fidelity Accounting & Rings 3-4:** Karpathy Autoresearch 4-part harness registry, autonomous ansatz genome search with cheap pre-checks, Ring 3 fidelity accounting gate (predict $\to$ probe $\to$ admit/downgrade/refuse), and §13 meta-search self-evolution engine.
- **Phase 5: The Seven Core Observatory Surfaces (Next.js 16 + TypeScript):** Surface 1 (Spatial Canvas via React Flow), Surface 2 (4D Scrubbable Flight Recorder), Surface 3 (Dynamic Wavefunction Rings), Surface 4 (Cryptographic Mandate Cockpit), Surface 5 (Fleet Radar & ITAR Shield), Surface 6 (Cognitive Mesh & Air-Gap Switch), Surface 7 (Citable Results Studio).
- **Phase 6: Multi-Tenant Sandboxing, Production CI & Zero-Placeholder Verification:** Docker workload container isolation, unified multi-language CI pipeline, automated end-to-end campaign tests, and zero-placeholder audit across the codebase.

## Critical (structural)

| Gap | Status | What's real / what's missing |
|---|---|---|
| **Persistent state root (data-loss)** | 🟢 M0 | Route through `kubona/config.py` (`state_root()`) — default `~/.kubona`. |
| **Prod auth default** | 🟢 M0 | Console uses constant-time compare against `KUBONA_CONSOLE_TOKEN`, production fails closed. |
| **Deployment entrypoint** | 🟢 M0 | `kubona serve` / `kubona doctor` package entrypoints. |
| **CI / release-gate** | 🟢 green | Single verdict `release_gate()` in `canary.py`, verified in hosted Actions. |
| **x402 fail-closed** | 🟢 M0 | Rejects proofs unless real verifier injected. |
| **Chat/Copilot & oMLX Integration** | 🔴 failing now | Chat returns canned string matches; oMLX on port 8787 (`Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit`) was omitted from port scanner and chat dispatch. |
| **Circuit Cutting & Knitting** | 🔴 failing now | `cut_circuit_qasm` replaced circuits with dummy 2-qubit `h(0); cx(0,1)`; needs genuine PennyLane `qcut` + KaHyPar graph partitioning. |
| **Local Classical-Quantum Testing Gate** | 🟠 open | Precondition gate before cloud QPUs: PySCF ab-initio FCI anchors, Qiskit Aer simulation, Clifford probes ($\le 64$ shots), chemical accuracy collapse. |
| **Hybrid LLM Configuration Plane** | 🟠 open | Settings view for local engines (oMLX, Ollama, LM Studio) and cloud models with Sovereign Air-Gap switch. |
| **Loop Supervisor Persistence** | 🔴 failing now | In-memory split-brain; console cannot observe or control MCP loops. |
| **Classical Chemistry Anchors** | 🟠 open | Needs strict PySCF ab-initio solver without empirical curve-fit fallbacks. |
| **TypeScript/Go Core Migration** | 🟠 open | Scaffold is pure Python/Flutter; Go orchestrator and Next.js UI required for production concurrency and workflow canvas. |

## Medium

| Gap | Status | Note |
|---|---|---|
| Structured observability/metrics/audit export | 🟢 partial | Gateway audit now persists to a JSONL export (restart-safe, `audit_export()`, wired by default in `kubona run`/`serve-gateway`). Structured logging + OTel/Prometheus metrics remain. |
| Perf-regression guards (G12) | 🟢 | tests/test_perf_guard.py guards the lower + local-simulator hot paths against accidental O(exponential) regressions (passed in ~0.1s). |
| Volume eval corpora (QuantumKatas/HumanEval/PennyLang) | 🟡 mechanism-ready | `kubona/eval/corpus_loader.py` loads declarative JSON corpora (ghz/uniform/grover/bell check-kinds) into the evaluate_codegen contract; ships a verified extended corpus (ghz5/ghz6/qft4) whose every task passes its invariant on the simulator, and rejects bad entries fail-closed. The third-party datasets (QuantumKatas/HumanEval/PennyLang) are the external ingredient that drop in via this loader. |
| GQE transformer (Unsloth) training | 🔵 | n-gram stand-in today; drop-in `propose` replacement on a GPU host. |
| Full circuit knitting | 🟡 partial | LOOC-Z single-CNOT wire cut now additionally reconstructs the **full post-cut target distribution** (`cut_histogram`, exact Z-diagonal, verified against uncut — strictly more informative than a single ⟨Z⟩). Honest scope: a single-CNOT cut reaches one target qubit, so multi-qubit-gate/product-Z cuts still need the full QPD/Qiskit-CKT integration (documented; that is the remaining piece). |
| Perf/load baselines | 🔵 | Pure-numpy simulator; add perf regression + memory guards. |

## Deferred by design (correct, per plan)
- x402 activation on a real rail (Phase 5+), meta-loop auto-apply (§13.5, Phase 5+), Unsloth personas (Phase 6), real third-party provider PR, third-party security review execution, live arXiv/OpenAlex default (network-acknowledged; mock is the offline default).

## Blueprint integrity (the wiki is the deliverable, so it is linted)

| Gap | Severity | Where |
|---|---|---|
| The blueprint accumulated **build language** for months — phase gates written as "implemented", `status` values encoding finish state. The failure mode where a blueprint quietly becomes a changelog | high (fixed 2026-09-27) | reframed in [[overview]] / [[roadmap]] / [[index]]; vocabulary defined in `AGENTS.md` |
| `scripts/wiki-lint.py` existed and exited 0, but **nothing ran it** | medium → closed 2026-09-27 | wired into `ci.yml` as a *Wiki structural lint* step in the `python-tests` job — deliberately added only after the release-gate fix above, because a step added to a pipeline that already fails 17/17 adds no signal. The check is text-only over `wiki/**`, so it needs neither `dev-docs/` nor the Rust toolchain on the runner |
| `AGENTS.md` drifted from the wiki it governs (stale page counts, an outdated repo-status rule, inline-cite keys that no longer matched source pages) | medium (fixed 2026-09-27) | schema now states purpose, vocabulary, exemptions, and the two-register rule |

The lint is deliberately narrow: it checks **structural** invariants — traceability, link
integrity, classification, index coverage — not prose quality or whether a decision is
*correct*. Those stay human judgement. What a script can guarantee is that a decision is
findable, linked, and traceable to a source, which is the property that makes a blueprint
accumulate instead of rot.

## Doc-overstatement corrections (see also [[overview]] / [[roadmap]])
- "canary CI gates releases" → **CI was absent**; now scaffolded (M0), first hosted run pending.
- "OAuth-shaped token issuance" → issuer existed but was **not wired**; now the production default with aud/nbf.
- "9 providers" → 2 execute; the rest are capability entries.
- "QIR (Rust core)" in Platform P1 → Rust core exists but was **off the compiler path**; integration is an open gap.
