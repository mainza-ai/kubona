import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { CopilotDrawer } from './CopilotDrawer';
import type { ChatMessage, CampaignState } from '@/types';

// No @/lib/api mock: the drawer's mount reads are answered at the fetch
// boundary, so the real request/contract path is exercised.
function stubDrawerFetch(messages: ChatMessage[] = []) {
  const calls: string[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const path = new URL(String(url)).pathname;
    calls.push(`${init?.method ?? 'GET'} ${path}`);
    if (path === '/api/sessions/s/messages') {
      return { ok: true, status: 200, json: async () => ({ messages }) };
    }
    if (path === '/api/chat/feedback') {
      return { ok: true, status: 200, json: async () => ({ feedback: [] }) };
    }
    if (path === '/api/sessions/context') {
      return { ok: true, status: 200, json: async () => ({ context: {} }) };
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

  it('keeps a long target and wide table inside the drawer', async () => {
    const campaign = {
      session_id: 's',
      target_system: 'Lithium Hydride (LiH)',
      research_goal: 'Map the LiH surface',
      quantum_factors: { target_molecule: 'LiH' },
      epistemics: { status: 'PROJECTED' },
      notebook: {},
    } as unknown as CampaignState;
    stubDrawerFetch([{
      id: 'long-table',
      session_id: 's',
      role: 'assistant',
      created_at: new Date().toISOString(),
      content: '| R (Å) | FCI | RHF |\n|---|---|---|\n| 1.0 | -7.78446028003123 [o3] | -7.767362 [o3] |\n| 4.0 | -7.784278178715385 [o9] | -7.624972 [o9] |',
    } as ChatMessage]);
    const screen = await render(
      <div data-testid="narrow-copilot" style={{ width: 320 }}>
        <CopilotDrawer sessionId="s" campaign={campaign} />
      </div>,
    );
    await expect.poll(() => screen.container.textContent).toContain('Lithium Hydride (LiH)');
    expect(screen.container.querySelector('table')).not.toBeNull();
    expect(screen.getByTestId('copilot-feed').element().scrollWidth).toBeLessThanOrEqual(320);
    expect(screen.getByTestId('narrow-copilot').element().scrollWidth).toBeLessThanOrEqual(320);
    expect(screen.container.textContent).not.toContain('UNVE…');
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
