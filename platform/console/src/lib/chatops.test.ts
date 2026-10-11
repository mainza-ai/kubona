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
    mockFetchOnce({ jobs: [{ id: 'j1', status: 'running' }] });
    await expect(fetchJobs('s')).resolves.toEqual([{ id: 'j1', status: 'running' }]);
  });

  it('fetchJobs falls back to empty on refusal', async () => {
    mockFetchOnce({}, false, 500);
    await expect(fetchJobs('s')).resolves.toEqual([]);
  });
});
