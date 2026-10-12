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
    expect(text).toContain('job-kry'.slice(0, 8));
    expect(text).not.toContain('RUNNING');
    // The newer FAILED krylov job must not displace (or appear as) measured.
    expect(text).not.toContain('bridge blew up');
    expect(text).not.toContain('job-fail'.slice(0, 8));
  });

  it('says so when nothing completed', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: true, json: async () => ({ jobs: [] }) }));
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<MeasuredOutputs sessionId="s" />);
    });
    expect(JSON.stringify(tree!.toJSON())).toContain('No completed jobs yet');
  });

  it('stays silent when the backend is unreachable', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: false }));
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<MeasuredOutputs sessionId="s" />);
    });
    expect(JSON.stringify(tree!.toJSON())).toContain('No completed jobs yet');
  });
});
