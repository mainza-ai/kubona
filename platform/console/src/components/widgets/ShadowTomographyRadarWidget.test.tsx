import { describe, it, expect } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { ShadowTomographyRadarWidget } from './ShadowTomographyRadarWidget';

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

describe('ShadowTomographyRadarWidget', () => {
  it('shows an empty measurement state without reference estimates', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<ShadowTomographyRadarWidget numQubits={4} />);
    });
    const text = textOf(tree!.toJSON());
    expect(text).toContain('No shadow-tomography execution is recorded');
    expect(text).not.toContain('14.8x');
    expect(text).not.toContain('99.2%');
    expect(text).not.toContain('Converged');
  });
});
