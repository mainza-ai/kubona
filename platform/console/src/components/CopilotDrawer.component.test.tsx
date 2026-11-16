import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { CopilotDrawer } from './CopilotDrawer';

// No @/lib/api mock: the drawer's mount reads are answered at the fetch
// boundary, so the real request/contract path is exercised.
function stubDrawerFetch() {
  const calls: string[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const path = new URL(String(url)).pathname;
    calls.push(`${init?.method ?? 'GET'} ${path}`);
    if (path === '/api/sessions/s/messages') {
      return { ok: true, status: 200, json: async () => ({ messages: [] }) };
    }
    if (path === '/api/chat/feedback') {
      return { ok: true, status: 200, json: async () => ({ feedback: [] }) };
    }
    throw new Error(`unexpected fetch ${init?.method ?? 'GET'} ${url}`);
  });
  return calls;
}

afterEach(() => vi.unstubAllGlobals());

describe('CopilotDrawer no-campaign state in a real browser', () => {
  it('does not invent an H2 campaign or swarm context', async () => {
    const calls = stubDrawerFetch();
    await render(<CopilotDrawer sessionId="s" />);

    await expect.poll(() => document.body.textContent)
      .toContain('No campaign selected. Create or open a campaign to start.');
    expect(document.body.textContent).not.toContain('Autonomous Swarm Initialized');
    expect(document.body.textContent).not.toContain('Hydrogen Molecule (H₂)');
    expect(calls).toContain('GET /api/sessions/s/messages');
  });

  it('offers a create-campaign action when no campaign exists', async () => {
    stubDrawerFetch();
    const onCreateCampaign = vi.fn();
    const screen = await render(<CopilotDrawer sessionId="s" onCreateCampaign={onCreateCampaign} />);

    const button = screen.getByRole('button', { name: 'Create campaign' });
    await expect.element(button).toBeVisible();
    await button.click();
    expect(onCreateCampaign).toHaveBeenCalledTimes(1);
  });
});
