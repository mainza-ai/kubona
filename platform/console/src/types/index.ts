export interface Session {
  id: string;
  title: string;
  tenant_id?: string;
  created_at: string;
  updated_at: string;
  status: string;
  // Spend caps are backend-omitted when the session has no priced policy;
  // consumers must treat absence as unknown, never as zero.
  spend_limit_usd?: number;
  current_spend_usd?: number;
}

// Canvas run-state: what the swarm is doing right now (Phase 1 awareness).
// Every phase has a canvas manifestation; idle is the only silent state.
export type RunPhase = 'idle' | 'thinking' | 'tribunal';

export type ProbeId = 'theorist' | 'formalist' | 'compiler' | 'critic';

export interface Lab {
  id: string;
  display_name: string;
  autonomy_level: string;
  // Backend may omit spend figures for unmetered labs; absence means
  // unknown/unbounded, never zero.
  spend_ceiling_usd?: number;
  spent_usd?: number;
  status: string;
  // Registered tenant-admin Ed25519 pubkey (hex). Empty/absent means no
  // admin is registered — the first SetAdminKey bootstraps it.
  admin_pubkey_hex?: string;
}

export interface LiteratureRecord {
  id: string;
  title: string;
  authors: string;
  abstract: string;
  url: string;
  publication_year: number;
  citation_count: number;
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
  designed_by?: string;
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
  // Model that served this reply (chat POST envelope). Shown under the
  // bubble so a stale selection is visible, never silent.
  model?: string;
  // Measured generation cost (envelope only; absent on history reloads).
  duration_ms?: number;
  usage?: {
    completion_tokens?: number;
    prompt_tokens?: number;
    total_tokens?: number;
  };
}

export interface Job {
  id: string;
  session_id: string;
  status: string;
  job_type: string;
  // The bridge may return params/result as a JSON string (large payloads)
  // or as a decoded object; consumers must handle both via asRecord().
  params_json: Record<string, unknown> | string;
  result_json?: Record<string, unknown> | string | null;
  created_at: string;
  completed_at?: string;
  compiler_used?: string;
  fallback_estimate?: boolean;
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
  anchor_computed?: boolean;
  anchor_source?: string;
}

export interface TrajectoryStep {
  step: number;
  energy: number;
  loss: number;
  theta: number[];
  grad_norm: number;
  watchdog: string;
  timestamp: string;
  source?: string;
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
  hypothesis_id?: string;
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

// Codesign scoring contracts (mirror Go pkg/codesign goldens + Python
// evaluate output; single source per layer, imported — never re-declared.
// Validity stays Python-owned: these are numbers served, never verdicts).
export interface CodesignTQResult {
  toffoli: number;
  qubits: number;
  kappa: number;
  tq: number;
  golden_check: boolean;
}

export interface CodesignFrontierEntry {
  toffoli: number;
  qubits: number;
  tq: number;
}

export interface CodesignFrontierResult {
  kappa: number;
  cells: number;
  best: CodesignFrontierEntry | null;
  entries: CodesignFrontierEntry[];
}

export interface CodesignBudgetEntry {
  source: string;
  per_use_rate: number;
  uses: number;
}

export interface CodesignBudgetRow extends CodesignBudgetEntry {
  contribution: number;
}

export interface CodesignBudgetResult {
  rows: CodesignBudgetRow[];
  union_bound: number;
}

export interface CodesignVerifiedComponent {
  name: string;
  toffoli: number;
  scope: string;
  pin: string;
}

export interface CodesignVerifiedResult {
  program: string;
  version: number;
  generated: string;
  generator: string;
  labels: string[];
  total: { toffoli: number; qubits: number; pin: string };
  components: CodesignVerifiedComponent[];
  proofs: string[];
}

export interface CodesignFamilyOffer {
  family: string;
  valid: boolean;
  toffoli?: number;
  qubits?: number;
  tq?: number;
  improved?: boolean;
}

export interface CodesignFamiliesResult {
  spec_kind: string;
  kappa: number;
  offers: CodesignFamilyOffer[];
  elites: Record<string, { toffoli: number; qubits: number; tq: number; artifact: unknown }>;
  stagnation: Record<string, number>;
  lineage_len: number;
  lineage: Array<Record<string, unknown>>;
  shares: Array<{ offers: number; improvements: number; details: Array<Record<string, unknown>> }>;
  widen_nudge: { family: string; offers_since_improvement: number; nudge: string };
}

export interface CodesignPack {
  name: string;
  version: string;
  anchor_required: boolean;
  evidence_reqs: string[];
  status: string;
  anchor_ref: string | null;
  proof_ref: string | null;
}

export interface CodesignPacksResult {
  order: string[];
  packs: CodesignPack[];
}

// C5b governed proposer rerun (BYOK estimate; no QPU-cap debit).
export interface CodesignRerunTrial {
  step: number;
  valid: boolean;
  toffoli: number;
  peak_qubits: number;
  tq: number;
  eps_bound_97_5?: number;
  reason: string;
  candidate: string;
}

export interface CodesignRerunResult {
  ok: true;
  run_id: string;
  session_id: string;
  spec_kind: string;
  budget_steps: number;
  seed: number;
  num_tests: number;
  max_failures: number;
  kappa: number;
  provider: string;
  model: string;
  billing: string;
  payer: string;
  estimated_usd: number;
  budget_usd: number;
  steps: number;
  trials: CodesignRerunTrial[];
  best: CodesignRerunTrial | null;
  stopped_reason?: string;
  provenance_status: string;
  ledger_ref?: unknown;
}

// Cost summary (read-only LLM spend aggregates; heuristic, never billing).
export interface CostSummary {
  rate_source?: string;
  rate_version?: number;
  effective_date?: string;
  rate_heuristic?: boolean;
  cap_usd: number;
  spent_usd: number;
  remaining_usd: number | null;
  unmetered_count: number;
  by_provider: Record<string, number>;
  active_provider: string;
  local_bypass: boolean;
  heuristic: boolean;
  currency: string;
  track_only: boolean;
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
  | 'sentinel_spend_governor'
  | 'ansatz_scout'
  | 'disconfirmation_officer'
  | 'device_auditor'
  | 'epistemic_scribe';

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
  // False = refusal (no model, air-gap block, unfunded cloud). Absent on
  // older daemons — treat missing as routed.
  ok?: boolean;
}

export interface CustomAgentManifest {
  id: string;
  name: string;
  role: string;
  description: string;
  system_prompt: string;
  avatar_emoji: string;
  badge_color: string;
  mode?: 'prompt' | 'pipeline';
  steps?: Array<{ id: string; tool: string; args?: Record<string, unknown>; cost_usd?: number; requires_approval?: boolean }>;
  capabilities: string[];
  modalities: string[];
  preferred_model: string;
  mcp_tools: string[];
  spend_budget_usd: number;
  memory_scopes: string[];
  scopes?: string[];
  created_by?: string | null;
  is_system_agent?: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface WorkflowStep {
  id: string;
  tool: string;
  args?: Record<string, unknown>;
  cost_usd?: number;
  requires_approval?: boolean;
}

export interface WorkflowDefinition {
  name: string;
  trigger?: 'manual' | 'schedule' | 'event';
  version?: number;
  budget_ceiling_usd?: number | null;
  steps: WorkflowStep[];
  on_failure?: WorkflowStep[];
  created_by_agent?: boolean;
  approved?: boolean;
}

export interface WorkflowPlanResponse {
  ok: boolean;
  dry_run: boolean;
  workflow_id: string;
  steps: WorkflowStep[];
  unresolved_references: string[];
  compensations_planned: WorkflowStep[];
  error?: string;
}

export interface WorkflowRunResponse {
  ok: boolean;
  workflow_id: string;
  steps_completed: number;
  step_records: string[];
  spent_usd: number;
  last_results: Record<string, unknown>;
  compensations: Array<{ id: string; ok: boolean; result?: unknown; error?: string }>;
  error?: string;
}

// -----------------------------------------------------------------------------
// Shared fleet / radar wire types (single source of truth — FleetRadar and
// CognitiveMesh import these instead of redeclaring).
// -----------------------------------------------------------------------------

export interface RadarEndpoint {
  port: number;
  listening: boolean;
  service: string;
  sovereign: boolean;
  is_local_ip: boolean;
}

export interface BackendInfo {
  name: string;
  provider?: string;
  type: string;
  // Null/absent means the daemon did not measure it. Never render a
  // fallback number for these fields.
  qubits?: number | null;
  t1_us?: string | null;
  avg_t1_us?: number | null;
  avg_t2_us?: number | null;
  status: string;
  // Credential dimension (fleet): 'ready' (credential-free sims or token
  // present) vs 'credentialless' (device may be fine — WE lack credentials).
  // Absent on older daemons: treat as unknown, never as ready.
  credential_state?: string;
  // Telemetry provenance (fleet): 'measured' fields were read by the
  // daemon (liveness dial, env presence); 'declared' fields are
  // vendor-catalog placeholders until the live canary proxy lands.
  // Absent on older daemons: treat queue/coherence as unprovenanced.
  telemetry?: string;
  catalog_verified?: string | null;
  queue_depth?: number | null;
  cost_per_shot?: number | null;
  jurisdiction?: string | null;
  itar_ear?: string | null;
  credential_env?: string;
  executable_now?: boolean;
  dispatch_ready?: boolean;
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

