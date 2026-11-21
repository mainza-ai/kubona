import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React, { useRef, useState } from 'react';
import { useDismissiblePopover } from './useDismissiblePopover';

function Harness({ onClosed }: { onClosed: () => void }) {
  const [open, setOpen] = useState(true);
  const panelRef = useRef<HTMLDivElement | null>(null);
  useDismissiblePopover(
    open,
    panelRef,
    () => {
      setOpen(false);
      onClosed();
    },
    { ignoreSelector: '[data-popover-trigger="test"]' },
  );

  return (
    <div>
      <button type="button" data-popover-trigger="test" onClick={() => setOpen((v) => !v)}>
        Toggle
      </button>
      <div data-testid="outside">Outside area</div>
      {open && (
        <div ref={panelRef} data-testid="popover">
          <button type="button">Inside action</button>
        </div>
      )}
    </div>
  );
}

describe('useDismissiblePopover in a real browser', () => {
  it('closes on outside click and ignores clicks inside the popover', async () => {
    const onClosed = vi.fn();
    const screen = await render(<Harness onClosed={onClosed} />);

    await expect.element(screen.getByTestId('popover')).toBeVisible();
    await screen.getByRole('button', { name: 'Inside action' }).click();
    await expect.element(screen.getByTestId('popover')).toBeVisible();
    expect(onClosed).toHaveBeenCalledTimes(0);

    await screen.getByTestId('outside').click();
    await expect.poll(() => screen.container.querySelector('[data-testid="popover"]')).toBeNull();
    expect(onClosed).toHaveBeenCalledTimes(1);
  });

  it('closes on Escape and toggles cleanly via its trigger without reopening bounce', async () => {
    const onClosed = vi.fn();
    const screen = await render(<Harness onClosed={onClosed} />);

    // Clicking trigger closes once (does not bounce open again).
    await screen.getByRole('button', { name: 'Toggle' }).click();
    await expect.poll(() => screen.container.querySelector('[data-testid="popover"]')).toBeNull();

    // Reopen via trigger, then dismiss with Escape.
    await screen.getByRole('button', { name: 'Toggle' }).click();
    await expect.element(screen.getByTestId('popover')).toBeVisible();
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    await expect.poll(() => screen.container.querySelector('[data-testid="popover"]')).toBeNull();
    expect(onClosed).toHaveBeenCalledTimes(1);
  });
});
