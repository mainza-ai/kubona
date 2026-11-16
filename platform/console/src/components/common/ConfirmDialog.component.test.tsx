import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
import ConfirmDialog from './ConfirmDialog';

type Props = React.ComponentProps<typeof ConfirmDialog>;

function renderDialog(props: Partial<Props> = {}) {
  return render(
    <ConfirmDialog
      open
      title="Refund $12.50"
      body="Irreversible."
      requireText="REFUND 12.50"
      confirmLabel="Refund $12.50"
      danger
      onConfirm={() => undefined}
      onCancel={() => undefined}
      {...props}
    />,
  );
}

function disabled(locator: { element(): Element }): boolean {
  return (locator.element() as HTMLButtonElement).disabled;
}

describe('ConfirmDialog in a real browser (typed irreversible-action gate)', () => {
  it('requires the exact typed text before enabling confirm', async () => {
    const onConfirm = vi.fn();
    const screen = await renderDialog({ onConfirm, settleMs: 0 });
    const confirm = screen.getByRole('button', { name: 'Refund $12.50' });
    const input = screen.getByRole('textbox');

    expect(disabled(confirm)).toBe(true);

    await input.fill('REFUND 12.5');
    expect(disabled(confirm)).toBe(true);

    await input.fill('REFUND 12.50');
    await expect.poll(() => disabled(confirm)).toBe(false);

    await confirm.click();
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it('exposes a labeled alertdialog and announces errors', async () => {
    const screen = await renderDialog({ error: 'refused: ceiling exceeded' });

    await expect.element(screen.getByRole('alertdialog')).toHaveAttribute('aria-label', 'Refund $12.50');
    await expect.element(screen.getByRole('alert')).toHaveTextContent('refused: ceiling exceeded');
  });

  it('renders nothing when closed', async () => {
    const screen = await renderDialog({ open: false });

    expect(screen.container.querySelector('[role="alertdialog"]')).toBeNull();
    expect(screen.container.textContent).toBe('');
  });

  it('single-confirm mode (no typed text) enables confirm directly', async () => {
    const onConfirm = vi.fn();
    const screen = await renderDialog({ requireText: undefined, confirmLabel: 'Abort now', onConfirm, settleMs: 0 });
    const confirm = screen.getByRole('button', { name: 'Abort now' });

    expect(disabled(confirm)).toBe(false);

    await confirm.click();
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it('holds confirm for the settle window and focuses cancel unless typing', async () => {
    const screen = await renderDialog({ requireText: undefined, confirmLabel: 'Release air-gap' });
    const confirm = screen.getByRole('button', { name: 'Release air-gap' });
    const cancel = screen.getByRole('button', { name: 'Cancel' });

    expect(disabled(confirm)).toBe(true);
    expect(confirm.element().getAttribute('data-settle')).toBe('pending');
    await expect.element(cancel).toHaveFocus();
    await expect.element(confirm).not.toHaveFocus();

    await expect.poll(() => confirm.element().getAttribute('data-settle'), { timeout: 3000 }).toBe('ready');
    expect(disabled(confirm)).toBe(false);
  });

  it('focuses the typed field instead of cancel, and escape does not confirm', async () => {
    const onConfirm = vi.fn();
    const onCancel = vi.fn();
    const screen = await renderDialog({ onConfirm, onCancel, settleMs: 0 });

    await expect.element(screen.getByRole('textbox')).toHaveFocus();
    await expect.element(screen.getByRole('button', { name: 'Cancel' })).not.toHaveFocus();

    await userEvent.keyboard('{Escape}');

    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onConfirm).not.toHaveBeenCalled();
  });
});
