import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { MeasuredOutputs } from './MeasuredOutputs';

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

describe('MeasuredOutputs', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders nothing without a session', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<MeasuredOutputs sessionId={null} />);
    });
    expect(tree!.toJSON()).toBeNull();
  });

  it('binds latest completed jobs per type, skipping the running', async () => {
    vi.stubGlobal('fetch', async (url: string) => {
      if (String(url).includes('/api/jobs/job-run-1')) {
        return { ok: true, json: async () => ({ job: { id: 'job-run-1', status: 'JOB_STATUS_RUNNING', job_type: 'circuit_run', created_at: new Date().toISOString() } }) };
      }
      expect(String(url)).toContain('/api/jobs?session_id=s');
      return { ok: true, json: async () => ({ jobs: JOBS }) };
    });
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<MeasuredOutputs sessionId="s" />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('krylov_solve');
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
    expect(text).toContain('job-kry'.slice(0, 8));
    // The running job must not appear as a measured row (R7 gate holds);
    // it renders in the live-progress section instead (S8b/c) — including
    // its sr-only status announcer, which legitimately names the job id.
    // So the assertion is scoped to measured <li> rows, not the whole tree.
    const textOf = (node: unknown): string => {
      if (typeof node === 'string' || typeof node === 'number') return String(node);
      if (node == null || typeof node !== 'object') return '';
      const kids = (node as { children?: unknown; props?: { children?: unknown } }).children
        ?? (node as { props?: { children?: unknown } }).props?.children;
      return Array.isArray(kids) ? kids.map(textOf).join('') : textOf(kids);
    };
    const rows = tree!.root.findAllByType('li');
    expect(rows.length).toBeGreaterThan(0);
    for (const row of rows) {
      expect(textOf(row)).not.toContain('job-run-');
    }
    // The newer FAILED krylov job must not displace (or appear as) measured.
    expect(text).not.toContain('bridge blew up');
    expect(text).not.toContain('job-fail'.slice(0, 8));
    // S8b/c: the running job renders live progress beside the table.
    expect(text).toContain('RUNNING');
  });

  it('says so when nothing completed', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: true, json: async () => ({ jobs: [] }) }));
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<MeasuredOutputs sessionId="s" />);
    });
    expect(JSON.stringify(tree!.toJSON())).toContain('No completed jobs yet');
  });

  it('surfaces a hardware QAOA final-state cut alongside the simulator result', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: true, json: async () => ({ jobs: [{
      id: 'job-qaoa-hw-1', session_id: 's', status: 'JOB_STATUS_COMPLETED', job_type: 'circuit_run',
      params_json: {}, result_json: {
        counts: { '01': 2, '10': 1 }, resolved_backend: 'ibm_marrakesh',
        measured_cut: 0.75, approximation_ratio: 0.375, qaoa_meta: { optimal_cut: 2 },
      }, created_at: '', completed_at: '2026-11-08T10:00:00Z',
    }] }) }));
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<MeasuredOutputs sessionId="s" />); });
    expect(JSON.stringify(tree!.toJSON())).toContain('QAOA hardware cut 0.7500');
  });

  it('stays silent when the backend is unreachable', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: false }));
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<MeasuredOutputs sessionId="s" />);
    });
    expect(JSON.stringify(tree!.toJSON())).toContain('No completed jobs yet');
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
    vi.stubGlobal('fetch', async () => ({ ok: true, json: async () => ({ jobs: hardwareJobs }) }));
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<MeasuredOutputs sessionId="s" />); });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('hardware VQE');
    expect(text).toContain('local optimizer');
    expect(text).toContain('hardware QAOA');
  });
});
