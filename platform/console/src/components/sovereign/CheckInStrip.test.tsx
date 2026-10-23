import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { CheckInStrip } from './CheckInStrip';
import { AttentionProvider, useAttention } from '@/lib/attention-context';

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

  it('shows the director pill on summon and collapses duplicates (R-11, R-18)', async () => {
    vi.stubGlobal('fetch', async () => new Response(JSON.stringify({ message: 'down' }), { status: 503 }));
    let api: { notify: (i: { level: 'anomalous'; source: string }) => void } | null = null;
    function Probe({ onReady }: { onReady: (v: { notify: (i: { level: 'anomalous'; source: string }) => void }) => void }) {
      const v = useAttention();
      React.useEffect(() => {
        onReady(v);
      }, [v, onReady]);
      return null;
    }
    let tree: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      tree = TestRenderer.create(
        <AttentionProvider>
          <Probe onReady={(v) => { api = v; }} />
          <CheckInStrip loops={[]} />
        </AttentionProvider>,
      );
    });
    // No alert yet: no pill, no directorspeak.
    expect(textOf(tree!.root)).not.toContain('summon');
    await act(async () => {
      api!.notify({ level: 'anomalous', source: 'vault' });
      api!.notify({ level: 'anomalous', source: 'vault' });
    });
    const text = textOf(tree!.root);
    expect(text).toContain('summon');
    expect(text).toContain('anomalous');
    // Duplicate collapsed: counter shows, single pill only.
    expect(text).toContain('(×2)');
    const pills = tree!.root.findAll(
      (n) => typeof n.props?.title === 'string' && (n.props.title as string).startsWith('vault'),
    );
    expect(pills).toHaveLength(1);
    tree!.unmount();
  });

  it('presence dial gates what surfaces (R-10)', async () => {
    vi.stubGlobal('fetch', async () => new Response(JSON.stringify({ message: 'down' }), { status: 503 }));
    let api: {
      notify: (i: { level: 'notable' | 'anomalous'; source: string }) => void;
      setPresence: (p: 'hands' | 'eyes' | 'ear' | 'away') => void;
    } | null = null;
    function Probe({ onReady }: { onReady: (v: typeof api) => void }) {
      const v = useAttention();
      React.useEffect(() => {
        onReady(v);
      }, [v, onReady]);
      return null;
    }
    let tree: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      tree = TestRenderer.create(
        <AttentionProvider>
          <Probe onReady={(v) => { api = v; }} />
          <CheckInStrip loops={[]} />
        </AttentionProvider>,
      );
    });
    // Dial renders all four presence modes.
    for (const label of ['Hands', 'Eyes', 'Ear', 'Away']) {
      expect(textOf(tree!.root)).toContain(label);
    }
    // Away: notable summons never surface.
    await act(async () => {
      api!.setPresence('away');
      api!.notify({ level: 'notable', source: 'vault' });
    });
    expect(textOf(tree!.root)).not.toContain('notable');
    // Hands: summons still break through.
    await act(async () => {
      api!.setPresence('hands');
      api!.notify({ level: 'anomalous', source: 'vault' });
    });
    expect(textOf(tree!.root)).toContain('anomalous');
    tree!.unmount();
  });
});
