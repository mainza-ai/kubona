import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
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
    statusText: ok ? 'OK' : 'Refused',
    text: async () => (typeof body === 'string' ? body : JSON.stringify(body)),
    json: async () => body,
  });
}

beforeEach(() => {
  vi.unstubAllGlobals();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
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
  it('returns the job payload scoped by session', async () => {
    const { fetchJob } = await import('./api');
    const fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ job: { id: 'j1', status: 3 } }),
    });
    (globalThis as { fetch?: unknown }).fetch = fetch;
    await expect(fetchJob('j1', 's1')).resolves.toEqual({ id: 'j1', status: 3 });
    expect(fetch).toHaveBeenCalledWith(
      'http://localhost:8080/api/jobs/j1?session_id=s1',
      expect.objectContaining({ cache: 'no-store' })
    );
  });

  it('returns null on 404 (never an empty shell)', async () => {
    const { fetchJob } = await import('./api');
    (globalThis as { fetch?: unknown }).fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 404,
      json: async () => ({}),
    });
    await expect(fetchJob('missing', 's1')).resolves.toBeNull();
  });

  it('refuses without a session (never makes the doomed call)', async () => {
    const { fetchJob } = await import('./api');
    const fetch = vi.fn();
    (globalThis as { fetch?: unknown }).fetch = fetch;
    await expect(fetchJob('j1', '')).resolves.toBeNull();
    expect(fetch).not.toHaveBeenCalled();
  });
});

describe('spend-critical refusal paths (fail-closed)', () => {
  it('verifySpendMandate fail-closed on HTTP refusal', async () => {
    const { verifySpendMandate } = await import('./api');
    (globalThis as { fetch?: unknown }).fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 403,
      statusText: 'Forbidden',
      text: async () => '',
      json: async () => ({}),
    });
    const res = await verifySpendMandate({
      tenant_id: 'lab-a',
      job_hash: 'j',
      max_budget_usd: 10,
      expiry_timestamp: '2030-01-01T00:00:00Z',
      public_key_hex: '00',
      signature_hex: '00',
    });
    expect(res.valid).toBe(false);
    expect(res.error).toMatch(/403|Forbidden/);
  });

  it('verifySpendMandate fail-closed on transport failure', async () => {
    const { verifySpendMandate } = await import('./api');
    (globalThis as { fetch?: unknown }).fetch = vi.fn().mockRejectedValue(new Error('down'));
    const res = await verifySpendMandate({
      tenant_id: 'lab-a',
      job_hash: 'j',
      max_budget_usd: 10,
      expiry_timestamp: '2030-01-01T00:00:00Z',
      public_key_hex: '00',
      signature_hex: '00',
    });
    expect(res.valid).toBe(false);
    expect(res.error).toMatch(/down/);
  });

  it('launchProposal surfaces the backend refusal text', async () => {
    const { launchProposal } = await import('./api');
    (globalThis as { fetch?: unknown }).fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 402,
      text: async () => 'tier-routing refused: hardware tier needs approval',
      json: async () => ({}),
    });
    const res = await launchProposal('s', { target: 'x' });
    expect(res).toEqual({ error: 'tier-routing refused: hardware tier needs approval' });
  });

  it('launchProposal returns null on transport failure (never throws)', async () => {
    const { launchProposal } = await import('./api');
    (globalThis as { fetch?: unknown }).fetch = vi.fn().mockRejectedValue(new Error('bridge down'));
    await expect(launchProposal('s', { target: 'x' })).resolves.toBeNull();
  });

  it('streamChatMessage throws on refusal (fail-closed)', async () => {
    const { streamChatMessage } = await import('./api');
    (globalThis as { fetch?: unknown }).fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 429,
      body: null,
    });
    await expect(streamChatMessage('s', 'hi', () => {})).rejects.toThrow(/refused.*429/);
  });

  it('streamChatMessage throws on transport error event', async () => {
    const { streamChatMessage } = await import('./api');
    const enc = new TextEncoder();
    const frames = enc.encode('event: error\ndata: {"error":"overloaded"}\n\n');
    const reader = {
      read: vi.fn().mockResolvedValueOnce({ value: frames, done: false }).mockResolvedValueOnce({ value: undefined, done: true }),
    };
    (globalThis as { fetch?: unknown }).fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      body: { getReader: () => reader },
    });
    await expect(streamChatMessage('s', 'hi', () => {})).rejects.toThrow(/overloaded/);
  });
});
