import { describe, it, expect } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { QuantumCircuitQIREditorWidget } from './QuantumCircuitQIREditorWidget';

// Audit 2026-10-21: the QIR copy button was icon-only with no accessible
// name (screen readers announced nothing). Pins the aria-label fix.
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

describe('QuantumCircuitQIREditorWidget empty state', () => {
  it('shows no receipt instead of a schematic circuit or fixed overheads', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<QuantumCircuitQIREditorWidget />);
    });
    const text = textOf(tree!.toJSON());
    expect(text).toContain('No post-QIR synthesis receipt is recorded');
    expect(text).not.toContain('4.2x');
    expect(text).not.toContain('12 : 288');
    expect(text).not.toContain('synthetic demo');
  });
});

describe('QuantumCircuitQIREditorWidget a11y', () => {
  it('names the QIR copy button for assistive tech', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<QuantumCircuitQIREditorWidget qirCode="define void @main() #0 {}" />);
    });
    const labels = tree!.root
      .findAllByType('button')
      .map((b) => (b.props as { 'aria-label'?: string })['aria-label']);
    expect(labels).toContain('Copy QIR to clipboard');
  });
});
