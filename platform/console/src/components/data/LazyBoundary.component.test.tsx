import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { LazyBoundary } from './LazyBoundary';

let armed = true;

function ThrowsWhileArmed() {
  if (armed) throw new Error('render failed');
  return <p>recovered</p>;
}

describe('LazyBoundary in a real browser', () => {
  it('refuses visibly and recovers on retry', async () => {
    armed = true;
    const screen = await render(
      <LazyBoundary label="Figure">
        <ThrowsWhileArmed />
      </LazyBoundary>,
    );
    await expect.element(screen.getByRole('alert')).toBeVisible();
    await expect.element(screen.getByText('Figure could not be rendered')).toBeVisible();
    armed = false;
    await screen.getByRole('button', { name: 'Retry' }).click();
    await expect.element(screen.getByText('recovered')).toBeVisible();
    await expect.element(screen.getByRole('alert')).not.toBeInTheDocument();
  });
});
