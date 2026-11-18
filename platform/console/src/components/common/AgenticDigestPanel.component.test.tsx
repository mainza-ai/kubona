import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { AgenticDigestPanel } from './AgenticDigestPanel';

afterEach(() => { vi.unstubAllGlobals(); });

function json(payload: unknown) {
  return { ok: true, status: 200, json: async () => payload };
}

describe('AgenticDigestPanel in a real browser', () => {
  it('renders the sealed notify-mode receipts', async () => {
    vi.stubGlobal('fetch', async (url: string) => {
      expect(url).toContain('/api/sessions/s1/agentic/digest?hours=24');
      return json({
        session_id: 's1',
        hours: 24,
        entries: [
          { plan_id: 'plan_aaa', notify: true, accepted: true, status: 'completed', risk: 'low', at: '2026-11-18T10:00:00Z' },
          { plan_id: 'plan_bbb', notify: true, accepted: false, status: 'rejected', risk: 'medium', at: '2026-11-18T11:00:00Z' },
        ],
        counts: { total: 2, accepted: 1, notify: 2 },
      });
    });
    const screen = await render(<AgenticDigestPanel sessionId="s1" />);
    await expect.poll(() => screen.container.textContent).toContain('plan_aaa');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('2 run · 1 accepted · 2 notify');
    expect(text).toContain('accepted');
    expect(text).toContain('rejected');
    expect(text).toContain('notify');
  });

  it('states an empty window instead of inventing activity', async () => {
    vi.stubGlobal('fetch', async () => json({
      session_id: 's1', hours: 24, entries: [], counts: { total: 0, accepted: 0, notify: 0 },
    }));
    const screen = await render(<AgenticDigestPanel sessionId="s1" />);
    await expect.poll(() => screen.container.textContent).toContain('No executions in this window.');
  });

  it('surfaces an unavailable digest honestly', async () => {
    vi.stubGlobal('fetch', async () => ({ ok: false, status: 500, json: async () => ({}) }));
    const screen = await render(<AgenticDigestPanel sessionId="s1" />);
    await expect.poll(() => screen.container.textContent).toContain('Digest unavailable.');
  });
});
