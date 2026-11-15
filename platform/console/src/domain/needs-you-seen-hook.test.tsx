import { describe, expect, it } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { useNeedsYouSeen } from './needs-you-seen';

function Probe({ mark, prune }: { mark: string[]; prune: string[] | null }) {
  const { seen, markSeen, prune: pruneSeen } = useNeedsYouSeen();
  return (
    <div>
      <button type="button" aria-label="mark" onClick={() => markSeen(mark)} />
      <button type="button" aria-label="prune" onClick={() => pruneSeen(prune)} />
      <span data-testid="size">{seen.size}</span>
      <span data-testid="list">{[...seen].sort().join(',')}</span>
    </div>
  );
}

function render(mark: string[], prune: string[] | null) {
  let tree!: TestRenderer.ReactTestRenderer;
  act(() => {
    tree = TestRenderer.create(<Probe mark={mark} prune={prune} />);
  });
  return tree;
}

function click(tree: TestRenderer.ReactTestRenderer, label: string) {
  act(() => {
    tree.root.findByProps({ 'aria-label': label }).props.onClick();
  });
}

describe('useNeedsYouSeen', () => {
  it('marks items seen, prunes inactive signatures, and re-raises a returning row', () => {
    const tree = render(['ob_1@t1', 'ob_2@t1'], ['ob_2@t1']);
    expect(tree.root.findByProps({ 'data-testid': 'size' }).props.children).toBe(0);

    click(tree, 'mark');
    expect(tree.root.findByProps({ 'data-testid': 'list' }).props.children).toBe('ob_1@t1,ob_2@t1');

    // A decision removes ob_1; its signature leaves the store.
    click(tree, 'prune');
    expect(tree.root.findByProps({ 'data-testid': 'list' }).props.children).toBe('ob_2@t1');
  });

  it('treats a new update stamp as a distinct unseen item', () => {
    const tree = render(['ob_1@t2'], null);
    click(tree, 'mark');
    expect(tree.root.findByProps({ 'data-testid': 'list' }).props.children).toBe('ob_1@t2');
  });

  it('does not prune while the list is still loading', () => {
    const tree = render(['ob_3@t1'], null);
    click(tree, 'mark');
    click(tree, 'prune');
    expect(tree.root.findByProps({ 'data-testid': 'size' }).props.children).toBe(1);
  });
});
