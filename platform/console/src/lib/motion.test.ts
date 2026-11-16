import { afterEach, describe, expect, it, vi } from 'vitest';
import { prefersReducedMotion } from './motion';

// Pure-logic coverage for the D3 union. Migrated from react-test-renderer
// (motion.test.tsx): the boolean union needs no renderer, and the reactive
// hook path is exercised against the real MediaQueryList/MutationObserver in
// motion.component.test.tsx.

function stubMotion({ media = false, attr = false } = {}) {
  const mql = { matches: media, addEventListener: () => {}, removeEventListener: () => {} };
  vi.stubGlobal('window', { matchMedia: () => mql });
  vi.stubGlobal('document', { documentElement: { dataset: attr ? { reduceMotion: 'true' } : {} } });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('reduced motion union (D3)', () => {
  it('unions the OS preference and the in-app attribute', () => {
    stubMotion({ media: true, attr: false });
    expect(prefersReducedMotion()).toBe(true);
    stubMotion({ media: false, attr: true });
    expect(prefersReducedMotion()).toBe(true);
    stubMotion({ media: false, attr: false });
    expect(prefersReducedMotion()).toBe(false);
  });

  it('defaults to full motion when the OS query is absent or throws', () => {
    vi.stubGlobal('window', {});
    vi.stubGlobal('document', { documentElement: { dataset: {} } });
    expect(prefersReducedMotion()).toBe(false);

    vi.stubGlobal('window', {
      matchMedia: () => {
        throw new Error('no media queries here');
      },
    });
    expect(prefersReducedMotion()).toBe(false);
  });
});
