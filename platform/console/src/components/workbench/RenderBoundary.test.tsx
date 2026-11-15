import { describe, expect, it, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { RenderBoundary } from './RenderBoundary';

let shouldThrow = true;

function Bomb() {
  if (shouldThrow) throw new Error('card exploded');
  return <span data-testid="recovered">card recovered</span>;
}

afterEach(() => {
  vi.restoreAllMocks();
  shouldThrow = true;
});

describe('RenderBoundary (C3)', () => {
  it('contains one card failure and retries without touching the rest', () => {
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {});
    let tree!: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <div>
          <span data-testid="neighbour">scene alive</span>
          <RenderBoundary
            label="card a"
            fallback={(reset) => (
              <button type="button" data-testid="retry" onClick={reset}>
                Retry
              </button>
            )}
          >
            <Bomb />
          </RenderBoundary>
        </div>,
      );
    });
    // The neighbour is untouched; only the bad card paid.
    expect(tree.root.findByProps({ 'data-testid': 'neighbour' })).toBeTruthy();
    expect(tree.root.findByProps({ 'data-testid': 'retry' })).toBeTruthy();
    expect(errorSpy).toHaveBeenCalled();

    shouldThrow = false;
    act(() => {
      tree.root.findByProps({ 'data-testid': 'retry' }).props.onClick();
    });
    expect(tree.root.findByProps({ 'data-testid': 'recovered' })).toBeTruthy();
  });
});
