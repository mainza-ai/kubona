import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { page } from 'vitest/browser';
import React from 'react';
import { CheckInStrip } from './CheckInStrip';
import { AttentionProvider, useAttention } from '@/lib/attention-context';
import { resetSharedResourcesForTests } from '@/domain/shared-resource';
import type { Loop } from '@/types';

const LOOPS: Loop[] = [
  { id: 'a', session_id: 's', status: 'running', step_count: 1, max_steps: 10, diverged: false, created_at: '', updated_at: '' },
  { id: 'b', session_id: 's', status: 'paused', step_count: 1, max_steps: 10, diverged: false, created_at: '', updated_at: '' },
  { id: 'c', session_id: 's', status: 'diverged', step_count: 1, max_steps: 10, diverged: true, created_at: '', updated_at: '' },
  { id: 'd', session_id: 's', status: 'completed', step_count: 10, max_steps: 10, diverged: false, created_at: '', updated_at: '' },
];

const COST_SUMMARY = {
  cap_usd: 10,
  spent_usd: 4,
  remaining_usd: 6,
  unmetered_count: 0,
  by_provider: { openai: 4 },
  active_provider: 'openai',
  local_bypass: false,
  heuristic: true,
  currency: 'USD',
  track_only: false,
};

/** Transport-level stub: only the cost read answers, everything else 404s. */
function stubCost(summary: Record<string, unknown> | null) {
  let calls = 0;
  vi.stubGlobal('fetch', async (url: string) => {
    const path = new URL(String(url)).pathname;
    if (path === '/api/cost/summary') {
      calls += 1;
      if (summary === null) {
        const body = { message: 'down' };
        return { ok: false, status: 503, json: async () => body, text: async () => JSON.stringify(body) };
      }
      return { ok: true, status: 200, json: async () => summary, text: async () => JSON.stringify(summary) };
    }
    return { ok: false, status: 404, json: async () => ({ message: 'not found' }), text: async () => 'not found' };
  });
  return () => calls;
}

function stripLabel(screen: Awaited<ReturnType<typeof render>>) {
  return screen.getByRole('status').element().getAttribute('aria-label');
}

/** The Director has no UI trigger of its own; a real button raises the alert. */
function RaiseButton({ level, label }: { level: 'notable' | 'anomalous'; label: string }) {
  const { notify } = useAttention();
  return (
    <button type="button" onClick={() => notify({ level, source: 'vault' })}>
      {label}
    </button>
  );
}

beforeEach(async () => {
  resetSharedResourcesForTests();
  // The presence dial and the spend line are `hidden sm:*`; a desktop viewport
  // makes them the visible controls an operator actually clicks.
  await page.viewport(1280, 720);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('CheckInStrip in a real browser', () => {
  it('summarizes loops by status with server-sealed spend, never rates', async () => {
    stubCost(COST_SUMMARY);
    const screen = await render(<CheckInStrip loops={LOOPS} />);

    await expect
      .poll(() => stripLabel(screen))
      .toBe('Check-in: 4 loops, 1 running, 1 need attention, LLM spend $4.00');

    const text = screen.container.textContent ?? '';
    expect(text).toContain('paused');
    expect(text).toContain('4.00');
    expect(text).toContain('/ $10 cap');
    expect(text).not.toContain('/min');
    expect(text).not.toContain('burn');
  });

  it('keeps loop counts when spend is refused (no invented figures)', async () => {
    const calls = stubCost(null);
    const screen = await render(<CheckInStrip loops={LOOPS} />);

    // Wait for the refusal to actually land before claiming no spend renders.
    await expect.poll(calls).toBeGreaterThan(0);
    await new Promise((resolve) => setTimeout(resolve, 50));

    expect(stripLabel(screen)).toBe('Check-in: 4 loops, 1 running, 1 need attention');
    const text = screen.container.textContent ?? '';
    expect(text).not.toContain('$');
  });

  it('shows the director pill on summon and collapses duplicates (R-11, R-18)', async () => {
    stubCost(null);
    const screen = await render(
      <AttentionProvider>
        <RaiseButton level="anomalous" label="raise-a" />
        <CheckInStrip loops={[]} />
      </AttentionProvider>,
    );

    // No alert yet: no pill, no directorspeak.
    expect(screen.container.textContent).not.toContain('summon');

    const raise = screen.getByRole('button', { name: 'raise-a' });
    await raise.click();
    await raise.click();

    const text = screen.container.textContent ?? '';
    expect(text).toContain('summon');
    expect(text).toContain('anomalous');
    // Duplicate collapsed: counter shows, single pill only.
    expect(text).toContain('(×2)');
    expect(screen.container.querySelectorAll('[title^="vault"]')).toHaveLength(1);
  });

  it('presence dial gates what surfaces (R-10)', async () => {
    stubCost(null);
    const screen = await render(
      <AttentionProvider>
        <RaiseButton level="notable" label="raise-a" />
        <RaiseButton level="anomalous" label="raise-b" />
        <CheckInStrip loops={[]} />
      </AttentionProvider>,
    );

    // Dial renders all four presence modes.
    const text = screen.container.textContent ?? '';
    for (const label of ['Hands', 'Eyes', 'Ear', 'Away']) {
      expect(text).toContain(label);
    }

    // Away: notable summons never surface.
    await screen.getByRole('button', { name: 'Away' }).click();
    await screen.getByRole('button', { name: 'raise-a' }).click();
    expect(screen.container.textContent).not.toContain('notable');

    // Hands: summons still break through.
    await screen.getByRole('button', { name: 'Hands' }).click();
    await screen.getByRole('button', { name: 'raise-b' }).click();
    expect(screen.container.textContent).toContain('anomalous');
  });
});
