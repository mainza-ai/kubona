import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { ProvenanceBadge, type ProvenanceKind } from './ProvenanceBadge';

const KINDS: ProvenanceKind[] = ['measured', 'proposed', 'catalog', 'model'];

describe('ProvenanceBadge in a real browser', () => {
  it('renders every controlled kind with its data attribute and title', async () => {
    for (const kind of KINDS) {
      const screen = await render(<ProvenanceBadge kind={kind} />);
      const badge = screen.getByTestId(`provenance-${kind}`);
      await expect.element(badge).toHaveAttribute('data-provenance', kind);
      expect(badge.element().getAttribute('title')).toBeTruthy();
    }
  });
});
