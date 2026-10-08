export interface Session {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
  status: string;
  spend_limit_usd: number;
  current_spend_usd: number;
}

export interface Proposal {
  target: string;
  molecule?: string;
  bond_distance?: number;
  recommended_tier: string;
  classical_anchor: string;
  spend_estimate: string;
  job_type?: string;
  qasm?: string;
  launch_params?: Record<string, unknown>;
}

export interface TribunalTurn {
  turn_index: number;
  role: 'ansatz_scout' | 'disconfirmation_officer' | 'device_auditor' | 'epistemic_scribe' | string;
  name: string;
  content: string;
  verdict: string;
  timestamp?: string;
}

export interface TribunalResult {
  target: string;
  molecule: string;
  turns: TribunalTurn[];
  proposal_json?: Proposal | null;
  consensus_passed: boolean;
  synthesis: string;
}

export interface ChatMessage {
  id: string;
  session_id: string;
  role: 'user' | 'assistant' | 'system' | string;
  content: string;
  proposal_json?: Proposal;
  tribunal?: TribunalResult;
  created_at: string;
}

export interface Job {
  id: string;
  session_id: string;
  status: string;
  job_type: string;
  params_json: Record<string, unknown>;
  result_json?: Record<string, unknown>;
  created_at: string;
  completed_at?: string;
}

export interface Loop {
  id: string;
  session_id: string;
  status: string;
  step_count: number;
  max_steps: number;
  diverged: boolean;
  created_at: string;
  updated_at: string;
}

export interface LedgerEvent {
  id: number;
  timestamp: string;
  session_id: string;
  event_type: string;
  payload: Record<string, unknown>;
}

export interface QuantumFactors {
  target_molecule: string;
  molecular_formula: string;
  bond_distance: number;
  active_electrons: number;
  active_orbitals: number;
  fermion_mapping: string;
  qubit_count: number;
  circuit_depth: number;
  two_qubit_gates: number;
  ansatz_family: string;
  meyer_wallach_q: number;
  gradient_variance: number;
  barren_plateau_risk: string;
  clifford_fidelity: number;
  recommended_backend: string;
  estimated_spend_usd: number;
  qasm: string;
  dag_nodes: Record<string, unknown>[];
  dag_edges: Record<string, unknown>[];
  wire_cuts: Record<string, unknown>[];
}

export interface ClassicalAnchor {
  method: string;
  rhf_mean_field_energy: number;
  fci_exact_energy: number;
  vqe_expectation_energy: number;
  discrepancy_mha: number;
  chemical_accuracy_passed: boolean;
  chemical_accuracy_limit: number;
}

export interface TrajectoryStep {
  step: number;
  energy: number;
  loss: number;
  theta: number[];
  grad_norm: number;
  watchdog: string;
  timestamp: string;
}

export interface EpistemicState {
  alpha_corroborated: number;
  beta_contested: number;
  gamma_unverified: number;
  p_correlated: number;
  status: string;
  literature_drift_mha: number;
  statement: string;
  sources: string[];
}

export interface SystematicNotebook {
  id: string;
  title: string;
  created_at: string;
  classical_anchor: string;
  quantum_energy: number;
  classical_energy: number;
  delta_mha: number;
  chemical_accuracy: boolean;
  provenance_hash: string;
  epistemic_verdict: string;
  sections: string[];
}

export interface CampaignState {
  session_id: string;
  research_goal: string;
  target_system: string;
  smiles?: string;
  quantum_factors: QuantumFactors;
  classical_anchor: ClassicalAnchor;
  trajectory: TrajectoryStep[];
  branches: Array<{ id: string; name: string; step: number; status: string; delta: string }>;
  epistemics: EpistemicState;
  admet?: Record<string, unknown>;
  notebook: SystematicNotebook;
  updated_at: string;
}

export interface AutoresearchHarness {
  id: string;
  fixed_setup: string;
  mutable_artifact: string;
}

export interface IterationRecord {
  iteration: number;
  timestamp: string;
  decision: 'keep' | 'discard' | 'crash';
  metric: number;
  metric_delta: number;
  circuit_depth: number;
  num_qubits: number;
  two_qubit_gate_count: number;
  entanglement_score: number;
  physical_value: number;
  mutated_artifact: string;
  unified_diff: string;
  rationale: string;
  duration_ms: number;
  sandboxed: boolean;
  isolation_tag: string;
  error?: string;
}

export interface ParetoPoint {
  iteration: number;
  metric: number;
  circuit_depth: number;
  two_qubit_gate_count: number;
}

export interface AutoresearchLoop {
  id: string;
  session_id: string;
  harness_id: string;
  status: 'idle' | 'running' | 'paused' | 'stagnant' | 'completed' | 'aborted';
  current_iteration: number;
  max_iterations: number;
  started_at: string;
  updated_at: string;
  champion_artifact: string;
  champion_metric: number;
  champion_depth: number;
  champion_2q_gates: number;
  consecutive_discards: number;
  iterations: IterationRecord[];
  pareto_frontier: ParetoPoint[];
  stagnation_reason?: string;
}

export interface MetaProposal {
  id: string;
  loop_id: string;
  harness_id: string;
  target_file: string;
  status: 'pending_approval' | 'approved' | 'rejected' | 'applied';
  current_instructions: string;
  proposed_diff: string;
  rationale: string;
  approved_by_human: boolean;
  approved_by?: string;
  created_at: string;
  resolved_at?: string;
}

// -----------------------------------------------------------------------------
// Frontier Scientific Agentic Experience (AX) & Multi-Agent Interface Types
// -----------------------------------------------------------------------------

export type AgentRoleType =
  | 'lead_theorist'
  | 'lean4_proof_engineer'
  | 'classical_anchor_solver'
  | 'quantum_circuit_compiler'
  | 'adversarial_falsification_critic'
  | 'sentinel_spend_governor';

export interface AgentPersona {
  id: string;
  name: string;
  role: AgentRoleType;
  avatar_emoji: string;
  badge_color: string;
  description: string;
  active_subagent_task?: string;
  thinking_horizon_seconds: number;
}

export interface EpistemicTribunalVote {
  turn_index: number;
  persona_id: string;
  role: AgentRoleType;
  name: string;
  verdict: 'corroborated' | 'contested' | 'falsified' | 'abstain';
  confidence_rho: number; // 0.000 to 1.000
  rationale: string;
  formal_ast_hash?: string;
  dissenting_argument?: string;
  timestamp: string;
}

export interface FlightRecorderBranchPoint {
  branch_id: string;
  parent_id?: string;
  timestamp: string;
  step_index: number;
  event_type:
    | 'conjecture_init'
    | 'cavity_expansion'
    | 'tree_depth_shift'
    | 'backtrack_prune'
    | 'comparator_check'
    | 'qpu_dispatch'
    | 'ledger_commit';
  cognitive_label: string;
  inner_monologue_snippet: string;
  duration_seconds: number;
  fork_available: boolean;
  active_cavity_lemma?: string;
}

export type GenerativeUIWidgetType =
  | 'molecular_active_space_3d'
  | 'quantum_circuit_qir_editor'
  | 'epistemic_manifold_surface'
  | 'lean4_goal_tree_inspector'
  | 'shadow_tomography_radar'
  | 'cryptographic_mandate_card';

export interface GenerativeUIWidget {
  widget_id: string;
  widget_type: GenerativeUIWidgetType;
  title: string;
  summary: string;
  interactive: boolean;
  payload: Record<string, unknown>;
  allowed_actions: Array<{
    action_id: string;
    label: string;
    variant: 'primary' | 'secondary' | 'danger';
    requires_ed25519_signature?: boolean;
  }>;
}

export interface CryptographicMandate {
  mandate_id: string;
  signer_public_key: string;
  session_id: string;
  target_system: string;
  max_spend_usd: number;
  max_qpu_shots: number;
  allowed_backends: string[];
  axiom_whitelist: string[];
  expiration_iso: string;
  signature_hex: string;
  verified: boolean;
}

// -----------------------------------------------------------------------------
// Multimodal Mission Control & Hybrid Routing Types
// -----------------------------------------------------------------------------

export type ModalityType =
  | 'text'
  | 'image'
  | 'molecule_3d'
  | 'circuit_qir'
  | 'spectroscopy'
  | 'audio'
  | 'lean4_ast';

export interface MultimodalItem {
  modality: ModalityType;
  data: string;
  mime_type?: string;
  metadata?: Record<string, unknown>;
}

export interface MultimodalPayload {
  prompt: string;
  system_prompt?: string;
  items?: MultimodalItem[];
  role_hint?: string;
  preferred_model?: string;
  max_tokens?: number;
  temperature?: number;
  spend_budget_usd?: number;
  has_ed25519_mandate?: boolean;
}

export interface RoutingDecision {
  provider: string;
  model: string;
  is_local: boolean;
  air_gap_enforced: boolean;
  estimated_cost_usd: number;
  explanation: string;
  modalities_supported: string[];
}

export interface CustomAgentManifest {
  id: string;
  name: string;
  role: string;
  description: string;
  system_prompt: string;
  avatar_emoji: string;
  badge_color: string;
  capabilities: string[];
  modalities: string[];
  preferred_model: string;
  mcp_tools: string[];
  spend_budget_usd: number;
  memory_scopes: string[];
  is_system_agent?: boolean;
  created_at?: string;
  updated_at?: string;
}

// -----------------------------------------------------------------------------
// Hugging Face Model Hub & In-Platform Downloader Types
// -----------------------------------------------------------------------------

export interface HFModelQuantization {
  filename: string;
  size_bytes?: number;
  quant_type: string;
  download_url: string;
}

export interface HFModelSummary {
  id: string;
  author: string;
  downloads: number;
  likes: number;
  pipeline_tag: string;
  tags: string[];
  quantizations: HFModelQuantization[];
  description?: string;
}

export interface HFDownloadTask {
  task_id: string;
  repo_id: string;
  filename: string;
  status: 'pending' | 'downloading' | 'completed' | 'failed' | 'cancelled';
  bytes_downloaded: number;
  total_bytes: number;
  progress_percent: number;
  speed_mb_s: number;
  local_path: string;
  error?: string;
  sha256?: string;
  started_at: number;
  completed_at?: number;
}

