import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { ObligationsPanel } from './ObligationsPanel';
import { resetSharedResourcesForTests } from '@/domain/shared-resource';

const row = {
  id: 'ob-1234567890', subject_kind: 'mandate', subject_id: 'm-1', owner: 'session:s1',
  disposition: 'open', required_cases: [], required_authorities: [], close_ready: true, close_reasons: [],
};

beforeEach(() => resetSharedResourcesForTests());
afterEach(() => vi.unstubAllGlobals());

describe('ObligationsPanel in a real browser', () => {
  it('lists an open obligation and triggers server closure', async () => {
    vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
      if (init?.method === 'POST') return { ok: true, status: 200, json: async () => ({ ok: true, closed: true }) };
      return { ok: true, status: 200, json: async () => ({ ok: true, obligations: [row] }) };
    });
    const screen = await render(<ObligationsPanel />);
    await expect.poll(() => document.body.textContent).toContain('ob-123456789');
    await screen.getByRole('button', { name: 'Evaluate close' }).click();
    await expect.poll(() => document.body.textContent).toContain('closed');
  });

  it('renders a refusal instead of an empty queue', async () => {
    vi.stubGlobal('fetch', async () => { throw new Error('login required'); });
    await render(<ObligationsPanel />);
    await expect.poll(() => document.body.textContent).toContain('Queue unavailable');
  });
});
