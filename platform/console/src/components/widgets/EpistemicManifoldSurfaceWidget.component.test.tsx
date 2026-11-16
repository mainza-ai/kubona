import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { EpistemicManifoldSurfaceWidget } from './EpistemicManifoldSurfaceWidget';

describe('EpistemicManifoldSurfaceWidget in a real browser', () => {
  it('shows an empty geometry state without template values', async () => {
    await render(<EpistemicManifoldSurfaceWidget />);
    await expect.poll(() => document.body.textContent).toContain('No epistemic manifold geometry is recorded');
    expect(document.body.textContent).not.toContain('4.218');
    expect(document.body.textContent).not.toContain('0.64');
    expect(document.body.textContent).not.toContain('-1.842');
  });

  it('renders supplied geometry without substituting missing fields', async () => {
    await render(<EpistemicManifoldSurfaceWidget currentCurvatureScalar={-0.4} fisherInfoNorm={2.5} />);
    await expect.poll(() => document.body.textContent).toContain('-0.400');
    expect(document.body.textContent).toContain('2.500');
    expect(document.body.textContent).not.toContain('ΔS = -');
  });
});
