import json, os

with open("deep_audit_interm.json") as f:
    pages = json.load(f)

page_map = {p["basename"]: p for p in pages}

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

# Forensic Evaluation Database for each of the 89 pages
page_evaluations = {
    # Cluster 1
    "claim-credibility-wavefunctions": {
        "verdict": "PARTIAL / MOCK",
        "code_files": ["platform/core/pkg/epistemics/wavefunction.go", "platform/core/pkg/api/server.go:186-215", "platform/console/src/components/surfaces/EpistemicManifold.tsx"],
        "invariant": "|Ψ⟩ = α|corr⟩ + β|cont⟩ + γ|unver⟩, normalized (|α|²+|β|²+|γ|²=1). Assertion blocked unless P_corr ≥ 0.75, sources ≥ 2, and chemical accuracy Δ ≤ 1.6 mHa.",
        "findings": "Wavefunction data structure and normalization math are implemented in Go (wavefunction.go). HOWEVER, server.go handleEpistemics returns a hardcoded static array of two mock claims (claim-h2-ground with alpha=0.95, and claim-lih-dissociation with alpha=0.65). Claims are not persisted or queried dynamically from active experiments in the SQLite WAL ledger.",
        "gap": "Wire EpistemicGate to SQLite WAL ledger; store claim wavefunctions dynamically from executed VQE/chemistry jobs instead of returning static JSON."
    },
    "epistemic-curvature": {
        "verdict": "HEURISTIC STUB",
        "code_files": ["platform/core/pkg/epistemics/wavefunction.go:107-118", "platform/core/pkg/api/server.go:187"],
        "invariant": "Epistemic curvature (κ) measures hypothesis space contraction vs stagnation. Literature drift measures difference against published ground truth.",
        "findings": "Implemented as a heuristic formula: kappa = (refusalRatio + 0.1) / (1.0 + 0.05*deltaMHa). Drift = |quantum - classical|. Called in server.go with hardcoded constants (0.12, -1.13727, -1.1365).",
        "gap": "Replace hardcoded constant call with dynamic calculation across time-series ledger events for the active molecule."
    },
    "quantum-advantage-realism": {
        "verdict": "REAL (ENFORCED IN PROMPT & ANCHOR)",
        "code_files": ["platform/bridge/chemistry.py", "platform/core/pkg/api/server.go:500-514", "wiki/concepts/quantum-advantage-realism.md"],
        "invariant": "Prohibits claiming quantum advantage without classical baseline. Mandates ab-initio classical anchor (PySCF FCI/RHF) for every molecular run.",
        "findings": "Strict PySCF RHF and FCI solvers implemented in chemistry.py. Empirical curve-fit fallbacks removed. Copilot system prompt explicitly forbids claiming advantage.",
        "gap": "Negative results ledger needs explicit queryable indexing in SQLite WAL ledger table."
    },
    "evaluation-and-benchmarks": {
        "verdict": "PARTIAL (LEGACY PYTHON ONLY)",
        "code_files": ["platform/tests/test_evals.py", "platform/tests/test_perf_guard.py", "kubona/eval/corpus_loader.py"],
        "invariant": "Standardized benchmark suite for quantum codegen, circuit depth, and execution correctness across corpora (QuantumKatas, HumanEval, PennyLang).",
        "findings": "Corpus loader exists in legacy Python scaffold (kubona/eval/corpus_loader.py) and passes pytest. NOT wired into the Go orchestrator or Next.js console.",
        "gap": "Port corpus evaluation runner into Go orchestrator (pkg/autoresearch) and expose via Next.js benchmark dashboard."
    },
    "chickensoup": {
        "verdict": "PARTIAL REFERENCE",
        "code_files": ["platform/core/pkg/epistemics/wavefunction.go", "dev-docs/gather_insights_from/chickensoup"],
        "invariant": "Seed repository for credibility math and honest degradation under noise.",
        "findings": "Tripartite wavefunction math was extracted and rewritten in Go. The multi-dimensional belief propagation graph from chickensoup was not ported.",
        "gap": "Decide whether full belief network is needed or whether tripartite wavefunction is the chosen final model (Q-05)."
    },
    "gods-eye-view": {
        "verdict": "PARTIAL",
        "code_files": ["platform/core/pkg/supervisor/supervisor.go:34-49", "dev-docs/gather_insights_from/chickensoup"],
        "invariant": "Automated system-wide policy rules verifiable by machine scripts (e.g. no unanchored runs, single-writer invariants).",
        "findings": "ReconciliationGate in supervisor.go enforces local IPv4 pin, single-writer lease, and spend ceiling checks. Missing formal policy engine language (e.g. OPA/Rego).",
        "gap": "Consolidate all invariant checks into a dedicated policy module in Go."
    },

    # Cluster 2
    "five-ring-research-loop": {
        "verdict": "PARTIAL / ARCHITECTURAL FRAGMENTATION",
        "code_files": ["platform/core/pkg/supervisor/supervisor.go", "platform/core/pkg/epistemics/wavefunction.go", "platform/core/pkg/fidelity/accounting.go", "platform/core/pkg/autoresearch/harness.go"],
        "invariant": "5 concentric research rings: Ring 0 (Reflex Watchdog), Ring 1 (Epistemic Gate), Ring 2 (Hypothesis / Experiment), Ring 3 (Methodology Gate), Ring 4 (Reflexive Evolution).",
        "findings": "Components exist as isolated Go packages (watchdog, epistemics, fidelity, autoresearch). They are NOT executed in a continuous, automated cyclic loop. Execution only occurs when user manually triggers an API endpoint.",
        "gap": "Create a unified LoopEngine goroutine that drives the 5 rings autonomously from hypothesis formulation to result archiving."
    },
    "autonomous-ansatz-optimization": {
        "verdict": "HEURISTIC STUB",
        "code_files": ["platform/core/pkg/autoresearch/harness.go:20-65", "platform/bridge/ansatz_search.py"],
        "invariant": "Karpathy-style autoresearch loop optimizing variational quantum ansatz parameters towards ab-initio FCI energy, avoiding barren plateaus.",
        "findings": "AnsatzSearchHarness.Metric counts the string cx in the artifact rather than simulating the circuit against a molecular Hamiltonian. ansatz_search.py in bridge is not called by Go.",
        "gap": "Connect AnsatzSearchHarness.Metric in Go to Python bridge gRPC to evaluate true expectation value ⟨ψ(θ)|H|ψ(θ)⟩ via Aer."
    },
    "meta-search-and-self-evolution": {
        "verdict": "IN-MEMORY PROPOSAL ONLY",
        "code_files": ["platform/core/pkg/metasearch/evolution.go"],
        "invariant": "Section 13 meta-loop that autonomously proposes mutations to research configurations, evaluated on a held-out benchmark and human-gated.",
        "findings": "evolution.go implements proposal generation and human approve/reject transitions in memory. It does not apply mutations to disk or evaluate held-out test sets.",
        "gap": "Wire evolution engine to mutate declarative YAML harness configurations and run benchmark regression tests."
    },
    "science-discovery": {
        "verdict": "PARTIAL (MANUAL INVOCATION)",
        "code_files": ["platform/core/pkg/api/server.go", "platform/bridge/chemistry.py", "platform/core/pkg/campaign_test.go"],
        "invariant": "End-to-end autonomous discovery loop: ground truth -> active space -> ansatz search -> hardware probe -> report -> archive.",
        "findings": "campaign_test.go proves the pipeline works when chained programmatically in Go test. In live server runtime, steps must be invoked manually via separate API endpoints.",
        "gap": "Implement an autonomous session runner that executes the full campaign pipeline from a single goal prompt."
    },
    "workflows-and-agent-authoring": {
        "verdict": "STATIC UI / UNWIRED ENGINE",
        "code_files": ["platform/console/src/components/surfaces/SpatialCanvas.tsx"],
        "invariant": "Visual node-based canvas for constructing, editing, and executing multi-agent quantum workflows with human-in-the-loop gates.",
        "findings": "SpatialCanvas.tsx renders a React Flow canvas with pre-defined static nodes. It can dispatch a single hardcoded cut execution, but cannot save or load arbitrary user-authored workflow graphs.",
        "gap": "Implement workflow graph serialization (JSON) and an execution engine in Go that executes arbitrary node DAGs."
    },
    "multi-agent-research-collective": {
        "verdict": "DECLARATIVE STUB (NO AUTONOMOUS CHAT)",
        "code_files": ["platform/core/pkg/agents/collective.go", "platform/core/pkg/api/server.go:161-164"],
        "invariant": "5 distinct specialist personas (epistemic-scribe, ansatz-scout, device-auditor, disconfirmation-officer, human-reviewer) with distinct prompts, capability grants, and keys.",
        "findings": "collective.go declares RoleManifest structs for all 5 personas. But in server.go, only the generic copilot persona is called during chat. The 5 specialist personas never debate or interact autonomously.",
        "gap": "Implement multi-agent turn orchestration where Epistemic Scribe, Ansatz Scout, and Disconfirmation Officer evaluate proposals collaboratively."
    },
    "karpathy-autoresearch": {
        "verdict": "HEURISTIC STUB",
        "code_files": ["platform/core/pkg/autoresearch/harness.go", "platform/core/pkg/autoresearch/harness_test.go"],
        "invariant": "Karpathy 4-part harness contract: Fixed Setup, Mutable Artifact, Metric, Validity Check.",
        "findings": "Harness interface and 3 harnesses defined in Go. Metrics use string-counting heuristics instead of running actual physics/simulation engines.",
        "gap": "Replace string-counting heuristics with gRPC calls to Python bridge (PySCF FCI delta and QAOA graph cut expectation)."
    },
    "model-optimizer": {
        "verdict": "UNIMPLEMENTED REFERENCE",
        "code_files": ["dev-docs/gather_insights_from/model-optimizer"],
        "invariant": "Recipe-driven plugin contract for model optimization and compilation passes.",
        "findings": "No model-optimizer recipe runner exists in platform/core or platform/bridge.",
        "gap": "Define declarative optimization recipe schema in Go if adopting this pattern."
    },
    "unsloth": {
        "verdict": "UNIMPLEMENTED",
        "code_files": [],
        "invariant": "Local model fine-tuning engine using Unsloth on Apple Silicon / CUDA for domain-adapted scientific copilots.",
        "findings": "Zero code exists in the repository for fine-tuning models. Platform relies entirely on pre-quantized oMLX/GGUF models.",
        "gap": "Document as deferred (Phase 6) or implement local fine-tuning script."
    },

    # Cluster 3
    "circuit-cutting": {
        "verdict": "REAL PARTITION / INCOMPLETE KNITTING",
        "code_files": ["platform/bridge/executor.py:168-245", "platform/bridge/cutting.py", "platform/tests/test_cutting.py"],
        "invariant": "LOCC wire cutting decomposing large circuits into subcircuits fitting small QPUs, with quasi-probability distribution (QPD) tensor reconstruction overhead O(4^K).",
        "findings": "executor.py cut_circuit_qasm implements genuine graph-partitioned wire cutting using Qiskit 2.x loads/dumps, generating valid subcircuits. Full QPD expectation value knitting is tested in Python unit tests but not exposed via gRPC.",
        "gap": "Expose full QPD circuit knitting expectation reconstruction via gRPC so Go can compute cut expectation values end-to-end."
    },
    "fidelity-accounting": {
        "verdict": "HEURISTIC CLIFFORD PROBE",
        "code_files": ["platform/core/pkg/fidelity/accounting.go", "platform/bridge/fidelity.py:97-106", "platform/core/pkg/api/server.go:166-184"],
        "invariant": "Pre-flight predictive fidelity gate: predicts fidelity from noise profile (T2 <= 2T1), runs cheap Clifford stabilizer probe (<= 64 shots), decides ADMIT / MITIGATE / DOWNGRADE / REFUSE.",
        "findings": "PredictFidelity implements physical error compounding. EvaluateAdmission implements the decision matrix. However, probe_clifford_fidelity in Python uses a heuristic formula raw_fid = exp(-0.015 * n_2q) rather than simulating Clifford gates on Aer/stim.",
        "gap": "Replace exponential heuristic in fidelity.py with real Clifford stabilizer simulation on Qiskit Aer (clifford method) for 64 shots."
    },
    "barren-plateau": {
        "verdict": "UNIMPLEMENTED IN RUNTIME WATCHDOG",
        "code_files": ["platform/core/pkg/watchdog/watchdog.go", "platform/bridge/ansatz_search.py"],
        "invariant": "Detects exponential gradient vanishing Var[∂_θ E] in O(2^-n) across parameterized ansatzes, aborting unviable training runs.",
        "findings": "Watchdog in Go monitors timeouts and budget, but does NOT compute gradient variance Var[∂_θ E]. No gradient calculation exists in bridge.",
        "gap": "Implement parameter-shift gradient variance calculation in bridge and wire to Go watchdog."
    },
    "error-mitigation": {
        "verdict": "LEGACY PYTHON ONLY (NOT IN BRIDGE)",
        "code_files": ["platform/tests/test_zne.py"],
        "invariant": "Quantum Error Mitigation (Zero-Noise Extrapolation via Richardson/polynomial scaling, Readout Error Mitigation).",
        "findings": "test_zne.py passes in pytest, but platform/bridge/ has NO mitigation.py and no gRPC RPC for applying ZNE to QASM jobs.",
        "gap": "Implement ZNE extrapolation module in platform/bridge/ and expose via gRPC to Go job runner."
    },
    "vqa-vqe": {
        "verdict": "REAL GROUND STATE ANCHORS / MANUAL VQE",
        "code_files": ["platform/bridge/chemistry.py", "platform/bridge/executor.py"],
        "invariant": "Variational Quantum Eigensolver mapping active space molecular Hamiltonians via Jordan-Wigner to qubit operators, converging to FCI within chemical accuracy (1.6 mHa).",
        "findings": "chemistry.py computes real ab-initio PySCF RHF and FCI ground states. executor.py runs circuits on Aer simulator. Parameterized VQE optimization loop is manual rather than a single automated RPC.",
        "gap": "Provide a complete VQE optimizer RPC in Python bridge that iterates parameters until energy converges to FCI."
    },
    "differentiable-quantum-stack": {
        "verdict": "UNIMPLEMENTED",
        "code_files": ["platform/bridge/executor.py"],
        "invariant": "Parameter-shift rule evaluation (∂f/∂θ = [f(θ+s) - f(θ-s)] / 2sin(s)) and automatic differentiation pipelines in PennyLane/PyTorch.",
        "findings": "No parameter-shift gradient evaluation function is exposed in platform/bridge/.",
        "gap": "Implement parameter_shift_gradients function in bridge using PennyLane/Qiskit."
    },
    "agentic-qir-compilation": {
        "verdict": "UNIMPLEMENTED",
        "code_files": [],
        "invariant": "LLVM/QIR bytecodes, adaptive control flow, universal post-QIR gate glyphs.",
        "findings": "All circuits are passed as OpenQASM 2.0 text strings. No QIR bytecode compilation or execution exists in the active platform.",
        "gap": "Integrate pyqir / qir-runner for compiling OpenQASM to QIR bitcode."
    },
    "qir-mlir-compilation": {
        "verdict": "UNIMPLEMENTED",
        "code_files": [],
        "invariant": "QIR / MLIR compilation pipeline and optimization pass managers.",
        "findings": "No MLIR/QIR compilation pass pipeline exists in platform/.",
        "gap": "Classify as deferred or implement QIR transpile pass via Qiskit QIR export."
    },
    "nisq-and-hybrid-orchestration": {
        "verdict": "REAL ARCHITECTURAL FOUNDATION",
        "code_files": ["platform/core/cmd/kubonad/main.go", "platform/bridge/server.py", "platform/core/pkg/jobs/jobs.go"],
        "invariant": "Decoupled architecture: Go high-throughput orchestrator for concurrency, Python gRPC microservice for scientific runtimes.",
        "findings": "Architecture is fully realized and operating: Go orchestrator on :8080 communicates with Python bridge on :50051 via gRPC protobufs.",
        "gap": "Expand gRPC protobuf definition to include gradient, mitigation, and multi-molecule active space RPCs."
    },
    "quantum-domain-packs": {
        "verdict": "PARTIAL (CHEMISTRY PACK ONLY)",
        "code_files": ["platform/bridge/chemistry.py"],
        "invariant": "Modular domain packs (molecular chemistry, materials science, portfolio optimization) with standardized Hamiltonian generators.",
        "findings": "Molecular chemistry domain pack is implemented for H2 and LiH in chemistry.py. Materials and optimization packs are unbuilt.",
        "gap": "Extract chemistry.py into a formal domain pack structure and add QAOA MaxCut graph domain pack."
    },
    "qiskit": {
        "verdict": "REAL & VERIFIED (QISKIT 2.5.2 & AER 0.17.2)",
        "code_files": ["platform/bridge/executor.py", "platform/bridge/fidelity.py"],
        "invariant": "Qiskit SDK used for circuit synthesis, Aer simulation, noise models, and OpenQASM parsing.",
        "findings": "Real Qiskit 2.5.2 and Qiskit Aer 0.17.2 installed in platform/.venv, handling Aer noisy density matrix simulation, statevectors, and OpenQASM 2.0 parsing.",
        "gap": "None for core simulation. Need Qiskit transpiler optimization level exposure (0-3) in API."
    },
    "pennylane": {
        "verdict": "PARTIAL",
        "code_files": ["platform/bridge/executor.py", "platform/bridge/cutting.py"],
        "invariant": "PennyLane used for differentiable quantum programming and QPD circuit cutting.",
        "findings": "PennyLane installed in .venv. Circuit cutting in executor.py uses a graph partitioner rather than PennyLane qcut transforms.",
        "gap": "Wire pennylane.qcut directly into executor.py."
    },
    "google-cirq": {
        "verdict": "UNWIRED IN BRIDGE",
        "code_files": ["dev-docs/submodules/cirq"],
        "invariant": "Google Cirq used for NISQ circuit simulation and device geometry mapping.",
        "findings": "Submodule exists in dev-docs/submodules/cirq. Cirq is not imported or used in platform/bridge/.",
        "gap": "Add Cirq OpenQASM exporter or mark as reference."
    },
    "qiskit-circuit-knitting": {
        "verdict": "PARTIAL / TEST ONLY",
        "code_files": ["platform/tests/test_cutting.py"],
        "invariant": "Qiskit Circuit Knitting Toolbox used for LOCC wire cut and gate cut reconstruction.",
        "findings": "Unit tests in test_cutting.py test wire-cutting concepts, but Qiskit CKT library is not imported in platform/bridge/executor.py.",
        "gap": "Integrate circuit knitting expectation reconstruction into executor.py."
    },
    "mitiq": {
        "verdict": "UNWIRED IN BRIDGE",
        "code_files": ["dev-docs/submodules/mitiq", "platform/tests/test_zne.py"],
        "invariant": "Mitiq used for Zero-Noise Extrapolation, CDR, and PEC error mitigation.",
        "findings": "Submodule in dev-docs/submodules/mitiq. Legacy test in test_zne.py. Not imported in platform/bridge/.",
        "gap": "Import Mitiq in bridge and expose zne_mitigate RPC."
    },
    "openqasm": {
        "verdict": "REAL & VERIFIED (OPENQASM 2.0)",
        "code_files": ["platform/bridge/executor.py:32-50", "platform/core/pkg/api/server.go:504-512"],
        "invariant": "Standard parse and export contract for circuits generated by agents and executed on backends.",
        "findings": "OpenQASM 2.0 loads and dumps fully functional via qiskit.qasm2 in executor.py. Used across all chat proposals and job submissions.",
        "gap": "Add OpenQASM 3.0 support for dynamic circuits and mid-circuit branching."
    },
    "nvidia-cuda-q": {
        "verdict": "UNIMPLEMENTED",
        "code_files": [],
        "invariant": "NVIDIA CUDA-Q / cuQuantum GPU accelerated simulation runtime.",
        "findings": "Zero code exists in platform/ for CUDA-Q. Platform targets Apple Silicon (Metal/Accelerate) and CPU.",
        "gap": "Mark as deferred hardware target in provider schema."
    },
    "xanadu-strawberry-fields": {
        "verdict": "UNIMPLEMENTED",
        "code_files": [],
        "invariant": "Continuous variable photonic quantum computing platform.",
        "findings": "Zero code exists in platform/ for continuous variable photonics.",
        "gap": "Mark as reference / tracked."
    },
    "dwave": {
        "verdict": "UNIMPLEMENTED",
        "code_files": [],
        "invariant": "D-Wave quantum annealing for QUBO / Ising optimization problems.",
        "findings": "Zero code exists in platform/ for D-Wave Ocean SDK.",
        "gap": "Mark as candidate for QAOA/Annealing comparison."
    },
    "qbraid": {
        "verdict": "UNIMPLEMENTED",
        "code_files": [],
        "invariant": "Unified quantum cloud access aggregator.",
        "findings": "Zero code exists in platform/ for qBraid SDK.",
        "gap": "Mark as reference."
    },
    "qflow-studio": {
        "verdict": "UNIMPLEMENTED",
        "code_files": [],
        "invariant": "Visual quantum circuit workflow design platform.",
        "findings": "Referenced in wiki as design inspiration for Surface 1 (Spatial Canvas).",
        "gap": "Design pattern adopted in SpatialCanvas.tsx."
    },
    "mainza-milimo-foundation": {
        "verdict": "REFERENCE",
        "code_files": ["platform/bridge/chemistry.py", "dev-docs/gather_insights_from/Milimo-QDD"],
        "invariant": "Seed material for ab-initio molecular Hamiltonian generation.",
        "findings": "Molecular geometry and PySCF integration in chemistry.py was derived from Milimo-QDD insights.",
        "gap": "Port multi-orbital active space selection logic from Milimo-QDD."
    },

    # Cluster 4
    "provider-capability-schema": {
        "verdict": "HARDCODED STRUCT",
        "code_files": ["platform/core/pkg/api/server.go:216-264", "platform/core/pkg/fidelity/accounting.go:53-78"],
        "invariant": "Schema declaring backend qubit count, topology, gate set, T1/T2 times, error rates, pricing, and ITAR jurisdiction.",
        "findings": "DeviceProfile structs hardcoded in Go for ibm_heron, quantinuum_h1, and rigetti_aspen. Not dynamically loaded from external provider manifests or APIs.",
        "gap": "Load provider capability manifests from external JSON/YAML schema files."
    },
    "quantum-cloud-pricing": {
        "verdict": "HARDCODED TELEMETRY",
        "code_files": ["platform/core/pkg/api/server.go:226-260", "platform/console/src/components/surfaces/FleetRadar.tsx"],
        "invariant": "Rate card pricing ($/shot, $/minute) used for pre-flight spend estimation and AP2 budget checks.",
        "findings": "Prices hardcoded in server.go: ibm_heron ($0.0016/shot), quantinuum_h1 ($0.0125/shot), rigetti_aspen ($0.0008/shot), local_simulator ($0.00).",
        "gap": "Implement dynamic rate card repository with currency conversion and volume discounts."
    },
    "ibm-quantum-composer": {
        "verdict": "REFERENCE",
        "code_files": ["platform/console/src/components/surfaces/SpatialCanvas.tsx"],
        "invariant": "Visual reference for drag-and-drop circuit composition and gate glyphs.",
        "findings": "Inspiration for Surface 1 node design. Not integrated with IBM Composer cloud service.",
        "gap": "Add drag-and-drop gate palette to SpatialCanvas."
    },
    "aws-braket": {
        "verdict": "UNWIRED",
        "code_files": ["dev-docs/gather_insights_from/milimoquantum"],
        "invariant": "AWS Braket quantum cloud provider driver.",
        "findings": "No AWS Braket SDK calls in platform/core or platform/bridge.",
        "gap": "Add Braket driver stub in provider schema."
    },
    "quantinuum": {
        "verdict": "HARDCODED PROFILE ONLY",
        "code_files": ["platform/core/pkg/fidelity/accounting.go:62-69", "platform/core/pkg/api/server.go:242-251"],
        "invariant": "Quantinuum H-Series trapped ion hardware driver and noise profile.",
        "findings": "Noise profile (t1=1s, 20 qubits) exists in accounting.go. No live API integration.",
        "gap": "Gated behind local testing gate by design."
    },
    "rigetti": {
        "verdict": "HARDCODED PROFILE ONLY",
        "code_files": ["platform/core/pkg/fidelity/accounting.go:70-78", "platform/core/pkg/api/server.go:253-262"],
        "invariant": "Rigetti Aspen/Ankaa superconducting QPU driver.",
        "findings": "Noise profile (t1=35us, 80 qubits) exists in accounting.go. No live API integration.",
        "gap": "Gated behind local testing gate by design."
    },
    "ionq": {
        "verdict": "UNWIRED",
        "code_files": [],
        "invariant": "IonQ trapped ion provider.",
        "findings": "Listed in provider capability schema in wiki; omitted from server.go fleet handler.",
        "gap": "Add IonQ profile to fleet handler and fidelity gate."
    },
    "neutral-atom-quera-pasqal": {
        "verdict": "UNWIRED",
        "code_files": [],
        "invariant": "QuEra and Pasqal neutral atom quantum processors (analog Hamiltonian simulation).",
        "findings": "Zero driver code in platform/.",
        "gap": "Mark as future hardware modality."
    },
    "microsoft-azure-quantum": {
        "verdict": "UNWIRED",
        "code_files": [],
        "invariant": "Azure Quantum cloud workspace provider.",
        "findings": "Zero driver code in platform/.",
        "gap": "Mark as future cloud aggregator."
    },
    "strangeworks": {
        "verdict": "REFERENCE",
        "code_files": [],
        "invariant": "Strangeworks quantum orchestration platform.",
        "findings": "Referenced in wiki as ecosystem comparative study.",
        "gap": "Reference only."
    },
    "qiskit-mcp-servers": {
        "verdict": "UNINTEGRATED EXTERNAL SUITE",
        "code_files": ["wiki/entities/qiskit-mcp-servers.md"],
        "invariant": "Official IBM Qiskit MCP servers for transpile and runtime jobs.",
        "findings": "Audited in wiki (Q-30). Not packaged or invoked by Kubona's Go orchestrator.",
        "gap": "Encapsulate Qiskit MCP server tools inside Kubona's MCP gateway."
    },

    # Cluster 5
    "spend-mandates-and-delegation": {
        "verdict": "REAL ED25519 CRYPTO / SERVER-SIGNED",
        "code_files": ["platform/core/pkg/mandates/mandate.go", "platform/core/pkg/mandates/mandate_test.go", "platform/core/pkg/api/server.go:305-340", "platform/console/src/components/surfaces/AutonomyCockpit.tsx"],
        "invariant": "Cryptographic Ed25519 spend mandates signing tenant_id, job_hash, max_budget, and expiry. Blocks execution if budget exceeded or signature invalid.",
        "findings": "Real Ed25519 crypto in Go (SignMandate, Verify). AutonomyCockpit.tsx calls POST /api/mandates/sign. HOWEVER, signing is done by server private key rather than human browser WebCrypto keypair.",
        "gap": "Implement client-side WebCrypto Ed25519 key generation and signing in Next.js browser console."
    },
    "cost-governance": {
        "verdict": "REAL PRE-FLIGHT CHECK",
        "code_files": ["platform/core/pkg/jobs/jobs.go:34-45", "platform/core/pkg/supervisor/supervisor.go:42"],
        "invariant": "Pre-flight pricing estimation with hard spend ceilings and fail-closed abortion.",
        "findings": "jobs.go checks max_cost_usd against session spend limit. Returns error if budget is exceeded.",
        "gap": "Expose multi-tier budget approvals in Autonomy Cockpit UI."
    },
    "agent-resource-economy": {
        "verdict": "PARTIAL",
        "code_files": ["platform/core/pkg/llm/client.go", "platform/core/pkg/jobs/jobs.go"],
        "invariant": "Micro-metering of agent token usage, execution time, and QPU shots.",
        "findings": "llm/client.go tracks generation tokens, prompt tokens, and duration. jobs.go records execution time and cost.",
        "gap": "Aggregate resource usage into a unified per-tenant billing ledger table in SQLite."
    },
    "protocol-strategy": {
        "verdict": "REAL CAPABILITY CORE DECOUPLING",
        "code_files": ["platform/core/pkg/api/server.go", "platform/core/pkg/jobs/jobs.go", "platform/core/pkg/mcp/gateway.go"],
        "invariant": "Capability core decoupled from protocol transports (MCP, HTTP, gRPC, A2A).",
        "findings": "Capability core lives in Go packages (jobs, ledger, llm, memory) independent of transport. HTTP REST and gRPC are cleanly layered.",
        "gap": "Add official A2A protocol endpoint adapter."
    },
    "user-identity-and-profiles": {
        "verdict": "BASIC SESSION MODEL",
        "code_files": ["platform/core/pkg/session/session.go"],
        "invariant": "Tenant profiles, cryptographic keyrings, and role-based access control.",
        "findings": "session.go tracks Session with ID, Title, SpendLimitUSD, and CurrentSpendUSD. Full multi-tenant RBAC and user account management is absent.",
        "gap": "Implement multi-tenant auth table and API key middleware."
    },
    "ap2": {
        "verdict": "REAL SPEC IMPLEMENTATION (MANDATES)",
        "code_files": ["platform/core/pkg/mandates/mandate.go", "platform/core/pkg/api/server.go:504-512"],
        "invariant": "Agent Payments Protocol (AP2) defining cryptographic mandates and spending limits.",
        "findings": "SpendMandate struct implements AP2 mandate specification with Ed25519 signature verification.",
        "gap": "Add AP2 cart and payment confirmation receipt schemas."
    },
    "a2a": {
        "verdict": "UNIMPLEMENTED PROTOCOL ADAPTER",
        "code_files": [],
        "invariant": "Google Agent2Agent (A2A) protocol for inter-agent delegation and task handoff.",
        "findings": "Adopted in wiki, but no A2A JSON-RPC endpoints exist in platform/core/.",
        "gap": "Implement A2A server endpoint in Go."
    },
    "x402": {
        "verdict": "DORMANT BY DESIGN",
        "code_files": ["platform/tests/test_payment.py"],
        "invariant": "HTTP 402 Payment Required micro-metering rail for agent compute.",
        "findings": "Fails closed in legacy test_payment.py. Dormant by design per roadmap Phase 5.",
        "gap": "Status correctly marked as dormant; no code action required for current phase."
    },
    "paperclip": {
        "verdict": "PARTIAL REFERENCE (PERMISSION SCOPES)",
        "code_files": ["platform/core/pkg/mcp/gateway.go:17-48", "dev-docs/gather_insights_from/paperclip"],
        "invariant": "Paperclip 4-plane MCP access governance with low_trust_review human intervention.",
        "findings": "gateway.go implements ScopeReadOnly, ScopeSimulation, ScopeHardwareExecution, and low_trust_review requirement checks.",
        "gap": "Integrate gateway permission check into HTTP API request pipeline."
    },
    "aaif": {
        "verdict": "TRACKED ONLY",
        "code_files": [],
        "invariant": "Agentic AI Foundation governance and interoperability standards.",
        "findings": "External foundation monitored in wiki; no code needed.",
        "gap": "None."
    },
    "cloudflare": {
        "verdict": "TRACKED ONLY",
        "code_files": [],
        "invariant": "Cloudflare Workers, Durable Objects, and agent economy deployment target.",
        "findings": "Monitored as potential edge deployment architecture. Platform currently targets local Docker/host.",
        "gap": "None for local-first architecture."
    },

    # Cluster 6
    "confidentiality-lattice-provenance": {
        "verdict": "REAL LATTICE MATH / UNWIRED TO DB",
        "code_files": ["platform/core/pkg/ifc/lattice.go", "platform/crates/ifc-core/src/lib.rs", "platform/core/pkg/ifc/lattice_test.go"],
        "invariant": "Buzz Information Flow Control (IFC) reader-set lattice enforcing src can flow to dst iff dst.Readers is subset of src.Readers.",
        "findings": "ReaderSetLattice implemented in Go and Rust with 100% passing tests. HOWEVER, database queries in ledger.go and jobs.go do NOT check CanFlowTo before returning rows.",
        "gap": "Wire lattice.CanFlowTo into SQL query filters in ledger.go and jobs.go."
    },
    "secrets-and-workload-isolation": {
        "verdict": "IN-MEMORY LEASE MOCK",
        "code_files": ["platform/core/pkg/vault/vault.go", "platform/core/pkg/sandbox/docker.go"],
        "invariant": "Dynamic single-use secrets leases via OpenBao vault and disposable container isolation.",
        "findings": "vault.go is an in-memory mock store (map[string]Lease). docker.go exists but is not invoked during standard job execution.",
        "gap": "Integrate real OpenBao sidecar client and execute user Python jobs inside Docker container runner."
    },
    "security-and-pqc": {
        "verdict": "PARTIAL (ED25519 & TLS ONLY)",
        "code_files": ["platform/core/pkg/mandates/mandate.go"],
        "invariant": "Post-Quantum Cryptography transport (ML-KEM / Dilithium) and Ed25519 mandate signatures.",
        "findings": "Ed25519 signatures implemented for mandates. Post-quantum key exchange (ML-KEM) is not configured on Go TLS listener.",
        "gap": "Configure Go crypto/tls for post-quantum hybrid key exchange (X25519MLKEM768)."
    },
    "integration-topology-and-licence-discipline": {
        "verdict": "SETTLED POLICY (HONORED)",
        "code_files": ["AGENTS.md", "wiki/concepts/integration-topology-and-licence-discipline.md"],
        "invariant": "Four integration tiers: Submodule, Sidecar, Blueprint, Reference. Core platform is clean Apache-2.0.",
        "findings": "Strictly honored: platform/core is pure Go Apache-2.0, platform/bridge is Python, dev-docs is gitignored scratch, no copy-pasting of MPL/GPL code.",
        "gap": "None. Standing policy is clean."
    },
    "risks": {
        "verdict": "TRACKED LIVING DOCUMENT",
        "code_files": ["wiki/concepts/risks.md"],
        "invariant": "Active register of security, operational, and financial risks.",
        "findings": "PAT leakage risk resolved; local credential helper store risk documented.",
        "gap": "Follow through on Q-27 git history cleanup before publication."
    },
    "buzz": {
        "verdict": "MATHEMATICAL CORE PORTED",
        "code_files": ["platform/core/pkg/ifc/lattice.go", "platform/crates/ifc-core/src/lib.rs", "dev-docs/gather_insights_from/buzz"],
        "invariant": "Hive mind communication platform with IFC reader-set security lattice.",
        "findings": "Lattice logic from buzz was extracted and implemented in Go and Rust.",
        "gap": "Wire into database query authorization."
    },
    "openbao": {
        "verdict": "IN-MEMORY MOCK ONLY",
        "code_files": ["platform/core/pkg/vault/vault.go"],
        "invariant": "Open-source MPL-2.0 secrets vault sidecar managing dynamic provider API keys.",
        "findings": "vault.go simulates lease creation and revocation in Go RAM without talking to an OpenBao daemon.",
        "gap": "Implement HTTP client for OpenBao sidecar /v1/sys/leases."
    },

    # Cluster 7
    "agentic-memory-and-observability": {
        "verdict": "REAL PERSISTENCE / UNWIRED RETRIEVAL",
        "code_files": ["platform/core/pkg/memory/hindsight.go", "platform/core/pkg/ledger/ledger.go"],
        "invariant": "Three memory tiers: short-term session buffer, episodic SQLite WAL ledger, semantic Hindsight memory banks with time-decay consolidation.",
        "findings": "Hindsight memory store with SQLite WAL persistence and time-decay consolidation is implemented. HOWEVER, copilot chat does NOT call Recall() to retrieve memories into prompt context.",
        "gap": "Inject Hindsight Recall() results into generateCopilotResponse prompt context."
    },
    "mcp-and-context-flooding": {
        "verdict": "BASIC GATEWAY ONLY",
        "code_files": ["platform/core/pkg/mcp/gateway.go"],
        "invariant": "Prevents context window flooding via lazy tool schemas, tool filtering, and semantic tool search.",
        "findings": "gateway.go filters tools by permission scope, but does not implement semantic search or lazy schema hydration.",
        "gap": "Implement lazy-loading tool registry."
    },
    "science-data-integrations": {
        "verdict": "REAL CONNECTOR / UNWIRED TO CHAT",
        "code_files": ["platform/bridge/literature.py"],
        "invariant": "Connectors to arXiv, OpenAlex, Semantic Scholar for literature grounding and DOI verification.",
        "findings": "literature.py implements arXiv search and OpenAlex metadata retrieval. It is NOT invoked by the Go chat engine.",
        "gap": "Expose literature search as an autonomous tool callable by Epistemic Scribe."
    },
    "hindsight": {
        "verdict": "REAL SQLITE WAL STORAGE",
        "code_files": ["platform/core/pkg/memory/hindsight.go", "platform/core/pkg/memory/hindsight_test.go"],
        "invariant": "Agent memory sidecar service managing world-facts, experiences, and mental-models.",
        "findings": "Fully implemented with SQLite WAL persistence in hindsight_memories table with passing Go test.",
        "gap": "Add semantic vector embedding search for memory recall."
    },
    "mainza-consciousness": {
        "verdict": "ALGORITHM EXTRACTED",
        "code_files": ["platform/core/pkg/memory/hindsight.go:102-108", "dev-docs/gather_insights_from/mainza-consciousness"],
        "invariant": "Time-decay memory consolidation algorithm.",
        "findings": "Exponential decay formula E(t) = max(0.1, 1.0 - 0.01t) extracted and implemented in hindsight.go.",
        "gap": "None for formula. Complete memory recall pipeline."
    },
    "arxiv-mcp-quantum": {
        "verdict": "PARTIAL (LOCAL PYTHON CONNECTOR)",
        "code_files": ["platform/bridge/literature.py"],
        "invariant": "MCP server wrapping arXiv for quantum scientific discovery.",
        "findings": "literature.py queries arXiv API directly via HTTP.",
        "gap": "Wrap literature.py as an MCP tool inside Gateway."
    },

    # Cluster 8
    "ui-ux-vision": {
        "verdict": "ADOPTED IN NEXT.JS CONSOLE",
        "code_files": ["platform/console/src/app/page.tsx", "platform/console/src/components/ObservatoryCockpit.tsx"],
        "invariant": "TypeScript/React Next.js 16 console with 7 Observatory surfaces, infinite canvas, and Copilot drawer.",
        "findings": "Complete migration from Flutter to Next.js 16 + Tailwind CSS. 8 surfaces mounted in ObservatoryCockpit.",
        "gap": "Replace remaining static cards and mock arrays in individual surfaces with dynamic live data."
    },
    "agent-ux-control-plane": {
        "verdict": "LIVE REST/GRPC HYBRID",
        "code_files": ["platform/core/pkg/api/server.go", "platform/console/src/lib/api.ts"],
        "invariant": "Bi-directional human-agent collaboration plane with telemetry speedometers and approval gates.",
        "findings": "API routes operational on :8080, serving health, sessions, chat, jobs, loops, radar, and LLM settings.",
        "gap": "Add WebSocket / SSE stream for real-time live event pushing instead of HTTP polling."
    },
    "provenance-and-reproducibility": {
        "verdict": "REAL SQLITE WAL LEDGER / MERKLE MISSING",
        "code_files": ["platform/core/pkg/ledger/ledger.go", "platform/console/src/components/surfaces/ResultsStudio.tsx"],
        "invariant": "Cryptographic reproducibility with SHA-256 event provenance, Merkle trees, and citable report export.",
        "findings": "ledger.go records immutable events with sequential IDs into SQLite WAL. ResultsStudio exports Markdown report with provenance hash. Full Merkle tree audit proofs are not built.",
        "gap": "Implement cryptographic Merkle tree parent-hash chaining across ledger events."
    },
    "design-evolution": {
        "verdict": "HISTORICAL SYNTHESIS",
        "code_files": ["wiki/concepts/design-evolution.md"],
        "invariant": "Records transition from initial white paper to buildable engineering blueprint.",
        "findings": "Accurately documents design evolution across the 4 major milestones.",
        "gap": "Living documentation."
    },
    "implementation-gaps": {
        "verdict": "LIVING GAP REGISTER",
        "code_files": ["wiki/concepts/implementation-gaps.md"],
        "invariant": "Tracked register of code reality vs blueprint.",
        "findings": "Updated in previous turn to record oMLX port scan gap and circuit cutting state.",
        "gap": "Update with this deep dive audit findings."
    },
    "kubona": {
        "verdict": "CORE IDENTITY",
        "code_files": ["wiki/entities/kubona.md", "platform/core/cmd/kubonad/main.go"],
        "invariant": "Project definition and naming (Tonga: to see).",
        "findings": "Identifies the core platform.",
        "gap": "None."
    },
    "univer": {
        "verdict": "UNINSTALLED / REPLACED",
        "code_files": ["wiki/entities/univer.md"],
        "invariant": "Univer collaborative data spreadsheet/canvas inside console.",
        "findings": "Univer npm packages are NOT installed in platform/console. Custom HTML tables and React Flow canvas are used instead.",
        "gap": "Decide whether Univer canvas is required or superseded by React Flow."
    },
    "overview": {
        "verdict": "LIVING BLUEPRINT SYNTHESIS",
        "code_files": ["wiki/overview.md"],
        "invariant": "Master synthesis of what Kubona is, its 8 core design tenets, and operational status.",
        "findings": "Updated with Point 8 (Cognitive Mesh, Sovereign Air-Gap, Local-First Quantum Validation).",
        "gap": "Reflect deep dive audit outcomes."
    },
    "roadmap": {
        "verdict": "LIVING ACCEPTANCE GATES",
        "code_files": ["wiki/roadmap.md"],
        "invariant": "Phased acceptance gates (Phases 0 through 6).",
        "findings": "Updated with modern TypeScript/Go implementation roadmap and Local Testing Gate.",
        "gap": "Align gate criteria with real code deliverables."
    },
    "questions": {
        "verdict": "LIVING WORK-LIST",
        "code_files": ["wiki/questions.md"],
        "invariant": "Ranked work-list of open questions and design forks.",
        "findings": "Q-22 and Q-25 closed; P0 questions Q-01, Q-02, Q-04, Q-05, Q-08 remain active.",
        "gap": "Close Q-01 once role of platform/ is formally certified."
    },
    "index": {
        "verdict": "LIVING CATALOG",
        "code_files": ["wiki/index.md"],
        "invariant": "Complete catalog of all 89 pages, layer map, and controlled vocabulary status legend.",
        "findings": "100% clean under python3 scripts/wiki-lint.py --strict.",
        "gap": "None."
    },
    "log": {
        "verdict": "LIVING CHRONOLOGY",
        "code_files": ["wiki/log.md"],
        "invariant": "Append-only chronological record of ingests, decisions, and verifications.",
        "findings": "Logged previous session entry.",
        "gap": "Append deep dive audit entry."
    },
    "source-gap-analysis-plan": {
        "verdict": "INGESTED SOURCE (IMMUTABLE)",
        "code_files": ["wiki/sources/source-gap-analysis-plan.md"],
        "invariant": "Summary of original 16-chapter gap analysis plan.",
        "findings": "Reference source document.",
        "gap": "None."
    },
    "source-architecture-paper": {
        "verdict": "INGESTED SOURCE (IMMUTABLE)",
        "code_files": ["wiki/sources/source-architecture-paper.md"],
        "invariant": "Summary of foundational white paper describing 8 layers and 5 rings.",
        "findings": "Reference source document.",
        "gap": "None."
    },
    "source-frontier-repo-audit-2026": {
        "verdict": "INGESTED SOURCE (IMMUTABLE)",
        "code_files": ["wiki/sources/source-frontier-repo-audit-2026.md"],
        "invariant": "Summary of 25 frontier repositories audit across licenses and architectures.",
        "findings": "Reference source document.",
        "gap": "None."
    },
    "source-cloudflare-economy": {
        "verdict": "INGESTED SOURCE (IMMUTABLE)",
        "code_files": ["wiki/sources/source-cloudflare-economy.md"],
        "invariant": "Summary of agent economic primitives on Cloudflare.",
        "findings": "Reference source document.",
        "gap": "None."
    },
    "source-agent-protocol-landscape-2026": {
        "verdict": "INGESTED SOURCE (IMMUTABLE)",
        "code_files": ["wiki/sources/source-agent-protocol-landscape-2026.md"],
        "invariant": "Summary of A2A, AP2, MCP spec revisions.",
        "findings": "Reference source document.",
        "gap": "None."
    }
}

print(f"Evaluated all {len(page_evaluations)} pages in dictionary.")
assert len(page_evaluations) == 89
print("All 89 pages have explicit granular audit evaluations.")

with open("audit_evaluations_89.json", "w") as out:
    json.dump(page_evaluations, out, indent=2)
