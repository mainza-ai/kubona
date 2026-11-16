import { afterEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import { cdp } from 'vitest/browser';
import React from 'react';
import { useReducedMotion } from './motion';

// Migrated from react-test-renderer (motion.test.tsx). The old suite faked
// window.matchMedia with a listener set and asserted on the renderer's props.
// Here the hook runs against the real browser: the OS preference is changed
// through the DevTools protocol (so the real MediaQueryList fires `change`)
// and the in-app axis is the real `data-reduce-motion` attribute on the
// document root, observed by the real MutationObserver.

// CDPSession is an opaque marker in Vitest's public types; only the two
// Emulation calls are used here.
type EmulationSession = {
  send(method: string, params?: Record<string, unknown>): Promise<unknown>;
};

function emulateReducedMotion(value: 'reduce' | 'no-preference') {
  return (cdp() as unknown as EmulationSession).send('Emulation.setEmulatedMedia', {
    features: [{ name: 'prefers-reduced-motion', value }],
  });
}

function Probe() {
  const reduced = useReducedMotion();
  return <span data-testid="reduced">{reduced ? 'reduced' : 'full'}</span>;
}

afterEach(async () => {
  delete document.documentElement.dataset.reduceMotion;
  await emulateReducedMotion('no-preference');
});

describe('useReducedMotion in a real browser (D3)', () => {
  it('reacts to a live OS preference change', async () => {
    const screen = await render(<Probe />);
    await expect.element(screen.getByTestId('reduced')).toHaveTextContent('full');

    await emulateReducedMotion('reduce');
    await expect.element(screen.getByTestId('reduced')).toHaveTextContent('reduced');

    await emulateReducedMotion('no-preference');
    await expect.element(screen.getByTestId('reduced')).toHaveTextContent('full');
    await screen.unmount();
  });

  it('reacts to the in-app reduce-motion attribute', async () => {
    const screen = await render(<Probe />);
    await expect.element(screen.getByTestId('reduced')).toHaveTextContent('full');

    document.documentElement.dataset.reduceMotion = 'true';
    await expect.element(screen.getByTestId('reduced')).toHaveTextContent('reduced');

    delete document.documentElement.dataset.reduceMotion;
    await expect.element(screen.getByTestId('reduced')).toHaveTextContent('full');
    await screen.unmount();
  });

  it('stays reduced when the OS preference and the attribute both say so', async () => {
    await emulateReducedMotion('reduce');
    document.documentElement.dataset.reduceMotion = 'true';

    const screen = await render(<Probe />);
    await expect.element(screen.getByTestId('reduced')).toHaveTextContent('reduced');
    await screen.unmount();
  });
});
