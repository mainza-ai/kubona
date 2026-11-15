import { describe, expect, it, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { CONSEQUENTIAL_SETTLE_MS, useConsequentialSettle } from './consequential';

function Probe({ active, settleMs }: { active: boolean; settleMs?: number }) {
  const settled = useConsequentialSettle(active, settleMs);
  return <span data-testid="settled">{settled ? 'ready' : 'inert'}</span>;
}

function state(tree: TestRenderer.ReactTestRenderer) {
  return tree.root.findByProps({ 'data-testid': 'settled' }).props.children;
}

afterEach(() => {
  vi.useRealTimers();
});

describe('useConsequentialSettle', () => {
  it('stays inert for the window, then releases', () => {
    vi.useFakeTimers();
    let tree!: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<Probe active settleMs={CONSEQUENTIAL_SETTLE_MS} />);
    });
    expect(state(tree)).toBe('inert');
    act(() => {
      vi.advanceTimersByTime(CONSEQUENTIAL_SETTLE_MS - 10);
    });
    expect(state(tree)).toBe('inert');
    act(() => {
      vi.advanceTimersByTime(20);
    });
    expect(state(tree)).toBe('ready');
  });

  it('never paints a re-activated control as ready in the first frame', () => {
    vi.useFakeTimers();
    let tree!: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<Probe active={false} settleMs={CONSEQUENTIAL_SETTLE_MS} />);
    });
    expect(state(tree)).toBe('ready');
    act(() => {
      tree.update(<Probe active settleMs={CONSEQUENTIAL_SETTLE_MS} />);
    });
    // Same render that observes `active` must already be inert.
    expect(state(tree)).toBe('inert');
    act(() => {
      vi.advanceTimersByTime(CONSEQUENTIAL_SETTLE_MS + 10);
    });
    expect(state(tree)).toBe('ready');
  });

  it('treats a zero window as an explicit test-only bypass', () => {
    let tree!: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<Probe active settleMs={0} />);
    });
    expect(state(tree)).toBe('ready');
  });
});
