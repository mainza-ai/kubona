import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { ExperimentPanel } from './ExperimentPanel';

// Migrated from react-test-renderer (ExperimentPanel.test.tsx). The old suite
// mocked runExperiment/fetchJob and invoked props.onClick() directly; this one
// drives the real form controls in Chromium and answers the transport wrappers
// at the fetch boundary, so the submitted spec is the one the operator built
// and the result line is read from the polled job record.
type Call = { method: string; path: string; body?: Record<string, unknown> };

type JobRecord = Record<string, unknown>;

function ok(payload: unknown) {
  return { ok: true, status: 200, json: async () => payload };
}

function job(id: string, jobType: string, status: string, result?: JobRecord): JobRecord {
  return {
    id,
    session_id: 's',
    status,
    job_type: jobType,
    params_json: {},
    result_json: result,
    created_at: '',
    completed_at: '',
  };
}

// Poll answers are consumed in order; the last one repeats, so a submission
// that settles on the first read never blocks the loop.
function installFetch(routes: { submit: JobRecord; polls: JobRecord[] }) {
  const calls: Call[] = [];
  let pollIndex = 0;
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const parsed = new URL(String(url));
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push({
      method,
      path: parsed.pathname,
      body: init?.body ? (JSON.parse(String(init.body)) as Record<string, unknown>) : undefined,
    });
    if (method === 'POST' && parsed.pathname === '/api/experiments') {
      return ok({ job: routes.submit });
    }
    if (method === 'GET' && parsed.pathname.startsWith('/api/jobs/')) {
      const next = routes.polls[Math.min(pollIndex, routes.polls.length - 1)];
      pollIndex += 1;
      return ok({ job: next });
    }
    return {
      ok: false,
      status: 404,
      json: async () => ({ error: `unexpected ${method} ${parsed.pathname}` }),
      text: async () => `unexpected ${method} ${parsed.pathname}`,
    };
  });
  return calls;
}

function submittedSpec(calls: Call[]): Record<string, unknown> {
  const submit = calls.filter((c) => c.method === 'POST' && c.path === '/api/experiments');
  expect(submit).toHaveLength(1);
  const body = submit[0].body as { session_id?: string; spec?: Record<string, unknown> };
  expect(body.session_id).toBe('s');
  return body.spec as Record<string, unknown>;
}

function polls(calls: Call[]) {
  return calls.filter((c) => c.method === 'GET' && c.path.startsWith('/api/jobs/'));
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('ExperimentPanel in a real browser', () => {
  it('submits a spin-chain experiment with the selected model', async () => {
    const calls = installFetch({
      submit: job('job-spin-ui', 'spin_chain', 'JOB_STATUS_QUEUED'),
      polls: [job('job-spin-ui', 'spin_chain', 'JOB_STATUS_COMPLETED', {
        energy: -3.1, energy_exact: -3.2, uncertainty: 0.01, gap: 0.7, claimable: false, model: 'tfim', n_sites: 4,
      })],
    });
    const screen = await render(<ExperimentPanel sessionId="s" />);
    await screen.getByLabelText('Experiment kind').selectOptions('spin_chain');
    await screen.getByRole('button', { name: /Run experiment/ }).click();

    await expect.poll(() => polls(calls).length, { timeout: 15000 }).toBe(1);
    const spec = submittedSpec(calls);
    expect(spec.kind).toBe('spin_chain');
    expect(spec.model).toBe('tfim');
    expect(spec.n_sites).toBe(4);
    expect(spec.shots).toBe(16384);

    // The rendered result is the polled job record, not a client simulation.
    await expect.poll(() => screen.container.textContent ?? '', { timeout: 15000 }).toContain('spin E -3.100000 vs exact -3.200000');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('not claimable');
    expect(polls(calls)[0].path).toBe('/api/jobs/job-spin-ui');
  });

  it('submits exact conjecture terms and labels the result as non-theorem', async () => {
    const calls = installFetch({
      submit: job('job-conj-ui', 'conjecture_run', 'JOB_STATUS_QUEUED'),
      polls: [job('job-conj-ui', 'conjecture_run', 'JOB_STATUS_COMPLETED', {
        candidate_count: 1, candidates: [{ law: 'a(n) = a(n-1) + a(n-2)', status: 'conjecture' }],
      })],
    });
    const screen = await render(<ExperimentPanel sessionId="s" />);
    await screen.getByLabelText('Experiment kind').selectOptions('conjecture');
    await screen.getByRole('button', { name: /Run experiment/ }).click();

    await expect.poll(() => polls(calls).length, { timeout: 15000 }).toBe(1);
    const spec = submittedSpec(calls);
    expect(spec.kind).toBe('conjecture');
    expect(spec.values).toEqual(['1', '1', '2', '3', '5', '8', '13', '21', '34']);

    await expect.poll(() => screen.container.textContent ?? '', { timeout: 15000 }).toContain('not a theorem/claim');
    expect(screen.container.textContent ?? '').toContain('a(n) = a(n-1) + a(n-2) [conjecture]');
  });
});

describe('ExperimentPanel custom molecule in a real browser', () => {
  it('submits an explicit PySCF geometry and active space', async () => {
    const calls = installFetch({
      submit: job('job-custom', 'vqe_run', 'JOB_STATUS_QUEUED'),
      polls: [job('job-custom', 'vqe_run', 'JOB_STATUS_COMPLETED', { energy: -1.1, fci_exact_energy: -1.13, claimable: false })],
    });
    const screen = await render(<ExperimentPanel sessionId="s" />);
    await screen.getByLabelText('Experiment molecule').selectOptions('custom');
    await screen.getByLabelText('Active orbitals').fill('2');
    await screen.getByLabelText('Active electrons').fill('2');
    await screen.getByRole('button', { name: /Run experiment/ }).click();

    await expect.poll(() => polls(calls).length, { timeout: 15000 }).toBe(1);
    const spec = submittedSpec(calls);
    expect(spec.molecule).toBe('custom');
    expect(String(spec.atom)).toContain('H 0 0 0');
    expect(spec.ncas).toBe(2);
    expect(spec.nelecas).toBe(2);
    expect(spec.basis).toBe('sto-3g');
  });
});

describe('ExperimentPanel multidimensional geometry in a real browser', () => {
  it('submits a custom atom geometry for full relaxation', async () => {
    const calls = installFetch({
      submit: job('job-geom-custom', 'geometry_optimization', 'JOB_STATUS_QUEUED'),
      polls: [job('job-geom-custom', 'geometry_optimization', 'JOB_STATUS_COMPLETED', {
        energy: -74.8, fci_exact_energy: -75.0, claimable: false,
        optimized_geometry: [{ symbol: 'O' }, { symbol: 'H' }, { symbol: 'H' }],
        relaxation: { iterations: 6 },
      })],
    });
    const screen = await render(<ExperimentPanel sessionId="s" />);
    await screen.getByLabelText('Experiment kind').selectOptions('geometry_optimization');
    await screen.getByLabelText('Experiment molecule').selectOptions('custom');
    await screen.getByLabelText('Custom atom geometry').fill('O 0 0 0; H 0.7586 0 0.5043; H -0.7586 0 0.5043');
    await screen.getByLabelText('Active orbitals').fill('4');
    await screen.getByLabelText('Active electrons').fill('4');
    await screen.getByRole('button', { name: /Run experiment/ }).click();

    await expect.poll(() => polls(calls).length, { timeout: 15000 }).toBe(1);
    const spec = submittedSpec(calls);
    expect(spec.kind).toBe('geometry_optimization');
    expect(spec.molecule).toBe('custom');
    expect(String(spec.atom)).toContain('O 0 0 0');
    expect(spec.ncas).toBe(4);
    expect(spec.nelecas).toBe(4);
    expect(spec.initial_bond_distance).toBeUndefined();

    await expect.poll(() => screen.container.textContent ?? '', { timeout: 15000 }).toContain('3 atoms relaxed · BFGS 6 steps');
  });
});

describe('ExperimentPanel thermodynamic limit in a real browser', () => {
  it('submits DMRG finite sizes and bond dimension', async () => {
    const calls = installFetch({
      submit: job('job-thermo', 'thermodynamic_limit', 'JOB_STATUS_QUEUED'),
      polls: [job('job-thermo', 'thermodynamic_limit', 'JOB_STATUS_COMPLETED', {
        kind: 'thermodynamic_limit', bond_dim: 32, finite_size: [{ n_sites: 8 }, { n_sites: 12 }],
        thermodynamic_limit: { available: true, energy_per_site: -1.27324 }, claimable: false,
      })],
    });
    const screen = await render(<ExperimentPanel sessionId="s" />);
    await screen.getByLabelText('Experiment kind').selectOptions('thermodynamic_limit');
    await screen.getByRole('button', { name: /Run experiment/ }).click();

    await expect.poll(() => polls(calls).length, { timeout: 15000 }).toBe(1);
    const spec = submittedSpec(calls);
    expect(spec.kind).toBe('thermodynamic_limit');
    expect(spec.sizes).toEqual([8, 12, 16]);
    expect(spec.bond_dim).toBe(32);
    expect(spec.sweeps).toBe(8);
    expect(spec.field).toBe(0.8);

    await expect.poll(() => screen.container.textContent ?? '', { timeout: 15000 })
      .toContain('2 sizes · DMRG bond dim 32 · exact limit -1.273240 / site');
  });

  // Real timers: the panel's poll loop sleeps 1000 ms between reads, so a
  // RUNNING -> COMPLETED transition costs ~2 s of wall clock. That is the
  // production cadence and is left intact instead of faking time.
  it('continues polling through JOB_STATUS_RUNNING until JOB_STATUS_COMPLETED and supports XXZ anisotropy', { timeout: 30000 }, async () => {
    const calls = installFetch({
      submit: job('job-xxz', 'spin_chain', 'JOB_STATUS_QUEUED'),
      polls: [
        job('job-xxz', 'spin_chain', 'JOB_STATUS_RUNNING'),
        job('job-xxz', 'spin_chain', 'JOB_STATUS_COMPLETED', {
          energy: -3.5, energy_exact: -3.5, uncertainty: 0.005, gap: 0.4, claimable: true, model: 'xxz', n_sites: 4,
        }),
      ],
    });
    const screen = await render(<ExperimentPanel sessionId="s" />);
    await screen.getByLabelText('Experiment kind').selectOptions('spin_chain');
    await screen.getByLabelText('Spin model').selectOptions('xxz');
    await screen.getByLabelText('Spin anisotropy').fill('1.5');
    await screen.getByRole('button', { name: /Run experiment/ }).click();

    await expect.poll(() => polls(calls).length, { timeout: 15000 }).toBe(2);
    const spec = submittedSpec(calls);
    expect(spec.anisotropy).toBe(1.5);
    expect(spec.model).toBe('xxz');

    await expect.poll(() => screen.container.textContent ?? '', { timeout: 15000 })
      .toContain('spin E -3.500000 vs exact -3.500000');
    expect(screen.container.textContent ?? '').toContain('claimable');
  });
});
