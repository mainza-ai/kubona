import { afterEach, describe, expect, it, vi } from 'vitest';
import { saveWorkbenchLayout } from './api';

afterEach(() => vi.unstubAllGlobals());

describe('saveWorkbenchLayout conflict recovery', () => {
  it('carries the current server revision on a 409', async () => {
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false,
      status: 409,
      json: async () => ({ error: 'revision_conflict', current_revision: 7 }),
      text: async () => '',
    })));
    const out = await saveWorkbenchLayout('s1', 6, { version: 2 });
    expect(out.ok).toBe(false);
    expect(out.conflict).toBe(true);
    expect(out.currentRevision).toBe(7);
  });
});
