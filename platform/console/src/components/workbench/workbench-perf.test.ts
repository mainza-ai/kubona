import { describe, expect, it } from 'vitest';
import type { CampaignState, Job, Loop } from '@/types';
import type { ObligationRow } from '@/lib/api';
import { buildWorkbench, workbenchDocument } from './workbench-model';

function campaign(): CampaignState {
  return {
    session_id: 'perf-session',
    research_goal: 'Performance ceiling',
    target_system: 'H2',
    hypothesis_id: 'hyp-perf',
    quantum_factors: {
      target_molecule: 'H2', molecular_formula: 'H2', bond_distance: 0.7414,
      active_electrons: 2, active_orbitals: 2, fermion_mapping: 'jordan_wigner',
      qubit_count: 4, circuit_depth: 6, two_qubit_gates: 3, ansatz_family: 'UCCSD',
      meyer_wallach_q: 0.1, gradient_variance: 0.01, barren_plateau_risk: 'low',
      clifford_fidelity: 0.99, recommended_backend: 'local_simulator',
      estimated_spend_usd: 0, qasm: '', dag_nodes: [], dag_edges: [], wire_cuts: [],
    },
    classical_anchor: {
      method: 'PySCF FCI', rhf_mean_field_energy: 0, fci_exact_energy: -1.13727,
      vqe_expectation_energy: -1.13, discrepancy_mha: 7.27,
      chemical_accuracy_passed: false, chemical_accuracy_limit: 1.6, anchor_computed: true,
    },
    trajectory: [], branches: [],
    epistemics: {
      alpha_corroborated: 0.8, beta_contested: 0.1, gamma_unverified: 0.1,
      p_correlated: 0.7, status: 'PROJECTED', literature_drift_mha: 0,
      statement: 'perf', sources: [],
    },
    notebook: {
      id: 'n-perf', title: 'perf', created_at: '', classical_anchor: '',
      quantum_energy: 0, classical_energy: 0, delta_mha: 0,
      chemical_accuracy: false, provenance_hash: '', epistemic_verdict: '', sections: [],
    },
    updated_at: '',
  } as unknown as CampaignState;
}

const loops: Loop[] = Array.from({ length: 160 }, (_, index) => ({
  id: `loop-${index}`, session_id: 'perf-session', status: 'running',
  step_count: index, max_steps: 100, diverged: false, created_at: '', updated_at: '',
}));
const jobs: Job[] = Array.from({ length: 160 }, (_, index) => ({
  id: `job-${index}`, session_id: 'perf-session', status: 2, job_type: 'vqe',
  params_json: {}, created_at: '',
}));
const obligations: ObligationRow[] = Array.from({ length: 32 }, (_, index) => ({
  id: `ob-${index}`, subject_kind: 'spend', subject_id: `spend-${index}`,
  owner: 'lab', disposition: 'REVIEW', required_cases: [], required_authorities: [],
  close_ready: false, close_reasons: [],
}));

describe('workbench projection ceiling', () => {
  it('projects and serializes 300+ objects within the transition budget', () => {
    const samples: number[] = [];
    let count = 0;
    for (let run = 0; run < 5; run += 1) {
      const started = performance.now();
      const objects = buildWorkbench(campaign(), loops, jobs, obligations);
      workbenchDocument('perf-session', objects, {}, {}, { x: 0, y: 0, zoom: 1 });
      samples.push(performance.now() - started);
      count = objects.length;
    }
    samples.sort((a, b) => a - b);
    const median = samples[Math.floor(samples.length / 2)];
    expect(count).toBeGreaterThanOrEqual(300);
    // Projection/serialization budget, not a browser FPS claim.
    expect(median).toBeLessThan(250);
  });
});
