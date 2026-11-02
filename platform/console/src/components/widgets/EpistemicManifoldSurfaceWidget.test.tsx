import { describe, it, expect } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { EpistemicManifoldSurfaceWidget } from './EpistemicManifoldSurfaceWidget';

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(textOf).join(' ');
  if (node !== null && typeof node === 'object') {
    const obj = node as { children?: unknown; props?: { children?: unknown } };
    if ('children' in obj && obj.children != null) return textOf(obj.children);
    if ('props' in obj && obj.props?.children != null) return textOf(obj.props.children);
  }
  return '';
}

describe('EpistemicManifoldSurfaceWidget', () => {
  it('shows an empty geometry state without template values', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<EpistemicManifoldSurfaceWidget />);
    });
    const text = textOf(tree!.toJSON());
    expect(text).toContain('No epistemic manifold geometry is recorded');
    expect(text).not.toContain('4.218');
    expect(text).not.toContain('0.64');
    expect(text).not.toContain('-1.842');
  });

  it('renders supplied geometry without substituting missing fields', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <EpistemicManifoldSurfaceWidget currentCurvatureScalar={-0.4} fisherInfoNorm={2.5} />
      );
    });
    const text = textOf(tree!.toJSON());
    expect(text).toContain('-0.400');
    expect(text).toContain('2.500');
    expect(text).not.toContain('ΔS = -');
  });
});
