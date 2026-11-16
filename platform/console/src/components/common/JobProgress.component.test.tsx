import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { JobProgress } from './JobProgress';
import { resetSharedResourcesForTests } from '@/domain/shared-resource';

function job(status: string) {
  return {
    ok: true,
    status: 200,
    json: async () => ({
      job: { id: 'j', status, job_type: 'circuit_run', created_at: new Date(Date.now() - 65000).toISOString() },
    }),
  };
}

function missing() {
  return { ok: false, status: 404, text: async () => '' };
}

beforeEach(() => {
  resetSharedResourcesForTests();
  vi.useFakeTimers();
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('JobProgress polling in a real browser', () => {
  it('advances queued to completed and stops polling', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(job('JOB_STATUS_QUEUED'))
      .mockResolvedValueOnce(job('JOB_STATUS_RUNNING'))
      .mockResolvedValue(job('JOB_STATUS_COMPLETED'));
    vi.stubGlobal('fetch', fetchMock);

    const screen = await render(<JobProgress jobId="j" sessionId="s" />);
    await expect.poll(() => screen.container.textContent).toContain('QUEUED');
    await expect.element(screen.getByLabelText('Job JOB_STATUS_QUEUED')).toBeVisible();

    await vi.advanceTimersByTimeAsync(2100);
    await expect.poll(() => screen.container.textContent).toContain('RUNNING');
    await expect.element(screen.getByLabelText('Job JOB_STATUS_RUNNING')).toBeVisible();

    await vi.advanceTimersByTimeAsync(2100);
    await expect.poll(() => screen.container.textContent).toContain('COMPLETED');
    // Measured elapsed time from the record, not an invented progress bar.
    expect(screen.container.textContent).toMatch(/\d+m \d+s/);
    expect(screen.container.textContent).toContain('circuit_run');

    // Terminal status ends the loop: further time passes with no more fetches.
    const calls = fetchMock.mock.calls.length;
    expect(calls).toBe(3);
    await vi.advanceTimersByTimeAsync(10000);
    expect(fetchMock.mock.calls.length).toBe(calls);
  });

  it('fetches scoped by session (no more 400s)', async () => {
    const fetchMock = vi.fn().mockResolvedValue(job('JOB_STATUS_COMPLETED'));
    vi.stubGlobal('fetch', fetchMock);

    const screen = await render(<JobProgress jobId="j" sessionId="s" />);
    await expect.poll(() => fetchMock.mock.calls.length).toBeGreaterThan(0);
    expect(String(fetchMock.mock.calls[0][0])).toBe('http://localhost:8080/api/jobs/j?session_id=s');
    await expect.poll(() => screen.container.textContent).toContain('COMPLETED');
  });

  it('marks failed terminal distinctly', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(job('JOB_STATUS_FAILED')));

    const screen = await render(<JobProgress jobId="j" sessionId="s" />);
    await expect.poll(() => screen.container.textContent).toContain('FAILED');
    await expect.element(screen.getByLabelText('Job JOB_STATUS_FAILED')).toBeVisible();

    const failedChip = Array.from(screen.container.querySelectorAll('span')).find((span) => span.textContent === 'FAILED');
    expect(failedChip).toBeTruthy();
    // Failure is visually distinct from the success wash used by COMPLETED.
    expect(failedChip!.className).toContain('bg-danger-wash');
    expect(failedChip!.className).not.toContain('bg-success-wash');
  });

  it('stops polling on repeated null jobs and offers retry — no infinite pulse', async () => {
    const fetchMock = vi.fn().mockResolvedValue(missing());
    vi.stubGlobal('fetch', fetchMock);

    const screen = await render(<JobProgress jobId="j" sessionId="s" />);
    // 1 immediate + 2 interval polls reach the consecutive-null budget (3).
    await vi.advanceTimersByTimeAsync(2100);
    await vi.advanceTimersByTimeAsync(2100);

    await expect.poll(() => screen.container.textContent).toContain('job unavailable');
    expect(screen.container.textContent).toContain('Retry');
    await expect.element(screen.getByRole('alert', { name: 'Job j unavailable' })).toBeVisible();

    const calls = fetchMock.mock.calls.length;
    // Loop stopped: no more fetches while gone.
    await vi.advanceTimersByTimeAsync(10000);
    expect(fetchMock.mock.calls.length).toBe(calls);

    // Retry restarts polling from a fresh component instance.
    await screen.getByRole('button', { name: 'Retry' }).click();
    await vi.advanceTimersByTimeAsync(0);
    await expect.poll(() => fetchMock.mock.calls.length).toBeGreaterThan(calls);
  });
});
