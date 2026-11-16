import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { CryptographicMandateCardWidget } from './CryptographicMandateCardWidget';

describe('CryptographicMandateCardWidget proposals in a real browser', () => {
  it('states the gap and disables signing without a proposal', async () => {
    const screen = await render(<CryptographicMandateCardWidget />);
    await expect.poll(() => document.body.textContent).toContain('No action proposed');
    expect(document.body.textContent).not.toContain('FeMoco');
    expect(document.body.textContent).not.toContain('48.50');
    // Signing chrome is not offered at all without a bound proposal.
    expect(screen.container.textContent).toContain('Authorize disabled — no proposal bound.');
    expect(screen.container.querySelector('button')).toBeNull();
  });

  it('renders explicit proposals unchanged', async () => {
    const screen = await render(
      <CryptographicMandateCardWidget
        actionDescription="Dispatch H2 Bell to local simulator"
        estimatedCostUsd={0}
        targetBackend="local_simulator"
      />
    );
    await expect.poll(() => document.body.textContent).toContain('Dispatch H2 Bell to local simulator');
    expect(document.body.textContent).toContain('Authorize via Ed25519 Card');
    await expect.element(screen.getByRole('button', { name: /Authorize via Ed25519 Card/ })).toBeVisible();
  });
});
