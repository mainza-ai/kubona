import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
import { KeyboardShortcutsModal } from './KeyboardShortcutsModal';

describe('KeyboardShortcutsModal in a real browser', () => {
  it('renders nothing when closed', async () => {
    const screen = await render(<KeyboardShortcutsModal isOpen={false} onClose={vi.fn()} />);
    expect(screen.container.querySelector('[role="dialog"]')).toBeNull();
    expect(screen.container.textContent).toBe('');
  });

  it('renders a labelled modal dialog with all three groups', async () => {
    const screen = await render(<KeyboardShortcutsModal isOpen={true} onClose={vi.fn()} />);

    const dialog = screen.getByRole('dialog', { name: 'Keyboard Shortcuts' });
    await expect.element(dialog).toBeVisible();
    await expect.element(dialog).toHaveAttribute('aria-modal', 'true');
    await expect.element(dialog).toHaveAttribute('aria-labelledby', 'shortcuts-title');

    await expect.element(screen.getByText('Surfaces Navigation')).toBeVisible();
    await expect.element(screen.getByText('Deep Links (Surface Tabs)')).toBeVisible();
    await expect.element(screen.getByText('Workspace & Windows')).toBeVisible();
  });

  it('closes via the Got it button', async () => {
    const onClose = vi.fn();
    const screen = await render(<KeyboardShortcutsModal isOpen={true} onClose={onClose} />);

    await screen.getByRole('button', { name: 'Got it' }).click();
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('closes on Escape', async () => {
    const onClose = vi.fn();
    await render(<KeyboardShortcutsModal isOpen={true} onClose={onClose} />);

    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('traps Tab inside the dialog and lands focus on the first control', async () => {
    const screen = await render(<KeyboardShortcutsModal isOpen={true} onClose={vi.fn()} />);

    const close = screen.getByRole('button', { name: 'Close' });
    const gotIt = screen.getByRole('button', { name: 'Got it' });
    await expect.element(close).toHaveFocus();

    await userEvent.keyboard('{Shift>}{Tab}{/Shift}');
    await expect.element(gotIt).toHaveFocus();

    await userEvent.keyboard('{Tab}');
    await expect.element(close).toHaveFocus();
  });
});
