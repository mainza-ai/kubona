import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
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

function stubContext(fixture: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation((url: string) => {
      if (String(url).includes('/api/sessions/context')) {
        return Promise.resolve({ ok: true, json: async () => fixture });
      }
      return Promise.resolve({ ok: false, json: async () => ({}) });
    })
  );
}

describe('OverflowCard', () => {
  it('stays hidden below pressure', async () => {
    stubContext(cool);
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<OverflowCard sessionId="s" refreshKey={1} />);
    });
    expect(tree.toJSON()).toBeNull();
  });

  it('offers real actions at pressure, no compact theater', async () => {
    stubContext(hot);
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<OverflowCard sessionId="s" refreshKey={1} />);
    });
    const buttons = tree.root.findAllByType('button').map((b) => (b.children ?? []).join(''));
    expect(buttons).toContain('New session');
    expect(buttons).toContain('Clear history');
    expect(buttons.join(' ').toLowerCase()).not.toContain('compact');
    expect(tree.root.findByProps({ role: 'alert' })).toBeTruthy();
  });

  it('new session calls createSession and notifies', async () => {
    stubContext(hot);
    let created: unknown = null;
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((url: string, opts?: { method?: string }) => {
        if (String(url).includes('/api/sessions/context')) {
          return Promise.resolve({ ok: true, json: async () => hot });
        }
        if (String(url).endsWith('/api/sessions') && opts?.method === 'POST') {
          created = true;
          return Promise.resolve({ ok: true, json: async () => ({ id: 'new-sess' }) });
        }
        return Promise.resolve({ ok: false, json: async () => ({}) });
      })
    );
    let seen = '';
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<OverflowCard sessionId="s" refreshKey={1} onSessionChanged={(id) => (seen = id)} />);
    });
    const btn = tree.root.findAllByType('button').find((b) => (b.children ?? []).join('') === 'New session')!;
    await act(async () => {
      await btn.props.onClick();
    });
    expect(created).toBe(true);
    expect(seen).toBe('new-sess');
  });
});
