import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { SpatialCanvas } from './SpatialCanvas';
import type { CampaignState } from '@/types';

afterEach(() => {
  vi.unstubAllGlobals();
});

vi.mock('@/lib/api', () => ({
  fetchMolecularADMET: vi.fn().mockImplementation(async (smiles: string) => {
    return {
      smiles,
      molecular_weight: 7.949,
      clogp: -2.88,
      tpsa: 0.0,
      synthesizability: 'High (Synthesizable)',
      lipinski: { pass: true, violations: 0 },
    };
  }),
  // P3 vault predictions: unsealed by default so the ghost reads illustrative.
  fetchLatestPrediction: vi.fn().mockImplementation(async () => ({ ok: true, sealed: null, evaluation: null })),
  sealPrediction: vi.fn(),
  evaluatePrediction: vi.fn(),
  requestObservatoryLens: vi.fn(),
  fetchObservatoryLensStatus: vi.fn(),
}));

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

describe('SpatialCanvas', () => {
  it('renders physical molecular system with bond distance and ADMET profile in Depth 1', async () => {
    const campaign = {
      target_system: 'LiH',
      quantum_factors: {
        target_molecule: 'LiH',
        molecular_formula: 'LiH',
        bond_distance: 1.5957,
      },
      classical_anchor: {
        anchor_computed: true,
        fci_exact_energy: -7.882,
        rhf_mean_field_energy: -7.863,
      },
    } as unknown as CampaignState;

    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<SpatialCanvas campaign={campaign} />);
    });

    const text = textOf(tree!.toJSON());
    expect(text).toContain('Lithium Hydride (LiH)');
    expect(text).toContain('Nuclear Core');
    expect(text).toContain('Molecular Pharmacology & ADMET');
    expect(text).toContain('7.95');
    expect(text).toContain('Da');
    expect(text).toContain('High');
  });

  it('allows switching progressive disclosure to Depth 2 (Quantum Circuit) and Depth 3 (Integrals)', async () => {
    const campaign = {
      target_system: 'H2',
      classical_anchor: {
        anchor_computed: true,
        fci_exact_energy: -1.137,
      },
    } as unknown as CampaignState;

    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<SpatialCanvas campaign={campaign} />);
    });

    // Find Depth 2 button
    const buttons = tree!.root.findAllByType('button');
    const depth2Btn = buttons.find((b) => textOf(b.props.children).includes('2. Quantum Circuit'));
    expect(depth2Btn).toBeDefined();

    await act(async () => {
      depth2Btn!.props.onClick();
    });

    let text = textOf(tree!.toJSON());
    expect(text).toContain('QUBITS');
    expect(text).toContain('OpenQASM');

    // Find Depth 3 button after rerender
    const depth3Btn = tree!.root
      .findAllByType('button')
      .find((b) => textOf(b.props.children).includes('3. Exact Integrals'));
    expect(depth3Btn).toBeDefined();

    await act(async () => {
      depth3Btn!.props.onClick();
    });

    text = textOf(tree!.toJSON());
    expect(text).toContain('Full Configuration Interaction (FCI) Benchmark');
    expect(text).toContain('Second-Quantized Hamiltonian Mapping');
  });

  it('switches between 2D Workbench and 3D Observatory representation mode', async () => {
    const campaign = {
      target_system: 'H2',
      classical_anchor: {
        anchor_computed: true,
        fci_exact_energy: -1.137,
      },
      epistemics: {
        p_correlated: 0.92,
      },
    } as unknown as CampaignState;

    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<SpatialCanvas campaign={campaign} />);
    });

    // Verify 2D Workbench is initially active
    let text = textOf(tree!.toJSON());
    expect(text).toContain('2D Workbench');
    expect(text).toContain('3D Observatory');
    expect(text).toContain('1. Physical System');

    // Click 3D Observatory mode button
    const buttons = tree!.root.findAllByType('button');
    const observatoryBtn = buttons.find((b) => textOf(b.props.children).includes('3D Observatory'));
    expect(observatoryBtn).toBeDefined();

    await act(async () => {
      observatoryBtn!.props.onClick();
    });

    text = textOf(tree!.toJSON());
    expect(text).toContain('Potential Energy Surface');
    expect(text).toContain('World Labs Atlas Lens');
    expect(text).toContain('Vault Ghost');
    expect(text).toContain('ρ = 0.92');
  });

  it('renders the Vault pre-registration panel in Observatory mode', async () => {
    const campaign = {
      session_id: 'sess-vault-test',
      target_system: 'H2',
      classical_anchor: {
        anchor_computed: true,
        fci_exact_energy: -1.137,
      },
      epistemics: {
        p_correlated: 0.92,
      },
    } as unknown as CampaignState;

    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<SpatialCanvas campaign={campaign} />);
    });

    const buttons = tree!.root.findAllByType('button');
    const observatoryBtn = buttons.find((b) => textOf(b.props.children).includes('3D Observatory'));
    await act(async () => {
      observatoryBtn!.props.onClick();
    });

    const text = textOf(tree!.toJSON());
    // P3: ledger-bound pre-registration surface (no sealed prediction in
    // this fixture, so the ghost reads illustrative and sealing is offered).
    expect(text).toContain('Vault Pre-registration');
    expect(text).toContain('No sealed prediction');
    expect(text).toContain('Seal');
  });
});
