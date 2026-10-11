import { describe, it, expect } from 'vitest';
import React from 'react';
import TestRenderer, { act, type ReactTestInstance } from 'react-test-renderer';
import { Tip } from './Tip';

function textOf(node: ReactTestInstance | string | number): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const children = (node as ReactTestInstance).children ?? [];
  return children.map((c) => textOf(c as string | number | ReactTestInstance)).join(' ');
}

describe('Tip', () => {
  it('renders label and sub with accessible tooltip role', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <Tip label="Clear chat" sub="Deletes messages" position="bottom">
          <button aria-label="Clear chat history">x</button>
        </Tip>
      );
    });
    const tip = tree!.root.findByProps({ role: 'tooltip' });
    const text = textOf(tip);
    expect(text).toContain('Clear chat');
    expect(text).toContain('Deletes messages');
  });
});
