import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { ShadowTomographyRadarWidget } from './ShadowTomographyRadarWidget';

describe('ShadowTomographyRadarWidget in a real browser', () => {
  it('shows an empty measurement state without reference estimates', async () => {
    await render(<ShadowTomographyRadarWidget numQubits={4} />);
    await expect.poll(() => document.body.textContent).toContain('No shadow-tomography execution is recorded');
    expect(document.body.textContent).not.toContain('14.8x');
    expect(document.body.textContent).not.toContain('99.2%');
    expect(document.body.textContent).not.toContain('Converged');
  });
});
