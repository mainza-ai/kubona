import { describe, it, expect, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
// The app stylesheet, so the fixed overlay and the `fill` logo lay out as they
// do in the app; without it the absolutely-positioned image covers the footer.
import '@/app/globals.css';
import { AboutLabModal } from './AboutLabModal';

describe('AboutLabModal focus trap (Wave-2) in a real browser', () => {
  it('renders nothing when closed', async () => {
    const screen = await render(<AboutLabModal isOpen={false} onClose={vi.fn()} />);
    expect(screen.container.querySelector('[role="dialog"]')).toBeNull();
    expect(screen.container.textContent).toBe('');
  });

  it('renders a labelled modal dialog', async () => {
    const screen = await render(<AboutLabModal isOpen={true} onClose={vi.fn()} />);
    const dialog = screen.getByRole('dialog');
    await expect.element(dialog).toBeVisible();
    await expect.element(dialog).toHaveAttribute('aria-modal', 'true');
    await expect.element(dialog).toHaveAttribute('aria-labelledby', 'about-lab-title');
    await expect.element(screen.getByRole('heading', { name: 'Kubona' })).toBeVisible();
    // Build version is an injected build fact, never a hard-coded claim.
    await expect.element(screen.getByText('Build:')).toBeVisible();
  });

  it('closes via the Done button', async () => {
    const onClose = vi.fn();
    const screen = await render(<AboutLabModal isOpen={true} onClose={onClose} />);
    await screen.getByRole('button', { name: 'Done' }).click();
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('closes on Escape', async () => {
    const onClose = vi.fn();
    await render(<AboutLabModal isOpen={true} onClose={onClose} />);
    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('traps Tab inside the dialog and lands focus on the first control', async () => {
    const screen = await render(<AboutLabModal isOpen={true} onClose={vi.fn()} />);
    const close = screen.getByRole('button', { name: 'Close dialog' });
    const done = screen.getByRole('button', { name: 'Done' });
    await expect.element(close).toHaveFocus();

    await userEvent.keyboard('{Shift>}{Tab}{/Shift}');
    await expect.element(done).toHaveFocus();

    await userEvent.keyboard('{Tab}');
    await expect.element(close).toHaveFocus();
  });
});
