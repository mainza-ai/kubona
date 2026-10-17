import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { ContextReadout } from './ContextReadout';

const fixture = {
  session_id: 's',
  turns: 30,
  turn_cap: 40,
  history_chars: 24000,
  history_chars_cap: 48000,
  would_drop: 0,
  pressure_pct: 75,
  prompt_tokens: 1000,
  completion_tokens: 500,
};

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map((c) => textOf(c)).join(' ');
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('ContextReadout', () => {
  it('renders measured pressure percent and tokens', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => fixture }));
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<ContextReadout sessionId="s" refreshKey={3} />);
    });
    const text = textOf(tree.root).replace(/\s+/g, ' ');
    expect(text).toContain('ctx 75 %');
    expect(tree.root.findByProps({ 'aria-label': 'Context pressure 75 percent' })).toBeTruthy();
  });

  it('flags hot pressure and pending drops', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, json: async () => ({ ...fixture, pressure_pct: 92, would_drop: 4 }) })
    );
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<ContextReadout sessionId="s" refreshKey={1} />);
    });
    expect(textOf(tree.root).replace(/\s+/g, ' ')).toContain('↓ 4');
  });

  it('renders nothing when the backend is unreachable', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new Error('down')));
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<ContextReadout sessionId="s" refreshKey={0} />);
    });
    expect(tree.toJSON()).toBeNull();
  });
});
