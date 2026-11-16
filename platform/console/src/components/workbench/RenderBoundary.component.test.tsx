import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { RenderBoundary } from './RenderBoundary';

let shouldThrow = true;

function Bomb() {
  if (shouldThrow) throw new Error('card exploded');
  return <span data-testid="recovered">card recovered</span>;
}

afterEach(() => {
  vi.restoreAllMocks();
  shouldThrow = true;
});

describe('RenderBoundary in a real browser', () => {
  it('contains one card failure and retries without touching the rest', async () => {
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    const screen = await render(
      <div>
        <span data-testid="neighbour">scene alive</span>
        <RenderBoundary
          label="card a"
          fallback={(reset) => (
            <button type="button" data-testid="retry" onClick={reset}>Retry</button>
          )}
        >
          <Bomb />
        </RenderBoundary>
      </div>,
    );
    await expect.element(screen.getByTestId('neighbour')).toBeVisible();
    await expect.element(screen.getByTestId('retry')).toBeVisible();
    expect(consoleSpy).toHaveBeenCalled();
    shouldThrow = false;
    await screen.getByTestId('retry').click();
    await expect.element(screen.getByTestId('recovered')).toBeVisible();
  });
});
