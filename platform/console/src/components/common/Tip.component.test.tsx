import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { Tip } from './Tip';

describe('Tip in a real browser', () => {
  it('renders the label and sub-line in an accessible tooltip', async () => {
    const screen = await render(
      <Tip label="Clear chat" sub="Deletes messages" position="bottom">
        <button aria-label="Clear chat history">x</button>
      </Tip>,
    );
    const tooltip = screen.getByRole('tooltip');
    expect(tooltip.element().textContent).toContain('Clear chat');
    expect(tooltip.element().textContent).toContain('Deletes messages');
    await expect.element(screen.getByRole('button', { name: 'Clear chat history' })).toBeVisible();
  });
});
