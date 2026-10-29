import { describe, it, expect, vi, afterEach, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { JobProgress } from './JobProgress';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

beforeEach(() => {
  vi.useFakeTimers();
});

function job(status: string) {
  return {
    ok: true,
    json: async () => ({
      job: { id: 'j', status, job_type: 'circuit_run', created_at: new Date(Date.now() - 65000).toISOString() },
    }),
  };
}

describe('JobProgress', () => {
  it('advances queued to completed and stops polling', async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(job('JOB_STATUS_QUEUED'))
      .mockResolvedValueOnce(job('JOB_STATUS_RUNNING'))
      .mockResolvedValue(job('JOB_STATUS_COMPLETED'));
    vi.stubGlobal('fetch', fetch);
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<JobProgress jobId="j" sessionId="s" />);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2100);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2100);
    });
    const text = JSON.stringify(tree.toJSON());
    expect(text).toContain('COMPLETED');
    const calls = fetch.mock.calls.length;
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });
    expect(fetch.mock.calls.length).toBe(calls);
  });

  it('fetches scoped by session (no more 400s)', async () => {
    const fetch = vi.fn().mockResolvedValue(job('JOB_STATUS_COMPLETED'));
    vi.stubGlobal('fetch', fetch);
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<JobProgress jobId="j" sessionId="s" />);
    });
    expect(fetch).toHaveBeenCalled();
    expect(String(fetch.mock.calls[0][0])).toBe('http://localhost:8080/api/jobs/j?session_id=s');
    expect(JSON.stringify(tree.toJSON())).toContain('COMPLETED');
  });

  it('marks failed terminal distinctly', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(job('JOB_STATUS_FAILED')));
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<JobProgress jobId="j" sessionId="s" />);
    });
    expect(JSON.stringify(tree.toJSON())).toContain('FAILED');
  });

  it('stops polling on repeated null jobs and offers retry — no infinite pulse', async () => {
    const fetch = vi.fn().mockResolvedValue({ ok: false, status: 404 });
    vi.stubGlobal('fetch', fetch);
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<JobProgress jobId="j" sessionId="s" />);
    });
    // 1 immediate + 3 interval polls reach the consecutive-null budget.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2100);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2100);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2100);
    });
    const text = JSON.stringify(tree.toJSON());
    expect(text).toContain('job unavailable');
    expect(text).toContain('Retry');
    const calls = fetch.mock.calls.length;
    // Loop stopped: no more fetches while gone.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(10000);
    });
    expect(fetch.mock.calls.length).toBe(calls);
    // Retry restarts polling.
    await act(async () => {
      await tree.root.findByType('button').props.onClick();
    });
    expect(fetch.mock.calls.length).toBeGreaterThan(calls);
  });
});
