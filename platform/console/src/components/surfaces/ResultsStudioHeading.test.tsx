import { describe, it, expect } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { ResultsStudio } from './ResultsStudio';
import type { CampaignState } from '@/types';

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(textOf).join(' ');
  if (node !== null && typeof node === 'object' && 'children' in node) {
    const children = (node as { children?: unknown }).children;
    if (Array.isArray(children)) return children.map(textOf).join(' ');
  }
  return '';
}

describe('ResultsStudio heading order (Wave-2)', () => {
  it('has no h1; the paper title is an h2', () => {
    const campaign = { target_system: 'H2', notebook: {} } as unknown as CampaignState;
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<ResultsStudio campaign={campaign} />);
    });
    expect(tree!.root.findAllByType('h1')).toHaveLength(0);
    const headings = tree!.root.findAllByType('h2');
    expect(headings.length).toBeGreaterThan(0);
    expect(textOf(tree!.toJSON())).toContain('Variational Quantum Ground State of H2');
  });
});
