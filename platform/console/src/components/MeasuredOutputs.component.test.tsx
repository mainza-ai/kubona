import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { MeasuredOutputs } from './MeasuredOutputs';

// Real-browser port of MeasuredOutputs.test.tsx. The component is driven by
// the real fetch rail (no `vi.mock('@/lib/api')`): each test stubs global
// `fetch` by URL/method with `{ ok, status, json }` and then asserts against
// the rendered DOM, including the scoping of measured <li> rows and the
// live-progress job that must never leak into them.

const JOBS = [
  {
    id: 'job-krylov-1', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'krylov_solve',
    params_json: {}, result_json: { ground_state_energy: -1.13699 }, created_at: '', completed_at: '2026-10-11T02:00:00Z',
  },
  {
    id: 'job-circuit-1', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'circuit_run',
    params_json: {}, result_json: { resolved_backend: 'aer', counts: { '00': 512, '11': 512 } },
    created_at: '', completed_at: '2026-10-11T01:00:00Z',
    compiler_used: 'go-fallback-estimate', fallback_estimate: true,
  },
  {
    id: 'job-run-1', session_id: 's', status: 'JOB_STATUS_RUNNING', job_type: 'circuit_run',
    params_json: {}, created_at: '',
  },
  {
    id: 'job-vqe-1', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'vqe_run',
    params_json: {}, result_json: {
      success: true, energy: -1.1368, fci_exact_energy: -1.13727, delta_mha: 0.47, full_space_status: 'contested',
      uncertainty: 0.001384, backend: 'qiskit-aer-simulator',
    }, created_at: '', completed_at: '2026-10-11T02:30:00Z',
  },
  {
    id: 'job-pes-1', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'vqe_pes',
    params_json: {}, result_json: {
      points: [
        { bond_distance: 0.7, energy: -1.1371, delta_mha: 0.95 },
        { bond_distance: 0.9, energy: -1.1212, delta_mha: 0.69 },
      ],
    }, created_at: '', completed_at: '2026-10-11T02:31:00Z',
  },
  {
    id: 'job-qaoa-1', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'qaoa_run',
    params_json: {}, result_json: { energy: 7.87, optimal_cut: 10, approximation_ratio: 0.787 },
    created_at: '', completed_at: '2026-10-11T02:32:00Z',
  },
  {
    id: 'job-spin-1', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'spin_chain',
    params_json: {}, result_json: {
      model: 'tfim', n_sites: 4, energy: -3.19, energy_exact: -3.21, uncertainty: 0.004,
      gap: 0.8, claimable: false,
    }, created_at: '', completed_at: '2026-10-11T02:33:00Z',
  },
  {
    id: 'job-excited-1', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'excited_state',
    params_json: {}, result_json: {
      ground_energy: -1.1346, excited_energy: -0.5312, gap: 0.6034, gap_exact: 0.6048, claimable: false,
    }, created_at: '', completed_at: '2026-10-11T02:36:00Z',
  },
  {
    id: 'job-reaction-1', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'reaction',
    params_json: {}, result_json: {
      reaction: 'H2 -> 2 H', reaction_energy: 0.1971, reaction_energy_exact: 0.2041,
      uncertainty: 0.0048, claimable: false,
    }, created_at: '', completed_at: '2026-10-11T02:37:00Z',
  },
  {
    id: 'job-spin-hw-1', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'spin_chain_hardware',
    params_json: {}, result_json: {
      model: 'tfim', n_sites: 4, energy: -3.15, energy_exact: -3.21, uncertainty: 0.02,
      claimable: false, backend: 'ibm_fez',
    }, created_at: '', completed_at: '2026-10-11T02:35:00Z',
  },
  {
    id: 'job-conj-1', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'conjecture_run',
    params_json: {}, result_json: {
      candidate_count: 1, candidates: [{ law: 'a(n) = a(n-1) + a(n-2)', status: 'conjecture' }],
    }, created_at: '', completed_at: '2026-10-11T02:34:00Z',
  },
  {
    // R7: FAILED jobs carry completed_at — they must not render as measured.
    id: 'job-fail-1', session_id: 's', status: 'JOB_STATUS_FAILED', job_type: 'krylov_solve',
    params_json: {}, result_json: { error_message: 'bridge blew up' },
    created_at: '', completed_at: '2026-10-11T03:00:00Z',
  },
];

const RUNNING_JOB = {
  job: {
    id: 'job-run-1', session_id: 's', status: 'JOB_STATUS_RUNNING', job_type: 'circuit_run',
    created_at: new Date().toISOString(),
  },
};

type StubResponse = { ok: boolean; status?: number; json: () => Promise<unknown> };

const ok = (body: unknown): StubResponse => ({ ok: true, status: 200, json: async () => body });

// One stub for the whole job rail: `/api/jobs?session_id=…` lists, and
// `/api/jobs/{id}` is the JobProgress detail poll.
function stubJobRail(listing: unknown, detail: unknown = RUNNING_JOB) {
  vi.stubGlobal('fetch', async (input: RequestInfo | URL) => {
    const url = String(input);
    const parsed = new URL(url, 'http://localhost');
    if (/^\/api\/jobs\/[^/]+$/.test(parsed.pathname)) return ok(detail);
    expect(parsed.pathname.endsWith('/api/jobs')).toBe(true);
    expect(parsed.searchParams.get('session_id')).toBe('s');
    return ok(listing);
  });
}

const waitForText = async (screen: { container: HTMLElement }, needle: string) => {
  await expect.poll(() => screen.container.textContent, { timeout: 3000 }).toContain(needle);
};

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('MeasuredOutputs in a real browser', () => {
  it('renders nothing without a session', async () => {
    const screen = await render(<MeasuredOutputs sessionId={null} />);
    expect(screen.container.textContent).toBe('');
    expect(screen.container.childElementCount).toBe(0);
  });

  it('binds latest completed jobs per type, skipping the running', async () => {
    stubJobRail({ jobs: JOBS });
    const screen = await render(<MeasuredOutputs sessionId="s" />);
    await waitForText(screen, 'krylov_solve');
    await waitForText(screen, 'RUNNING');
    const text = screen.container.textContent ?? '';

    expect(text).toContain('-1.13699');
    expect(text).toContain('classical emulation');
    expect(text).toContain('circuit_run');
    expect(text).toContain('1024 shots');
    expect(text).toContain('go-fallback-estimate');
    expect(text).toContain('(estimate)');
    expect(text).toContain('VQE -1.136800 Ha');
    expect(text).toContain('± 1.384 mHa');
    expect(text).toContain('Δ 0.470 mHa');
    expect(text).toContain('full-space contested');
    expect(text).toContain('min E R=0.7000 Å');
    expect(text).toContain('closest R=0.9000 Å');
    expect(text).toContain('QAOA MaxCut 7.870 vs classical 10');
    expect(text).toContain('ratio 0.7870');
    expect(text).toContain('spin chain tfim n=4');
    expect(text).toContain('not claimable');
    expect(text).toContain('math conjecture 1 candidate(s)');
    expect(text).toContain('not a theorem');
    expect(text).toContain('hardware spin chain tfim n=4');
    expect(text).toContain('excited state ground -1.134600');
    expect(text).toContain('reaction H2 -> 2 H');
    expect(text).toContain('job-kry');

    // The running job must not appear as a measured row (R7 gate holds);
    // it renders in the live-progress section instead (S8b/c) — including
    // its sr-only status announcer, which legitimately names the job id.
    // So the assertion is scoped to measured <li> rows, not the whole tree.
    const rows = Array.from(screen.container.querySelectorAll('li'));
    expect(rows.length).toBeGreaterThan(0);
    for (const row of rows) {
      expect(row.textContent ?? '').not.toContain('job-run-');
    }
    // The newer FAILED krylov job must not displace (or appear as) measured.
    expect(text).not.toContain('bridge blew up');
    expect(text).not.toContain('job-fail');
    // S8b/c: the running job renders live progress beside the table.
    expect(text).toContain('RUNNING');
  });

  it('says so when nothing completed', async () => {
    stubJobRail({ jobs: [] });
    const screen = await render(<MeasuredOutputs sessionId="s" />);
    await waitForText(screen, 'No completed jobs yet');
    expect(screen.container.textContent).toContain('No completed jobs yet');
  });

  it('surfaces a hardware QAOA final-state cut alongside the simulator result', async () => {
    stubJobRail({ jobs: [{
      id: 'job-qaoa-hw-1', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'circuit_run',
      params_json: {}, result_json: {
        counts: { '01': 2, '10': 1 }, resolved_backend: 'ibm_marrakesh',
        measured_cut: 0.75, approximation_ratio: 0.375, qaoa_meta: { optimal_cut: 2 },
      }, created_at: '', completed_at: '2026-11-08T10:00:00Z',
    }] });
    const screen = await render(<MeasuredOutputs sessionId="s" />);
    await waitForText(screen, 'QAOA hardware cut 0.7500');
    expect(screen.container.textContent).toContain('QAOA hardware cut 0.7500');
  });

  it('excludes wrong-molecule, FCI-incompatible, and off-domain jobs when targetScope is set', async () => {
    stubJobRail({ jobs: JOBS });
    const screen = await render(
      <MeasuredOutputs
        sessionId="s"
        targetScope={{
          targetMolecule: 'Hydrogen Molecule (H2)',
          molecularFormula: 'H2',
          researchGoal: 'Live QPU loop acceptance',
          anchorFci: -1.13727,
        }}
      />
    );
    await waitForText(screen, 'off-target jobs excluded');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('VQE -1.136800 Ha');
    expect(text).toContain('reaction H2 -> 2 H');
    expect(text).not.toContain('spin_chain');
    expect(text).not.toContain('conjecture_run');
    expect(text).not.toContain('qaoa_run');
  });

  it('stays silent when the backend is unreachable', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: false, status: 503, json: async () => ({}) }));
    const screen = await render(<MeasuredOutputs sessionId="s" />);
    await waitForText(screen, 'No completed jobs yet');
    expect(screen.container.textContent).toContain('No completed jobs yet');
  });

  it('renders governed hardware VQE/QAOA with the local-optimizer label', async () => {
    const hardwareJobs = [
      {
        id: 'job-hw-vqe', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'hardware_experiment',
        params_json: {}, created_at: '', completed_at: '2026-11-08T12:00:00Z',
        result_json: { success: true, energy: -1.1269, uncertainty: 0.0059, fci_exact_energy: -1.13727, delta_mha: 10.369, simulated_optimizer: true },
      },
      {
        id: 'job-hw-qaoa', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'qaoa_hardware',
        params_json: {}, created_at: '', completed_at: '2026-11-08T12:01:00Z',
        result_json: { success: true, measured_cut: 7.95, approximation_ratio: 0.795, simulated_optimizer: true },
      },
    ];
    stubJobRail({ jobs: hardwareJobs });
    const screen = await render(<MeasuredOutputs sessionId="s" />);
    await waitForText(screen, 'hardware VQE');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('hardware VQE');
    expect(text).toContain('local optimizer');
    expect(text).toContain('hardware QAOA');
  });
});

describe('MeasuredOutputs new science variants in a real browser', () => {
  it('renders geometry, repeated-VQE, and lattice trajectories', async () => {
    const jobs = [
      {
        id: 'job-geom', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'geometry_optimization',
        params_json: {}, created_at: '', completed_at: '2026-11-09T12:00:00Z',
        result_json: { energy: -1.105, fci_exact_energy: -1.137, optimized_bond_distance: 0.735, claimable: false },
      },
      {
        id: 'job-rep', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'vqe_repeats',
        params_json: {}, created_at: '', completed_at: '2026-11-09T12:01:00Z',
        result_json: { energy: -1.115, repeats: 2, between_run_standard_error: 0.0099, claimable: false },
      },
      {
        id: 'job-lat', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'lattice_dynamics',
        params_json: {}, created_at: '', completed_at: '2026-11-09T12:02:00Z',
        result_json: { rows: 2, cols: 2, times: [{ t: 0, total_magnetization: 0 }, { t: 0.5, total_magnetization: 0.25 }] },
      },
      {
        id: 'job-thermo', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'thermodynamic_limit',
        params_json: {}, created_at: '', completed_at: '2026-11-09T12:03:00Z',
        result_json: {
          kind: 'thermodynamic_limit', bond_dim: 32,
          finite_size: [{ n_sites: 8 }, { n_sites: 12 }],
          thermodynamic_limit: { available: true, energy_per_site: -1.27324 },
        },
      },
    ];
    stubJobRail({ jobs });
    const screen = await render(<MeasuredOutputs sessionId="s" />);
    await waitForText(screen, 'geometry optimized at R=0.73500');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('geometry optimized at R=0.73500');
    expect(text).toContain('repeated VQE 2 runs');
    expect(text).toContain('2D TFIM quench 2×2');
    expect(text).toContain('exact limit -1.273240 / site');
  });
});
