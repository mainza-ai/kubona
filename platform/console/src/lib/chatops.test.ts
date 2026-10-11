import { describe, it, expect, vi, beforeEach } from 'vitest';
import {
  clearMessages,
  regenerateChat,
  sendFeedback,
  rewriteChat,
  fetchJobs,
} from './api';

function mockFetchOnce(body: unknown, ok = true, status = 200) {
  (globalThis as { fetch?: unknown }).fetch = vi.fn().mockResolvedValue({
    ok,
    status,
    json: async () => body,
  });
}

beforeEach(() => {
  vi.unstubAllGlobals();
});

describe('chat ops client', () => {
  it('clearMessages returns the deleted count', async () => {
    mockFetchOnce({ cleared: true, deleted: 4, session_id: 's' });
    await expect(clearMessages('s')).resolves.toBe(4);
  });

  it('clearMessages returns null on refusal', async () => {
    mockFetchOnce({ error: 'nope' }, false, 500);
    await expect(clearMessages('s')).resolves.toBeNull();
  });

  it('regenerateChat backfills model and duration', async () => {
    const assistant = { id: 'a', role: 'assistant', content: 'again' };
    mockFetchOnce({ assistant, model: 'deepseek-v4.1-flash', duration_ms: 19000 });
    const res = await regenerateChat('s');
    expect(res?.assistant.model).toBe('deepseek-v4.1-flash');
    expect(res?.assistant.duration_ms).toBe(19000);
  });

  it('sendFeedback posts the vote and reports status', async () => {
    mockFetchOnce({ recorded: true });
    await expect(sendFeedback('s', 'm', 'up')).resolves.toBe(true);
    mockFetchOnce({}, false, 404);
    await expect(sendFeedback('s', 'm', 'down')).resolves.toBe(false);
  });

  it('rewriteChat returns the rewritten message', async () => {
    const message = { id: 'u', content: 'edited' };
    mockFetchOnce({ rewritten: true, message });
    await expect(rewriteChat('s', 'u', 'edited')).resolves.toEqual(message);
  });

  it('fetchJobs returns the session job list', async () => {
    mockFetchOnce({ jobs: [{ id: 'j1', status: 2 }] });
    await expect(fetchJobs('s')).resolves.toEqual([{ id: 'j1', status: 2 }]);
  });

  it('fetchJobs falls back to empty on refusal', async () => {
    mockFetchOnce({}, false, 500);
    await expect(fetchJobs('s')).resolves.toEqual([]);
  });
});

describe('jobStatusLabel', () => {
  it('maps numeric wire values to canonical labels', async () => {
    const { jobStatusLabel, isTerminalJobStatus } = await import('./api');
    expect(jobStatusLabel(1)).toBe('QUEUED');
    expect(jobStatusLabel(2)).toBe('RUNNING');
    expect(jobStatusLabel(3)).toBe('COMPLETED');
    expect(jobStatusLabel(4)).toBe('FAILED');
    expect(jobStatusLabel('JOB_STATUS_COMPLETED')).toBe('COMPLETED');
    expect(jobStatusLabel('completed')).toBe('COMPLETED');
    expect(isTerminalJobStatus(3)).toBe(true);
    expect(isTerminalJobStatus(4)).toBe(true);
    expect(isTerminalJobStatus(2)).toBe(false);
    expect(isTerminalJobStatus('running')).toBe(false);
  });
});

describe('fetchJob', () => {
  it('returns the job payload', async () => {
    const { fetchJob } = await import('./api');
    (globalThis as { fetch?: unknown }).fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ job: { id: 'j1', status: 3 } }),
    });
    await expect(fetchJob('j1')).resolves.toEqual({ id: 'j1', status: 3 });
  });

  it('returns null on 404 (never an empty shell)', async () => {
    const { fetchJob } = await import('./api');
    (globalThis as { fetch?: unknown }).fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      json: async () => ({}),
    });
    await expect(fetchJob('missing')).resolves.toBeNull();
  });
});
