import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { CitationBadge } from './CitationBadge';
import type { CitationReport } from '@/types';

const verified: CitationReport = {
  observations: [{ id: 'o1', tool: 'get_job' }],
  claims: [
    { number: '-1.13727', citation: 'o1', status: 'verified' },
    { number: '1024', citation: 'o1', status: 'verified' },
  ],
  verified: 2,
  unverified: 0,
};

const partial: CitationReport = {
  observations: [{ id: 'o1', tool: 'get_job' }],
  claims: [
    { number: '-1.13727', citation: 'o1', status: 'verified' },
    { number: '0.7414', status: 'uncited' },
    { number: '5.5', citation: 'o9', status: 'unknown_observation' },
  ],
  verified: 1,
  unverified: 2,
};

describe('CitationBadge in a real browser', () => {
  it('states verified claims plainly', async () => {
    const screen = await render(<CitationBadge report={verified} />);
    const badge = screen.getByTestId('citation-badge');
    await expect.element(badge).toHaveAttribute('data-state', 'verified');
    await expect.element(badge).toHaveTextContent('2/2 numeric claims verified');
  });

  it('names unverified claims instead of hiding them', async () => {
    const screen = await render(<CitationBadge report={partial} />);
    const badge = screen.getByTestId('citation-badge');
    const element = badge.element();
    expect(element.getAttribute('data-state')).toBe('partial');
    expect(element.textContent).toContain('1/3 numeric claims verified');
    expect(element.textContent).toContain('2 unverified');
    expect(element.getAttribute('title')).toContain('0.7414 — no citation');
    expect(element.getAttribute('title')).toContain('[o9] — unknown observation id');
  });

  it('renders nothing without a report', async () => {
    const screen = await render(<CitationBadge report={null} />);
    await expect.element(screen.getByTestId('citation-badge')).not.toBeInTheDocument();
  });
});
