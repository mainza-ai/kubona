import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React from 'react';
import { CommandPalette } from './CommandPalette';

const baseProps = {
  onClose: vi.fn(),
  onSelectSurface: vi.fn(),
  onNewCampaign: vi.fn(),
  onEmergencyAbort: vi.fn(),
  onOpenAbout: vi.fn(),
};

describe('CommandPalette focus trap in a real browser', () => {
  it('renders a modal dialog and lands focus on the search input', async () => {
    const screen = await render(<CommandPalette isOpen={true} {...baseProps} />);

    const dialog = screen.getByRole('dialog', { name: 'Command palette' });
    await expect.element(dialog).toHaveAttribute('aria-modal', 'true');

    const input = screen.getByLabelText('Search surfaces and actions');
    await expect.element(input).toBeVisible();
    await expect.element(input).toHaveFocus();
  });

  it('filters on typing and closes on Escape', async () => {
    const onClose = vi.fn();
    const screen = await render(
      <CommandPalette isOpen={true} {...baseProps} onClose={onClose} />,
    );

    const input = screen.getByLabelText('Search surfaces and actions');
    await input.fill('tribunal');

    await expect.element(screen.getByText(/Epistemic Tribunal Room/)).toBeVisible();
    expect(screen.container.textContent).not.toContain('4D Flight Recorder');

    await userEvent.keyboard('{Escape}');
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('traps Shift+Tab from the search input to the last result', async () => {
    const screen = await render(<CommandPalette isOpen={true} {...baseProps} />);

    const input = screen.getByLabelText('Search surfaces and actions');
    await expect.element(input).toHaveFocus();

    await userEvent.keyboard('{Shift>}{Tab}{/Shift}');

    await expect
      .element(screen.getByRole('button', { name: /Emergency Abort/ }))
      .toHaveFocus();
    await expect.element(input).not.toHaveFocus();
  });
});
