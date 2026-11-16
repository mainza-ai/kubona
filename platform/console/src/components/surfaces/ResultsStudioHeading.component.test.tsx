import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { ResultsStudio } from './ResultsStudio';
import type { CampaignState } from '@/types';

describe('ResultsStudio heading order in a real browser', () => {
  it('has no h1; the paper title is an h2', async () => {
    const campaign = { target_system: 'H2', notebook: {} } as unknown as CampaignState;
    const screen = await render(<ResultsStudio campaign={campaign} />);
    expect(screen.container.querySelectorAll('h1')).toHaveLength(0);
    expect(screen.container.querySelectorAll('h2').length).toBeGreaterThan(0);
    await expect.poll(() => document.body.textContent).toContain('No citable result is recorded');
  });
});
