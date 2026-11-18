import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { ApprovalInbox } from './ApprovalInbox';

afterEach(() => { vi.unstubAllGlobals(); });

const plan = {
  id: 'p1',
  statement: 'Run the H2 expressivity probe',
  risk: 'low',
  method: 'vqe',
  observable: 'energy',
  acceptance_metric: 'energy_exact',
  created_at: '2026-11-18T10:00:00Z',
};

function stub(decisions: Array<Record<string, unknown>>, opts: { deferredOnRead?: boolean } = {}) {
  let decided = false;
  let deferred = false;
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    if (!url.endsWith('/api/sessions/s1/approvals') && !url.includes('/approvals/p1')) {
      throw new Error(`unexpected ${url}`);
    }
    if (init?.method === 'POST') {
      const body = JSON.parse(String(init.body ?? '{}')) as Record<string, unknown>;
      decisions.push(body);
      decided = true;
      deferred = body.decision === 'defer';
      return { ok: true, status: 200, text: async () => JSON.stringify({ decision: body.decision, plan: {} }), json: async () => ({ decision: body.decision, plan: {} }) };
    }
    return {
      ok: true,
      status: 200,
      json: async () => ({
        session_id: 's1',
        pending: decided ? [] : [{ ...plan, deferred_until: deferred ? '2026-11-18T12:00:00Z' : '' }],
        deferred: decided && deferred && opts.deferredOnRead ? [{ ...plan, deferred_until: '2026-11-18T12:00:00Z' }] : [],
        counts: { pending: decided ? 0 : 1, deferred: 0 },
      }),
    };
  });
}

describe('ApprovalInbox in a real browser', () => {
  it('approves a pending plan once and refreshes the inbox', async () => {
    const decisions: Array<Record<string, unknown>> = [];
    stub(decisions);
    const screen = await render(<ApprovalInbox sessionId="s1" />);
    await expect.poll(() => screen.container.textContent).toContain('Run the H2 expressivity probe');
    await screen.getByRole('button', { name: 'Approve' }).click();
    await expect.poll(() => decisions).toEqual([{ decision: 'approve' }]);
    await expect.poll(() => screen.container.textContent).toContain('No pending approvals.');
    await expect.poll(() => screen.container.textContent).toContain('Approved.');
  });

  it('defers with a future timestamp', async () => {
    const decisions: Array<Record<string, unknown>> = [];
    stub(decisions);
    const screen = await render(<ApprovalInbox sessionId="s1" />);
    await expect.poll(() => screen.container.textContent).toContain('Run the H2 expressivity probe');
    await screen.getByRole('button', { name: 'Defer 1h' }).click();
    await expect.poll(() => decisions.length).toBe(1);
    expect(decisions[0].decision).toBe('defer');
    const until = new Date(String(decisions[0].deferred_until)).getTime();
    expect(until).toBeGreaterThan(Date.now() + 55 * 60 * 1000);
    await expect.poll(() => screen.container.textContent).toContain('Deferred.');
  });
});
