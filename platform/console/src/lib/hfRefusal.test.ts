import { describe, it, expect, vi, afterEach } from 'vitest';
import { searchHFModels, listHFDownloads, createCustomAgent } from './api';

// Audit 2026-10-21: the hub client swallowed HTTP refusals into `[]`,
// which Model Hub rendered as "No models found" — a lie masking a 401
// (no operator token in stock Docker). Refusals must throw so surfaces
// toast them; only a genuine empty hub reads empty.
describe('HF hub refusal honesty', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('searchHFModels throws on refusal instead of returning []', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: false, status: 401 }));
    await expect(searchHFModels('qwen', 'gguf', 5)).rejects.toThrow(/hub refused search \(HTTP 401\)/);
  });

  it('searchHFModels returns models on success, [] on genuinely empty hub', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: true, json: async () => ({ models: [{ id: 'a/b' }] }) }));
    expect(await searchHFModels('qwen', 'gguf', 5)).toEqual([{ id: 'a/b' }]);
    vi.stubGlobal('fetch', async () => ({ ok: true, json: async () => ({ models: [] }) }));
    expect(await searchHFModels('qwen', 'gguf', 5)).toEqual([]);
  });

  it('listHFDownloads throws on refusal instead of returning []', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: false, status: 401 }));
    await expect(listHFDownloads()).rejects.toThrow(/hub refused download list \(HTTP 401\)/);
  });

  it('createCustomAgent throws on refusal instead of resolving null', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: false, status: 401, text: async () => 'custom agent registration requires the operator token' }));
    await expect(createCustomAgent({ id: 'x' } as never)).rejects.toThrow(/agent registration refused \(HTTP 401\)/);
  });

  it('createCustomAgent returns the agent on success', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: true, json: async () => ({ agent: { id: 'a1' } }) }));
    expect(await createCustomAgent({ id: 'a1' } as never)).toEqual({ id: 'a1' });
  });
});
