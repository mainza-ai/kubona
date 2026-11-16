import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { AppearanceSettings } from './AppearanceSettings';
import { APPEARANCE_STORAGE_KEY, TINT_OPTIONS } from '@/lib/appearance';

// The settings panel writes through to the real document root and localStorage,
// so every test starts and ends with the same clean axis state.
function resetAppearanceAxes() {
  const root = document.documentElement;
  root.classList.remove('dark');
  delete root.dataset.theme;
  delete root.dataset.tint;
  delete root.dataset.contrast;
  delete root.dataset.density;
  delete root.dataset.reduceMotion;
  window.localStorage.removeItem(APPEARANCE_STORAGE_KEY);
}

beforeEach(resetAppearanceAxes);
afterEach(resetAppearanceAxes);

describe('AppearanceSettings in a real browser', () => {
  it('applies theme, tint, contrast, density and reduced motion to the real document, then resets', async () => {
    const screen = await render(<AppearanceSettings />);
    const root = document.documentElement;

    // Every accent tint is exposed as a radio in the tint group.
    const tintRadios = screen.container.querySelectorAll(
      '[role="radiogroup"][aria-label="Accent tint"] [role="radio"]',
    );
    expect(tintRadios).toHaveLength(TINT_OPTIONS.length);
    await expect.element(screen.getByRole('radio', { name: 'Graphite' })).toHaveAttribute('aria-checked', 'true');

    // Tint writes the axis onto the document root.
    await screen.getByRole('radio', { name: 'Lavender' }).click();
    await expect.element(screen.getByRole('radio', { name: 'Lavender' })).toHaveAttribute('aria-checked', 'true');
    expect(root.dataset.tint).toBe('lavender');

    // Light paper inverts the theme and drops the dark class.
    await screen.getByRole('radio', { name: 'Light paper' }).click();
    await expect.element(screen.getByRole('radio', { name: 'Light paper' })).toHaveAttribute('aria-checked', 'true');
    await expect.poll(() => root.dataset.theme).toBe('light');
    expect(root.classList.contains('dark')).toBe(false);

    // Legibility axes.
    await screen.getByText('Increase contrast').click();
    await expect.poll(() => root.dataset.contrast).toBe('true');

    await screen.getByRole('radio', { name: 'Compact' }).click();
    await expect.poll(() => root.dataset.density).toBe('compact');

    await screen.getByText('Reduce motion').click();
    await expect.poll(() => root.dataset.reduceMotion).toBe('true');

    // Reset restores the warm-graphite defaults and clears every optional axis.
    await screen.getByRole('button', { name: 'Reset defaults' }).click();
    await expect.element(screen.getByRole('radio', { name: 'Dark graphite' })).toHaveAttribute('aria-checked', 'true');
    await expect.poll(() => root.dataset.theme).toBe('dark');
    expect(root.classList.contains('dark')).toBe(true);
    expect(root.dataset.tint).toBeUndefined();
    expect(root.dataset.contrast).toBeUndefined();
    expect(root.dataset.density).toBeUndefined();
    expect(root.dataset.reduceMotion).toBeUndefined();
  });
});
