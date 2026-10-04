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
  molecule: string;
  bond_distance: number;
  recommended_tier: string;
  classical_anchor: string;
  spend_estimate: string;
  job_type: string;
  launch_params: Record<string, unknown>;
}

export interface ChatMessage {
  id: string;
  session_id: string;
  role: 'user' | 'assistant' | 'system';
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
