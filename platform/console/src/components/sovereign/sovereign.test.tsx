import { describe, it, expect, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act, type ReactTestInstance } from 'react-test-renderer';
import { SovereignDock } from './SovereignDock';
import { ArenaHud } from './ArenaHud';
import { SwarmUniverse } from './SwarmUniverse';
import { ActionGate } from './ActionGate';
import type { CampaignState } from '@/types';

function render(el: React.ReactElement) {
  let tree: TestRenderer.ReactTestRenderer;
  act(() => {
    tree = TestRenderer.create(el);
  });
  return tree!.root;
}

function textOf(node: ReactTestInstance | string | number): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const children = (node as ReactTestInstance).children ?? [];
  return children.map((c) => textOf(c as string | number | ReactTestInstance)).join(' ');
}

const CAMPAIGN = {
  session_id: 'sess-1',
  research_goal: 'ground state',
  target_system: 'FeMoco [Fe7MoS9C]',
  quantum_factors: {
    target_molecule: 'FeMoco [Fe7MoS9C]',
    molecular_formula: 'Fe7MoS9C',
    bond_distance: 1.0,
    active_electrons: 113,
    active_orbitals: 76,
    fermion_mapping: 'jordan-wigner',
    qubit_count: 48,
    circuit_depth: 120,
    two_qubit_gates: 1420,
    ansatz_family: 'UCCSD',
    meyer_wallach_q: 0.87,
    gradient_variance: 2.4e-7,
    barren_plateau_risk: 'low',
    clifford_fidelity: 0.998,
    recommended_backend: 'QuEra Aquila neutral atoms',
    estimated_spend_usd: 48.5,
    qasm: '',
    dag_nodes: [],
    dag_edges: [],
    wire_cuts: [],
  },
  classical_anchor: {
    method: 'pyscf',
    rhf_mean_field_energy: -1980.0,
    fci_exact_energy: -1984.0,
    vqe_expectation_energy: -1984.3418,
    discrepancy_mha: 1.2,
    chemical_accuracy_passed: true,
    chemical_accuracy_limit: 1.6,
  },
  trajectory: [],
  branches: [],
  epistemics: {
    alpha_corroborated: 3,
    beta_contested: 1,
    gamma_unverified: 0,
    p_correlated: 0.92,
    status: 'corroborated',
    literature_drift_mha: 0.4,
    statement: 'Ground state within chemical accuracy.',
    sources: ['a', 'b'],
  },
  notebook: {
    id: 'n1', title: 't', created_at: '', classical_anchor: '',
    quantum_energy: 0, classical_energy: 0, delta_mha: 0,
    chemical_accuracy: true, provenance_hash: '', epistemic_verdict: '', sections: [],
  },
  updated_at: '',
} as unknown as CampaignState;

describe('SovereignDock', () => {
  it('renders six surfaces and marks the active one', () => {
    const onSelect = vi.fn();
    const root = render(
      <SovereignDock active="fleet" onSelect={onSelect} airGapSovereign onOpenSettings={() => {}} onOpenCampaigns={() => {}} />
    );
    for (const id of ['arena', 'recorder', 'manifold', 'fleet', 'proofs', 'mesh']) {
      root.findByProps({ 'data-testid': `dock-${id}` });
    }
    expect(root.findByProps({ 'data-testid': 'dock-fleet' }).props['aria-pressed']).toBe(true);
    act(() => {
      root.findByProps({ 'data-testid': 'dock-arena' }).props.onClick();
    });
    expect(onSelect).toHaveBeenCalledWith('arena');
  });
});

describe('ArenaHud', () => {
  it('shows target, spend, elapsed and switches views', () => {
    const onViewChange = vi.fn();
    const root = render(
      <ArenaHud
        targetLabel="FeMoco · Ground State Target"
        swarmDetail="3 Loops Live"
        view="orbital"
        onViewChange={onViewChange}
        spendUsd={4.18}
        spendCapUsd={150}
        sessionElapsed="03:14:22"
        onRecenter={() => {}}
      />
    );
    const text = textOf(root.findByType('header'));
    expect(text).toContain('FeMoco');
    expect(text).toContain('03:14:22');
    act(() => {
      root.findByProps({ 'data-testid': 'arena-view-proof-dag' }).props.onClick();
    });
    expect(onViewChange).toHaveBeenCalledWith('proof-dag');
  });
});

describe('SwarmUniverse', () => {
  it('renders four probes and the live campaign energy', () => {
    const root = render(<SwarmUniverse campaign={CAMPAIGN} />);
    for (const probe of ['probe-theorist', 'probe-formalist', 'probe-compiler', 'probe-critic']) {
      root.findByProps({ 'data-testid': probe });
    }
    const energy = root.findByProps({ 'data-testid': 'universe-energy' });
    expect(textOf(energy)).toContain('-1984.3418');
  });

  it('renders placeholders without a campaign (never invented numbers)', () => {
    const root = render(<SwarmUniverse campaign={null} />);
    expect(textOf(root.findByProps({ 'data-testid': 'swarm-universe' }))).toContain('—');
  });
});

describe('ActionGate', () => {
  it('shows the authorize affordance with live cost and backend', () => {
    const root = render(
      <ActionGate campaign={CAMPAIGN} session={null} onAuthorized={() => {}} onDenied={() => {}} />
    );
    root.findByProps({ 'data-testid': 'action-gate-authorize' });
    const flat = textOf(root.findByProps({ 'data-testid': 'action-gate' }));
    expect(flat).toContain('48.50');
    expect(flat).toContain('QuEra Aquila');
  });
});
