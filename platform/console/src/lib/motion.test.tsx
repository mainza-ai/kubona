import { afterEach, describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { prefersReducedMotion, useReducedMotion } from './motion';

type Listener = () => void;

function stubMotion({ media = false, attr = false } = {}) {
  const listeners = new Set<Listener>();
  const mql = {
    matches: media,
    addEventListener: (_: string, listener: Listener) => listeners.add(listener),
    removeEventListener: (_: string, listener: Listener) => listeners.delete(listener),
  };
  vi.stubGlobal('window', {
    matchMedia: () => mql,
  });
  vi.stubGlobal('document', { documentElement: { dataset: attr ? { reduceMotion: 'true' } : {} } });
  return {
    setMedia(next: boolean) {
      mql.matches = next;
      for (const listener of listeners) listener();
    },
  };
}

afterEach(() => {
  vi.unstubAllGlobals();
});

function Probe() {
  const reduced = useReducedMotion();
  return <span data-testid="reduced">{reduced ? 'reduced' : 'full'}</span>;
}

describe('reduced motion (D3)', () => {
  it('unions the OS preference and the in-app attribute', () => {
    stubMotion({ media: true, attr: false });
    expect(prefersReducedMotion()).toBe(true);
    stubMotion({ media: false, attr: true });
    expect(prefersReducedMotion()).toBe(true);
    stubMotion({ media: false, attr: false });
    expect(prefersReducedMotion()).toBe(false);
  });

  it('reacts to a live OS preference change', async () => {
    const control = stubMotion({ media: false });
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<Probe />);
    });
    expect(tree.root.findByProps({ 'data-testid': 'reduced' }).props.children).toBe('full');
    await act(async () => {
      control.setMedia(true);
    });
    expect(tree.root.findByProps({ 'data-testid': 'reduced' }).props.children).toBe('reduced');
  });
});
