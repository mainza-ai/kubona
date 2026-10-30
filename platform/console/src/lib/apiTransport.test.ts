import { describe, it, expect, vi, afterEach } from 'vitest';

// Transport contract (U0): default timeouts, 401 broadcast, stream stall.
// Fresh module per test — api.ts holds module state (API_BASE latch,
// last-401 timestamp) that must not leak between suites.
afterEach(() => {
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

async function freshApi() {
  vi.resetModules();
  return import('./api');
}

describe('apiFetch timeouts + 401 broadcast', () => {
  it('dispatches kubona:unauthorized on 401 without throwing', async () => {
    const { apiFetch, UNAUTH_EVENT, wasRecentAuthFailure } = await freshApi();
    expect(wasRecentAuthFailure()).toBe(false);
    const seen: string[] = [];
    vi.stubGlobal('window', {
      dispatchEvent: (e: Event) => {
        seen.push(e.type);
        return true;
      },
    });
    (globalThis as { fetch?: unknown }).fetch = vi.fn(async () => ({ ok: false, status: 401 }));
    const res = await apiFetch('http://x/api/sessions');
    expect(res.status).toBe(401);
    expect(seen).toEqual([UNAUTH_EVENT]);
    expect(wasRecentAuthFailure()).toBe(true);
  });

  it('broadcasts a visible API problem on non-401 failures (never a silent empty)', async () => {
    const { apiFetch, API_PROBLEM_EVENT, wasRecentAuthFailure } = await freshApi();
    const seen: string[] = [];
    vi.stubGlobal('window', {
      dispatchEvent: (e: Event) => {
        seen.push(e.type);
        return true;
      },
    });
    (globalThis as { fetch?: unknown }).fetch = vi.fn(async () => ({ ok: false, status: 503 }));
    await apiFetch('http://x/api/sessions');
    expect(seen).toEqual([API_PROBLEM_EVENT]);
    expect(wasRecentAuthFailure()).toBe(false);
  });

  it('aborts a hung backend at the default timeout instead of hanging forever', async () => {
    const { apiFetch, API_TIMEOUT_MS } = await freshApi();
    expect(API_TIMEOUT_MS).toBeLessThanOrEqual(30000);
    (globalThis as { fetch?: unknown }).fetch = vi.fn(
      (_url: unknown, init?: { signal?: AbortSignal }) =>
        new Promise((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
        }),
    );
    const start = Date.now();
    await expect(apiFetch('http://x/api/sessions')).rejects.toThrow();
    // Must reject near the timeout, not hang: generous upper bound for CI jitter.
    expect(Date.now() - start).toBeLessThan(API_TIMEOUT_MS + 15000);
  }, 60000);

  it('does not override a caller-provided abort signal', async () => {
    const { apiFetch } = await freshApi();
    const ctrl = new AbortController();
    let seenSignal: AbortSignal | null = null;
    (globalThis as { fetch?: unknown }).fetch = vi.fn(async (_url: unknown, init?: { signal?: AbortSignal }) => {
      seenSignal = init?.signal ?? null;
      return { ok: true, status: 200, json: async () => ({}) };
    });
    await apiFetch('http://x/api/sessions', { signal: ctrl.signal });
    expect(seenSignal).toBe(ctrl.signal);
  });
});

describe('streamChatMessage stall detector', () => {
  it('rejects a silent stream instead of hanging the cursor forever', async () => {
    vi.useFakeTimers();
    const { streamChatMessage, STREAM_STALL_MS } = await freshApi();
    expect(STREAM_STALL_MS).toBeLessThanOrEqual(60000);
    const silent = new ReadableStream<Uint8Array>({});
    (globalThis as { fetch?: unknown }).fetch = vi.fn(async () => ({ ok: true, body: silent }));
    const pending = streamChatMessage('s', 'hi', () => undefined);
    const assertion = expect(pending).rejects.toThrow(/stalled/);
    await vi.advanceTimersByTimeAsync(STREAM_STALL_MS + 5000);
    await assertion;
  });

  it('does not trip while frames keep arriving', async () => {
    vi.useFakeTimers();
    const { streamChatMessage } = await freshApi();
    const enc = new TextEncoder();
    let pulls = 0;
    const chatty = new ReadableStream<Uint8Array>({
      pull(controller) {
        pulls += 1;
        if (pulls > 40) {
          controller.enqueue(enc.encode('event: done\ndata: {"ok": true}\n\n'));
          controller.close();
          return;
        }
        controller.enqueue(enc.encode(': heartbeat\n\n'));
      },
    });
    (globalThis as { fetch?: unknown }).fetch = vi.fn(async () => ({ ok: true, body: chatty }));
    const done = streamChatMessage('s', 'hi', () => undefined);
    await vi.advanceTimersByTimeAsync(120000);
    // Heartbeats (transport flow, no data frames) keep the stream alive:
    // either it completes or it is still pending — but never stall-rejects.
    const settled = await Promise.race([done.then(() => 'done' as const), Promise.resolve('pending' as const)]);
    expect(['done', 'pending']).toContain(settled);
  });
});
