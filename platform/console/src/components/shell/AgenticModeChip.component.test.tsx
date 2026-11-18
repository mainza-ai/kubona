import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { AgenticModeChip } from './AgenticModeChip';

afterEach(() => { vi.unstubAllGlobals(); });

function stub(opts: { enabled?: boolean; locked?: boolean; puts?: string[] } = {}) {
  let enabled = opts.enabled ?? true;
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    if (!url.endsWith('/api/sessions/s1/agentic')) throw new Error(`unexpected ${url}`);
    if (init?.method === 'PUT') {
      opts.puts?.push(String(init.body ?? ''));
      enabled = (JSON.parse(String(init.body ?? '{}')) as { enabled?: boolean }).enabled === true;
      const payload = {
        agentic_enabled: enabled,
        agentic_locked: false,
        paused_loops: 0,
        policy: {
          stages: { hypothesize: 'act', plan: 'act', execute: 'approve', collapse: 'act' },
          max_act_budget_usd: 1,
          agentic_enabled: enabled,
          agentic_locked: false,
        },
      };
      return { ok: true, status: 200, text: async () => JSON.stringify(payload), json: async () => payload };
    }
    return {
      ok: true,
      status: 200,
      json: async () => ({
        session_id: 's1',
        agentic_enabled: enabled,
        agentic_locked: opts.locked ?? false,
        stages: { hypothesize: 'act', plan: 'act', execute: 'approve', collapse: 'act' },
        max_act_budget_usd: 1,
      }),
    };
  });
}

describe('AgenticModeChip in a real browser', () => {
  it('shows Auto, toggles to Manual, and sends the switch', async () => {
    const puts: string[] = [];
    stub({ puts });
    const screen = await render(<AgenticModeChip sessionId="s1" />);
    await expect.poll(() => screen.container.textContent).toContain('Auto');
    const toggle = screen.getByRole('switch', { name: 'Auto agent mode' });
    expect(toggle.element().getAttribute('aria-checked')).toBe('true');
    await toggle.click();
    await expect.poll(() => puts).toEqual(['{"enabled":false}']);
    await expect.poll(() => screen.container.textContent).toContain('Manual');
    expect(toggle.element().getAttribute('aria-checked')).toBe('false');
  });

  it('shows Locked and refuses the toggle after an operator halt', async () => {
    stub({ enabled: false, locked: true });
    const screen = await render(<AgenticModeChip sessionId="s1" />);
    await expect.poll(() => screen.container.textContent).toContain('Locked');
    const toggle = screen.getByRole('switch', { name: 'Auto agent mode' }) as unknown as { element: () => HTMLButtonElement };
    expect(toggle.element().disabled).toBe(true);
  });

  it('renders nothing without a session', async () => {
    const screen = await render(<AgenticModeChip sessionId={null} />);
    expect(screen.container.childElementCount).toBe(0);
  });
});
