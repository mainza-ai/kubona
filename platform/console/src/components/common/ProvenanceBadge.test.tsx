import { describe, it, expect } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { ProvenanceBadge, type ProvenanceKind } from './ProvenanceBadge';

const KINDS: ProvenanceKind[] = ['measured', 'proposed', 'catalog', 'model'];

describe('ProvenanceBadge', () => {
  it('renders every controlled provenance kind with its data attribute', () => {
    for (const kind of KINDS) {
      let tree: TestRenderer.ReactTestRenderer;
      act(() => {
        tree = TestRenderer.create(<ProvenanceBadge kind={kind} />);
      });
      const badge = tree!.root.findByProps({ 'data-provenance': kind });
      expect(badge.props.title).toBeTruthy();
    }
  });
});
