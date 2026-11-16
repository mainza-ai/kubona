import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { CONSEQUENTIAL_SETTLE_MS, useConsequentialSettle } from './consequential';

function Probe({ active, settleMs }: { active: boolean; settleMs?: number }) {
  const settled = useConsequentialSettle(active, settleMs);
  return <span data-testid="settled">{settled ? 'ready' : 'inert'}</span>;
}

afterEach(() => vi.useRealTimers());

describe('useConsequentialSettle in a real browser', () => {
  it('stays inert for the window, then releases', async () => {
    vi.useFakeTimers();
    const screen = await render(<Probe active settleMs={CONSEQUENTIAL_SETTLE_MS} />);
    const settled = screen.getByTestId('settled');
    expect(settled.element().textContent).toBe('inert');
    await vi.advanceTimersByTimeAsync(CONSEQUENTIAL_SETTLE_MS - 10);
    expect(settled.element().textContent).toBe('inert');
    await vi.advanceTimersByTimeAsync(20);
    await expect.poll(() => settled.element().textContent).toBe('ready');
  });

  it('treats a zero window as an explicit bypass', async () => {
    const screen = await render(<Probe active settleMs={0} />);
    expect(screen.getByTestId('settled').element().textContent).toBe('ready');
  });
});
