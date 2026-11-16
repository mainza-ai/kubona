import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
import type { CampaignState, Job, Loop, Need, Task } from '@/types';
import type { ObligationRow } from '@/lib/api';
import { Workbench, type WorkbenchHandle } from './Workbench';
import { publishWorkflowPlan, setWorkflowPlanTaskIds } from '@/domain/workflow-plan-store';

// Migrated from react-test-renderer (Workbench.test.tsx). The old suite read
// `tree.root.findAllByProps(...)`, called `props.onClick()` and stringified
// `toJSON()`, so it never proved that a card was painted, that a control was
// clickable, or that a keyboard event reached the canvas. These assertions run
// in Chromium against the live DOM: real cards, real style/geometry, real
// events.
//
// No `vi.mock('@/lib/api')`: the mount reads and both save channels are
// answered at the fetch boundary, so the operation paths, methods and bodies
// the console really sends stay under test.
//
// Canvas nodes live inside a transformed, overflow-clipped world. Playwright's
// actionability checks cannot scroll such a node into the viewport, so clicks
// on canvas cards dispatch a real DOM MouseEvent at the node (with modifier
// flags where the old test used them) and keyboard assertions either focus the
// node and use userEvent, or dispatch a real KeyboardEvent at the node. Every
// handler still runs through React's real event system; no component props are
// invoked directly.

type Call = {
  method: string;
  path: string;
  url: string;
  body?: unknown;
};

function ok(payload: unknown) {
  return {
    ok: true,
    status: 200,
    json: async () => payload,
    text: async () => JSON.stringify(payload),
  };
}

function refused(status: number, message: string) {
  return {
    ok: false,
    status,
    json: async () => ({ message }),
    text: async () => message,
  };
}

/** Answers every mount/save read; records every request the workbench makes. */
function installFetch(handler?: (call: Call) => unknown): Call[] {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const parsed = new URL(String(url));
    const call: Call = {
      method: (init?.method ?? 'GET').toUpperCase(),
      path: parsed.pathname,
      url: String(url),
      body: init?.body ? JSON.parse(String(init.body)) : undefined,
    };
    calls.push(call);
    const custom = handler?.(call);
    if (custom !== undefined) return custom;
    if (call.method === 'GET' && call.path === '/api/workbench/layout') {
      return refused(404, 'no saved workbench layout');
    }
    if (call.method === 'GET' && call.path === '/api/workbench/view') {
      return refused(404, 'no saved workbench view');
    }
    if (call.method === 'GET' && call.path === '/api/canvas/layout') return ok({});
    if (call.method === 'PUT' && call.path === '/api/workbench/layout') return ok({ status: 'applied', session_id: 's1', revision: 1 });
    if (call.method === 'PUT' && call.path === '/api/workbench/view') {
      return ok({ status: 'applied', session_id: 's1', view_revision: 1, unchanged: false });
    }
    if (call.method === 'GET' && /^\/api\/loops\/[^/]+\/parts$/.test(call.path)) {
      return ok({ ok: true, loop_id: 'l1', parts: [], count: 0 });
    }
    if (call.method === 'GET' && /^\/api\/loops\/[^/]+\/steers$/.test(call.path)) {
      return ok({ steers: [] });
    }
    if (call.method === 'GET' && call.path === '/api/run-allowances') {
      return ok({ ok: true, allowances: [], count: 0 });
    }
    return refused(404, `unexpected ${call.method} ${call.path}`);
  });
  return calls;
}

beforeEach(() => {
  installFetch();
});

afterEach(() => {
  vi.unstubAllGlobals();
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

const reviewTask: Task = {
  id: 't1',
  session_id: 's1',
  title: 'Review anchor',
  status: 'open',
  source_domain: 'measurement',
  source_id: 'm1',
  created_at: '',
  updated_at: '',
};

function q<T extends Element = Element>(root: ParentNode, selector: string): T | null {
  return root.querySelector<T>(selector);
}

function qa<T extends Element = Element>(root: ParentNode, selector: string): T[] {
  return Array.from(root.querySelectorAll<T>(selector));
}

function cards(root: ParentNode): HTMLElement[] {
  return qa<HTMLElement>(root, '.k-workbench-card');
}

function cardsOfKind(root: ParentNode, kind: string): HTMLElement[] {
  return cards(root).filter((card) => card.dataset.kind === kind);
}

function buttonText(root: ParentNode, text: string): HTMLButtonElement | undefined {
  return qa<HTMLButtonElement>(root, 'button').find((button) => (button.textContent ?? '').trim() === text);
}

/**
 * Let the concurrent root commit. A native event schedules React work; the
 * assertion must run after that commit, not inside the dispatch.
 */
async function flushDom() {
  await new Promise((resolve) => setTimeout(resolve, 0));
}

async function click(target: Element | null | undefined) {
  if (!target) throw new Error('click target not found');
  (target as HTMLElement).click();
  await flushDom();
}

async function clickWith(target: Element | null | undefined, init: MouseEventInit) {
  if (!target) throw new Error('click target not found');
  target.dispatchEvent(new MouseEvent('click', { bubbles: true, cancelable: true, ...init }));
  await flushDom();
}

async function press(target: Element | null | undefined, init: KeyboardEventInit) {
  if (!target) throw new Error('keyboard target not found');
  target.dispatchEvent(new KeyboardEvent('keydown', { bubbles: true, cancelable: true, ...init }));
  await flushDom();
}

function leftOf(element: HTMLElement): number {
  return Number.parseFloat(element.style.left || '0');
}

/** Set a controlled input the way the browser does: value setter + input event. */
async function setInputValue(input: HTMLInputElement, value: string) {
  const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set;
  setter?.call(input, value);
  input.dispatchEvent(new Event('input', { bubbles: true }));
  await flushDom();
}

describe('Workbench in a real browser', () => {
  it('shows an explicit empty state without a campaign', async () => {
    const screen = await render(<Workbench campaign={null} />);
    expect(q(screen.container, '[data-testid="workbench"]')).not.toBeNull();
    expect(cards(screen.container)).toHaveLength(0);
    expect(q(screen.container, '.k-workbench-empty')).not.toBeNull();
  });

  it('routes an empty multi-lab workspace to sign-in instead of campaign creation', async () => {
    const onSignIn = vi.fn();
    const screen = await render(<Workbench campaign={null} multiLab onSignIn={onSignIn} />);
    expect(screen.container.textContent).toContain('Multi-lab workspace');
    expect(q(screen.container, '[data-testid="workbench-create-campaign"]')).toBeNull();
    await click(q(screen.container, '[data-testid="workbench-sign-in"]'));
    expect(onSignIn).toHaveBeenCalledTimes(1);
  });

  it('stops asking a signed-in multi-lab user to sign in', async () => {
    const onCreateCampaign = vi.fn();
    const screen = await render(
      <Workbench campaign={null} multiLab authenticated onCreateCampaign={onCreateCampaign} />,
    );
    expect(screen.container.textContent).toContain('Create or open a campaign');
    expect(screen.container.textContent).not.toContain('sign in to create or open');
    expect(q(screen.container, '[data-testid="workbench-sign-in"]')).toBeNull();
    expect(q(screen.container, '[data-testid="workbench-create-campaign"]')).not.toBeNull();
  });

  it('offers a first-campaign action and a sign-in action for auth refusals', async () => {
    const onCreateCampaign = vi.fn();
    const first = await render(
      <Workbench campaign={null} localMode onCreateCampaign={onCreateCampaign} />,
    );
    await click(q(first.container, '[data-testid="workbench-create-campaign"]'));
    expect(onCreateCampaign).toHaveBeenCalledTimes(1);
    await first.unmount();

    const onSignIn = vi.fn();
    const second = await render(
      <Workbench campaign={null} error="multi-lab store: log in" authError onSignIn={onSignIn} />,
    );
    expect(second.container.textContent).toContain('Sign in to open this campaign.');
    await click(q(second.container, '[data-testid="workbench-sign-in"]'));
    expect(onSignIn).toHaveBeenCalledTimes(1);
  });

  it('shows a campaign read refusal instead of an empty campaign', async () => {
    const onRetry = vi.fn();
    const screen = await render(
      <Workbench campaign={null} error="campaign read refused" onRetry={onRetry} />,
    );
    expect(screen.container.textContent).toContain('Campaign unavailable');
    expect(screen.container.textContent).toContain('campaign read refused');
    await click(q(screen.container, '[data-testid="retry-campaign"]'));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it('projects the campaign into typed cards and a figure', async () => {
    const screen = await render(<Workbench campaign={campaign} sessionId="s1" tasks={[reviewTask]} />);
    const painted = cards(screen.container);
    expect(painted).toHaveLength(12);
    expect(painted.map((card) => card.dataset.kind)).toEqual(
      expect.arrayContaining(['campaign', 'hypothesis', 'circuit', 'measurement', 'chart', 'table', 'note', 'molecule', 'task']),
    );
    for (const card of painted) {
      expect(['measured', 'proposed', 'catalog', 'model']).toContain(card.dataset.provenance);
    }
    const metrics = qa(screen.container, '[data-metric]');
    expect(metrics.length).toBeGreaterThan(0);
    for (const metric of metrics) {
      expect(metric.closest('[data-provenance]'), 'workbench metric missing provenance').not.toBeNull();
    }
    expect(qa(screen.container, '[data-testid="k-chart"]')).toHaveLength(1);
  });

  it('completes a task inline and shows recorded session spend', async () => {
    const onTaskStatus = vi.fn();
    const screen = await render(
      <Workbench
        campaign={campaign}
        sessionId="s1"
        sessionSpendUsd={4.18}
        onTaskStatus={onTaskStatus}
        tasks={[reviewTask]}
      />,
    );
    expect(screen.container.textContent).toContain('4.18');
    await click(cardsOfKind(screen.container, 'task')[0]);
    const done = buttonText(screen.container, 'Mark done');
    expect(done).toBeDefined();
    await click(done);
    expect(onTaskStatus).toHaveBeenCalledWith('t1', 'done');
  });

  it('closes a close-ready approval from the card', async () => {
    const onCloseObligation = vi.fn();
    const obligation: ObligationRow = {
      id: 'ob-1234567890',
      subject_kind: 'job',
      subject_id: 'job-1',
      owner: 'operator',
      disposition: 'open',
      required_cases: [],
      required_authorities: [],
      close_ready: true,
      close_reasons: [],
    };
    const screen = await render(
      <Workbench
        campaign={campaign}
        sessionId="s1"
        onCloseObligation={onCloseObligation}
        obligations={[obligation]}
      />,
    );
    await click(cardsOfKind(screen.container, 'approval')[0]);
    await click(q(screen.container, '[data-testid="close-approval"]'));
    await expect.poll(() => onCloseObligation.mock.calls.length).toBe(1);
    expect(onCloseObligation).toHaveBeenCalledWith('ob-1234567890');
  });

  it('offers a list equivalent of the canvas', async () => {
    const screen = await render(<Workbench campaign={campaign} sessionId="s1" />);
    await click(q(screen.container, 'button[title="Switch between canvas and list (text equivalent)"]'));
    expect(q(screen.container, '.k-workbench-list')).not.toBeNull();
    expect(qa(screen.container, 'table')).toHaveLength(1);
    const rows = qa(screen.container, 'tr[data-provenance]');
    expect(rows.length).toBeGreaterThan(0);
    for (const row of rows) {
      expect(['measured', 'proposed', 'catalog', 'model']).toContain(row.getAttribute('data-provenance'));
    }
  });

  it('projects a running loop as an agent-run card', async () => {
    const loops: Loop[] = [
      { id: 'loop-123456789', session_id: 's1', status: 'running', step_count: 3, max_steps: 10, diverged: false, created_at: '', updated_at: '' },
    ];
    const screen = await render(<Workbench campaign={campaign} sessionId="s1" loops={loops} />);
    expect(cardsOfKind(screen.container, 'agent_run')).toHaveLength(1);
    expect(screen.container.textContent).toContain('3 / 10');
  });

  it('renames an area and assigns the selected card to it', async () => {
    const screen = await render(<Workbench campaign={campaign} sessionId="s1" />);
    await click(q(screen.container, 'button[title="Group all objects into a spatial area"]'));
    const titleInput = q<HTMLInputElement>(screen.container, 'input[aria-label="Area title"]');
    expect(titleInput).not.toBeNull();
    await setInputValue(titleInput!, 'Evidence chain');
    const area = q<HTMLElement>(screen.container, '.k-workbench-area');
    expect(area?.querySelector('span')?.textContent).toBe('Evidence chain');

    await click(cardsOfKind(screen.container, 'campaign')[0]);
    await click(buttonText(screen.container, 'Assign to area'));
    expect(cardsOfKind(screen.container, 'campaign')[0].textContent).toContain('area · Evidence chain');
  });

  it('projects a job as a job card', async () => {
    const jobs: Job[] = [
      { id: 'job-123456789', session_id: 's1', status: 2, job_type: 'vqe', params_json: {}, created_at: '2026-11-01T00:00:00Z' },
    ];
    const screen = await render(<Workbench campaign={campaign} sessionId="s1" jobs={jobs} />);
    expect(cardsOfKind(screen.container, 'job')).toHaveLength(1);
    // C9: every job fact names its producing record and can focus it.
    const chips = qa<HTMLButtonElement>(cardsOfKind(screen.container, 'job')[0], '[data-testid="fact-source"]');
    expect(chips.length).toBeGreaterThan(0);
    await click(chips[0]);
    expect(cardsOfKind(screen.container, 'job')[0].dataset.selected).toBe('true');
  });

  it('renders one sentence and one typed remedy for a needed job (C2)', async () => {
    const onNeedRemedy = vi.fn();
    const need: Need = { kind: 'backend_offline', target: 'ibm_fez', reason: 'bridge unreachable' };
    const jobs: Job[] = [
      {
        id: 'job-need',
        session_id: 's1',
        status: 'FAILED',
        job_type: 'vqe',
        params_json: { requested_backend: 'ibm_fez' },
        created_at: '2026-11-01T00:00:00Z',
        need,
      },
    ];
    const screen = await render(
      <Workbench campaign={campaign} sessionId="s1" onNeedRemedy={onNeedRemedy} jobs={jobs} />,
    );
    expect(qa(screen.container, '[data-need="backend_offline"]').length).toBeGreaterThan(0);
    // The server's reason is verbatim; the button is the contract's one remedy.
    expect(screen.container.textContent).toContain('bridge unreachable');
    expect(screen.container.textContent).toContain('Open hardware settings');
    const remedy = q<HTMLButtonElement>(screen.container, 'button.k-workbench-need-remedy');
    expect(remedy).not.toBeNull();
    await click(remedy);
    expect(onNeedRemedy).toHaveBeenCalledWith(expect.objectContaining({ kind: 'backend_offline' }));
  });

  it('projects a published plan and flips Make tasks to Open in Tasks (C8)', async () => {
    const onMakePlanTasks = vi.fn();
    publishWorkflowPlan({
      workflowId: 'wf-test',
      name: 'audit plan',
      steps: [{ id: 's1', tool: 'execution/list_provider_registry', cost_usd: 0.5 }],
      unresolved: [],
    });
    const screen = await render(
      <Workbench campaign={campaign} sessionId="s1" onMakePlanTasks={onMakePlanTasks} />,
    );
    await click(q(screen.container, '[data-testid="plan-make-tasks"]'));
    expect(onMakePlanTasks).toHaveBeenCalledTimes(1);
    // The page attaches ids after the batch; the card then offers Open in Tasks.
    setWorkflowPlanTaskIds(['task-1']);
    await expect.poll(() => q(screen.container, '[data-testid="plan-open-tasks"]') !== null).toBe(true);
    expect(q(screen.container, '[data-testid="plan-make-tasks"]')).toBeNull();
  });

  it('announces the selected object to the shell inspector', async () => {
    const onSelectObject = vi.fn();
    const screen = await render(
      <Workbench campaign={campaign} sessionId="s1" onSelectObject={onSelectObject} />,
    );
    await click(cardsOfKind(screen.container, 'campaign')[0]);
    expect(onSelectObject).toHaveBeenCalledWith(expect.objectContaining({ id: 'campaign' }));
  });

  it('removes a selected card from the canvas and restores it without deleting the record', async () => {
    const screen = await render(<Workbench campaign={campaign} sessionId="s1" />);
    await click(cardsOfKind(screen.container, 'campaign')[0]);
    await click(buttonText(screen.container, 'Remove'));
    expect(cardsOfKind(screen.container, 'campaign')).toHaveLength(0);
    await click(q(screen.container, 'button[title="Restore the last removed card"]'));
    expect(cardsOfKind(screen.container, 'campaign')).toHaveLength(1);
  });

  it('multi-selects cards and removes/restores the group', async () => {
    const screen = await render(<Workbench campaign={campaign} sessionId="s1" />);
    await click(cardsOfKind(screen.container, 'campaign')[0]);
    await clickWith(cardsOfKind(screen.container, 'hypothesis')[0], { metaKey: true });
    expect(qa(screen.container, '[data-selected="true"]')).toHaveLength(2);
    await click(buttonText(screen.container, 'Remove (2)'));
    expect(cardsOfKind(screen.container, 'campaign')).toHaveLength(0);
    expect(cardsOfKind(screen.container, 'hypothesis')).toHaveLength(0);

    // Undo restores the whole group as one step.
    await click(q(screen.container, 'button[title="Restore the last removed card"]'));
    expect(cardsOfKind(screen.container, 'campaign')).toHaveLength(1);
    expect(cardsOfKind(screen.container, 'hypothesis')).toHaveLength(1);
  });

  it('creates separate areas from all objects and the current selection', async () => {
    const screen = await render(<Workbench campaign={campaign} sessionId="s1" />);
    await click(q(screen.container, 'button[title="Group all objects into a spatial area"]'));
    await click(cardsOfKind(screen.container, 'campaign')[0]);
    await click(q(screen.container, 'button[title="Group the selected cards into a spatial area"]'));
    expect(qa(screen.container, '.k-workbench-area')).toHaveLength(2);
  });

  it('creates and removes a typed custom link between two selected cards', async () => {
    const screen = await render(<Workbench campaign={campaign} sessionId="s1" />);
    await click(cardsOfKind(screen.container, 'circuit')[0]);
    await clickWith(cardsOfKind(screen.container, 'evidence')[0], { metaKey: true });
    await click(buttonText(screen.container, 'Link'));
    await click(q(screen.container, 'button[title="Switch between canvas and list (text equivalent)"]'));

    const isCustomLink = (button: HTMLButtonElement) => {
      const text = button.textContent ?? '';
      return text.includes('Circuit') && text.includes('Evidence') && text.includes('supports');
    };
    const linksSection = q<HTMLElement>(screen.container, '[aria-label="Workbench links"]');
    expect(linksSection).not.toBeNull();
    const removeLink = qa<HTMLButtonElement>(linksSection!, 'button').find(isCustomLink);
    expect(removeLink).toBeDefined();
    await click(removeLink);
    const after = q<HTMLElement>(screen.container, '[aria-label="Workbench links"]');
    expect(qa<HTMLButtonElement>(after!, 'button').some(isCustomLink)).toBe(false);
  });

  it('exposes focus and remove actions to the inspector', async () => {
    const ref = React.createRef<WorkbenchHandle>();
    const screen = await render(<Workbench ref={ref} campaign={campaign} sessionId="s1" />);
    expect(ref.current).not.toBeNull();
    expect(ref.current!.focusObject('campaign')).toBe(true);
    ref.current!.removeObject('campaign');
    await expect.poll(() => cardsOfKind(screen.container, 'campaign').length).toBe(0);
  });

  it('clears the whole canvas and restores it as one undoable step', async () => {
    const ref = React.createRef<WorkbenchHandle>();
    const screen = await render(<Workbench ref={ref} campaign={campaign} sessionId="s1" />);
    expect(cardsOfKind(screen.container, 'campaign').length).toBeGreaterThan(0);
    ref.current!.clearCanvas();
    await expect.poll(() => cardsOfKind(screen.container, 'campaign').length).toBe(0);
    expect(q(screen.container, 'button[title="Restore every hidden card"]')).not.toBeNull();
    ref.current!.restoreAll();
    await expect.poll(() => cardsOfKind(screen.container, 'campaign').length).toBeGreaterThan(0);
  });

  it('groups all objects into a persisted spatial area', async () => {
    const screen = await render(<Workbench campaign={campaign} sessionId="s1" />);
    await click(q(screen.container, 'button[title="Group all objects into a spatial area"]'));
    expect(q(screen.container, '.k-workbench-area')).not.toBeNull();
  });

  it('selects, opens, and nudges a card from the keyboard', async () => {
    const onSelectObject = vi.fn();
    const onOpenObject = vi.fn();
    const screen = await render(
      <Workbench campaign={campaign} sessionId="s1" onSelectObject={onSelectObject} onOpenObject={onOpenObject} />,
    );
    const card = () => cardsOfKind(screen.container, 'campaign')[0];
    card().focus();
    await userEvent.keyboard(' ');
    expect(card().dataset.selected).toBe('true');
    expect(onSelectObject).toHaveBeenCalledWith(expect.objectContaining({ id: 'campaign' }));

    await userEvent.keyboard('{Enter}');
    expect(onOpenObject).toHaveBeenCalledWith('campaign');

    const before = leftOf(card());
    await userEvent.keyboard('{ArrowRight}');
    expect(leftOf(card())).toBe(before + 10);
  }, 15_000);

  it('removes a selected card with Delete and can undo', async () => {
    const screen = await render(<Workbench campaign={campaign} sessionId="s1" />);
    await click(cardsOfKind(screen.container, 'campaign')[0]);
    const workbench = q<HTMLElement>(screen.container, '[data-testid="workbench"]');
    workbench!.focus();
    await userEvent.keyboard('{Delete}');
    expect(cardsOfKind(screen.container, 'campaign')).toHaveLength(0);
    await click(q(screen.container, 'button[title="Restore the last removed card"]'));
    expect(cardsOfKind(screen.container, 'campaign')).toHaveLength(1);
  });

  it('saves camera pan/zoom to view channel without mutating semantic layout', async () => {
    const calls = installFetch();
    const screen = await render(<Workbench campaign={campaign} sessionId="s1" />);
    await click(q(screen.container, '[aria-label="Zoom in"]'));
    await expect
      .poll(() => calls.filter((call) => call.method === 'PUT' && call.path === '/api/workbench/view').length, { timeout: 3000 })
      .toBeGreaterThan(0);
    // Wait past the layout debounce (450ms) before proving the semantic
    // channel was never touched by a camera move.
    await new Promise((resolve) => setTimeout(resolve, 300));
    expect(calls.filter((call) => call.method === 'PUT' && call.path === '/api/workbench/layout')).toHaveLength(0);
  });

  it('suppresses Unavailable placeholder cards during initial hydration window', async () => {
    const calls = installFetch((call) => {
      if (call.method === 'GET' && call.path === '/api/workbench/layout') {
        return ok({
          session_id: 's1',
          revision: 2,
          document: {
            version: 2,
            session_id: 's1',
            objects: [
              { id: 'missing-loop', kind: 'agent_run', ref: { domain: 'loop', id: 'l-missing' }, position: { x: 0, y: 0 }, pinned: true },
              { id: 'stale-job', kind: 'job', ref: { domain: 'job', id: '9001d28a-b88' }, position: { x: 400, y: 0 }, pinned: true },
            ],
            links: [],
            areas: [],
            hidden: [],
          },
        });
      }
      return undefined;
    });
    const screen = await render(<Workbench campaign={campaign} sessionId="s1" />);
    expect(calls.some((call) => call.path === '/api/workbench/layout')).toBe(true);
    expect(screen.container.textContent).not.toContain('Unavailable');

    await expect.poll(() => screen.container.textContent, { timeout: 3000 }).toContain('Unavailable');
    expect(screen.container.textContent).not.toContain('9001d28a-b88');
    const removeStale = buttonText(screen.container, 'Remove stale ref');
    expect(removeStale).toBeDefined();
    await click(removeStale);
    expect(screen.container.textContent).not.toContain('Unavailable');
  });

  it('cycles edge visibility (hybrid -> all -> off -> focus) and routes orthogonal edges capped at 6 in focus mode', async () => {
    const screen = await render(<Workbench campaign={campaign} sessionId="s1" />);
    const edgeToggle = () => qa<HTMLButtonElement>(screen.container, 'button').find((button) => (button.textContent ?? '').includes('Edges'))!;
    const paths = () => qa<SVGPathElement>(screen.container, '.k-workbench-links path');
    expect(edgeToggle()).toBeDefined();

    // Default 'hybrid' renders primary spine links even at rest (no card selected).
    const restPaths = paths();
    expect(restPaths.length).toBeGreaterThan(0);
    expect(restPaths.every((path) => path.getAttribute('data-edge-role') === 'rest')).toBe(true);

    await click(cardsOfKind(screen.container, 'campaign')[0]);
    const hybridSelectedPaths = paths();
    expect(hybridSelectedPaths.some((path) => path.getAttribute('data-edge-role') === 'focus')).toBe(true);

    await click(edgeToggle()); // All
    const allPaths = paths();
    expect(allPaths.length).toBeGreaterThanOrEqual(hybridSelectedPaths.length);

    await click(edgeToggle()); // Off
    expect(paths()).toHaveLength(0);

    await click(edgeToggle()); // Focus (max 6)
    const focusedPaths = paths();
    expect(focusedPaths.length).toBeGreaterThan(0);
    expect(focusedPaths.length).toBeLessThanOrEqual(6);
    expect(focusedPaths[0].getAttribute('d')).not.toContain(' C');
    expect(focusedPaths.every((path) => path.getAttribute('data-edge-role') === 'focus')).toBe(true);
  });

  it('renders a floating SelectionBar when cards are selected and supports multi-card Align + Undo', async () => {
    const screen = await render(<Workbench campaign={campaign} sessionId="s1" />);
    const cardA = cardsOfKind(screen.container, 'campaign')[0];
    const beforeLeftB = leftOf(cardsOfKind(screen.container, 'circuit')[0]);

    await click(cardA);
    await clickWith(cardsOfKind(screen.container, 'circuit')[0], { metaKey: true });
    expect(q(screen.container, '.k-workbench-selection-bar')).not.toBeNull();

    await click(buttonText(screen.container, 'Align'));
    expect(leftOf(cardsOfKind(screen.container, 'circuit')[0])).toBe(leftOf(cardsOfKind(screen.container, 'campaign')[0]));

    const workbench = q<HTMLElement>(screen.container, '[data-testid="workbench"]');
    await press(workbench, { key: 'z', ctrlKey: true });
    await expect.poll(() => leftOf(cardsOfKind(screen.container, 'circuit')[0])).toBe(beforeLeftB);
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
    const originalClipboard = Object.getOwnPropertyDescriptor(navigator, 'clipboard');
    Object.defineProperty(navigator, 'clipboard', { configurable: true, value: { writeText } });

    try {
      const screen = await render(
        <Workbench
          campaign={richCampaign}
          jobs={[
            { id: 'job-1', session_id: 's1', job_type: 'vqe', status: 2, params_json: { backend: 'local_simulator', shots: 1024 }, result_json: { energy: -1.13 }, created_at: '2026-11-01T00:00:00Z' },
            { id: 'job-2', session_id: 's1', job_type: 'vqe', status: 2, params_json: { backend: 'ibm_marrakesh', shots: 4096 }, result_json: { energy: -1.11 }, created_at: '2026-11-01T00:01:00Z' },
          ]}
          sessionId="s1"
        />,
      );

      // CodeBlock 16-line cap + expand/collapse
      const circuitCard = () => cardsOfKind(screen.container, 'circuit')[0];
      expect(qa(circuitCard(), '.k-workbench-code-line')).toHaveLength(16);
      await click(q(circuitCard(), '.k-workbench-code-more'));
      expect(qa(circuitCard(), '.k-workbench-code-line')).toHaveLength(20);

      // CompareMatrix Copy CSV (lives on the Column 3 table card when classical_anchor is recorded)
      const compareCard = () => cardsOfKind(screen.container, 'table')[0];
      expect(compareCard()).toBeDefined();
      await click(buttonText(compareCard(), 'Copy CSV'));
      expect(writeText).toHaveBeenCalledTimes(1);
      expect(writeText.mock.calls[0]![0]).toContain('RHF mean-field');

      // CompareMatrix MANUAL override validation (> 8192 bytes rejected) + valid save preserving MEASURED orig + Undo
      await click(q(compareCard(), '.k-compare-edit-btn'));
      const input = () => q<HTMLInputElement>(compareCard(), 'input')!;
      const saveBtn = () => buttonText(compareCard(), 'Save')!;
      await setInputValue(input(), 'x'.repeat(8193));
      await click(saveBtn());
      const alert = q<HTMLElement>(compareCard(), '[role="alert"]');
      expect(alert).not.toBeNull();
      expect(alert!.textContent).toContain('8192');

      await setInputValue(input(), '-1.14500 Ha');
      await click(saveBtn());
      expect(q<HTMLElement>(compareCard(), '.k-compare-manual-badge')?.textContent).toBe('MANUAL');
      expect(q<HTMLElement>(compareCard(), '.k-compare-orig')?.textContent).toContain('orig ');

      const workbench = q<HTMLElement>(screen.container, '[data-testid="workbench"]');
      await press(workbench, { key: 'z', ctrlKey: true });
      await expect.poll(() => q(compareCard(), '.k-compare-manual-badge')).toBeNull();
    } finally {
      if (originalClipboard) Object.defineProperty(navigator, 'clipboard', originalClipboard);
      else delete (navigator as { clipboard?: unknown }).clipboard;
    }
  });

  it('filters cards with recovery empty state, renders inline ConfirmCard + AskReceipt, minimap, Follow live, Lift, and held state', async () => {
    // A saved viewport means load performs no framing camera move, so the
    // follow-live default is observable on a laid-out canvas rather than being
    // paused by the initial fit.
    installFetch((call) => {
      if (call.method === 'GET' && call.path === '/api/workbench/view') {
        return ok({
          session_id: 's1',
          view_revision: 1,
          view: { camera: { x: 36, y: 36, zoom: 0.82 }, selected: [], view_mode: 'canvas', lod: 'detail' },
        });
      }
      return undefined;
    });
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
    const loops: Loop[] = [
      { id: 'l1', session_id: 's1', status: 'running', step_count: 1, max_steps: 4, diverged: false, created_at: '', updated_at: '' },
    ];

    const screen = await render(
      <Workbench
        campaign={meteredCampaign}
        sessionId="s1"
        loops={loops}
        confirmGate={gate}
        onInlineGateVisibility={onInlineGateVisibility}
        onGateSettled={onGateSettled}
        onLiftObject={onLiftObject}
      />,
    );

    // Single-home guarantee: circuit card owns the gate while on Canvas view.
    expect(onInlineGateVisibility).toHaveBeenCalledWith(true);
    expect(q(screen.container, '[data-testid="confirm-card"]')).not.toBeNull();

    // Downgrade inside the inline ConfirmCard emits AskReceipt + calls onDowngrade.
    await click(q(screen.container, '[data-testid="confirm-card-downgrade"]'));
    expect(onDowngrade).toHaveBeenCalledTimes(1);
    expect(onGateSettled).toHaveBeenCalledWith(expect.objectContaining({ outcome: 'downgraded' }));

    // Even when confirmGate clears after settlement, AskReceipt stays rendered on the circuit card.
    await screen.rerender(
      <Workbench
        campaign={meteredCampaign}
        sessionId="s1"
        loops={loops}
        confirmGate={null}
        onInlineGateVisibility={onInlineGateVisibility}
        onGateSettled={onGateSettled}
        onLiftObject={onLiftObject}
      />,
    );
    expect(q(screen.container, '[data-testid="ask-receipt"]')).not.toBeNull();
    expect(onInlineGateVisibility).toHaveBeenCalledWith(false);

    // Filter chips: Jobs shows only job/agent_run cards.
    await click(q(screen.container, '[data-filter="jobs"]'));
    expect(
      cards(screen.container).every((card) => card.dataset.kind === 'agent_run' || card.dataset.kind === 'job'),
    ).toBe(true);

    await click(q(screen.container, '[data-filter="all"]'));

    // Bottom-right CanvasControls with collapsible minimap + Follow live toggle on canvas view.
    expect(q(screen.container, '[data-testid="workbench-canvas-controls"]')).not.toBeNull();
    expect(qa(screen.container, '[data-testid="workbench-minimap"]')).toHaveLength(0);
    const minimapToggle = () => q<HTMLElement>(screen.container, '[data-testid="workbench-minimap-toggle"]');
    expect(minimapToggle()!.getAttribute('aria-expanded')).toBe('false');
    await click(minimapToggle());
    expect(minimapToggle()!.getAttribute('aria-expanded')).toBe('true');
    expect(q(screen.container, '[data-testid="workbench-minimap"]')).not.toBeNull();

    // C5: follow-live is on by default, and the toggle pauses/resumes it.
    const followBtn = () => q<HTMLElement>(screen.container, '[data-testid="workbench-follow-live"]');
    expect(followBtn()!.getAttribute('aria-pressed')).toBe('true');
    await click(followBtn());
    expect(followBtn()!.getAttribute('aria-pressed')).toBe('false');
    await click(followBtn());
    expect(followBtn()!.getAttribute('aria-pressed')).toBe('true');

    // Open on a selected card routes to onLiftObject when provided.
    await press(cardsOfKind(screen.container, 'campaign')[0], { key: 'Enter' });
    expect(onLiftObject).toHaveBeenCalledWith(expect.objectContaining({ id: 'campaign', kind: 'campaign' }));

    // Held state transitions true on zoom interaction and releases after 1000ms.
    const viewport = () => q<HTMLElement>(screen.container, '.k-workbench-viewport');
    await click(q(screen.container, '[aria-label="Zoom in"]'));
    expect(viewport()!.getAttribute('data-held')).toBe('true');
    await expect.poll(() => viewport()!.getAttribute('data-held'), { timeout: 3000 }).toBeNull();
  });

  it('renders single-select NodeToolbar vs multi-select SelectionBar cleanly', async () => {
    const screen = await render(
      <Workbench campaign={campaign} sessionId="s1" onLiftObject={vi.fn()} onAskSelection={vi.fn()} />,
    );
    await click(cardsOfKind(screen.container, 'campaign')[0]);
    expect(qa(screen.container, '.k-workbench-node-toolbar')).toHaveLength(1);
    expect(qa(screen.container, '.k-workbench-selection-bar')).toHaveLength(0);

    await clickWith(cardsOfKind(screen.container, 'hypothesis')[0], { metaKey: true });
    expect(qa(screen.container, '.k-workbench-node-toolbar')).toHaveLength(0);
    expect(qa(screen.container, '.k-workbench-selection-bar')).toHaveLength(1);
  });

  it('auto-clusters >24 jobs into a collapsible Jobs & Runs region without dropping filtered views', async () => {
    const manyJobs: Job[] = Array.from({ length: 22 }, (_, i) => ({
      id: `job-${i + 1}`,
      session_id: 's1',
      status: 'COMPLETED',
      job_type: 'vqe',
      params_json: { step: i + 1 },
      result_json: { energy: -1.1 },
      created_at: '2026-11-11T00:00:00Z',
    }));
    // A laid-out canvas: the C7 near-viewport set is only exercised when the
    // viewport has real dimensions, so this one test gives the canvas a box.
    const screen = await render(
      <div style={{ width: 5000, height: 5000 }}>
        <Workbench campaign={campaign} sessionId="s1" jobs={manyJobs} />
      </div>,
    );
    const cluster = q<HTMLElement>(screen.container, '.k-workbench-area[data-cluster="true"]');
    expect(cluster).not.toBeNull();
    const totalCardsBefore = cards(screen.container).length;
    expect(totalCardsBefore).toBeGreaterThan(24);

    await click(buttonText(cluster!, 'Collapse'));
    expect(cards(screen.container).length).toBeLessThan(totalCardsBefore);

    // Filter chip 'Jobs' bypasses cluster hiding so jobs remain individually inspectable.
    await click(q(screen.container, '[data-filter="jobs"]'));
    expect(cards(screen.container)).toHaveLength(22);

    await click(q(screen.container, '[data-filter="all"]'));
    const area = q<HTMLElement>(screen.container, '.k-workbench-area');
    await click(buttonText(area!, 'Expand'));
    expect(cards(screen.container).length).toBe(totalCardsBefore);
  });
});
