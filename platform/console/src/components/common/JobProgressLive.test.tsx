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

describe('JobProgress live region (Wave-2)', () => {
  it('announces phase transitions politely and stays visually hidden', async () => {
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(job('JOB_STATUS_QUEUED'))
      .mockResolvedValue(job('JOB_STATUS_RUNNING'));
    vi.stubGlobal('fetch', fetch);
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<JobProgress jobId="j" sessionId="s" />);
    });
    const live = tree.root.findByProps({ 'aria-live': 'polite' });
    expect(live.props.role).toBe('status');
    expect(String(live.props.className)).toContain('sr-only');
    expect(String(live.props.children)).toContain('QUEUED');
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2100);
    });
    expect(String(tree.root.findByProps({ 'aria-live': 'polite' }).props.children)).toContain('RUNNING');
    await act(async () => {
      tree.unmount();
    });
  });
});
