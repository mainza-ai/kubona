import { describe, it, expect } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { QuantumCircuitQIREditorWidget } from './QuantumCircuitQIREditorWidget';

// Audit 2026-10-21: the QIR copy button was icon-only with no accessible
// name (screen readers announced nothing). Pins the aria-label fix.
describe('QuantumCircuitQIREditorWidget a11y', () => {
  it('names the QIR copy button for assistive tech', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<QuantumCircuitQIREditorWidget qirCode="define void @main() #0 {}" />);
    });
    const qirTab = tree!.root.findAllByType('button').find((b) => b.children.includes('QIR LLVM'));
    expect(qirTab).toBeDefined();
    act(() => {
      (qirTab!.props as { onClick: () => void }).onClick();
    });
    const labels = tree!.root
      .findAllByType('button')
      .map((b) => (b.props as { 'aria-label'?: string })['aria-label']);
    expect(labels).toContain('Copy QIR to clipboard');
  });
});
