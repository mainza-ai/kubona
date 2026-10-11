import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { MemoryPanel } from './MemoryPanel';

// B1-2: the panel renders nothing without a session, lists inventory with
// null-similarity labeling, and posts retains with explicit weight.
describe('MemoryPanel', () => {
  beforeEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders nothing without a session', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<MemoryPanel sessionId={null} />);
    });
    expect(tree!.toJSON()).toBeNull();
  });

  it('recalls inventory and retains with explicit weight', async () => {
    const calls: Array<{ url: string; init?: RequestInit }> = [];
    vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
      calls.push({ url, init });
      if (String(url).includes('/api/memory/recall')) {
        return {
          ok: true,
          json: async () => ({
            snippets: [
              { content: 'prior anchor note', bank: 'experiences', similarity: null, created_at: '2026-10-11T00:00:00Z' },
            ],
            tenant: 'default',
          }),
        };
      }
      return {
        ok: true,
        json: async () => ({ id: 'mem-1', bank: 'world-facts', importance: 0.8 }),
      };
    });

    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<MemoryPanel sessionId="sess-1" />);
    });
    const root = tree!.root;

    // Expand the panel.
    act(() => {
      root.findAllByType('button')[0].props.onClick();
    });

    // Recall with empty query (inventory mode).
    const recallBtn = root.findByProps({ 'aria-label': 'Recall memory' });
    await act(async () => {
      await recallBtn.props.onClick();
    });
    const recallCall = calls.find((c) => c.url.includes('/api/memory/recall'));
    expect(recallCall).toBeTruthy();
    expect(recallCall!.url).toContain('session_id=sess-1');
    const rendered = JSON.stringify(tree!.toJSON());
    expect(rendered).toContain('prior anchor note');
    expect(rendered).toContain('inventory');

    // Retain posts content + bank + explicit weight.
    const inputs = root.findAllByType('input');
    const noteInput = inputs.find((i) => i.props['aria-label'] === 'Operator note to retain');
    act(() => {
      noteInput!.props.onChange({ target: { value: 'operator note' } });
    });
    const selects = root.findAllByType('select');
    const weightSelect = selects.find((s) => s.props['aria-label'] === 'Note importance weight');
    act(() => {
      weightSelect!.props.onChange({ target: { value: '0.8' } });
    });
    const retainBtn = root.findByProps({ 'aria-label': 'Retain operator note' });
    await act(async () => {
      await retainBtn.props.onClick();
    });
    const retainCall = calls.find((c) => c.url.includes('/api/memory/retain'));
    expect(retainCall).toBeTruthy();
    const sentBody = JSON.parse(String(retainCall!.init?.body));
    expect(sentBody.content).toBe('operator note');
    expect(sentBody.importance).toBe(0.8);
  });
});
