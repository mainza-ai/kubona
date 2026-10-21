import { describe, it, expect, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act, type ReactTestInstance } from 'react-test-renderer';
import { SovereignDock } from './SovereignDock';
import { ArenaHud } from './ArenaHud';
import { SwarmUniverse } from './SwarmUniverse';
import { ActionGate } from './ActionGate';
import { canonicalJobBinding } from '@/lib/mandates';
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

  it('tap-to-open: probe cards toggle on tap and close on Escape', () => {
    const listeners: Record<string, Array<(e: unknown) => void>> = {};
    vi.stubGlobal('window', {
      addEventListener: (t: string, f: (e: unknown) => void) => {
        listeners[t] = [...(listeners[t] ?? []), f];
      },
      removeEventListener: (t: string, f: (e: unknown) => void) => {
        listeners[t] = (listeners[t] ?? []).filter((g) => g !== f);
      },
      dispatchEvent: (e: { type: string }) => {
        for (const f of listeners[e.type] ?? []) f(e);
        return true;
      },
    });
    try {
      let tree: TestRenderer.ReactTestRenderer;
      act(() => {
        tree = TestRenderer.create(<SwarmUniverse campaign={CAMPAIGN} />);
      });
      const root = tree!.root;
      const probe = root.findByProps({ 'data-testid': 'probe-theorist' });
      expect(probe.props['aria-expanded']).toBe(false);
      act(() => {
        (probe.props.onClick as () => void)();
      });
      expect(root.findByProps({ 'data-testid': 'probe-theorist' }).props['aria-expanded']).toBe(true);
      act(() => {
        for (const f of listeners['keydown'] ?? []) f({ key: 'Escape' });
      });
      expect(root.findByProps({ 'data-testid': 'probe-theorist' }).props['aria-expanded']).toBe(false);
      tree!.unmount();
    } finally {
      vi.unstubAllGlobals();
    }
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
    expect(textOf(stage)).toContain('MIX');
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
      <WidgetStage widget={{ kind: 'circuit', title: 'Circuit · mixed' }} {...base} onDismiss={() => {}} />
    );
    const circuitText = textOf(circuit.findByProps({ 'data-testid': 'widget-stage' }));
    expect(circuitText).toContain('MIX');
    expect(circuitText).not.toMatch(/\bLIVE\b/);
    const molecule = render(
      <WidgetStage widget={{ kind: 'molecule', title: 'Molecule' }} {...base} onDismiss={() => {}} />
    );
    expect(textOf(molecule.findByProps({ 'data-testid': 'widget-stage' }))).toContain('MIX');
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
    // Stronger contract: unmeasured cuts render known fields with a planned
    // prefix, never a bare generic chip.
    expect(text).toContain('planned: type KaHyPar QPD');
    expect(text).toContain('(not executed)');
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

describe('ProofDagView campaign signals (C-2)', () => {
  it('surfaces stuck, rescoped, and contested states as chips', () => {
    const signalCampaign = {
      ...CAMPAIGN,
      quantum_factors: {
        ...CAMPAIGN.quantum_factors,
        dag_nodes: [{ id: 'q0-in', data: { type: 'Qubit Wire', label: 'q[0]' } }],
        dag_edges: [],
      },
      trajectory: [
        { step: 15, energy: -1.0, loss: 0.09, theta: [], grad_norm: 4e-8, watchdog: 'barren_plateau_trip', timestamp: '' },
        { step: 18, energy: -1.0, loss: 0.045, theta: [], grad_norm: 0.04, watchdog: 'rescoped_rollback', timestamp: '' },
      ],
      branches: [{ id: 'fork-01', name: 'fork', step: 18, status: 'rescoped', delta: '1.2 mHa' }],
      epistemics: { ...CAMPAIGN.epistemics, status: 'CONTESTED' },
    } as unknown as CampaignState;
    const root = render(<ProofDagView campaign={signalCampaign} />);
    const text = textOf(root.findByProps({ 'data-testid': 'proof-dag' }));
    expect(text).toContain('stuck: step 15 (barren_plateau_trip)');
    expect(text).toContain('rescoped: step 18 (rescoped_rollback)');
    expect(text).toContain('branch fork-01: rescoped');
    expect(text).toContain('epistemics: CONTESTED');
  });

  it('stays quiet on clean campaigns', () => {
    const clean = {
      ...CAMPAIGN,
      quantum_factors: {
        ...CAMPAIGN.quantum_factors,
        dag_nodes: [{ id: 'q0-in', data: { type: 'Qubit Wire', label: 'q[0]' } }],
        dag_edges: [],
      },
      trajectory: [
        { step: 1, energy: -1.0, loss: 0.4, theta: [], grad_norm: 0.1, watchdog: 'normal', timestamp: '' },
      ],
      branches: [],
      epistemics: { ...CAMPAIGN.epistemics, status: 'COLLAPSED_VERIFIED' },
    } as unknown as CampaignState;
    const root = render(<ProofDagView campaign={clean} />);
    const text = textOf(root.findByProps({ 'data-testid': 'proof-dag' }));
    expect(text).not.toContain('stuck:');
    expect(text).not.toContain('epistemics: CONTESTED');
  });
});

describe('ActionGate tenant attribution (J4)', () => {
  it('authorizes against the lab tenant, never the session id', async () => {
    vi.unstubAllGlobals();
    let seenBody: { tenant_id?: string; job_hash?: string } | null = null;
    vi.stubGlobal(
      'fetch',
      async (_url: string, init: { body: string }) => {
        seenBody = JSON.parse(init.body);
        return { ok: true, json: async () => ({ valid: true }) };
      }
    );
    const session = {
      id: 'sess-tenant-test',
      title: 't',
      tenant_id: 'lab-alpha',
      created_at: '',
      updated_at: '',
      status: 'active',
      spend_limit_usd: 150,
      current_spend_usd: 0,
    };
    let tree: TestRenderer.ReactTestRenderer;
    // W0.4: the gate signs the exact program — the fixture carries QASM so
    // authorization can proceed to the tenant assertion below.
    const withProgram = {
      ...CAMPAIGN,
      quantum_factors: { ...CAMPAIGN.quantum_factors, qasm: 'OPENQASM 2.0;\nqreg q[1];\n' },
    };
    await act(async () => {
      tree = TestRenderer.create(
        <ActionGate campaign={withProgram} session={session} onAuthorized={() => {}} onDenied={() => {}} onDowngrade={() => {}} />
      );
    });
    const btn = tree!.root.findByProps({ 'data-testid': 'action-gate-authorize' });
    await act(async () => {
      await (btn.props as { onClick: () => Promise<void> }).onClick();
    });
    expect(seenBody).not.toBeNull();
    expect(seenBody!.tenant_id).toBe('lab-alpha');
    // The minted mandate binds the exact session + program (daemon-side
    // admission recomputes this and refuses anything else).
    expect(seenBody!.job_hash).toBe(canonicalJobBinding('sess-tenant-test', 'OPENQASM 2.0;\nqreg q[1];\n'));
  });
});

describe('ActionGate cost honesty', () => {
  it('disables authorization without a priced campaign', async () => {
    // Deliberately violates QuantumFactors (spend undefined): the gate
    // must refuse an unpriced campaign, so the fixture is invalid on
    // purpose — double-cast documents intent, not compatibility.
    const unpriced = {
      ...CAMPAIGN,
      quantum_factors: {
        ...CAMPAIGN.quantum_factors,
        estimated_spend_usd: undefined,
        qasm: 'OPENQASM 2.0;\nqreg q[1];\n',
      },
    } as unknown as CampaignState;
    const session = {
      id: 'sess-p', title: 't', tenant_id: 'lab-a', created_at: '', updated_at: '',
      status: 'active', spend_limit_usd: 150, current_spend_usd: 0,
    };
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <ActionGate campaign={unpriced} session={session} onAuthorized={() => {}} onDenied={() => {}} onDowngrade={() => {}} />
      );
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('unpriced — authorization disabled');
    expect(text).not.toContain('0.12 max');
    const btn = tree!.root.findByProps({ 'data-testid': 'action-gate-authorize' });
    expect((btn.props as { disabled?: boolean }).disabled).toBe(true);
    // Direct invocation (bypassing disabled) still refuses, never mints.
    await act(async () => {
      await (btn.props as { onClick: () => Promise<void> }).onClick();
    });
    expect(JSON.stringify(tree!.toJSON())).toContain('No cost estimate');
  });

  it('disables authorization without a proposed program (W0.4)', async () => {
    // Priced but program-less: the mandate would bind nothing, so the gate
    // refuses naming the program — never mints over an empty binding.
    const programless = {
      ...CAMPAIGN,
      quantum_factors: { ...CAMPAIGN.quantum_factors, qasm: '' },
    };
    const session = {
      id: 'sess-p', title: 't', tenant_id: 'lab-a', created_at: '', updated_at: '',
      status: 'active', spend_limit_usd: 150, current_spend_usd: 0,
    };
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <ActionGate campaign={programless} session={session} onAuthorized={() => {}} onDenied={() => {}} onDowngrade={() => {}} />
      );
    });
    const btn = tree!.root.findByProps({ 'data-testid': 'action-gate-authorize' });
    expect((btn.props as { disabled?: boolean }).disabled).toBe(true);
    await act(async () => {
      await (btn.props as { onClick: () => Promise<void> }).onClick();
    });
    expect(JSON.stringify(tree!.toJSON())).toContain('No program in this campaign');
  });
});
