import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { ResultsStudio } from './ResultsStudio';
import type { CampaignState } from '@/types';

function campaign(status: string) {
  return {
    target_system: 'H2',
    notebook: {
      title: 'H2 notebook',
      quantum_energy: -1.1368,
      classical_energy: -1.13727,
      delta_mha: 0.47,
      provenance_hash: 'sha256:abc123',
      sections: ['1. objective'],
    },
    epistemics: {
      status,
      alpha_corroborated: 0.5,
      beta_contested: 0.1,
      gamma_unverified: 0.4,
      p_correlated: 25,
      statement: 'test',
      sources: [],
    },
    quantum_factors: {},
    trajectory: [],
  } as unknown as CampaignState;
}

const MEASURED = {
  job_id: 'job-1', claim_id: 'claim-1', method: 'VQE', backend: 'qiskit-aer-simulator',
  energy: -1.1378, fci_exact_energy: -1.13727, uncertainty_mha: 1.384, delta_mha: 0.513,
  chemical_accuracy: true, converged: false, measured: true, below_fci: true,
};

type Call = { method: string; path: string; search: string; body: Record<string, unknown> | undefined };

function json(body: unknown) {
  return { ok: true, status: 200, json: async () => body };
}

type StudioStub = {
  /** Jobs returned by GET /api/jobs (the session projection). */
  jobs?: unknown[];
  /** Answer for POST /api/chat/messages (annotation → swarm). */
  chat?: () => unknown;
};

// The studio reads the real transport wrappers: the job projection, the
// claim history and the annotation prompt are all answered at fetch level.
function installFetch(stub: StudioStub = {}) {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const parsed = new URL(String(url));
    const path = parsed.pathname;
    const method = (init?.method ?? 'GET').toUpperCase();
    const body = init?.body === undefined ? undefined : JSON.parse(String(init.body)) as Record<string, unknown>;
    calls.push({ method, path, search: parsed.search, body });
    if (path === '/api/jobs' && method === 'GET') {
      return json({ jobs: stub.jobs ?? [] });
    }
    if (/\/claims$/.test(path)) {
      return json({ claims: [], offset: 0, limit: 50, next_offset: 0, has_more: false });
    }
    if (path === '/api/chat/messages' && method === 'POST') {
      return stub.chat ? stub.chat() : json({ user: {}, assistant: {} });
    }
    if (path === '/api/results/notebooks') {
      return json({ notebooks: [] });
    }
    throw new Error(`unexpected fetch ${method} ${path}`);
  });
  return calls;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('ResultsStudio measured distribution in a real browser', () => {
  it('charts the latest completed job counts with job provenance', async () => {
    installFetch({
      jobs: [{
        id: 'job-42',
        session_id: 'sess-test-456',
        status: 3,
        job_type: 'circuit_run',
        params_json: {},
        result_json: { resolved_backend: 'local_simulator', counts: { '00': 512, '11': 488 } },
        created_at: '2026-11-01T10:00:00Z',
        completed_at: '2026-11-01T10:01:00Z',
      }],
    });

    const screen = await render(
      <ResultsStudio campaign={campaign('COLLAPSED_VERIFIED')} sessionId="sess-test-456" />,
    );

    await expect.poll(
      () => screen.container.querySelectorAll('[data-testid="k-chart"]').length,
      { timeout: 3000 },
    ).toBe(2);
    const text = screen.container.textContent ?? '';
    expect(text).toContain('Measured bitstring distribution');
    expect(text).toContain('job-42');
    expect(text).toContain('local_simulator');
  });
});

describe('ResultsStudio verification gating (J1) in a real browser', () => {
  it('withholds a manuscript on a PROJECTED campaign even when numbers exist', async () => {
    installFetch();
    const screen = await render(<ResultsStudio campaign={campaign('PROJECTED')} sessionId={null} />);

    const text = screen.container.textContent ?? '';
    expect(text).toContain('No citable result is recorded');
    expect(text).toContain('No manuscript, citation, or proof receipt yet');
    expect(text).not.toContain('converged to an energy');
    expect(text).not.toContain('PROJECTED DRAFT');
    expect(text).not.toContain('Variational Exploration Active');
    expect(screen.container.querySelectorAll('[data-testid="k-chart"]')).toHaveLength(0);
  });

  it('states evaluated claims on a COLLAPSED_VERIFIED campaign', async () => {
    installFetch();
    const screen = await render(<ResultsStudio campaign={campaign('COLLAPSED_VERIFIED')} sessionId={null} />);

    const text = screen.container.textContent ?? '';
    expect(text).toContain('measured variational quantum eigensolver result');
    expect(text).not.toContain('The optimization converged');
    expect(text).toContain('COLLAPSED_VERIFIED');
    expect(text).not.toContain('PROJECTED DRAFT');
    // The whole studio is tagged with its provenance class.
    expect(screen.container.querySelector('[data-provenance="measured"]')).not.toBeNull();
  });

  it('renders the PES point table from structured campaign data', async () => {
    installFetch();
    const measured = {
      ...campaign('COLLAPSED_VERIFIED'),
      measurement: MEASURED,
      pes_points: [
        { bond_distance: 0.7, energy: -1.1371, fci_exact_energy: -1.1362, delta_mha: 0.946, uncertainty_mha: 1.867, chemical_accuracy: true },
      ],
    } as unknown as CampaignState;

    const screen = await render(<ResultsStudio campaign={measured} sessionId={null} />);

    const text = screen.container.textContent ?? '';
    expect(text).toContain('PES points');
    expect(text).toContain('0.7000');
    expect(text).toContain('0.946');
  });

  it('renders the typed measured-experiment table for spin and excited records', async () => {
    installFetch();
    const measured = {
      ...campaign('COLLAPSED_VERIFIED'),
      experiments: [
        { job_id: 'job-spin-1', kind: 'spin_chain', model: 'tfim', n_sites: 4, energy: -3.19, energy_exact: -3.21, gap: 0.8, uncertainty: 0.004, claimable: false, measured: true, hardware: false },
        { job_id: 'job-exc-1', kind: 'excited_state', molecule: 'H2', ground_energy: -1.1346, excited_energy: -0.5312, gap: 0.6034, uncertainty: 0.005, claimable: false, measured: true, hardware: false },
      ],
    } as unknown as CampaignState;

    const screen = await render(<ResultsStudio campaign={measured} sessionId={null} />);

    const text = screen.container.textContent ?? '';
    expect(text).toContain('Measured experiments');
    expect(text).toContain('spin_chain');
    expect(text).toContain('excited_state');
    expect(text).toContain('not claimable');
  });

  it('offers a measured CSV export only when a result exists', async () => {
    installFetch();
    const createObjectURL = vi.spyOn(URL, 'createObjectURL').mockReturnValue('blob:measured');
    // Keep the browser from navigating to the blob URL: the anchor click is
    // intercepted, but the blob handed to it is still the real export.
    const anchorClick = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined);

    const measured = {
      ...campaign('COLLAPSED_VERIFIED'),
      measurement: MEASURED,
    } as unknown as CampaignState;

    const screen = await render(<ResultsStudio campaign={measured} sessionId={null} />);
    const csv = screen.getByTestId('result-csv');
    expect(disabled(csv)).toBe(false);

    await csv.click();
    await expect.poll(() => createObjectURL.mock.calls.length, { timeout: 3000 }).toBe(1);
    expect(anchorClick).toHaveBeenCalledTimes(1);
    const blob = createObjectURL.mock.calls[0][0] as Blob;
    const text = await blob.text();
    // No PES scan → the measurement summary row is the export.
    expect(text.split('\n')[0]).toContain('job_id,claim_id,method');
    expect(text).toContain('job-1');
    expect(text).toContain('-1.1378');

    // With stored PES points the same button exports the scan instead.
    await screen.unmount();
    const withPes = {
      ...campaign('COLLAPSED_VERIFIED'),
      measurement: MEASURED,
      pes_points: [
        { bond_distance: 0.7, energy: -1.1371, fci_exact_energy: -1.1362, delta_mha: 0.946, uncertainty_mha: 1.867, chemical_accuracy: true },
      ],
    } as unknown as CampaignState;
    const scan = await render(<ResultsStudio campaign={withPes} sessionId={null} />);
    await scan.getByTestId('result-csv').click();
    await expect.poll(() => createObjectURL.mock.calls.length, { timeout: 3000 }).toBe(2);
    const pesCsv = await (createObjectURL.mock.calls[1][0] as Blob).text();
    expect(pesCsv.split('\n')[0]).toContain('bond_distance_angstrom');
    expect(pesCsv).toContain('0.7');
  });

  it('renders an empty citable state with no notebook telemetry', async () => {
    installFetch();
    const screen = await render(
      <ResultsStudio campaign={{ target_system: 'H2' } as unknown as CampaignState} sessionId={null} />,
    );

    const text = screen.container.textContent ?? '';
    expect(text).toContain('No citable result is recorded');
    expect(text).not.toContain('UNVERIFIED DRAFT');
    expect(text).not.toContain('placeholders');
  });

  it('marks the optimistic annotation failed (never silently open) when the send returns null', async () => {
    // A 200 with no user/assistant records is the transport-null shape the
    // wrapper reports as "send returned null".
    installFetch({ chat: () => json({}) });

    const screen = await render(
      <ResultsStudio campaign={campaign('COLLAPSED_VERIFIED')} sessionId="sess-test-456" />,
    );

    const input = screen.getByLabelText('Annotate §Abstract');
    await input.fill('Check signs on overlap integrals');
    await screen.getByRole('button', { name: /Prompt Swarm to Re-verify/ }).click();

    await expect.poll(() => screen.container.textContent, { timeout: 3000 }).toContain('returned null');
    const text = screen.container.textContent ?? '';
    // The comment stays visible but is explicitly failed — never 'open'.
    expect(text).toContain('Check signs on overlap integrals');
    expect(text).toContain('failed');
    expect(text).toContain('returned null');
  });

  it('submits human annotation and dispatches re-verification prompt to active session', async () => {
    const calls = installFetch();

    const screen = await render(
      <ResultsStudio campaign={campaign('COLLAPSED_VERIFIED')} sessionId="sess-test-456" />,
    );

    const input = screen.getByLabelText('Annotate §Abstract');
    await input.fill('Check signs on overlap integrals');
    await screen.getByRole('button', { name: /Prompt Swarm to Re-verify/ }).click();

    await expect.poll(
      () => calls.filter((c) => c.path === '/api/chat/messages').length,
      { timeout: 3000 },
    ).toBe(1);
    const sent = calls.find((c) => c.path === '/api/chat/messages')!;
    expect(sent.method).toBe('POST');
    // The prompt is dispatched to the active session, not a default one.
    expect(new URLSearchParams(sent.search).get('session_id')).toBe('sess-test-456');
    expect(sent.body?.session_id).toBe('sess-test-456');
    expect(String(sent.body?.content)).toContain(
      '[Human Review on §Abstract]: Check signs on overlap integrals',
    );
    // The optimistic annotation is acknowledged once the swarm took it.
    await expect.poll(() => screen.container.textContent, { timeout: 3000 }).toContain('addressed');
    expect(screen.container.textContent).toContain('Check signs on overlap integrals');
  });
});

describe('ResultsStudio non-molecular experiment visibility in a real browser', () => {
  it('shows measured non-molecular experiments even before a molecular claim exists', async () => {
    installFetch();
    const projected = {
      ...campaign('PROJECTED'),
      experiments: [
        { job_id: 'job-spin-early', kind: 'spin_chain', model: 'tfim', n_sites: 4, energy: -3.19, energy_exact: -3.21, gap: 0.8, uncertainty: 0.004, claimable: false, measured: true, hardware: false },
        { job_id: 'job-conj-early', kind: 'conjecture_run', candidate_count: 1, novelty_status: 'known', laws: ['a(n) = a(n-1) + a(n-2) [conjecture]'], claimable: false, measured: false, hardware: false },
        { job_id: 'job-failed-early', kind: 'spin_chain', status: 'failed', error: 'bridge unavailable', claimable: false, measured: false, hardware: false },
      ],
      result_receipts: [{
        job_id: 'job-spin-early', kind: 'spin_chain', result_version: 'experiment-result-v1',
        result_receipt: { algorithm: 'ed25519', ledger_entry_hash: 'ledger-hash-1', result_sha256: 'result-hash-1', signed: true, signature: 'sig' },
      }],
    } as unknown as CampaignState;

    const screen = await render(<ResultsStudio campaign={projected} sessionId={null} />);

    const text = screen.container.textContent ?? '';
    expect(text).toContain('Measured experiments');
    expect(text).toContain('spin_chain');
    expect(text).toContain('conjecture_run');
    expect(text).toContain('Download experiment report');
    expect(text).toContain('failed');
    expect(text).toContain('bridge unavailable');
    expect(text).toContain('Result receipts');
    expect(text).toContain('ledger-hash-1');
    expect(text).toContain('signed');
  });
});

function disabled(locator: { element(): Element }): boolean {
  return (locator.element() as HTMLButtonElement).disabled;
}
