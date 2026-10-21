import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { CheckInStrip } from './CheckInStrip';

afterEach(() => {
  vi.unstubAllGlobals();
});

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map((c) => textOf(c)).join(' ');
}

const LOOPS = [
  { id: 'a', session_id: 's', status: 'running', step_count: 1, max_steps: 10, diverged: false, created_at: '', updated_at: '' },
  { id: 'b', session_id: 's', status: 'paused', step_count: 1, max_steps: 10, diverged: false, created_at: '', updated_at: '' },
  { id: 'c', session_id: 's', status: 'diverged', step_count: 1, max_steps: 10, diverged: true, created_at: '', updated_at: '' },
  { id: 'd', session_id: 's', status: 'completed', step_count: 10, max_steps: 10, diverged: false, created_at: '', updated_at: '' },
];

describe('CheckInStrip', () => {
  it('summarizes loops by status with server-sealed spend, never rates', async () => {
    vi.stubGlobal(
      'fetch',
      async (url: unknown) =>
        String(url).includes('/api/cost/summary')
          ? new Response(
              JSON.stringify({
                cap_usd: 10, spent_usd: 4, remaining_usd: 6, unmetered_count: 0,
                by_provider: { openai: 4 }, active_provider: 'openai',
                local_bypass: false, heuristic: true, currency: 'USD', track_only: false,
              }),
              { status: 200 }
            )
          : new Response('not found', { status: 404 })
    );
    let tree: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      tree = TestRenderer.create(<CheckInStrip loops={LOOPS as never} />);
    });
    await act(async () => {});
    const root = tree!.root;
    const text = textOf(root);
    const strip = root.findByProps({ role: 'status' });
    expect(String(strip.props['aria-label'])).toBe('Check-in: 4 loops, 1 running, 1 need attention, LLM spend $4.00');
    expect(text).toContain('paused');
    expect(text).toContain('4.00');
    expect(text).toContain('/ $10 cap');
    expect(text).not.toContain('/min');
    expect(text).not.toContain('burn');
    tree!.unmount();
  });

  it('keeps loop counts when spend is refused (no invented figures)', async () => {
    vi.stubGlobal('fetch', async () => new Response(JSON.stringify({ message: 'down' }), { status: 503 }));
    let tree: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      tree = TestRenderer.create(<CheckInStrip loops={LOOPS as never} />);
    });
    await act(async () => {});
    const root = tree!.root;
    const text = textOf(root);
    const strip = root.findByProps({ role: 'status' });
    expect(String(strip.props['aria-label'])).toBe('Check-in: 4 loops, 1 running, 1 need attention');
    expect(text).not.toContain('$');
    tree!.unmount();
  });
});
