import { describe, it, expect } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { CryptographicMandateCardWidget } from './CryptographicMandateCardWidget';

describe('CryptographicMandateCardWidget proposals', () => {
  it('states the gap and disables signing without a proposal', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<CryptographicMandateCardWidget />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('No action proposed');
    expect(text).not.toContain('FeMoco');
    expect(text).not.toContain('48.50');
  });

  it('renders explicit proposals unchanged', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <CryptographicMandateCardWidget
          actionDescription="Dispatch H2 Bell to local simulator"
          estimatedCostUsd={0}
          targetBackend="local_simulator"
        />
      );
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('Dispatch H2 Bell to local simulator');
    expect(text).toContain('Authorize via Ed25519 Card');
  });
});
