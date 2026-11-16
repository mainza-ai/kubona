import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { OverflowCard } from './OverflowCard';

const cool = {
  session_id: 's',
  turns: 2,
  turn_cap: 40,
  history_chars: 100,
  history_chars_cap: 48000,
  would_drop: 0,
  pressure_pct: 5,
  prompt_tokens: 10,
  completion_tokens: 5,
};
const hot = { ...cool, turns: 36, history_chars: 44000, would_drop: 3, pressure_pct: 92 };

afterEach(() => {
  vi.unstubAllGlobals();
});

/** Stub the context read; `onRead` fires once the payload is handed back. */
function stubContext(fixture: unknown, onRead?: () => void) {
  vi.stubGlobal('fetch', async (url: string) => {
    if (String(url).includes('/api/sessions/context')) {
      return {
        ok: true,
        status: 200,
        json: async () => {
          onRead?.();
          return fixture;
        },
      };
    }
    return { ok: false, status: 404, json: async () => ({}) };
  });
}

/** A resolved read is not yet a render: let React commit the state update. */
function settle() {
  return new Promise((resolve) => setTimeout(resolve, 50));
}

describe('OverflowCard in a real browser', () => {
  it('stays hidden below pressure', async () => {
    let read = 0;
    stubContext(cool, () => { read += 1; });
    const screen = await render(<OverflowCard sessionId="s" refreshKey={1} />);
    await expect.poll(() => read).toBe(1);
    await settle();
    expect(screen.container.querySelector('[role="alert"]')).toBeNull();
    expect(screen.container.textContent).not.toContain('Context at');
  });

  it('offers real actions at pressure, no compact theater', async () => {
    stubContext(hot);
    const screen = await render(<OverflowCard sessionId="s" refreshKey={1} />);

    const alert = screen.getByRole('alert', { name: 'Context pressure 92 percent' });
    await expect.element(alert).toBeVisible();
    await expect.element(screen.getByText('Context at 92% — 3 oldest turns would drop next')).toBeVisible();
    await expect.element(screen.getByRole('button', { name: 'New session' })).toBeVisible();
    await expect.element(screen.getByRole('button', { name: 'Clear history' })).toBeVisible();
    expect(screen.container.textContent?.toLowerCase()).not.toContain('compact');
  });

  it('new session calls createSession and notifies', async () => {
    const posts: Array<{ url: string; body: Record<string, unknown> }> = [];
    vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
      if (String(url).includes('/api/sessions/context')) {
        return { ok: true, status: 200, json: async () => hot };
      }
      if (String(url).endsWith('/api/sessions') && init?.method === 'POST') {
        posts.push({ url: String(url), body: JSON.parse(String(init.body)) as Record<string, unknown> });
        return {
          ok: true,
          status: 200,
          json: async () => ({ id: 'new-sess', title: 'Quantum Research Campaign (overflow)', status: 'active', created_at: '', updated_at: '' }),
        };
      }
      return { ok: false, status: 404, json: async () => ({}) };
    });

    const seen: string[] = [];
    const screen = await render(
      <OverflowCard sessionId="s" refreshKey={1} onSessionChanged={(id) => seen.push(id)} />,
    );
    await screen.getByRole('button', { name: 'New session' }).click();

    await expect.element(screen.getByText('Fresh session started — history preserved in the old one.')).toBeVisible();
    expect(posts).toHaveLength(1);
    expect(posts[0].body.title).toBe('Quantum Research Campaign (overflow)');
    expect(posts[0].body.spend_limit_usd).toBe(150);
    expect(seen).toEqual(['new-sess']);
  });
});
