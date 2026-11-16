import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { GenerativeUIStudio } from './GenerativeUIStudio';
import { CampaignState } from '@/types';

// Surface 9 in a real browser: the header, the widget gallery and the filter
// pills are real DOM, so the tab interaction is a real click rather than a
// synthetic onClick call.
const mockCampaign: CampaignState = {
  session_id: 'sess-test-01',
  research_goal: 'Lithium Hydride equilibrium state VQE',
  target_system: 'Lithium Hydride (LiH)',
  smiles: '[Li][H]',
  quantum_factors: {
    target_molecule: 'LiH',
    molecular_formula: 'LiH',
    bond_distance: 1.5957,
    active_electrons: 4,
    active_orbitals: 4,
    fermion_mapping: 'Parity',
    qubit_count: 4,
    circuit_depth: 8,
    two_qubit_gates: 3,
    ansatz_family: 'TwoLocal',
    meyer_wallach_q: 0.76,
    gradient_variance: 0.0021,
    barren_plateau_risk: 'LOW',
    clifford_fidelity: 0.984,
    recommended_backend: 'local_simulator',
    estimated_spend_usd: 0.0,
    qasm: 'OPENQASM 2.0;',
    dag_nodes: [],
    dag_edges: [],
    wire_cuts: [],
  },
  classical_anchor: {
    method: 'PySCF STO-3G FCI',
    rhf_mean_field_energy: -7.862,
    fci_exact_energy: -7.88239,
    vqe_expectation_energy: -7.88192,
    discrepancy_mha: 0.47,
    chemical_accuracy_passed: true,
    chemical_accuracy_limit: 1.6,
    anchor_computed: true,
    anchor_source: 'pyscf-live',
  },
  trajectory: [],
  branches: [],
  epistemics: {
    alpha_corroborated: 0.95,
    beta_contested: 0.05,
    gamma_unverified: 0.1,
    p_correlated: 90.25,
    status: 'PROJECTED',
    literature_drift_mha: 0.47,
    statement: 'LiH test statement',
    sources: [],
  },
  notebook: {
    id: 'nb-1',
    title: 'LiH Notebook',
    created_at: '2026-10-18T00:00:00Z',
    classical_anchor: 'PySCF FCI',
    quantum_energy: -7.88192,
    classical_energy: -7.88239,
    delta_mha: 0.47,
    chemical_accuracy: true,
    provenance_hash: 'sha256:70a69dd84d5bfcf47',
    epistemic_verdict: 'PROJECTION',
    sections: [],
  },
  updated_at: '2026-10-18T00:00:00Z',
};

describe('GenerativeUIStudio (Surface 9) in a real browser', () => {
  it('renders Surface 9 header and all 6 widgets when tab is all', async () => {
    const screen = await render(<GenerativeUIStudio campaign={mockCampaign} />);

    const text = screen.container.textContent ?? '';
    expect(text).toContain('Surface 9 · Scientific Widgets');
    expect(text).toContain('Generative Scientific UI Studio');
    expect(text).toContain('Molecular Active Space');
    expect(text).toContain('Post-QIR Quantum Circuit');
    expect(text).toContain('Lean 4 Goal Tree Inspector');
    expect(text).toContain('Classical Shadow Tomography');
    expect(text).toContain('Riemannian Epistemic Manifold Surface');
    expect(text).toContain('AP2 High-Friction Spend Mandate Card');
    expect(text).not.toContain('14.8x');
    expect(text).not.toContain('99.2%');
    expect(text).not.toContain('4.2x');
  });

  it('filters to individual widget when tab is selected', async () => {
    const screen = await render(<GenerativeUIStudio campaign={mockCampaign} />);

    await screen.getByRole('button', { name: '3D Active Space' }).click();

    await expect.poll(() => screen.container.textContent).toContain('Molecular Active Space');
    expect(screen.container.textContent).not.toContain('Classical Shadow Tomography');
    // The other widget panels are unmounted, not merely hidden.
    expect(screen.container.querySelectorAll('h3')).toHaveLength(1);
  });

  it('renders synthetic demo notice when campaign is null', async () => {
    const screen = await render(<GenerativeUIStudio campaign={null} />);

    const text = screen.container.textContent ?? '';
    expect(text).toContain('No measured campaign data for the widget gallery.');
    expect(text).not.toContain('synthetic demo');
  });
});
