import { describe, it, expect, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act, type ReactTestInstance } from 'react-test-renderer';
import { SovereignDock } from './SovereignDock';
import { ArenaHud } from './ArenaHud';
import { SwarmUniverse } from './SwarmUniverse';
import { ActionGate } from './ActionGate';
import { WidgetStage } from './WidgetStage';
import { ProofDagView } from './ProofDagView';
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

  it('shows the run status pill while the swarm is working', () => {
    const root = render(
      <ArenaHud
        targetLabel="H2"
        swarmDetail="0 Loops Live"
        view="orbital"
        onViewChange={() => {}}
        spendUsd={0}
        spendCapUsd={150}
        sessionElapsed="00:00:01"
        statusText="reasoning · 12s"
        onRecenter={() => {}}
      />
    );
    expect(textOf(root.findByProps({ 'data-testid': 'hud-status' }))).toContain('reasoning · 12s');
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

  it('lights the active probe and leaves the rest dark', () => {
    const root = render(<SwarmUniverse campaign={CAMPAIGN} activeProbe="compiler" />);
    expect(root.findByProps({ 'data-testid': 'probe-compiler' }).props['data-active']).toBe('true');
    expect(root.findByProps({ 'data-testid': 'probe-theorist' }).props['data-active']).toBe(undefined);
  });

  it('rests when idle: no sweep, rest note shown', () => {
    const root = render(<SwarmUniverse campaign={CAMPAIGN} atRest />);
    expect(root.findByProps({ 'data-testid': 'swarm-universe' }).props['data-rest']).toBe('true');
    expect(textOf(root.findByProps({ 'data-testid': 'universe-rest-note' }))).toContain('idle');
  });

  it('animates when working: no rest markers', () => {
    const root = render(<SwarmUniverse campaign={CAMPAIGN} atRest={false} />);
    expect(root.findByProps({ 'data-testid': 'swarm-universe' }).props['data-rest']).toBe(undefined);
    expect(root.findAllByProps({ 'data-testid': 'universe-rest-note' }).length).toBe(0);
  });
});

describe('ActionGate', () => {
  it('shows the authorize affordance with live cost and backend', () => {
    const root = render(
      <ActionGate campaign={CAMPAIGN} session={null} onAuthorized={() => {}} onDenied={() => {}} onDowngrade={() => {}} />
    );
    root.findByProps({ 'data-testid': 'action-gate-authorize' });
    root.findByProps({ 'data-testid': 'action-gate-downgrade' });
    const flat = textOf(root.findByProps({ 'data-testid': 'action-gate' }));
    expect(flat).toContain('48.50');
    expect(flat).toContain('QuEra Aquila');
  });
});

describe('WidgetStage', () => {
  const base = {
    campaign: CAMPAIGN,
    gateOpen: true,
    onDismiss: () => {},
    onRequestGateFocus: () => {},
  };

  it('stays empty by default (zero clutter)', () => {
    const root = render(<WidgetStage widget={null} {...base} />);
    expect(root.findAllByProps({ 'data-testid': 'widget-stage' }).length).toBe(0);
  });

  it('materializes one widget with title and dismisses on demand', () => {
    const onDismiss = vi.fn();
    const root = render(
      <WidgetStage widget={{ kind: 'circuit', title: 'Circuit · live' }} {...base} onDismiss={onDismiss} />
    );
    const stage = root.findByProps({ 'data-testid': 'widget-stage' });
    expect(textOf(stage)).toContain('Circuit · live');
    expect(textOf(stage)).toContain('LIVE');
    act(() => {
      root.findByProps({ 'data-testid': 'widget-dismiss' }).props.onClick();
    });
    expect(onDismiss).toHaveBeenCalled();
  });

  it('spend card shows live figures and is read-only once the gate has decided', () => {
    const decided = render(
      <WidgetStage widget={{ kind: 'mandate', title: 'Spend mandate' }} {...base} gateOpen={false} />
    );
    const flat = textOf(decided.findByProps({ 'data-testid': 'widget-stage' }));
    expect(flat).toContain('read-only');
    expect(flat).toContain('48.50');
  });

  it('minimizes to the header bar and expands back', () => {
    const root = render(
      <WidgetStage widget={{ kind: 'circuit', title: 'Circuit · live' }} {...base} onDismiss={() => {}} />
    );
    const stage = root.findByProps({ 'data-testid': 'widget-stage' });
    expect(stage.props['data-minimized']).toBe(undefined);
    act(() => {
      root.findByProps({ 'data-testid': 'widget-minimize' }).props.onClick();
    });
    expect(root.findByProps({ 'data-testid': 'widget-stage' }).props['data-minimized']).toBe('true');
  });

  it('marks static reference content REF, live feeds LIVE', () => {
    const shadows = render(
      <WidgetStage widget={{ kind: 'shadows', title: 'Shadow estimates' }} {...base} onDismiss={() => {}} />
    );
    expect(textOf(shadows.findByProps({ 'data-testid': 'widget-stage' }))).toContain('REF');
    const circuit = render(
      <WidgetStage widget={{ kind: 'circuit', title: 'Circuit · live' }} {...base} onDismiss={() => {}} />
    );
    expect(textOf(circuit.findByProps({ 'data-testid': 'widget-stage' }))).toContain('LIVE');
  });

  it('spend card defers the decision to the Action Gate (never signs itself)', () => {    const onRequestGateFocus = vi.fn();
    const root = render(
      <WidgetStage
        widget={{ kind: 'mandate', title: 'Spend mandate' }}
        {...base}
        gateOpen
        onRequestGateFocus={onRequestGateFocus}
      />
    );
    const review = root.findAllByType('button').find((b) => textOf(b).includes('Review at Action Gate'));
    expect(review).toBeDefined();
    act(() => {
      (review as ReactTestInstance).props.onClick();
    });
    expect(onRequestGateFocus).toHaveBeenCalled();
  });
});

describe('ProofDagView', () => {
  const dagCampaign = {
    ...CAMPAIGN,
    quantum_factors: {
      ...CAMPAIGN.quantum_factors,
      dag_nodes: [
        { id: 'q0-in', data: { type: 'Qubit Wire', label: 'q[0]: |0>' } },
        { id: 'rz-q0', data: { type: 'Param Rotation', label: '[ Rz(θ1) ]', angle: 0.785 } },
      ],
      dag_edges: [{ id: 'e1', source: 'q0-in', target: 'rz-q0' }],
    },
  } as unknown as CampaignState;

  it('lays out nodes by layer and draws the real edge set', () => {
    const root = render(<ProofDagView campaign={dagCampaign} />);
    expect(root.findByProps({ 'data-testid': 'proof-dag' })).toBeDefined();
    const svg = root.findByType('svg');
    expect(textOf(svg)).toContain('Rz');
    expect(svg.findAllByType('path').length).toBe(1);
  });

  it('drops dangling edges and stays honest when empty', () => {
    const broken = {
      ...dagCampaign,
      quantum_factors: {
        ...dagCampaign.quantum_factors,
        dag_edges: [{ id: 'ghost', source: 'nope', target: 'rz-q0' }],
      },
    } as unknown as CampaignState;
    const root = render(<ProofDagView campaign={broken} />);
    expect(root.findByType('svg').findAllByType('path').length).toBe(0);
    const empty = render(<ProofDagView campaign={CAMPAIGN} />);
    expect(textOf(empty.findByType(ProofDagView))).toContain('No proof DAG');
  });

  it('selecting a node traces its record', () => {
    const root = render(<ProofDagView campaign={dagCampaign} />);
    const groups = root.findAll(
      (n) => typeof n.type === 'string' && n.type === 'g' && n.props.onClick
    );
    expect(groups.length).toBe(2);
    act(() => {
      groups[1].props.onClick();
    });
    expect(textOf(root.findByProps({ 'data-testid': 'proof-dag' }))).toContain('rz-q0');
  });
});

describe('ProofDagView wire cuts + compile receipt (C-1)', () => {
  const cutCampaign = {
    ...CAMPAIGN,
    quantum_factors: {
      ...CAMPAIGN.quantum_factors,
      qasm: 'OPENQASM 2.0;\nqreg q[2];',
      dag_nodes: [{ id: 'q0-in', data: { type: 'Qubit Wire', label: 'q[0]' } }],
      dag_edges: [],
      wire_cuts: [
        { planned: true, cut_type: 'KaHyPar QPD', overhead: '4.0x (estimate, not measured)' },
        { measured: true, job_id: 'abcdef123456', total_cuts: 2, fragments: [{ fragment: 0 }, { fragment: 1 }] },
      ],
    },
  } as unknown as CampaignState;

  it('labels planned cuts planned and measured cuts measured', () => {
    const root = render(<ProofDagView campaign={cutCampaign} />);
    const text = textOf(root.findByProps({ 'data-testid': 'proof-dag' }));
    expect(text).toContain('planned cut (not executed)');
    expect(text).toContain('measured: 2 cuts');
    expect(text).toContain('2 fragments');
    expect(text).toContain('abcdef12');
  });

  it('compiles via backend and shows the receipt honestly', async () => {
    vi.stubGlobal(
      'fetch',
      async (url: string) => {
        expect(String(url)).toContain('/api/qir/compile');
        return {
          ok: true,
          json: async () => ({
            num_qubits: 2,
            depth: 4,
            gate_count: 6,
            t_count: 0,
            qir_profile: 'BaseProfile-Estimate',
            llvm_ir: '; ModuleID',
            compiler_used: 'go-fallback-estimate',
            fallback_estimate: true,
          }),
        };
      }
    );
    try {
      const root = render(<ProofDagView campaign={cutCampaign} />);
      const toggles = root.findAllByType('button');
      const compileToggle = toggles.find((b) => textOf(b).includes('Compile receipt'));
      expect(compileToggle).toBeTruthy();
      act(() => {
        compileToggle!.props.onClick();
      });
      expect(textOf(root.findByProps({ 'data-testid': 'proof-dag' }))).toContain('OPENQASM 2.0');
      const compileBtn = root
        .findAllByType('button')
        .find((b) => textOf(b).includes('Compile via backend'));
      expect(compileBtn).toBeTruthy();
      await act(async () => {
        await compileBtn!.props.onClick();
      });
      const text = textOf(root.findByProps({ 'data-testid': 'proof-dag' }));
      expect(text).toContain('go-fallback-estimate');
      expect(text).toContain('ESTIMATE (not compiler-verified)');
      expect(text).toContain('; ModuleID');
    } finally {
      vi.unstubAllGlobals();
    }
  });

  it('says so when compile refuses', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: false }));
    try {
      const root = render(<ProofDagView campaign={cutCampaign} />);
      const toggles = root.findAllByType('button');
      act(() => {
        toggles.find((b) => textOf(b).includes('Compile receipt'))!.props.onClick();
      });
      const compileBtn = root
        .findAllByType('button')
        .find((b) => textOf(b).includes('Compile via backend'));
      await act(async () => {
        await compileBtn!.props.onClick();
      });
      expect(textOf(root.findByProps({ 'data-testid': 'proof-dag' }))).toContain('Compile refused');
    } finally {
      vi.unstubAllGlobals();
    }
  });
});
