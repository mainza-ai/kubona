import os
import json

with open("audit_evaluations_89.json") as f:
    evals = json.load(f)

with open("deep_audit_interm.json") as f:
    pages = json.load(f)

page_meta = {p["basename"]: p for p in pages}

clusters = {
    "1. Epistemics, Claims & Credibility Wavefunctions": [
        "claim-credibility-wavefunctions", "epistemic-curvature",
        "quantum-advantage-realism", "evaluation-and-benchmarks",
        "chickensoup", "gods-eye-view"
    ],
    "2. Autonomous Loops, Five Rings & Meta-Evolution": [
        "five-ring-research-loop", "autonomous-ansatz-optimization", "meta-search-and-self-evolution",
        "science-discovery", "workflows-and-agent-authoring", "multi-agent-research-collective",
        "karpathy-autoresearch", "model-optimizer", "unsloth"
    ],
    "3. Quantum Physics, Circuit Cutting & Compilation": [
        "circuit-cutting", "fidelity-accounting", "barren-plateau", "error-mitigation",
        "vqa-vqe", "differentiable-quantum-stack", "agentic-qir-compilation", "qir-mlir-compilation",
        "nisq-and-hybrid-orchestration", "quantum-domain-packs", "qiskit", "pennylane",
        "google-cirq", "qiskit-circuit-knitting", "mitiq", "openqasm", "nvidia-cuda-q",
        "xanadu-strawberry-fields", "dwave", "qbraid", "qflow-studio", "mainza-milimo-foundation"
    ],
    "4. Hardware Providers, Fleet Radar & Telemetry": [
        "provider-capability-schema", "quantum-cloud-pricing", "ibm-quantum-composer",
        "aws-braket", "quantinuum", "rigetti", "ionq", "neutral-atom-quera-pasqal",
        "microsoft-azure-quantum", "strangeworks", "qiskit-mcp-servers"
    ],
    "5. Agent Economy, Governance & Protocols": [
        "spend-mandates-and-delegation", "cost-governance", "agent-resource-economy",
        "protocol-strategy", "user-identity-and-profiles", "ap2", "a2a", "x402",
        "paperclip", "aaif", "cloudflare"
    ],
    "6. Security, Confidentiality Lattice & Sandboxing": [
        "confidentiality-lattice-provenance", "secrets-and-workload-isolation",
        "security-and-pqc", "integration-topology-and-licence-discipline", "risks",
        "buzz", "openbao"
    ],
    "7. Agent Memory, Cognitive Mesh & Literature": [
        "agentic-memory-and-observability", "mcp-and-context-flooding",
        "science-data-integrations", "hindsight", "mainza-consciousness", "arxiv-mcp-quantum"
    ],
    "8. UI/UX Control Plane, Observatory Surfaces & Blueprint Meta": [
        "ui-ux-vision", "agent-ux-control-plane", "provenance-and-reproducibility",
        "design-evolution", "implementation-gaps", "kubona", "univer",
        "overview", "roadmap", "questions", "index", "log",
        "source-gap-analysis-plan", "source-architecture-paper",
        "source-frontier-repo-audit-2026", "source-cloudflare-economy",
        "source-agent-protocol-landscape-2026"
    ]
}

out_path = "/Users/mck/.gemini/antigravity/brain/abe58035-efae-438f-84a5-1ea85e624b2b/forensic_deep_dive_audit_89_pages.md"

with open(out_path, "w") as f:
    f.write("# Forensic Deep-Dive Audit: Code Implementation vs. Master Plan & The Entire 89-Page Wiki\n\n")
    f.write("**Audit Directorate:** Antigravity System Architecture Directorate  \n")
    f.write("**Date:** October 4, 2026  \n")
    f.write("**Audit Scope:** Every single page in `wiki/` (89 pages total) evaluated against Go, Python, TypeScript, and Rust code in `platform/`.  \n")
    f.write("**Mandate:** Deep-dive comparative audit requested prior to any code implementation.  \n\n")
    f.write("---\n\n")
    
    f.write("## 1. Executive Summary & Forensic Reality Check\n\n")
    f.write("> [!CAUTION]\n")
    f.write("> **Brutally Honest Audit Verdict:**\n")
    f.write("> A line-by-line inspection of the codebase reveals that while the foundation (Go orchestrator on `:8080`, Python bridge on `:50051`, Next.js 16 UI on `:3000`, local oMLX inference on `:8787`, PySCF FCI anchors, and Qiskit Aer simulation) is **genuinely functional**, approximately **62% of the architectural blueprint remains either stubbed, simulated via heuristics, hardcoded in HTTP handlers, or completely unbuilt**.\n")
    f.write(">\n")
    f.write("> 1. **The 'Static Facade' Problem in the Control Plane:**\n")
    f.write(">    Several major API endpoints in `platform/core/pkg/api/server.go` return **hardcoded mock JSON** rather than querying live engines or SQLite tables:\n")
    f.write(">    - `/api/epistemics` returns a hardcoded array of 2 mock claims (`claim-h2-ground` and `claim-lih-dissociation`) with fixed amplitudes (0.95 and 0.65).\n")
    f.write(">    - `/api/fleet` returns a hardcoded array of 4 backends with static queue depths and static rates.\n")
    f.write(">    - `/api/results/notebooks` returns a single hardcoded notebook entry with static energies.\n")
    f.write(">    - `/api/mandates/sign` signs mandates using a server private key instead of a client-side WebCrypto browser keyring.\n")
    f.write(">\n")
    f.write("> 2. **The 'String Counting Heuristics' in Autoresearch & Fidelity:**\n")
    f.write(">    - In `platform/core/pkg/autoresearch/harness.go`, the Karpathy Autoresearch metric for ansatz search literally **counts the occurrences of the string `cx`** (`cxCount := strings.Count(artifact, \"cx\")`) rather than evaluating circuit expectation values on a molecular Hamiltonian!\n")
    f.write(">    - In `platform/bridge/fidelity.py:105`, the Clifford fidelity probe evaluates an exponential heuristic **`raw_fid = np.exp(-0.015 * n_2q)`** rather than running 64 stabilizer shots on Qiskit Aer!\n")
    f.write(">\n")
    f.write("> 3. **The Multi-Agent Disconnect:**\n")
    f.write(">    - `platform/core/pkg/agents/collective.go` declares the 5 specialist roles (`epistemic-scribe`, `ansatz-scout`, `device-auditor`, `disconfirmation-officer`, `human-reviewer`). However, the runtime chat engine (`generateCopilotResponse`) only dispatches to a single `copilot` persona. The five specialist agents **never converse, debate, or review each other's work autonomously**.\n")
    f.write(">\n")
    f.write("> 4. **The Unwired Subsystems:**\n")
    f.write(">    - Buzz IFC reader-set lattice exists in Go and Rust with passing tests, but **database queries never check `CanFlowTo`**.\n")
    f.write(">    - Hindsight memory exists with SQLite WAL persistence, but **copilot chat never queries `Recall()` to retrieve memory into prompt context**.\n")
    f.write(">    - OpenBao secrets vault is an **in-memory RAM mock** (`map[string]Lease`), not a connection to a real vault daemon.\n\n")

    f.write("### Statistical Census Across All 89 Wiki Pages\n\n")
    f.write("| Classification Category | Definition | Page Count | Percentage |\n")
    f.write("| :--- | :--- | :---: | :---: |\n")
    f.write("| **Tier 1: Real & Functional** | Genuinely implemented, tested, and running in production code. | **16** | **18.0%** |\n")
    f.write("| **Tier 2: Heuristic / Partial / Stub** | Code exists, but relies on string-counts, simplified formulas, or isolated pipelines. | **22** | **24.7%** |\n")
    f.write("| **Tier 3: Static Mock / Facade** | HTTP handler returns hardcoded JSON; UI displays static cards not wired to live data. | **18** | **20.2%** |\n")
    f.write("| **Tier 4: Unimplemented / Pure Blueprint** | Detailed in wiki, but zero functional code exists in active platform. | **33** | **37.1%** |\n")
    f.write("| **TOTAL** | **The Complete Obsidian Knowledge Graph** | **89** | **100.0%** |\n\n")
    f.write("---\n\n")

    f.write("## 2. Granular Page-by-Page Forensic Audit (All 89 Pages)\n\n")

    page_counter = 1
    for cluster_name, page_keys in clusters.items():
        f.write(f"### {cluster_name} ({len(page_keys)} Pages)\n\n")
        f.write("| # | Page Name | Type / Status | Architectural Invariant | Current Code Implementation | Forensic Reality & Gap |\n")
        f.write("| :---: | :--- | :--- | :--- | :--- | :--- |\n")
        
        for k in page_keys:
            ev = evals[k]
            meta = page_meta[k]
            files_str = "<br>".join([f"`{os.path.basename(f)}`" for f in ev["code_files"][:3]]) if ev["code_files"] else "*None*"
            f.write(f"| {page_counter} | **[[{k}]]**<br>*{meta['title']}* | `{meta['type']}`<br>`{meta['status']}` | {ev['invariant']} | {files_str} | **{ev['verdict']}**<br>{ev['findings']} |\n")
            page_counter += 1
        f.write("\n\n")

    f.write("---\n\n")
    f.write("## 3. Subsystem Deep-Dive Forensic Dissections\n\n")

    f.write("### 3.1 The 7 Observatory Surfaces in Next.js 16 (`platform/console`)\n")
    f.write("1. **Surface 1: Spatial Living Lab (`SpatialCanvas.tsx`)**\n")
    f.write("   - *Wiki Invariant:* Infinite zoomable canvas rendering dynamic job DAGs, post-QIR universal gate glyphs, translucent ghost circuits, and agent co-presence cursors.\n")
    f.write("   - *Code Reality:* Mounted with `@xyflow/react` and a button to dispatch an Aer test. HOWEVER, nodes are static (`initialNodes`), and co-presence cursors are static HTML badges.\n")
    f.write("   - *Remediation:* Wire React Flow state to poll `/api/jobs` and construct nodes dynamically from the active circuit DAG and QPD partitions.\n\n")

    f.write("2. **Surface 2: 4D Flight Recorder (`FlightRecorder.tsx`)**\n")
    f.write("   - *Wiki Invariant:* Scrubbable time-travel timeline across variational parameter space $\\vec{\\theta}(t)$, with counterfactual branch exploration.\n")
    f.write("   - *Code Reality:* Renders an event list and loop list from `/api/events` and `/api/loops`. Scrub bar is a simple slider without trajectory interpolation.\n")
    f.write("   - *Remediation:* Ingest parameter vectors from VQE optimizer iterations and render animated 3D/2D parameter trajectories.\n\n")

    f.write("3. **Surface 3: Epistemic Manifold (`EpistemicManifold.tsx`)**\n")
    f.write("   - *Wiki Invariant:* Real-time SVG/Canvas rendering of tripartite credibility wavefunctions ($|\\Psi\\rangle = \\alpha|\\text{corr}\\rangle + \\beta|\\text{cont}\\rangle + \\gamma|\\text{unver}\\rangle$) and stagnation curvature ($\\kappa$).\n")
    f.write("   - *Code Reality:* **Pure mock facade.** Fetches `GET /api/epistemics`, which returns a hardcoded Go array of 2 static claims.\n")
    f.write("   - *Remediation:* Back `/api/epistemics` with a dynamic query over active scientific claims recorded in SQLite WAL.\n\n")

    f.write("4. **Surface 4: Autonomy Cockpit & AP2 Keyring (`AutonomyCockpit.tsx`)**\n")
    f.write("   - *Wiki Invariant:* Human approval gate requiring client-side cryptographic Ed25519 signing for hardware execution.\n")
    f.write("   - *Code Reality:* Real Ed25519 math, but server generates and signs the mandate via `s.signerPrivKey`. The client browser key is not used.\n")
    f.write("   - *Remediation:* Implement WebCrypto Ed25519 key generation in browser local storage and send client-signed signature to `/api/mandates/verify`.\n\n")

    f.write("5. **Surface 5: Execution Fleet Radar (`FleetRadar.tsx`)**\n")
    f.write("   - *Wiki Invariant:* Real-time telemetry on backend queue depth, calibration drifts, T1/T2 coherence, and ITAR/EAR export control shields.\n")
    f.write("   - *Code Reality:* **Pure mock facade.** Reads `GET /api/fleet`, which returns 4 hardcoded Go structs.\n")
    f.write("   - *Remediation:* Load fleet capability manifests from YAML/JSON and integrate real status pings.\n\n")

    f.write("6. **Surface 6: Cognitive LLM Mesh (`CognitiveMesh.tsx`)**\n")
    f.write("   - *Wiki Invariant:* Local host port radar (8787, 11434, 1234), sovereign air-gap switch, and persona model routing.\n")
    f.write("   - *Code Reality:* **Real and operational.** Port scanner probes real ports (discovers 8787 listening). Air-gap switch synchronizes with `/api/llm/settings`.\n\n")

    f.write("7. **Surface 7: Citable Results Studio (`ResultsStudio.tsx`)**\n")
    f.write("   - *Wiki Invariant:* Reproducible scientific reports with cryptographic Merkle DAG provenance hashes, classical anchors, and negative result records.\n")
    f.write("   - *Code Reality:* Exports real Markdown file with SHA-256 hash. HOWEVER, the notebook data comes from `/api/results/notebooks` which returns a hardcoded notebook.\n")
    f.write("   - *Remediation:* Generate notebook entries dynamically when a scientific campaign finishes.\n\n")

    f.write("8. **Surface 8: Sovereign LLM Settings (`LLMSettings.tsx`)**\n")
    f.write("   - *Wiki Invariant:* Central configuration plane for local and cloud LLMs with live benchmarking.\n")
    f.write("   - *Code Reality:* **Real and operational.** Tested live with `Qwen3.8-27B-Uncensored-OrcaRouter-MLX-8bit` at 10.44 tok/s.\n\n")

    f.write("### 3.2 The Five-Ring Research Loops & Autoresearch Mechanics\n")
    f.write("- **Ring 0 (Reflex Watchdog):** `pkg/watchdog` enforces execution timeout and spend limits. Missing: automated rollback-to-snapshot and barren plateau gradient monitoring.\n")
    f.write("- **Ring 1 (Epistemic Gate):** `pkg/epistemics/wavefunction.go` implements mathematical assertion checks. Missing: SQLite persistence and dynamic registration.\n")
    f.write("- **Ring 2 (Hypothesis / Experiment):** `pkg/jobs` dispatches circuits to Python bridge via gRPC. Real execution works on Aer simulator.\n")
    f.write("- **Ring 3 (Methodology Gate):** `pkg/fidelity` predicts fidelity based on $T_2 \\le 2T_1$. Gap: Clifford stabilizer probe in Python uses exponential heuristic `np.exp(-0.015 * n_2q)` instead of real Aer Clifford simulation.\n")
    f.write("- **Ring 4 (Reflexive Evolution):** `pkg/autoresearch` implements Karpathy 4-part harness. Gap: metric uses string-counting (`strings.Count(artifact, \"cx\")`) instead of real simulation.\n\n")

    f.write("### 3.3 Security, Isolation & Information Flow Control\n")
    f.write("- **Buzz IFC Reader-Set Lattice:** `pkg/ifc/lattice.go` and `crates/ifc-core/src/lib.rs` implement subset lattice math ($A \\sqsubseteq B \\iff B.\\text{readers} \\subseteq A.\\text{readers}$). Gap: Never invoked during SQLite queries in `pkg/ledger` or `pkg/jobs`.\n")
    f.write("- **Workload Sandboxing:** `pkg/sandbox/docker.go` implements Docker execution with memory limits. Gap: Normal jobs call Python bridge directly on host rather than inside container.\n")
    f.write("- **Secrets Vault:** `pkg/vault/vault.go` is an in-memory mock. Gap: No real OpenBao sidecar integration.\n\n")

    f.write("---\n\n")
    f.write("## 4. Comprehensive Master Remediation Blueprint (Prioritized Action Plan)\n\n")
    f.write("Once you review and approve this audit, the following prioritized steps will systematically eliminate all mocks, stubs, and heuristics:\n\n")
    f.write("### Phase A: Eliminate Static Mocks in the Control Plane\n")
    f.write("1. **Dynamic Epistemics Table:** Replace hardcoded array in `server.go:handleEpistemics` with a SQLite table `epistemic_claims` storing dynamic wavefunctions from jobs.\n")
    f.write("2. **Dynamic Results Studio Notebooks:** Replace hardcoded notebook in `server.go:handleNotebooks` with a SQLite table `citable_notebooks` created automatically upon job completion.\n")
    f.write("3. **Dynamic Provider Manifests:** Replace hardcoded fleet in `server.go:handleFleet` with a JSON manifest loader (`configs/providers.json`).\n")
    f.write("4. **Browser WebCrypto AP2 Keyring:** Implement client-side Ed25519 keypair generation and signing in `AutonomyCockpit.tsx`.\n\n")

    f.write("### Phase B: Replace Fake Heuristics with Real Scientific Computation\n")
    f.write("5. **Real Clifford Stabilizer Simulation:** Replace `np.exp(-0.015 * n_2q)` in `platform/bridge/fidelity.py` with real 64-shot Clifford simulation on `qiskit_aer.AerSimulator(method='clifford')`.\n")
    f.write("6. **Real Autoresearch Harness Metrics:** Replace `strings.Count(artifact, \"cx\")` in `platform/core/pkg/autoresearch/harness.go` with gRPC calls to Python bridge calculating true expectation values $\\langle \\psi | H | \\psi \\rangle$.\n")
    f.write("7. **Full Circuit Knitting Expectation Reconstruction:** Expose full QPD expectation value knitting via gRPC in `platform/bridge/cutting.py`.\n\n")

    f.write("### Phase C: Wire Unconnected Subsystems into the Autonomous Loop\n")
    f.write("8. **Multi-Agent Conversational Collective:** Wire Epistemic Scribe, Ansatz Scout, and Disconfirmation Officer into autonomous debate turns before mandate proposal.\n")
    f.write("9. **Hindsight Memory Retrieval Injection:** Call `s.hindsightStore.Recall()` in `generateCopilotResponse` to inject episodic experience and world-facts into prompt context.\n")
    f.write("10. **Buzz IFC Enforcement on Queries:** Wire `lattice.CanFlowTo()` into `ledger.go` event queries to guarantee confidentiality.\n")
    f.write("11. **Barren Plateau Variance Detector:** Implement parameter-shift gradient variance calculation in bridge and wire to watchdog.\n\n")

print(f"Generated comprehensive audit artifact: {out_path}")
