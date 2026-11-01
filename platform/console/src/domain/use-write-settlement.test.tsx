import { describe, expect, it } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { useWriteSettlement } from './use-write-settlement';

function Probe() {
  const { write, begin, applied } = useWriteSettlement();
  const id = React.useRef<string | null>(null);
  return (
    <button
      type="button"
      onClick={() => {
        if (!id.current) id.current = begin('doing work');
        else applied(id.current, 'done');
      }}
    >
      {write.phase}:{write.detail}
    </button>
  );
}

describe('useWriteSettlement', () => {
  it('does not report accepted as applied', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => { tree = TestRenderer.create(<Probe />); });
    const button = tree!.root.findByType('button');
    act(() => { button.props.onClick(); });
    const accepted = JSON.stringify(tree!.toJSON());
    expect(accepted).toContain('accepted');
    expect(accepted).toContain('doing work');
    act(() => { tree!.root.findByType('button').props.onClick(); });
    const settled = JSON.stringify(tree!.toJSON());
    expect(settled).toContain('applied');
    expect(settled).toContain('done');
  });
});
