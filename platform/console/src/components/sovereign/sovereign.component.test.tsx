import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { page, userEvent } from 'vitest/browser';
import React from 'react';
import { ModeRail } from '@/components/shell/ModeRail';
import { ArenaHud } from './ArenaHud';
import { SwarmUniverse } from './SwarmUniverse';
import { ActionGate } from './ActionGate';
import { canonicalJobBinding } from '@/lib/mandates';
import { WidgetStage } from './WidgetStage';
import { ProofDagView } from './ProofDagView';
import type { CampaignState } from '@/types';

// Sovereign arena surfaces in a real browser: the mode rail, HUD, probe
// universe, action gate, widget stage, and proof DAG. Everything below drives
// real DOM (clicks, typing, Escape) and observes real layout/attributes; the
// daemon is stubbed at the transport boundary by URL.

type Screen = Awaited<ReturnType<typeof render>>;
type Call = { url: string; method: string; body: string | undefined };

const ok = (data: unknown, status = 200) => ({
  ok: true,
  status,
  json: async () => data,
  text: async () => JSON.stringify(data),
});

const refuse = (status: number, body: unknown) => ({
  ok: false,
  status,
  json: async () => body,
  text: async () => (typeof body === 'string' ? body : JSON.stringify(body)),
});

type Route = (call: Call) => ReturnType<typeof ok>;

/** Stub the daemon by exact pathname (query ignored) and record every call. */
function stubDaemon(routes: Record<string, Route>, fallback: Route = () => refuse(404, { message: 'not found' }) as never) {
  const calls: Call[] = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (input: unknown, init?: RequestInit) => {
      const url = String(input);
      const path = new URL(url, 'http://localhost:3000').pathname;
      const call = { url, method: (init?.method ?? 'GET').toUpperCase(), body: init?.body as string | undefined };
      calls.push(call);
      const route = routes[path];
      return route ? route(call) : fallback(call);
    }),
  );
  return calls;
}

const text = (screen: Screen) => screen.container.textContent ?? '';

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
    anchor_computed: true,
    anchor_source: 'pyscf-live',
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

beforeEach(async () => {
  // The arena surfaces position probes and widgets against the viewport;
  // a desktop size is the geometry an operator actually sees.
  await page.viewport(1280, 720);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('ModeRail in a real browser', () => {
  it('renders four modes and marks the active one', async () => {
    const onSelect = vi.fn();
    const screen = await render(
      <ModeRail active="operations" onSelect={onSelect} airGapSovereign onOpenCampaigns={() => {}} />,
    );

    for (const id of ['arena', 'proofs', 'mesh', 'settings']) {
      expect(screen.container.querySelector(`[data-testid="dock-${id}"]`), id).not.toBeNull();
    }
    expect(screen.getByTestId('dock-mesh').element().getAttribute('aria-pressed')).toBe('true');

    await screen.getByTestId('dock-arena').click();
    expect(onSelect).toHaveBeenCalledWith('workbench');

    const logo = screen.container.querySelector('img[alt="Kubona"]') as HTMLImageElement;
    expect(logo).not.toBeNull();
    expect(logo.getAttribute('src')).toContain('milimo_kubona_logo.png');
  });

  it('names every mode natively and keeps the air-gap sentinel', async () => {
    const screen = await render(
      <ModeRail active="operations" onSelect={() => {}} airGapSovereign onOpenCampaigns={() => {}} />,
    );

    for (const id of ['arena', 'proofs', 'mesh', 'settings']) {
      const element = screen.getByTestId(`dock-${id}`).element() as HTMLButtonElement;
      expect(element.title, id).toBeTruthy();
      expect(element.getAttribute('aria-label'), id).toBeTruthy();
    }

    const sentinels = screen.container.querySelectorAll('div[role="status"][aria-label^="Air-gap"]');
    expect(sentinels).toHaveLength(1);
    expect((sentinels[0] as HTMLElement).tabIndex).toBe(0);
  });
});

describe('ArenaHud in a real browser', () => {
  it('shows target, spend, elapsed and switches views', async () => {
    const onViewChange = vi.fn();
    const screen = await render(
      <ArenaHud
        targetLabel="FeMoco · Ground State Target"
        swarmDetail="3 Loops Live"
        view="orbital"
        onViewChange={onViewChange}
        spendUsd={4.18}
        spendCapUsd={150}
        sessionElapsed="03:14:22"
        onRecenter={() => {}}
      />,
    );

    const header = screen.container.querySelector('header') as HTMLElement;
    expect(header).not.toBeNull();
    expect(header.textContent).toContain('FeMoco');
    expect(header.textContent).toContain('03:14:22');

    await screen.getByTestId('arena-view-proof-dag').click();
    expect(onViewChange).toHaveBeenCalledWith('proof-dag');
  });

  it('shows the run status pill while the swarm is working', async () => {
    const screen = await render(
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
      />,
    );

    expect(screen.getByTestId('hud-status').element().textContent).toContain('reasoning · 12s');
  });
});

describe('SwarmUniverse in a real browser', () => {
  it('renders four probes and the live campaign energy', async () => {
    const screen = await render(<SwarmUniverse campaign={CAMPAIGN} />);

    for (const probe of ['probe-theorist', 'probe-formalist', 'probe-compiler', 'probe-critic']) {
      expect(screen.container.querySelector(`[data-testid="${probe}"]`), probe).not.toBeNull();
    }
    expect(screen.getByTestId('swarm-universe').element().getAttribute('data-provenance')).toBe('measured');
    expect(screen.getByTestId('universe-energy').element().textContent).toContain('-1984.3418');
  });

  it('renders placeholders without a campaign (never invented numbers)', async () => {
    const screen = await render(<SwarmUniverse campaign={null} />);
    expect(text(screen)).toContain('—');
  });

  it('lights the active probe and leaves the rest dark', async () => {
    const screen = await render(<SwarmUniverse campaign={CAMPAIGN} activeProbe="compiler" />);
    expect(screen.getByTestId('probe-compiler').element().getAttribute('data-active')).toBe('true');
    expect(screen.getByTestId('probe-theorist').element().getAttribute('data-active')).toBeNull();
  });

  it('tap-to-open: probe cards toggle on tap and close on Escape', async () => {
    const screen = await render(<SwarmUniverse campaign={CAMPAIGN} />);
    const probe = screen.getByTestId('probe-theorist');
    expect(probe.element().getAttribute('aria-expanded')).toBe('false');

    // Tap the probe's own label (the hover card stops propagation by design,
    // and without the stylesheet it is laid out inline inside the button).
    await probe.getByText('Theorist').click();
    await expect.poll(() => probe.element().getAttribute('aria-expanded')).toBe('true');

    // Real Escape key on the live window listener the component installed.
    await userEvent.keyboard('{Escape}');
    await expect.poll(() => probe.element().getAttribute('aria-expanded')).toBe('false');
  });

  it('rests when idle: no sweep, rest note shown', async () => {
    const screen = await render(<SwarmUniverse campaign={CAMPAIGN} atRest />);
    expect(screen.getByTestId('swarm-universe').element().getAttribute('data-rest')).toBe('true');
    expect(screen.getByTestId('universe-rest-note').element().textContent).toContain('idle');
  });

  it('animates when working: no rest markers', async () => {
    const screen = await render(<SwarmUniverse campaign={CAMPAIGN} atRest={false} />);
    expect(screen.getByTestId('swarm-universe').element().getAttribute('data-rest')).toBeNull();
    expect(screen.container.querySelectorAll('[data-testid="universe-rest-note"]')).toHaveLength(0);
  });
});

describe('ActionGate in a real browser', () => {
  it('shows the authorize affordance with live cost and backend', async () => {
    const screen = await render(
      <ActionGate campaign={CAMPAIGN} session={null} onAuthorized={() => {}} onDenied={() => {}} onDowngrade={() => {}} />,
    );

    expect(screen.container.querySelector('[data-testid="action-gate-authorize"]')).not.toBeNull();
    expect(screen.container.querySelector('[data-testid="action-gate-downgrade"]')).not.toBeNull();
    const flat = screen.getByTestId('action-gate').element().textContent ?? '';
    expect(flat).toContain('48.50');
    expect(flat).toContain('QuEra Aquila');
  });
});

describe('WidgetStage in a real browser', () => {
  const base = {
    campaign: CAMPAIGN,
    gateOpen: true,
    onDismiss: () => {},
    onRequestGateFocus: () => {},
  };

  it('stays empty by default (zero clutter)', async () => {
    const screen = await render(<WidgetStage widget={null} {...base} />);
    expect(screen.container.querySelectorAll('[data-testid="widget-stage"]')).toHaveLength(0);
  });

  it('materializes one widget with title and dismisses on demand', async () => {
    const onDismiss = vi.fn();
    const screen = await render(
      <WidgetStage widget={{ kind: 'circuit', title: 'Circuit · live' }} {...base} onDismiss={onDismiss} />,
    );

    const stage = screen.getByTestId('widget-stage').element().textContent ?? '';
    expect(stage).toContain('Circuit · live');
    expect(stage).toContain('EMPTY');

    await screen.getByTestId('widget-dismiss').click();
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it('spend card shows live figures and is read-only once the gate has decided', async () => {
    const screen = await render(
      <WidgetStage widget={{ kind: 'mandate', title: 'Spend mandate' }} {...base} gateOpen={false} />,
    );

    const flat = screen.getByTestId('widget-stage').element().textContent ?? '';
    expect(flat).toContain('read-only');
    expect(flat).toContain('48.50');
  });

  it('minimizes to the header bar and expands back', async () => {
    const screen = await render(
      <WidgetStage widget={{ kind: 'circuit', title: 'Circuit · live' }} {...base} onDismiss={() => {}} />,
    );

    const stage = screen.getByTestId('widget-stage');
    expect(stage.element().getAttribute('data-minimized')).toBeNull();

    await screen.getByTestId('widget-minimize').click();
    await expect.poll(() => stage.element().getAttribute('data-minimized')).toBe('true');

    await screen.getByTestId('widget-minimize').click();
    await expect.poll(() => stage.element().getAttribute('data-minimized')).toBeNull();
  });

  it('marks empty feeds EMPTY, proposal fields PROPOSAL, and live feeds LIVE', async () => {
    const shadows = await render(
      <WidgetStage widget={{ kind: 'shadows', title: 'Shadow estimates' }} {...base} onDismiss={() => {}} />,
    );
    expect(text(shadows)).toContain('EMPTY');

    const circuit = await render(
      <WidgetStage widget={{ kind: 'circuit', title: 'Circuit · empty' }} {...base} onDismiss={() => {}} />,
    );
    const circuitText = text(circuit);
    expect(circuitText).toContain('EMPTY');
    expect(circuitText).not.toMatch(/\bLIVE\b/);

    const molecule = await render(
      <WidgetStage widget={{ kind: 'molecule', title: 'Molecule' }} {...base} onDismiss={() => {}} />,
    );
    expect(text(molecule)).toContain('PROPOSAL');
  });

  it('spend card defers the decision to the Action Gate (never signs itself)', async () => {
    const onRequestGateFocus = vi.fn();
    const screen = await render(
      <WidgetStage
        widget={{ kind: 'mandate', title: 'Spend mandate' }}
        {...base}
        gateOpen
        onRequestGateFocus={onRequestGateFocus}
      />,
    );

    await screen.getByRole('button', { name: /Review at Action Gate/ }).click();
    expect(onRequestGateFocus).toHaveBeenCalledTimes(1);
  });
});

describe('ProofDagView in a real browser', () => {
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

  it('lays out nodes by layer and draws the real edge set', async () => {
    const screen = await render(<ProofDagView campaign={dagCampaign} />);

    const root = screen.container.querySelector('[data-testid="proof-dag"]');
    expect(root).not.toBeNull();
    const svg = screen.container.querySelector('[data-testid="proof-dag"] svg') as SVGSVGElement;
    expect(svg.textContent).toContain('Rz');
    expect(svg.querySelectorAll('path')).toHaveLength(1);
    // One focusable node per real DAG node.
    expect(screen.container.querySelectorAll('[data-testid="proof-dag"] g[role="button"]')).toHaveLength(2);
  });

  it('drops dangling edges and stays honest when empty', async () => {
    const broken = {
      ...dagCampaign,
      quantum_factors: {
        ...dagCampaign.quantum_factors,
        dag_edges: [{ id: 'ghost', source: 'nope', target: 'rz-q0' }],
      },
    } as unknown as CampaignState;
    const screen = await render(<ProofDagView campaign={broken} />);
    expect(screen.container.querySelectorAll('[data-testid="proof-dag"] svg path')).toHaveLength(0);
    await screen.unmount();

    const empty = await render(<ProofDagView campaign={CAMPAIGN} />);
    expect(text(empty)).toContain('No circuit DAG');
  });

  it('labels a proposal DAG as proposed, not measured', async () => {
    const proposal = { ...dagCampaign, notebook: null } as unknown as CampaignState;
    const screen = await render(<ProofDagView campaign={proposal} />);
    expect(text(screen)).toContain('Proposed circuit DAG');
  });

  it('selecting a node traces its record', async () => {
    const screen = await render(<ProofDagView campaign={dagCampaign} />);

    const nodes = screen.container.querySelectorAll('[data-testid="proof-dag"] g[role="button"]');
    expect(nodes).toHaveLength(2);
    expect(text(screen)).toContain('Select a node to trace its record.');

    await screen.getByRole('button', { name: 'Select circuit node rz-q0' }).click();
    await expect.poll(() => text(screen)).toContain('id rz-q0 · Param Rotation');
  });
});

describe('ProofDagView wire cuts + compile receipt (C-1) in a real browser', () => {
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

  it('labels planned cuts planned and measured cuts measured', async () => {
    const screen = await render(<ProofDagView campaign={cutCampaign} />);
    const body = text(screen);
    // Stronger contract: unmeasured cuts render known fields with a planned
    // prefix, never a bare generic chip.
    expect(body).toContain('planned: type KaHyPar QPD');
    expect(body).toContain('(not executed)');
    expect(body).toContain('measured: 2 cuts');
    expect(body).toContain('2 fragments');
    expect(body).toContain('abcdef12');
  });

  it('compiles via backend and shows the receipt honestly', async () => {
    const calls = stubDaemon({
      '/api/qir/compile': () =>
        ok({
          num_qubits: 2,
          depth: 4,
          gate_count: 6,
          t_count: 0,
          qir_profile: 'BaseProfile-Estimate',
          llvm_ir: '; ModuleID',
          compiler_used: 'go-fallback-estimate',
          fallback_estimate: true,
        }),
    });
    const screen = await render(<ProofDagView campaign={cutCampaign} />);

    await screen.getByRole('button', { name: /Compile receipt/ }).click();
    await expect.poll(() => text(screen)).toContain('OPENQASM 2.0');

    await screen.getByRole('button', { name: 'Compile via backend' }).click();
    await expect.poll(() => text(screen)).toContain('go-fallback-estimate');

    const body = text(screen);
    expect(body).toContain('ESTIMATE (not compiler-verified)');
    expect(body).toContain('; ModuleID');
    expect(calls.filter((call) => call.url.includes('/api/qir/compile'))).toHaveLength(1);
  });

  it('says so when compile refuses', async () => {
    stubDaemon({ '/api/qir/compile': () => refuse(503, { message: 'compiler down' }) as never });
    const screen = await render(<ProofDagView campaign={cutCampaign} />);

    await screen.getByRole('button', { name: /Compile receipt/ }).click();
    await screen.getByRole('button', { name: 'Compile via backend' }).click();

    await expect.poll(() => text(screen)).toContain('Compile refused');
  });
});

describe('ProofDagView campaign signals (C-2) in a real browser', () => {
  it('surfaces stuck, rescoped, and contested states as chips', async () => {
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
    const screen = await render(<ProofDagView campaign={signalCampaign} />);

    const body = text(screen);
    expect(body).toContain('stuck: step 15 (barren_plateau_trip)');
    expect(body).toContain('rescoped: step 18 (rescoped_rollback)');
    expect(body).toContain('branch fork-01: rescoped');
    expect(body).toContain('epistemics: CONTESTED');
  });

  it('stays quiet on clean campaigns', async () => {
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
    const screen = await render(<ProofDagView campaign={clean} />);

    const body = text(screen);
    expect(body).not.toContain('stuck:');
    expect(body).not.toContain('epistemics: CONTESTED');
  });
});

describe('ActionGate tenant attribution (J4) in a real browser', () => {
  it('authorizes against the lab tenant, never the session id', async () => {
    let seenBody: { tenant_id?: string; job_hash?: string } | null = null;
    let seenMint: { session_id?: string; source?: string; qasm?: string } | null = null;
    let seenLaunch: { mandate_ref?: string; proposal_id?: string; digest?: string; qasm?: string } | null = null;
    const onAuthorized = vi.fn();
    stubDaemon({
      '/api/mandates/verify': (call) => {
        seenBody = JSON.parse(String(call.body));
        return ok({ valid: true });
      },
      '/api/proposals': (call) => {
        seenMint = JSON.parse(String(call.body));
        return ok({ ok: true, proposal: { id: 'pr_gate', digest: 'sha256:gate', body: { backend: 'ibm_test' } } });
      },
      '/api/chat/launch': (call) => {
        seenLaunch = JSON.parse(String(call.body));
        return ok({ job: { id: 'job-gate-1' }, loop: { id: 'loop-gate-1' } });
      },
    });

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
    // W0.4: the gate signs the exact program — the fixture carries QASM so
    // authorization can proceed to the tenant assertion below.
    const withProgram = {
      ...CAMPAIGN,
      quantum_factors: { ...CAMPAIGN.quantum_factors, qasm: 'OPENQASM 2.0;\nqreg q[1];\n' },
    };
    const screen = await render(
      <ActionGate campaign={withProgram} session={session} onAuthorized={onAuthorized} onDenied={() => {}} onDowngrade={() => {}} />,
    );

    // The consequential settle window arms in real time; the button unlocks.
    const authorize = screen.getByTestId('action-gate-authorize');
    await expect.poll(() => (authorize.element() as HTMLButtonElement).disabled).toBe(false);
    await authorize.click();

    await expect.poll(() => seenLaunch).not.toBeNull();
    expect(seenBody).not.toBeNull();
    expect(seenBody!.tenant_id).toBe('lab-alpha');
    // The minted mandate binds the exact session + program (daemon-side
    // admission recomputes this and refuses anything else).
    expect(seenBody!.job_hash).toBe(canonicalJobBinding('sess-tenant-test', 'OPENQASM 2.0;\nqreg q[1];\n'));
    // B1: the surface mints from stored campaign state (never sending the
    // program itself), then launches by proposal identity + the signed
    // mandate. A verified receipt alone is still not a dispatch.
    expect(seenMint).not.toBeNull();
    expect(seenMint!.source).toBe('campaign');
    expect(seenMint!.qasm).toBeUndefined();
    expect(JSON.parse(String(seenLaunch!.mandate_ref)).job_hash).toBe(seenBody!.job_hash);
    expect(seenLaunch!.proposal_id).toBe('pr_gate');
    expect(seenLaunch!.digest).toBe('sha256:gate');
    expect(seenLaunch!.qasm).toBeUndefined();
    await expect.poll(() => onAuthorized.mock.calls.length).toBe(1);
    await expect.poll(() => text(screen)).toContain('Dispatched job job-gate');
  });
});

describe('ActionGate cost honesty in a real browser', () => {
  it('disables authorization without a priced campaign', async () => {
    const calls = stubDaemon({});
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
    const screen = await render(
      <ActionGate campaign={unpriced} session={session} onAuthorized={() => {}} onDenied={() => {}} onDowngrade={() => {}} />,
    );

    const body = text(screen);
    expect(body).toContain('unpriced — authorization disabled');
    expect(body).not.toContain('0.12 max');
    const authorize = screen.getByTestId('action-gate-authorize').element() as HTMLButtonElement;
    expect(authorize.disabled).toBe(true);

    // The burden packet's own approve action reaches the same handler, so an
    // unpriced campaign still refuses there — never mints over no estimate.
    await screen.getByRole('button', { name: /burden packet/ }).click();
    await screen.getByRole('button', { name: 'Approve once' }).click();
    await expect.poll(() => text(screen)).toContain('No cost estimate');
    expect(calls.filter((call) => call.url.includes('/api/mandates/verify'))).toHaveLength(0);
  });

  it('disables authorization without a proposed program (W0.4)', async () => {
    const calls = stubDaemon({});
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
    const screen = await render(
      <ActionGate campaign={programless} session={session} onAuthorized={() => {}} onDenied={() => {}} onDowngrade={() => {}} />,
    );

    const authorize = screen.getByTestId('action-gate-authorize').element() as HTMLButtonElement;
    expect(authorize.disabled).toBe(true);

    await screen.getByRole('button', { name: /burden packet/ }).click();
    await screen.getByRole('button', { name: 'Approve once' }).click();
    await expect.poll(() => text(screen)).toContain('No program in this campaign');
    expect(calls.filter((call) => call.url.includes('/api/mandates/verify'))).toHaveLength(0);
  });
});

describe('SwarmUniverse canary honesty in a real browser', () => {
  it('marks the template canary until a job fold collapses the campaign', async () => {
    const screen = await render(<SwarmUniverse campaign={CAMPAIGN} />);
    const critic = screen.getByTestId('probe-critic').element().textContent ?? '';
    // Fixture status is 'corroborated', not COLLAPSED_VERIFIED: the 0.998
    // template constant must read as template, never as a probe result.
    expect(critic).toContain('(template)');
    await screen.unmount();

    const verified = {
      ...CAMPAIGN,
      epistemics: { ...(CAMPAIGN as { epistemics: object }).epistemics, status: 'COLLAPSED_VERIFIED' },
    } as unknown as CampaignState;
    const screen2 = await render(<SwarmUniverse campaign={verified} />);
    expect(screen2.getByTestId('probe-critic').element().textContent).not.toContain('(template)');
  });
});
