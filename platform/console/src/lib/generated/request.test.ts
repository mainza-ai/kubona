import { afterEach, describe, expect, it, vi } from 'vitest';
import { requestOperation } from './request';

afterEach(() => vi.unstubAllGlobals());

describe('generated operation client', () => {
  it('substitutes path params and returns the declared response type', async () => {
    const fetchMock = vi.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({ sessions: [{ id: 's1' }] }),
    }));
    vi.stubGlobal('fetch', fetchMock);
    const data = await requestOperation('fetchSessions');
    expect(data.sessions[0].id).toBe('s1');
    expect(String((fetchMock.mock.calls as unknown[][])[0]?.[0])).toContain('/api/sessions');
  });

  it('refuses a missing path parameter before fetch', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    await expect(requestOperation('fetchSession')).rejects.toThrow('missing path parameter id');
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
