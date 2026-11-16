import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { QuantumCircuitQIREditorWidget } from './QuantumCircuitQIREditorWidget';

// Audit 2026-10-21: the QIR copy button was icon-only with no accessible
// name (screen readers announced nothing). Pins the aria-label fix in a real
// browser, where the accessibility tree is observable.
describe('QuantumCircuitQIREditorWidget empty state in a real browser', () => {
  it('shows no receipt instead of a schematic circuit or fixed overheads', async () => {
    await render(<QuantumCircuitQIREditorWidget />);
    await expect.poll(() => document.body.textContent).toContain('No post-QIR synthesis receipt is recorded');
    expect(document.body.textContent).not.toContain('4.2x');
    expect(document.body.textContent).not.toContain('12 : 288');
    expect(document.body.textContent).not.toContain('synthetic demo');
  });
});

describe('QuantumCircuitQIREditorWidget a11y in a real browser', () => {
  it('names the QIR copy button for assistive tech', async () => {
    const screen = await render(<QuantumCircuitQIREditorWidget qirCode="define void @main() #0 {}" />);
    await expect.element(screen.getByRole('button', { name: 'Copy QIR to clipboard' })).toBeVisible();
    await expect.poll(() => document.body.textContent).toContain('define void @main() #0 {}');
  });
});
