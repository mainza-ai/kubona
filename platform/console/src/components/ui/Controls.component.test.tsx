import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import { userEvent } from 'vitest/browser';
import React, { useState } from 'react';
import { IconButton, SegmentedControl } from './Controls';

function SegmentedHarness() {
  const [value, setValue] = useState<'alpha' | 'beta' | 'gamma'>('alpha');
  return (
    <SegmentedControl
      label="View mode"
      value={value}
      onChange={setValue}
      segments={[
        { value: 'alpha', label: 'Alpha' },
        { value: 'beta', label: 'Beta' },
        { value: 'gamma', label: 'Gamma' },
      ]}
    />
  );
}

describe('SegmentedControl in a real browser', () => {
  it('exposes one checked radio and a roving tabindex', async () => {
    const screen = await render(<SegmentedHarness />);
    const radios = screen.getByRole('radio');

    await expect.element(radios.nth(0)).toHaveAttribute('aria-checked', 'true');
    await expect.element(radios.nth(0)).toHaveAttribute('tabindex', '0');
    await expect.element(radios.nth(1)).toHaveAttribute('aria-checked', 'false');
    await expect.element(radios.nth(1)).toHaveAttribute('tabindex', '-1');
  });

  it('moves selection and focus with arrows, End and Home', async () => {
    const screen = await render(<SegmentedHarness />);
    const radios = screen.getByRole('radio');

    await radios.nth(0).click();
    await expect.element(radios.nth(0)).toHaveFocus();

    await userEvent.keyboard('{ArrowRight}');
    await expect.element(radios.nth(1)).toHaveFocus();
    await expect.element(radios.nth(1)).toHaveAttribute('aria-checked', 'true');
    await expect.element(radios.nth(0)).toHaveAttribute('tabindex', '-1');
    await expect.element(radios.nth(1)).toHaveAttribute('tabindex', '0');

    await userEvent.keyboard('{End}');
    await expect.element(radios.nth(2)).toHaveAttribute('aria-checked', 'true');

    await userEvent.keyboard('{ArrowRight}');
    await expect.element(radios.nth(0)).toHaveFocus();
    await expect.element(radios.nth(0)).toHaveAttribute('aria-checked', 'true');

    await userEvent.keyboard('{End}');
    await userEvent.keyboard('{Home}');
    await expect.element(radios.nth(0)).toHaveFocus();
  });

  it('keeps IconButton a non-submitting button inside a form', async () => {
    let submitted = false;
    const screen = await render(
      <form
        onSubmit={(event) => {
          event.preventDefault();
          submitted = true;
        }}
      >
        <IconButton label="Close" />
      </form>,
    );
    const button = screen.getByRole('button', { name: 'Close' });

    await expect.element(button).toHaveAttribute('type', 'button');
    await button.click();
    expect(submitted).toBe(false);
  });
});
