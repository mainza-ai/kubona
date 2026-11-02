import { afterEach, describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import type { CampaignState } from '@/types';
import { Workbench, type WorkbenchHandle } from './Workbench';

vi.mock('@/lib/api', () => ({
  fetchCanvasLayout: vi.fn(async () => ({})),
  fetchWorkbenchLayout: vi.fn(async () => null),
  fetchOpenObligations: vi.fn(async () => ({ ok: true, obligations: [] })),
  saveWorkbenchLayout: vi.fn(async () => ({ ok: true, revision: 1 })),
}));

afterEach(() => {
  vi.clearAllMocks();
});

const campaign = {
  session_id: 's1',
  research_goal: 'Find the ground state',
  target_system: 'H2',
  quantum_factors: {
    target_molecule: 'H2',
    molecular_formula: 'H2',
    bond_distance: 0.7414,
    active_electrons: 2,
    active_orbitals: 2,
    fermion_mapping: 'jordan_wigner',
    qubit_count: 4,
    circuit_depth: 6,
    two_qubit_gates: 3,
    ansatz_family: 'UCCSD',
    meyer_wallach_q: 0.1,
    gradient_variance: 0.01,
    barren_plateau_risk: 'low',
    clifford_fidelity: 0.99,
    recommended_backend: 'local_simulator',
    estimated_spend_usd: 0,
    qasm: '',
    dag_nodes: [],
    dag_edges: [],
    wire_cuts: [],
  },
  classical_anchor: {
    method: 'PySCF FCI',
    rhf_mean_field_energy: -1.11,
    fci_exact_energy: -1.13727,
    vqe_expectation_energy: -1.13,
    discrepancy_mha: 7.27,
    chemical_accuracy_passed: false,
    chemical_accuracy_limit: 1.6,
    anchor_computed: true,
  },
  trajectory: [{ step: 0, energy: -1.1, loss: 0.1, theta: [], grad_norm: 0.1, watchdog: '', timestamp: '' }],
  branches: [],
  epistemics: {
    alpha_corroborated: 0.8,
    beta_contested: 0.1,
    gamma_unverified: 0.1,
    p_correlated: 0.7,
    status: 'COLLAPSED_VERIFIED',
    literature_drift_mha: 0.2,
    statement: 'The ground state is within chemical accuracy.',
    sources: ['ledger:1'],
  },
  notebook: {
    id: 'n1',
    title: 'Hydrogen notebook',
    created_at: '',
    classical_anchor: 'PySCF FCI',
    quantum_energy: -1.13,
    classical_energy: -1.13727,
    delta_mha: 7.27,
    chemical_accuracy: false,
    provenance_hash: 'sha256:abc',
    epistemic_verdict: 'VERIFIED',
    sections: ['Abstract', 'Results'],
  },
  updated_at: '',
} as unknown as CampaignState;

describe('Workbench', () => {
  it('shows an explicit empty state without a campaign', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<Workbench campaign={null} />);
    });
    expect(tree!.root.findByProps({ 'data-testid': 'workbench' })).toBeTruthy();
    expect(tree!.root.findAllByProps({ className: 'k-workbench-card' })).toHaveLength(0);
    expect(tree!.root.findByProps({ className: 'k-workbench-empty' })).toBeTruthy();
  });

  it('shows a campaign read refusal instead of an empty campaign', async () => {
    const onRetry = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<Workbench campaign={null} error="campaign read refused" onRetry={onRetry} />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('Campaign unavailable');
    expect(text).toContain('campaign read refused');
    act(() => {
      tree!.root.findByProps({ 'data-testid': 'retry-campaign' }).props.onClick();
    });
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it('projects the campaign into typed cards and a figure', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<Workbench campaign={campaign} sessionId="s1" tasks={[{
        id: 't1', session_id: 's1', title: 'Review anchor', status: 'open',
        source_domain: 'measurement', source_id: 'm1', created_at: '', updated_at: '',
      }]} />);
    });
    const cards = tree!.root.findAllByProps({ className: 'k-workbench-card' });
    expect(cards.length).toBe(12);
    expect(cards.map((card) => card.props['data-kind'])).toEqual(
      expect.arrayContaining(['campaign', 'hypothesis', 'circuit', 'measurement', 'chart', 'table', 'note', 'molecule', 'task']),
    );
    expect(tree!.root.findAllByProps({ 'data-testid': 'k-chart' }).length).toBe(1);
  });

  it('completes a task inline and shows recorded session spend', async () => {
    const onTaskStatus = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <Workbench
          campaign={campaign}
          sessionId="s1"
          sessionSpendUsd={4.18}
          onTaskStatus={onTaskStatus}
          tasks={[{
            id: 't1', session_id: 's1', title: 'Review anchor', status: 'open',
            source_domain: 'measurement', source_id: 'm1', created_at: '', updated_at: '',
          }]}
        />,
      );
    });
    expect(JSON.stringify(tree!.toJSON())).toContain('4.18');
    const task = tree!.root.findAllByProps({ 'data-kind': 'task' })[0];
    await act(async () => { task.props.onClick({ metaKey: false, ctrlKey: false, shiftKey: false }); });
    const done = tree!.root.findAllByType('button').find((button) => button.props.children === 'Mark done');
    act(() => { done!.props.onClick({ stopPropagation: () => {} }); });
    expect(onTaskStatus).toHaveBeenCalledWith('t1', 'done');
  });

  it('closes a close-ready approval from the card', async () => {
    const onCloseObligation = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <Workbench
          campaign={campaign}
          sessionId="s1"
          onCloseObligation={onCloseObligation}
          obligations={[{
            id: 'ob-1234567890', subject_kind: 'job', subject_id: 'job-1', owner: 'operator',
            disposition: 'open', required_cases: [], required_authorities: [], close_ready: true, close_reasons: [],
          }]}
        />,
      );
    });
    const approval = tree!.root.findAllByProps({ 'data-kind': 'approval' })[0];
    act(() => { approval.props.onClick({ metaKey: false, ctrlKey: false, shiftKey: false }); });
    const close = tree!.root.findByProps({ 'data-testid': 'close-approval' });
    act(() => { close!.props.onClick({ stopPropagation: () => {} }); });
    expect(onCloseObligation).toHaveBeenCalledWith('ob-1234567890');
  });

  it('offers a list equivalent of the canvas', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<Workbench campaign={campaign} sessionId="s1" />);
    });
    const toggle = tree!.root.findByProps({ title: 'Switch between canvas and list (text equivalent)' });
    await act(async () => { toggle.props.onClick(); });
    expect(tree!.root.findByProps({ className: 'k-workbench-list' })).toBeTruthy();
    expect(tree!.root.findAllByType('table')).toHaveLength(1);
  });

  it('projects a running loop as an agent-run card', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <Workbench
          campaign={campaign}
          sessionId="s1"
          loops={[{ id: 'loop-123456789', session_id: 's1', status: 'running', step_count: 3, max_steps: 10, diverged: false, created_at: '', updated_at: '' }]}
        />,
      );
    });
    const runs = tree!.root.findAllByProps({ 'data-kind': 'agent_run' });
    expect(runs.length).toBe(1);
    expect(JSON.stringify(tree!.toJSON())).toContain('3 / 10');
  });

  it('renames an area and assigns the selected card to it', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<Workbench campaign={campaign} sessionId="s1" />);
    });
    const group = tree!.root.findByProps({ title: 'Group all objects into a spatial area' });
    await act(async () => { group.props.onClick(); });
    const title = tree!.root.findByProps({ 'aria-label': 'Area title' });
    await act(async () => { title.props.onChange({ target: { value: 'Evidence chain' } }); });
    expect(tree!.root.findByProps({ className: 'k-workbench-area' }).findAllByType('span')[0].props.children).toBe('Evidence chain');

    const card = tree!.root.findAllByProps({ 'data-kind': 'campaign' })[0];
    await act(async () => { card.props.onClick(); });
    const assign = tree!.root.findAllByType('button').find((button) => button.props.children === 'Assign to area');
    await act(async () => { assign!.props.onClick(); });
    expect(JSON.stringify(tree!.toJSON())).toContain('area');
  });

  it('projects a job as a job card', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <Workbench
          campaign={campaign}
          sessionId="s1"
          jobs={[{ id: 'job-123456789', session_id: 's1', status: 2, job_type: 'vqe', params_json: {}, created_at: '2026-11-01T00:00:00Z' }]}
        />,
      );
    });
    expect(tree!.root.findAllByProps({ 'data-kind': 'job' }).length).toBe(1);
  });

  it('announces the selected object to the shell inspector', async () => {
    const onSelectObject = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<Workbench campaign={campaign} sessionId="s1" onSelectObject={onSelectObject} />);
    });
    const card = tree!.root.findAllByProps({ 'data-kind': 'campaign' })[0];
    await act(async () => { card.props.onClick(); });
    expect(onSelectObject).toHaveBeenCalledWith(expect.objectContaining({ id: 'campaign' }));
  });

  it('removes a selected card from the canvas and restores it without deleting the record', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<Workbench campaign={campaign} sessionId="s1" />);
    });
    const card = tree!.root.findAllByProps({ 'data-kind': 'campaign' })[0];
    await act(async () => { card.props.onClick(); });
    const remove = tree!.root.findAllByType('button').find((button) => button.props.children === 'Remove');
    await act(async () => { remove!.props.onClick(); });
    expect(tree!.root.findAllByProps({ 'data-kind': 'campaign' })).toHaveLength(0);
    const undo = tree!.root.findAllByType('button').find((button) => button.props.children === 'Undo remove');
    await act(async () => { undo!.props.onClick(); });
    expect(tree!.root.findAllByProps({ 'data-kind': 'campaign' })).toHaveLength(1);
  });

  it('multi-selects cards and removes/restores the group', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<Workbench campaign={campaign} sessionId="s1" />);
    });
    const cards = tree!.root.findAllByProps({ 'data-kind': 'campaign' });
    const second = tree!.root.findAllByProps({ 'data-kind': 'hypothesis' })[0];
    await act(async () => { cards[0].props.onClick({ metaKey: false, ctrlKey: false, shiftKey: false }); });
    await act(async () => { second.props.onClick({ metaKey: true, ctrlKey: false, shiftKey: false }); });
    expect(tree!.root.findAll((node) => node.props?.['data-selected'] === 'true')).toHaveLength(2);
    const remove = tree!.root.findAllByType('button').find((button) => button.props.children === 'Remove (2)');
    await act(async () => { remove!.props.onClick(); });
    expect(tree!.root.findAllByProps({ 'data-kind': 'campaign' })).toHaveLength(0);
    expect(tree!.root.findAllByProps({ 'data-kind': 'hypothesis' })).toHaveLength(0);
  });

  it('creates separate areas from all objects and the current selection', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<Workbench campaign={campaign} sessionId="s1" />); });
    const groupAll = tree!.root.findByProps({ title: 'Group all objects into a spatial area' });
    await act(async () => { groupAll.props.onClick(); });
    const card = tree!.root.findAllByProps({ 'data-kind': 'campaign' })[0];
    await act(async () => { card.props.onClick({ metaKey: false, ctrlKey: false, shiftKey: false }); });
    const groupSelected = tree!.root.findByProps({ title: 'Group the selected cards into a spatial area' });
    await act(async () => { groupSelected.props.onClick(); });
    expect(tree!.root.findAllByProps({ className: 'k-workbench-area' })).toHaveLength(2);
  });

  it('creates and removes a typed custom link between two selected cards', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<Workbench campaign={campaign} sessionId="s1" />); });
    const circuit = tree!.root.findAllByProps({ 'data-kind': 'circuit' })[0];
    const evidence = tree!.root.findAllByProps({ 'data-kind': 'evidence' })[0];
    await act(async () => { circuit.props.onClick({ metaKey: false, ctrlKey: false, shiftKey: false }); });
    await act(async () => { evidence.props.onClick({ metaKey: true, ctrlKey: false, shiftKey: false }); });
    const link = tree!.root.findAllByType('button').find((button) => button.props.children === 'Link');
    await act(async () => { link!.props.onClick(); });
    const toggle = tree!.root.findByProps({ title: 'Switch between canvas and list (text equivalent)' });
    await act(async () => { toggle.props.onClick(); });
    const linksSection = tree!.root.findByProps({ 'aria-label': 'Workbench links' });
    const linkText = (button: TestRenderer.ReactTestInstance) => button.children.map((child) => typeof child === 'string' ? child : '').join('');
    const isCustomLink = (button: TestRenderer.ReactTestInstance) => {
      const text = linkText(button);
      return text.includes('Circuit') && text.includes('Evidence') && text.includes('supports');
    };
    const removeLink = linksSection.findAllByType('button').find(isCustomLink);
    expect(removeLink).toBeDefined();
    await act(async () => { removeLink!.props.onClick(); });
    expect(linksSection.findAllByType('button').some(isCustomLink)).toBe(false);
  });

  it('exposes focus and remove actions to the inspector', async () => {
    const ref = React.createRef<WorkbenchHandle>();
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<Workbench ref={ref} campaign={campaign} sessionId="s1" />); });
    await act(async () => { ref.current!.focusObject('campaign'); });
    await act(async () => { ref.current!.removeObject('campaign'); });
    expect(tree!.root.findAllByProps({ 'data-kind': 'campaign' })).toHaveLength(0);
  });

  it('groups all objects into a persisted spatial area', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<Workbench campaign={campaign} sessionId="s1" />);
    });
    const group = tree!.root.findByProps({ title: 'Group all objects into a spatial area' });
    await act(async () => { group.props.onClick(); });
    expect(tree!.root.findByProps({ className: 'k-workbench-area' })).toBeTruthy();
  });

  it('selects, opens, and nudges a card from the keyboard', async () => {
    const onSelectObject = vi.fn();
    const onOpenObject = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <Workbench campaign={campaign} sessionId="s1" onSelectObject={onSelectObject} onOpenObject={onOpenObject} />,
      );
    });
    const card = () => tree!.root.findAllByProps({ className: 'k-workbench-card' })
      .find((node) => node.props['data-kind'] === 'campaign')!;
    await act(async () => {
      card().props.onKeyDown({ key: ' ', preventDefault: vi.fn(), metaKey: false, ctrlKey: false, shiftKey: false });
    });
    expect(card().props['data-selected']).toBe('true');
    expect(onSelectObject).toHaveBeenCalledWith(expect.objectContaining({ id: 'campaign' }));
    await act(async () => {
      card().props.onKeyDown({ key: 'Enter', preventDefault: vi.fn(), metaKey: false, ctrlKey: false, shiftKey: false });
    });
    expect(onOpenObject).toHaveBeenCalledWith('campaign');
    const before = card().props.style.left;
    await act(async () => {
      card().props.onKeyDown({ key: 'ArrowRight', preventDefault: vi.fn(), metaKey: false, ctrlKey: false, shiftKey: false });
    });
    expect(card().props.style.left).toBe(before + 10);
  }, 15000);

  it('removes a selected card with Delete and can undo', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<Workbench campaign={campaign} sessionId="s1" />); });
    const card = () => tree!.root.findAllByProps({ className: 'k-workbench-card' })
      .find((node) => node.props['data-kind'] === 'campaign')!;
    await act(async () => {
      card().props.onKeyDown({ key: ' ', preventDefault: vi.fn(), metaKey: false, ctrlKey: false, shiftKey: false });
    });
    const workbench = tree!.root.findByProps({ 'data-testid': 'workbench' });
    await act(async () => {
      workbench.props.onKeyDown({ key: 'Delete', preventDefault: vi.fn(), target: { tagName: 'DIV', isContentEditable: false } });
    });
    expect(tree!.root.findAllByProps({ 'data-kind': 'campaign' })).toHaveLength(0);
    const undo = tree!.root.findByProps({ title: 'Restore the last removed card' });
    await act(async () => { undo.props.onClick(); });
    expect(tree!.root.findAllByProps({ 'data-kind': 'campaign' })).toHaveLength(1);
  });
});
