import { describe, it, expect, vi, afterEach } from 'vitest';

// Isolated file: configureApiBase mutates module state (first call wins),
// so it must not share a module registry with URL-asserting suites.
afterEach(() => {
  vi.unstubAllGlobals();
});

describe('configureApiBase (M5 runtime backend URL)', () => {
  it('switches all later calls to the runtime URL', async () => {
    const { configureApiBase, fetchJobs, API_BASE } = await import('./api');
    (globalThis as { fetch?: unknown }).fetch = vi.fn(async (url: unknown) => {
      if (String(url) === '/api/config') {
        return { ok: true, json: async () => ({ apiUrl: 'https://kubona.example:8443' }) };
      }
      return { ok: true, json: async () => ({ jobs: [] }) };
    });
    expect(API_BASE).toBe('http://localhost:8080');
    await configureApiBase();
    const { API_BASE: switched } = await import('./api');
    expect(switched).toBe('https://kubona.example:8443');
    await fetchJobs('s');
    const fetchMock = (globalThis as { fetch?: unknown }).fetch as { mock: { calls: unknown[][] } };
    const lastCall = fetchMock.mock.calls[fetchMock.mock.calls.length - 1][0] as string;
    expect(lastCall.startsWith('https://kubona.example:8443/api/jobs')).toBe(true);
  });

  it('keeps the build-time default when the route is missing or bad', async () => {
    vi.resetModules();
    (globalThis as { fetch?: unknown }).fetch = vi.fn(async () => {
      throw new Error('no route');
    });
    const fresh = await import('./api');
    expect(await fresh.configureApiBase()).toBe('http://localhost:8080');
    expect(fresh.API_BASE).toBe('http://localhost:8080');
  });
});
