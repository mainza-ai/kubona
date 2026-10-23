import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchLensHistory, requestObservatoryLens, resolveObservatoryLens, uploadObservatoryAsset } from './api';

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

  it('passes depth asset + camera for chisel dispatches', async () => {
    mockFetchOk({ ok: true, operation_id: 'op_1' });
    await requestObservatoryLens({
      sessionId: 'sess-1',
      targetSystem: 'H2',
      taskType: 'atlasChisel',
      depthAssetId: 'asset_depth_9',
      camera: { position: [0, 1.5, 3.5], orientation: [0, 0, 0, 1] },
    });
    const [, init] = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit];
    const body = JSON.parse(init.body as string);
    expect(body.task_type).toBe('atlasChisel');
    expect(body.depth_asset_id).toBe('asset_depth_9');
    expect(body.camera).toEqual({ position: [0, 1.5, 3.5], orientation: [0, 0, 0, 1] });
    expect('image_url' in body).toBe(false);
  });

  it('upload posts inline base64 and returns the asset id', async () => {
    mockFetchOk({ ok: true, asset_id: 'asset_up_1', idempotency_key: 'k' });
    const res = await uploadObservatoryAsset({
      sessionId: 'sess-1',
      dataBase64: 'data:image/x-exr;base64,AAAA',
      displayName: 'kubona-pes-H2.exr',
    });
    expect(res.ok).toBe(true);
    expect(res.asset_id).toBe('asset_up_1');
    const [url, init] = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0] as [string, RequestInit];
    expect(url).toContain('/api/observatory/assets');
    expect(JSON.parse(init.body as string).data_base64).toBe('data:image/x-exr;base64,AAAA');
  });

  it('resolve surfaces the terminal record', async () => {
    mockFetchOk({ ok: true, done: true, prompt_used: 'pavilion prompt', asset_urls: ['https://x/y.png'] });
    const res = await resolveObservatoryLens({ sessionId: 'sess-1', operationId: 'op_1' });
    expect(res.ok).toBe(true);
    expect(res.prompt_used).toBe('pavilion prompt');
    expect(res.asset_urls).toEqual(['https://x/y.png']);
  });

  it('history lists reservation records, not billed amounts', async () => {
    mockFetchOk({
      ok: true,
      operations: [{ operation_id: 'op_1', task_type: 'atlasChisel', cost_usd: 0.5, resolved: true }],
    });
    const res = await fetchLensHistory('sess-1');
    expect(res.ok).toBe(true);
    expect(res.operations).toHaveLength(1);
    expect(res.operations?.[0].cost_usd).toBe(0.5);
  });
});
