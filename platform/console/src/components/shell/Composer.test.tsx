import { describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { Composer } from './Composer';

function render(onSubmit: (text: string) => Promise<void> | void) {
  let tree: TestRenderer.ReactTestRenderer;
  act(() => { tree = TestRenderer.create(<Composer onSubmit={onSubmit} contextLabel="Campaign: H2" />); });
  return tree!;
}

describe('Composer', () => {
  it('sends on Enter and clears the field', async () => {
    const onSubmit = vi.fn(async () => {});
    const tree = render(onSubmit);
    const area = tree.root.findByType('textarea');
    act(() => { area.props.onChange({ target: { value: 'run the campaign' } }); });
    const preventDefault = vi.fn();
    await act(async () => {
      area.props.onKeyDown({ key: 'Enter', shiftKey: false, nativeEvent: { isComposing: false }, preventDefault });
    });
    expect(preventDefault).toHaveBeenCalled();
    expect(onSubmit).toHaveBeenCalledWith('run the campaign');
    expect(tree.root.findByType('textarea').props.value).toBe('');
  });

  it('keeps Shift+Enter and IME composition as text entry', async () => {
    const onSubmit = vi.fn();
    const tree = render(onSubmit);
    const area = tree.root.findByType('textarea');
    act(() => { area.props.onChange({ target: { value: 'line' } }); });
    act(() => {
      area.props.onKeyDown({ key: 'Enter', shiftKey: true, nativeEvent: { isComposing: false }, preventDefault: vi.fn() });
      area.props.onKeyDown({ key: 'Enter', shiftKey: false, nativeEvent: { isComposing: true }, preventDefault: vi.fn() });
    });
    expect(onSubmit).not.toHaveBeenCalled();
  });
});
