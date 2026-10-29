import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { dedupedGet, fetchCampaignState, resetDedupForTests, reportError } from './api';

// Wave-3 inflight GET coalescing contract: module state is the dedup map,
// so every case starts from a cleared map and a fresh fetch stub.
beforeEach(() => {
  resetDedupForTests();
});

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function stubFetch(impl: (url: unknown, init?: RequestInit) => unknown) {
  const fn = vi.fn(async (url: unknown, init?: RequestInit) => impl(url, init));
  (globalThis as { fetch?: unknown }).fetch = fn;
  return fn;
}

const okJson = (body: unknown) => ({ ok: true, status: 200, json: async () => body });

describe('dedupedGet', () => {
  it('coalesces concurrent identical GETs into ONE fetch', async () => {
    const fetch = stubFetch(async () => {
      await new Promise((r) => setTimeout(r, 10));
      return okJson({ v: 1 });
    });
    const [a, b] = await Promise.all([
      dedupedGet('http://x/api/a'),
      dedupedGet('http://x/api/a'),
    ]);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(a).toEqual({ v: 1 });
    expect(b).toEqual({ v: 1 });
  });

  it('issues two fetches for distinct URLs', async () => {
    const fetch = stubFetch(async () => okJson({}));
    await Promise.all([dedupedGet('http://x/api/a'), dedupedGet('http://x/api/b')]);
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it('keys on headers: same URL with different headers re-fetches', async () => {
    const fetch = stubFetch(async () => okJson({}));
    await dedupedGet('http://x/api/a', { headers: { Authorization: 'Bearer one' } });
    await dedupedGet('http://x/api/a', { headers: { Authorization: 'Bearer two' } });
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it('clears the key on rejection so a retry re-fetches', async () => {
    let calls = 0;
    stubFetch(async () => {
      calls += 1;
      if (calls === 1) throw new Error('backend down');
      return okJson({ v: 2 });
    });
    await expect(dedupedGet('http://x/api/flaky')).rejects.toThrow('backend down');
    const retry = await dedupedGet<{ v: number }>('http://x/api/flaky');
    expect(retry).toEqual({ v: 2 });
    expect(calls).toBe(2);
  });

  it('rejects a refused GET instead of parsing the error body as data', async () => {
    stubFetch(async () => ({ ok: false, status: 401, json: async () => ({ error: 'expired' }) }));
    await expect(dedupedGet('http://x/api/refused')).rejects.toThrow(/refused/);
  });

  it('coalesces concurrent campaign reads from co-mounted surfaces', async () => {
    const fetch = stubFetch(async () => {
      await new Promise((r) => setTimeout(r, 10));
      return okJson({ target_system: 'H2' });
    });
    const [a, b] = await Promise.all([fetchCampaignState('s1'), fetchCampaignState('s1')]);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(a).toEqual({ target_system: 'H2' });
    expect(b).toEqual({ target_system: 'H2' });
  });

  it('never coalesces POSTs', async () => {
    const fetch = stubFetch(async () => okJson({ ok: true }));
    await Promise.all([
      dedupedGet('http://x/api/a', { method: 'POST' }),
      dedupedGet('http://x/api/a', { method: 'POST' }),
    ]);
    expect(fetch).toHaveBeenCalledTimes(2);
  });
});

describe('reportError', () => {
  it('logs via console.error as the single chokepoint', () => {
    const spy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const err = new Error('kaput');
    reportError('test-context', err);
    expect(spy).toHaveBeenCalledTimes(1);
    expect(spy).toHaveBeenCalledWith(expect.stringContaining('test-context'), err);
  });
});
