import { describe, it, expect } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { GenerativeUIStudio } from './GenerativeUIStudio';
import { CampaignState } from '@/types';

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(textOf).join(' ');
  if (node !== null && typeof node === 'object') {
    const obj = node as { children?: unknown; props?: { children?: unknown } };
    if ('children' in obj && obj.children != null) {
      return textOf(obj.children);
    }
    if ('props' in obj && obj.props?.children != null) {
      return textOf(obj.props.children);
    }
  }
  return '';
}

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

describe('GenerativeUIStudio (Surface 9)', () => {
  it('renders Surface 9 header and all 6 widgets when tab is all', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<GenerativeUIStudio campaign={mockCampaign} />);
    });

    const text = textOf(tree!.toJSON());
    expect(text).toContain('Surface 9 · Preview Gallery');
    expect(text).toContain('Generative Scientific UI Studio');
    expect(text).toContain('3D Molecular Active Space Viewer');
    expect(text).toContain('Post-QIR Quantum Circuit Editor');
    expect(text).toContain('Lean 4 Goal Tree Inspector');
    expect(text).toContain('Classical Shadow Tomography Radar');
    expect(text).toContain('Riemannian Epistemic Manifold Surface');
    expect(text).toContain('AP2 High-Friction Spend Mandate Card');
  });

  it('filters to individual widget when tab is selected', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<GenerativeUIStudio campaign={mockCampaign} />);
    });

    // Find tab button for "3D Active Space"
    const buttons = tree!.root.findAllByType('button');
    const molTab = buttons.find((b) => textOf(b.props.children).includes('3D Active Space'));
    expect(molTab).toBeDefined();

    await act(async () => {
      molTab!.props.onClick();
    });

    const text = textOf(tree!.toJSON());
    expect(text).toContain('3D Molecular Active Space Viewer');
    expect(text).not.toContain('Classical Shadow Tomography Radar');
  });

  it('renders synthetic demo notice when campaign is null', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<GenerativeUIStudio campaign={null} />);
    });

    const text = textOf(tree!.toJSON());
    expect(text).toContain('synthetic demo — not session data');
  });
});
