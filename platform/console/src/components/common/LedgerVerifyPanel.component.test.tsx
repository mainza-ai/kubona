import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { LedgerVerifyPanel } from './LedgerVerifyPanel';


describe('LedgerVerifyPanel in a real browser', () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => vi.unstubAllGlobals());

  it('renders the measured fault count', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: true, status: 200, json: async () => ({ ok: false, fault_count: 2, faults: [] }) }));
    const screen = await render(<LedgerVerifyPanel sessionId="s1" />);
    await screen.getByRole('button', { name: /Verify ledger/ }).click();
    await expect.poll(() => document.body.textContent).toContain('2 fault');

  });

  it('renders a refusal verbatim', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: true, status: 200, json: async () => ({ ok: false, error: 'bearer required' }) }));
    const screen = await render(<LedgerVerifyPanel sessionId={null} />);
    await screen.getByRole('button', { name: /Verify ledger/ }).click();
    await expect.poll(() => document.body.textContent).toContain('bearer required');
  });
});
