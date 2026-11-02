import type { Page, Route } from '@playwright/test';

// Shared mocked 300+ object workbench fixtures for browser evidence.

export const BASE = process.env.KUBONA_CONSOLE_URL || 'http://localhost:3000';
export const LOOPS = 160;
export const JOBS = 160;
export const OBLIGATIONS = 32;

function session() {
  return {
    id: 'perf-session', title: 'Performance campaign', status: 'active',
    created_at: '2026-11-01T00:00:00Z', updated_at: '2026-11-01T00:00:00Z',
    spend_limit_usd: 100, current_spend_usd: 0,
  };
}

function campaign() {
  return {
    session_id: 'perf-session', research_goal: 'Perf campaign', target_system: 'H2',
    quantum_factors: {
      target_molecule: 'H2', molecular_formula: 'H2', bond_distance: 0.735,
      active_electrons: 2, active_orbitals: 2, fermion_mapping: 'jw', qubit_count: 4,
      circuit_depth: 12, two_qubit_gates: 8, ansatz_family: 'uccsd', meyer_wallach_q: 0.5,
      gradient_variance: 0.1, barren_plateau_risk: 'low', clifford_fidelity: 0.99,
      recommended_backend: 'local_simulator', estimated_spend_usd: 0, qasm: '',
      dag_nodes: [], dag_edges: [], wire_cuts: [],
    },
    classical_anchor: {
      method: 'FCI', rhf_mean_field_energy: -1.1, fci_exact_energy: -1.137,
      vqe_expectation_energy: -1.13, discrepancy_mha: 7, chemical_accuracy_passed: true,
      chemical_accuracy_limit: 1.6, anchor_computed: true, anchor_source: 'perf',
    },
    trajectory: [], branches: [],
    epistemics: {
      alpha_corroborated: 0.9, beta_contested: 0.1, gamma_unverified: 0.2,
      p_correlated: 0.8, status: 'corroborated', literature_drift_mha: 0.2,
      statement: 'H2 ground state', sources: [],
    },
    notebook: {
      id: 'nb', title: 'Perf notebook', created_at: '2026-11-01T00:00:00Z',
      classical_anchor: 'FCI', quantum_energy: -1.13, classical_energy: -1.137,
      delta_mha: 7, chemical_accuracy: true, provenance_hash: 'perf',
      epistemic_verdict: 'corroborated', sections: [],
    },
    updated_at: '2026-11-01T00:00:00Z',
  };
}

function loops(count = LOOPS) {
  return Array.from({ length: count }, (_, index) => ({
    id: `loop-${index.toString().padStart(3, '0')}`, session_id: 'perf-session',
    status: index % 7 === 0 ? 'paused' : 'running', step_count: index, max_steps: 50,
    diverged: index % 31 === 0, created_at: '2026-11-01T00:00:00Z', updated_at: '2026-11-01T00:00:00Z',
  }));
}

function jobs(count = JOBS) {
  return Array.from({ length: count }, (_, index) => ({
    id: `job-${index.toString().padStart(3, '0')}`, session_id: 'perf-session',
    status: 'COMPLETED', job_type: `perf-job-${index}`, params_json: '{}',
    result_json: '{}', created_at: '2026-11-01T00:00:00Z', completed_at: '2026-11-01T00:01:00Z',
    compiler_used: 'perf',
  }));
}

function obligations(count = OBLIGATIONS) {
  return Array.from({ length: count }, (_, index) => ({
    id: `obligation-${index.toString().padStart(3, '0')}`, subject_kind: 'job',
    subject_id: `job-${index.toString().padStart(3, '0')}`, owner: 'operator',
    disposition: 'open', required_cases: [], required_authorities: [],
    close_ready: false, close_reasons: [],
  }));
}

export async function mockApi(page: Page, counts: { loops?: number; jobs?: number; obligations?: number } = {}) {
  await page.route('**/api/**', async (route: Route) => {
    const url = new URL(route.request().url());
    const path = url.pathname;
    const json = (body: unknown) => route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
    if (path === '/api/config') return json({ apiUrl: BASE, workbench: true });
    if (path === '/api/sessions') return json({ sessions: [session()] });
    if (path === '/api/auth/whoami') return json({ authenticated: false });
    if (path === '/api/sessions/perf-session') return json(session());
    if (path === '/api/loops') return json({ loops: loops(counts.loops) });
    if (path === '/api/jobs') return json({ jobs: jobs(counts.jobs) });
    if (path === '/api/obligations/open') return json({ obligations: obligations(counts.obligations) });
    if (path === '/api/tasks') return json({ tasks: [] });
    if (path === '/api/events') return json({ events: [] });
    if (path === '/api/events/stream') return route.fulfill({ status: 204, body: '' });
    if (path === '/api/sessions/perf-session/campaign') return json(campaign());
    if (path === '/api/workbench/layout') {
      if (route.request().method() === 'PUT') return json({ status: 'saved', session_id: 'perf-session', revision: 1 });
      return json({ session_id: 'perf-session', revision: 0, document: {} });
    }
    if (path === '/api/canvas/layout') return json({});
    if (path === '/api/health' || path === '/api/ready') return json({ ok: true, status: 'ready', ready: true });
    if (path === '/api/cost/summary') return json({ cap_usd: 100, spent_usd: 0, remaining_usd: 100, unmetered_count: 0, by_provider: {}, active_provider: 'local', local_bypass: true, heuristic: false, currency: 'USD', track_only: false });
    if (path === '/api/llm/settings') return json({ active_provider: 'local', active_model: '', providers: {}, role_assignments: {}, air_gapped: false, disable_dsml_fallback: false, temperature: 0, max_tokens: 0 });
    if (path === '/api/llm/models') return json({ models: [] });
    if (path === '/api/quantum/backends') return json({ backends: [] });
    if (path === '/api/quantum/settings') return json({ default_backend: 'local-simulator', harness_backends: {} });

    return json({});
  });
}
