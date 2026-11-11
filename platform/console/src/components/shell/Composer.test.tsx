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

  it('attaches a readable text file and detaches it', async () => {
    const onAttach = vi.fn();
    const onDetach = vi.fn();
    const onOpenModel = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => { tree = TestRenderer.create(<Composer onSubmit={vi.fn()} onAttach={onAttach} onDetach={onDetach} onOpenModel={onOpenModel} />); });
    const input = tree!.root.findByType('input');
    await act(async () => {
      await input.props.onChange({ target: { files: [{ name: 'notes.txt', text: async () => 'hello' }], value: 'x' } });
    });
    expect(onAttach).toHaveBeenCalledWith('notes.txt', 'hello');
    expect(tree!.root.findAll((node) => node.props.children === 'notes.txt')).toHaveLength(1);
    const remove = tree!.root.findByProps({ 'aria-label': 'Remove notes.txt' });
    act(() => { remove.props.onClick(); });
    expect(onDetach).toHaveBeenCalledWith('notes.txt');
    act(() => { tree!.root.findByProps({ 'aria-label': 'Open Model Hub' }).props.onClick(); });
    expect(onOpenModel).not.toHaveBeenCalled();
    const settingsBtn = tree!.root.findAllByType('button').find((b) => String(b.props.children).includes('Open LLM Settings'));
    expect(settingsBtn).toBeTruthy();
    act(() => { settingsBtn!.props.onClick(); });
    expect(onOpenModel).toHaveBeenCalled();
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
