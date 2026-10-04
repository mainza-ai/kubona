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

export interface ChatMessage {
  id: string;
  session_id: string;
  role: 'user' | 'assistant' | 'system' | string;
  content: string;
  proposal_json?: Proposal;
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
  dag_nodes: any[];
  dag_edges: any[];
  wire_cuts: any[];
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
  admet?: any;
  notebook: SystematicNotebook;
  updated_at: string;
}
