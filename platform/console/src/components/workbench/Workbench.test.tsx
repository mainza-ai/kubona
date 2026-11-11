import { afterEach, describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import type { CampaignState } from '@/types';
import { Workbench, type WorkbenchHandle } from './Workbench';

vi.mock('@/lib/api', () => ({
  fetchCanvasLayout: vi.fn(async () => ({})),
  fetchWorkbenchLayout: vi.fn(async () => null),
  fetchWorkbenchView: vi.fn(async () => null),
  fetchOpenObligations: vi.fn(async () => ({ ok: true, obligations: [] })),
  saveWorkbenchLayout: vi.fn(async () => ({ ok: true, revision: 1 })),
  saveWorkbenchView: vi.fn(async () => ({ ok: true, view_revision: 1 })),
}));

const mountedTrees: TestRenderer.ReactTestRenderer[] = [];
function createTree(element: React.ReactElement) {
  const rendered = TestRenderer.create(element);
  mountedTrees.push(rendered);
  return rendered;
}

afterEach(async () => {
  await act(async () => {
    while (mountedTrees.length > 0) mountedTrees.pop()!.unmount();
  });
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
      tree = createTree(<Workbench campaign={null} />);
    });
    expect(tree!.root.findByProps({ 'data-testid': 'workbench' })).toBeTruthy();
    expect(tree!.root.findAllByProps({ className: 'k-workbench-card' })).toHaveLength(0);
    expect(tree!.root.findByProps({ className: 'k-workbench-empty' })).toBeTruthy();
  });

  it('routes an empty multi-lab workspace to sign-in instead of campaign creation', async () => {
    const onSignIn = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = createTree(<Workbench campaign={null} multiLab onSignIn={onSignIn} />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('Multi-lab workspace');
    expect(tree!.root.findAllByProps({ 'data-testid': 'workbench-create-campaign' })).toHaveLength(0);
    act(() => { tree!.root.findByProps({ 'data-testid': 'workbench-sign-in' }).props.onClick(); });
    expect(onSignIn).toHaveBeenCalledTimes(1);
  });

  it('offers a first-campaign action and a sign-in action for auth refusals', async () => {
    const onCreateCampaign = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = createTree(
        <Workbench campaign={null} localMode onCreateCampaign={onCreateCampaign} />,
      );
    });
    const create = tree!.root.findByProps({ 'data-testid': 'workbench-create-campaign' });
    act(() => { create.props.onClick(); });
    expect(onCreateCampaign).toHaveBeenCalledTimes(1);

    const onSignIn = vi.fn();
    await act(async () => {
      tree = createTree(
        <Workbench campaign={null} error="multi-lab store: log in" authError onSignIn={onSignIn} />,
      );
    });
    expect(JSON.stringify(tree!.toJSON())).toContain('Sign in to open this campaign.');
    const signIn = tree!.root.findByProps({ 'data-testid': 'workbench-sign-in' });
    act(() => { signIn.props.onClick(); });
    expect(onSignIn).toHaveBeenCalledTimes(1);
  });

  it('shows a campaign read refusal instead of an empty campaign', async () => {
    const onRetry = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = createTree(<Workbench campaign={null} error="campaign read refused" onRetry={onRetry} />);
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
      tree = createTree(<Workbench campaign={campaign} sessionId="s1" tasks={[{
        id: 't1', session_id: 's1', title: 'Review anchor', status: 'open',
        source_domain: 'measurement', source_id: 'm1', created_at: '', updated_at: '',
      }]} />);
    });
    const cards = tree!.root.findAllByProps({ className: 'k-workbench-card' });
    expect(cards.length).toBe(12);
    expect(cards.map((card) => card.props['data-kind'])).toEqual(
      expect.arrayContaining(['campaign', 'hypothesis', 'circuit', 'measurement', 'chart', 'table', 'note', 'molecule', 'task']),
    );
    for (const card of cards) {
      expect(['measured', 'proposed', 'catalog', 'model']).toContain(card.props['data-provenance']);
    }
    const metrics = tree!.root.findAll((node) => node.props?.['data-metric'] !== undefined);
    expect(metrics.length).toBeGreaterThan(0);
    for (const metric of metrics) {
      let current: typeof metric | null = metric;
      let tagged = false;
      while (current) {
        if (typeof current.props?.['data-provenance'] === 'string') { tagged = true; break; }
        current = current.parent;
      }
      expect(tagged, 'workbench metric missing provenance').toBe(true);
    }
    expect(tree!.root.findAllByProps({ 'data-testid': 'k-chart' }).length).toBe(1);
  });

  it('completes a task inline and shows recorded session spend', async () => {
    const onTaskStatus = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = createTree(
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
      tree = createTree(
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
      tree = createTree(<Workbench campaign={campaign} sessionId="s1" />);
    });
    const toggle = tree!.root.findByProps({ title: 'Switch between canvas and list (text equivalent)' });
    await act(async () => { toggle.props.onClick(); });
    expect(tree!.root.findByProps({ className: 'k-workbench-list' })).toBeTruthy();
    expect(tree!.root.findAllByType('table')).toHaveLength(1);
    const rows = tree!.root.findAllByType('tr').filter((row) => row.props['data-provenance']);
    expect(rows.length).toBeGreaterThan(0);
    for (const row of rows) {
      expect(['measured', 'proposed', 'catalog', 'model']).toContain(row.props['data-provenance']);
    }
  });

  it('projects a running loop as an agent-run card', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = createTree(
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
      tree = createTree(<Workbench campaign={campaign} sessionId="s1" />);
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
      tree = createTree(
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
      tree = createTree(<Workbench campaign={campaign} sessionId="s1" onSelectObject={onSelectObject} />);
    });
    const card = tree!.root.findAllByProps({ 'data-kind': 'campaign' })[0];
    await act(async () => { card.props.onClick(); });
    expect(onSelectObject).toHaveBeenCalledWith(expect.objectContaining({ id: 'campaign' }));
  });

  it('removes a selected card from the canvas and restores it without deleting the record', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = createTree(<Workbench campaign={campaign} sessionId="s1" />);
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
      tree = createTree(<Workbench campaign={campaign} sessionId="s1" />);
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
    await act(async () => { tree = createTree(<Workbench campaign={campaign} sessionId="s1" />); });
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
    await act(async () => { tree = createTree(<Workbench campaign={campaign} sessionId="s1" />); });
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
    await act(async () => { tree = createTree(<Workbench ref={ref} campaign={campaign} sessionId="s1" />); });
    await act(async () => { ref.current!.focusObject('campaign'); });
    await act(async () => { ref.current!.removeObject('campaign'); });
    expect(tree!.root.findAllByProps({ 'data-kind': 'campaign' })).toHaveLength(0);
  });

  it('groups all objects into a persisted spatial area', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = createTree(<Workbench campaign={campaign} sessionId="s1" />);
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
      tree = createTree(
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
    await act(async () => { tree = createTree(<Workbench campaign={campaign} sessionId="s1" />); });
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

  it('saves camera pan/zoom to view channel without mutating semantic layout', async () => {
    vi.useFakeTimers();
    try {
      const api = await import('@/lib/api');
      let tree: TestRenderer.ReactTestRenderer;
      await act(async () => {
        tree = createTree(<Workbench campaign={campaign} sessionId="s1" />);
        await Promise.resolve();
        await Promise.resolve();
        await Promise.resolve();
      });
      const zoomIn = tree!.root.findByProps({ 'aria-label': 'Zoom in' });
      await act(async () => {
        zoomIn.props.onClick();
      });
      await act(async () => {
        await vi.advanceTimersByTimeAsync(500);
      });
      expect(api.saveWorkbenchView).toHaveBeenCalled();
      expect(api.saveWorkbenchLayout).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  it('suppresses Unavailable placeholder cards during initial hydration window', async () => {
    vi.useFakeTimers();
    try {
      const api = await import('@/lib/api');
      vi.mocked(api.fetchWorkbenchLayout).mockResolvedValueOnce({
        revision: 2,
        document: {
          version: 2,
          session_id: 's1',
          objects: [{ id: 'missing-loop', kind: 'agent_run', ref: { domain: 'loop', id: 'l-missing' }, position: { x: 0, y: 0 }, pinned: true }],
          links: [],
          areas: [],
          hidden: [],
        },
      });
      let tree: TestRenderer.ReactTestRenderer;
      await act(async () => {
        tree = createTree(<Workbench campaign={campaign} sessionId="s1" />);
        await Promise.resolve();
        await Promise.resolve();
      });
      expect(JSON.stringify(tree!.toJSON())).not.toContain('Unavailable');
      await act(async () => {
        await vi.advanceTimersByTimeAsync(300);
      });
      expect(JSON.stringify(tree!.toJSON())).toContain('Unavailable');
    } finally {
      vi.useRealTimers();
    }
  });

  it('cycles edge visibility (hybrid -> all -> off -> focus) and routes orthogonal edges capped at 6 in focus mode', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = createTree(<Workbench campaign={campaign} sessionId="s1" />);
    });
    const edgeToggle = () => tree!.root.findAllByType('button').find((b) => String(b.props.children?.[0] ?? b.props.children).includes('Edges'))!;
    expect(edgeToggle()).toBeDefined();

    // Default 'hybrid' renders primary spine links even at rest (no card selected).
    const restPaths = tree!.root.findAllByProps({ className: 'k-workbench-links' })[0].findAllByType('path');
    expect(restPaths.length).toBeGreaterThan(0);
    expect(restPaths.every((p) => p.props['data-edge-role'] === 'rest')).toBe(true);

    const card = tree!.root.findAllByProps({ 'data-kind': 'campaign' })[0];
    await act(async () => { card.props.onClick({ metaKey: false, ctrlKey: false, shiftKey: false }); });
    const hybridSelectedPaths = tree!.root.findAllByProps({ className: 'k-workbench-links' })[0].findAllByType('path');
    expect(hybridSelectedPaths.some((p) => p.props['data-edge-role'] === 'focus')).toBe(true);

    await act(async () => { edgeToggle().props.onClick(); }); // All
    const allPaths = tree!.root.findAllByProps({ className: 'k-workbench-links' })[0].findAllByType('path');
    expect(allPaths.length).toBeGreaterThanOrEqual(hybridSelectedPaths.length);

    await act(async () => { edgeToggle().props.onClick(); }); // Off
    const offPaths = tree!.root.findAllByProps({ className: 'k-workbench-links' })[0].findAllByType('path');
    expect(offPaths).toHaveLength(0);

    await act(async () => { edgeToggle().props.onClick(); }); // Focus (max 6)
    const focusedPaths = tree!.root.findAllByProps({ className: 'k-workbench-links' })[0].findAllByType('path');
    expect(focusedPaths.length).toBeGreaterThan(0);
    expect(focusedPaths.length).toBeLessThanOrEqual(6);
    expect(focusedPaths[0].props.d).not.toContain(' C');
    expect(focusedPaths.every((p) => p.props['data-edge-role'] === 'focus')).toBe(true);
  });

  it('renders a floating SelectionBar when cards are selected and supports multi-card Align + Undo', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = createTree(<Workbench campaign={campaign} sessionId="s1" />);
    });
    const cardA = tree!.root.findAllByProps({ 'data-kind': 'campaign' })[0];
    const cardB = tree!.root.findAllByProps({ 'data-kind': 'circuit' })[0];
    const beforeLeftB = cardB.props.style.left;

    await act(async () => { cardA.props.onClick({ metaKey: false, ctrlKey: false, shiftKey: false }); });
    await act(async () => { cardB.props.onClick({ metaKey: true, ctrlKey: false, shiftKey: false }); });

    const alignBtn = tree!.root.findAllByType('button').find((b) => b.props.children === 'Align');
    expect(alignBtn).toBeDefined();
    await act(async () => { alignBtn!.props.onClick(); });

    const alignedB = tree!.root.findAllByProps({ 'data-kind': 'circuit' })[0];
    expect(alignedB.props.style.left).toBe(cardA.props.style.left);

    const workbench = tree!.root.findByProps({ 'data-testid': 'workbench' });
    await act(async () => {
      workbench.props.onKeyDown({ key: 'z', preventDefault: vi.fn(), target: { tagName: 'DIV', isContentEditable: false } });
    });
    const revertedB = tree!.root.findAllByProps({ 'data-kind': 'circuit' })[0];
    expect(revertedB.props.style.left).toBe(beforeLeftB);
  });

  it('caps CodeBlock at 16 lines with expand toggle and supports CompareMatrix CSV + validated MANUAL overrides + Undo', async () => {
    const longQasm = Array.from({ length: 20 }, (_, i) => `rz(0.${i + 1}) q[0];`).join('\n');
    const richCampaign: CampaignState = {
      ...campaign,
      quantum_factors: {
        ...campaign.quantum_factors!,
        qasm: longQasm,
      },
    };
    const writeText = vi.fn<(text: string) => Promise<void>>(async () => undefined);
    Object.defineProperty(globalThis.navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    });

    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = createTree(
        <Workbench
          campaign={richCampaign}
          jobs={[
            { id: 'job-1', session_id: 's1', job_type: 'vqe', status: 2, params_json: { backend: 'local_simulator', shots: 1024 }, result_json: { energy: -1.13 }, created_at: '2026-11-01T00:00:00Z' },
            { id: 'job-2', session_id: 's1', job_type: 'vqe', status: 2, params_json: { backend: 'ibm_marrakesh', shots: 4096 }, result_json: { energy: -1.11 }, created_at: '2026-11-01T00:01:00Z' },
          ]}
          sessionId="s1"
        />,
      );
    });

    // CodeBlock 16-line cap + expand/collapse
    const circuitCard = tree!.root.findAllByProps({ 'data-kind': 'circuit' })[0];
    expect(circuitCard.findAllByProps({ className: 'k-workbench-code-line' })).toHaveLength(16);
    const expandBtn = circuitCard.findByProps({ className: 'k-workbench-code-more' });
    await act(async () => { expandBtn.props.onClick(); });
    expect(circuitCard.findAllByProps({ className: 'k-workbench-code-line' })).toHaveLength(20);

    // CompareMatrix Copy CSV (lives on the Column 3 table card when classical_anchor is recorded)
    const compareCard = tree!.root.findAllByProps({ 'data-kind': 'table' })[0];
    expect(compareCard).toBeDefined();
    const csvBtn = compareCard.findAllByType('button').find((b) => b.props.children === 'Copy CSV');
    expect(csvBtn).toBeDefined();
    await act(async () => { csvBtn!.props.onClick(); });
    expect(writeText).toHaveBeenCalledTimes(1);
    expect(writeText.mock.calls[0]![0]).toContain('RHF mean-field');

    // CompareMatrix MANUAL override validation (> 8192 bytes rejected) + valid save preserving MEASURED orig + Undo
    const editBtn = compareCard.findAllByProps({ className: 'k-compare-edit-btn' })[0];
    await act(async () => { editBtn.props.onClick(); });
    const input = compareCard.findByType('input');
    const saveBtn = () => compareCard.findAllByType('button').find((b) => b.props.children === 'Save')!;
    await act(async () => { input.props.onChange({ target: { value: 'x'.repeat(8193) } }); });
    await act(async () => { saveBtn().props.onClick(); });
    const alert = compareCard.findByProps({ role: 'alert' });
    expect(alert.props.children).toContain('8192');

    await act(async () => { input.props.onChange({ target: { value: '-1.14500 Ha' } }); });
    await act(async () => { saveBtn().props.onClick(); });
    expect(compareCard.findByProps({ className: 'k-compare-manual-badge' }).props.children).toBe('MANUAL');
    expect(compareCard.findByProps({ className: 'k-compare-orig' }).props.children.join('')).toContain('orig ');

    const workbench = tree!.root.findByProps({ 'data-testid': 'workbench' });
    await act(async () => {
      workbench.props.onKeyDown({ key: 'z', preventDefault: vi.fn(), target: { tagName: 'DIV', isContentEditable: false } });
    });
    expect(compareCard.findAllByProps({ className: 'k-compare-manual-badge' })).toHaveLength(0);
  });

  it('filters cards with recovery empty state, renders inline ConfirmCard + AskReceipt, minimap, Follow live, Lift, and held state', async () => {
    const onInlineGateVisibility = vi.fn();
    const onDowngrade = vi.fn();
    const onGateSettled = vi.fn();
    const onLiftObject = vi.fn();
    const meteredCampaign: CampaignState = {
      ...campaign,
      quantum_factors: {
        ...campaign.quantum_factors!,
        recommended_backend: 'ibm_marrakesh',
        estimated_spend_usd: 3.2,
        qasm: 'OPENQASM 2.0;\nqreg q[2];\nh q[0];',
      },
    };
    const gate = {
      campaign: meteredCampaign,
      session: { id: 's1', target_system: 'H2', spend_limit_usd: 25, current_spend_usd: 1.5, status: 'active' } as never,
      onAuthorized: vi.fn(),
      onDenied: vi.fn(),
      onDowngrade,
    };

    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = createTree(
        <Workbench
          campaign={meteredCampaign}
          sessionId="s1"
          loops={[{ id: 'l1', session_id: 's1', status: 'running', step_count: 1, max_steps: 4, diverged: false, created_at: '', updated_at: '' }]}
          confirmGate={gate}
          onInlineGateVisibility={onInlineGateVisibility}
          onGateSettled={onGateSettled}
          onLiftObject={onLiftObject}
        />,
      );
    });

    // Single-home guarantee: circuit card owns the gate while on Canvas view.
    expect(onInlineGateVisibility).toHaveBeenCalledWith(true);
    expect(tree!.root.findByProps({ 'data-testid': 'confirm-card' })).toBeTruthy();

    // Downgrade inside the inline ConfirmCard emits AskReceipt + calls onDowngrade.
    const downgradeBtn = tree!.root.findByProps({ 'data-testid': 'confirm-card-downgrade' });
    await act(async () => { downgradeBtn.props.onClick(); });
    expect(onDowngrade).toHaveBeenCalledTimes(1);
    expect(onGateSettled).toHaveBeenCalledWith(expect.objectContaining({ outcome: 'downgraded' }));

    // Even when confirmGate clears after settlement, AskReceipt stays rendered on the circuit card.
    await act(async () => {
      tree!.update(
        <Workbench
          campaign={meteredCampaign}
          sessionId="s1"
          loops={[{ id: 'l1', session_id: 's1', status: 'running', step_count: 1, max_steps: 4, diverged: false, created_at: '', updated_at: '' }]}
          confirmGate={null}
          onInlineGateVisibility={onInlineGateVisibility}
          onGateSettled={onGateSettled}
          onLiftObject={onLiftObject}
        />,
      );
    });
    expect(tree!.root.findByProps({ 'data-testid': 'ask-receipt' })).toBeTruthy();
    expect(onInlineGateVisibility).toHaveBeenCalledWith(false);

    // Filter chips: Jobs shows only job/agent_run cards; Spine hides circuit -> wait no, Spine includes circuit; Issues with zero issues offers "Show all cards".
    const jobsFilter = tree!.root.findByProps({ 'data-filter': 'jobs' });
    await act(async () => { jobsFilter.props.onClick(); });
    expect(tree!.root.findAllByProps({ className: 'k-workbench-card' }).every((n) => n.props['data-kind'] === 'agent_run' || n.props['data-kind'] === 'job')).toBe(true);

    const allFilter = tree!.root.findByProps({ 'data-filter': 'all' });
    await act(async () => { allFilter.props.onClick(); });

    // Corner minimap + Follow live toggle on canvas view.
    expect(tree!.root.findByProps({ 'data-testid': 'workbench-minimap' })).toBeTruthy();
    const followBtn = tree!.root.findByProps({ 'data-testid': 'workbench-follow-live' });
    await act(async () => { followBtn.props.onClick(); });
    expect(followBtn.props['aria-pressed']).toBe(true);

    // Open on a selected card routes to onLiftObject when provided.
    const campaignCard = tree!.root.findAllByProps({ 'data-kind': 'campaign' })[0];
    await act(async () => { campaignCard.props.onKeyDown({ key: 'Enter', preventDefault: vi.fn(), metaKey: false, ctrlKey: false, shiftKey: false }); });
    expect(onLiftObject).toHaveBeenCalledWith(expect.objectContaining({ id: 'campaign', kind: 'campaign' }));

    // Held state transitions true on zoom interaction and releases after 1000ms.
    vi.useFakeTimers();
    try {
      const zoomIn = tree!.root.findByProps({ 'aria-label': 'Zoom in' });
      await act(async () => { zoomIn.props.onClick(); });
      expect(tree!.root.findByProps({ className: 'k-workbench-viewport' }).props['data-held']).toBe('true');
      await act(async () => { await vi.advanceTimersByTimeAsync(1050); });
      expect(tree!.root.findByProps({ className: 'k-workbench-viewport' }).props['data-held']).toBeUndefined();
    } finally {
      vi.useRealTimers();
    }
  });
});
