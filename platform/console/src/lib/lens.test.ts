import { beforeEach, describe, expect, it, vi } from 'vitest';
import { requestObservatoryLens } from './api';

// Q-37/P0a client pins: snapshot imagery rides through, and every dispatch
// carries a single-use idempotency key (no key reuse across submissions).

describe('observatory lens client', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });

  function mockFetchOk(body: unknown) {
    (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      status: 200,
      json: async () => body,
    });
  }

  it('passes the snapshot through as image_url with an idempotency key', async () => {
    mockFetchOk({ ok: true, operation_id: 'op_1' });
    await requestObservatoryLens({
      sessionId: 'sess-1',
      targetSystem: 'H2',
      imageUrl: 'data:image/png;base64,iVBORw0KGgo=',
    });
    const [, init] = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit];
    const body = JSON.parse(init.body as string);
    expect(body.image_url).toBe('data:image/png;base64,iVBORw0KGgo=');
    const headers = init.headers as Record<string, string>;
    expect(typeof headers['Idempotency-Key']).toBe('string');
    expect(headers['Idempotency-Key'].length).toBeGreaterThan(0);
  });

  it('omits image_url when no snapshot was captured (backend fail-closes)', async () => {
    mockFetchOk({ ok: true, operation_id: 'op_1' });
    await requestObservatoryLens({ sessionId: 'sess-1', targetSystem: 'H2' });
    const [, init] = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit];
    const body = JSON.parse(init.body as string);
    expect('image_url' in body).toBe(false);
  });

  it('mints a fresh idempotency key per request', async () => {
    mockFetchOk({ ok: true });
    mockFetchOk({ ok: true });
    await requestObservatoryLens({ sessionId: 'sess-1', targetSystem: 'H2' });
    await requestObservatoryLens({ sessionId: 'sess-1', targetSystem: 'H2' });
    const calls = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls as [string, RequestInit][];
    const k1 = (calls[0][1].headers as Record<string, string>)['Idempotency-Key'];
    const k2 = (calls[1][1].headers as Record<string, string>)['Idempotency-Key'];
    expect(k1).not.toBe(k2);
  });
});
