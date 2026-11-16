import { describe, it, expect, vi, afterEach } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { ErrorBoundary } from './ErrorBoundary';


afterEach(() => {
  vi.restoreAllMocks();
});

function Thrower(): React.ReactNode {
  throw new Error('boom from fixture');
}

describe('ErrorBoundary in a real browser', () => {
  it('renders healthy children', async () => {
    const screen = await render(
      <ErrorBoundary>
        <span>healthy child</span>
      </ErrorBoundary>,
    );
    await expect.element(screen.getByText('healthy child')).toBeVisible();
  });

  it('renders the fail-closed fallback, logs via reportError, and hides the thrown message', async () => {
    const consoleSpy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
    const screen = await render(
      <ErrorBoundary>
        <Thrower />
      </ErrorBoundary>,
    );
    const alert = screen.getByRole('alert');
    await expect.element(alert).toBeVisible();
    expect(alert.element().textContent).toContain('Something broke rendering this view');
    await expect.element(screen.getByRole('button', { name: 'Retry' })).toBeVisible();
    expect(alert.element().textContent).not.toContain('boom from fixture');
    consoleSpy.mockRestore();
  });
});
