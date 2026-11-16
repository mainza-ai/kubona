import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { JobProgress } from './JobProgress';
import { resetSharedResourcesForTests } from '@/domain/shared-resource';

function job(status: string) {
  return {
    ok: true, status: 200,
    json: async () => ({ job: { id: 'j', status, job_type: 'circuit_run', created_at: new Date(Date.now() - 65000).toISOString() } }),
  };
}

beforeEach(() => { resetSharedResourcesForTests(); vi.useFakeTimers(); });
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });

describe('JobProgress live region in a real browser', () => {
  it('announces phase transitions politely and stays visually hidden', async () => {
    vi.stubGlobal('fetch', vi.fn()
      .mockResolvedValueOnce(job('JOB_STATUS_QUEUED'))
      .mockResolvedValue(job('JOB_STATUS_RUNNING')));
    const screen = await render(<JobProgress jobId="j" sessionId="s" />);
    const live = screen.getByRole('status');
    await expect.poll(() => live.element().textContent).toContain('QUEUED');
    expect(live.element().className).toContain('sr-only');
    await vi.advanceTimersByTimeAsync(2100);
    await expect.poll(() => live.element().textContent).toContain('RUNNING');
  });
});
