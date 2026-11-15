import { describe, it, expect, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import ConfirmDialog from './ConfirmDialog';

function renderDialog(props: Partial<React.ComponentProps<typeof ConfirmDialog>> = {}) {
  let renderer: TestRenderer.ReactTestRenderer | undefined;
  act(() => {
    renderer = TestRenderer.create(
      <ConfirmDialog
        open
        title="Refund $12.50"
        body="Irreversible."
        requireText="REFUND 12.50"
        confirmLabel="Refund $12.50"
        danger
        onConfirm={() => undefined}
        onCancel={() => undefined}
        {...props}
      />,
    );
  });
  return renderer!;
}

function buttonsOf(renderer: TestRenderer.ReactTestRenderer) {
  return renderer.root.findAllByType('button');
}

describe('ConfirmDialog (typed irreversible-action gate)', () => {
  it('requires the exact typed text before enabling confirm', () => {
    const onConfirm = vi.fn();
    const r = renderDialog({ onConfirm, settleMs: 0 });
    const confirm = buttonsOf(r).find((b) => b.props.children === 'Refund $12.50')!;
    expect(confirm.props.disabled).toBe(true);
    const input = r.root.findAllByType('input')[0];
    act(() => {
      input.props.onChange({ target: { value: 'REFUND 12.5' } });
    });
    expect(buttonsOf(r).find((b) => b.props.children === 'Refund $12.50')!.props.disabled).toBe(true);
    act(() => {
      input.props.onChange({ target: { value: 'REFUND 12.50' } });
    });
    expect(buttonsOf(r).find((b) => b.props.children === 'Refund $12.50')!.props.disabled).not.toBe(true);
    act(() => {
      buttonsOf(r).find((b) => b.props.children === 'Refund $12.50')!.props.onClick();
    });
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it('exposes a labeled alertdialog and announces errors', () => {
    const r = renderDialog({ error: 'refused: ceiling exceeded' });
    const dlg = r.root.findByProps({ role: 'alertdialog' });
    expect(dlg.props['aria-label']).toBe('Refund $12.50');
    expect(r.root.findByProps({ role: 'alert' }).children.join('')).toContain('refused');
  });

  it('renders nothing when closed', () => {
    const r = renderDialog({ open: false });
    expect(r.toJSON()).toBeNull();
  });

  it('single-confirm mode (no typed text) enables confirm directly', () => {
    const onConfirm = vi.fn();
    const r = renderDialog({ requireText: undefined, confirmLabel: 'Abort now', onConfirm, settleMs: 0 });
    const confirm = buttonsOf(r).find((b) => b.props.children === 'Abort now')!;
    expect(confirm.props.disabled).not.toBe(true);
    act(() => {
      confirm.props.onClick();
    });
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it('holds confirm for the settle window and focuses cancel unless typing', () => {
    vi.useFakeTimers();
    const r = renderDialog({ requireText: undefined, confirmLabel: 'Release air-gap' });
    const confirm = () => buttonsOf(r).find((b) => b.props['data-settle'])!;
    const cancel = buttonsOf(r).find((b) => b.props.children === 'Cancel')!;
    expect(confirm().props.disabled).toBe(true);
    expect(confirm().props['data-settle']).toBe('pending');
    expect(cancel.props.autoFocus).toBe(true);
    expect(confirm().props.autoFocus).toBeUndefined();
    act(() => {
      vi.advanceTimersByTime(500);
    });
    expect(confirm().props.disabled).not.toBe(true);
    expect(confirm().props['data-settle']).toBe('ready');
    vi.useRealTimers();
  });

  it('focuses the typed field instead of cancel, and escape does not confirm', () => {
    const onConfirm = vi.fn();
    const onCancel = vi.fn();
    const listeners = new Map<string, (event: { key: string; preventDefault: () => void; stopPropagation: () => void }) => void>();
    vi.stubGlobal('window', {
      addEventListener: (type: string, fn: (event: { key: string; preventDefault: () => void; stopPropagation: () => void }) => void) => {
        listeners.set(type, fn);
      },
      removeEventListener: (type: string) => listeners.delete(type),
    });
    const r = renderDialog({ onConfirm, onCancel, settleMs: 0 });
    expect(r.root.findAllByType('input')[0].props.autoFocus).toBe(true);
    expect(buttonsOf(r).find((b) => b.props.children === 'Cancel')!.props.autoFocus).toBe(false);
    act(() => {
      listeners.get('keydown')?.({ key: 'Escape', preventDefault() {}, stopPropagation() {} });
    });
    expect(onCancel).toHaveBeenCalledTimes(1);
    expect(onConfirm).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });
});
