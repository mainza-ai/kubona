import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { KeyboardShortcutsModal } from './KeyboardShortcutsModal';

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubWindow(): EventTarget {
  const target = new EventTarget();
  vi.stubGlobal('window', target);
  return target;
}

function keydown(key: string): Event {
  return Object.assign(new Event('keydown'), { key }) as Event;
}

describe('KeyboardShortcutsModal focus trap (Wave-2)', () => {
  it('renders nothing when closed', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<KeyboardShortcutsModal isOpen={false} onClose={vi.fn()} />);
    });
    expect(tree!.toJSON()).toBeNull();
  });

  it('renders a labelled modal dialog without a real DOM (SSR-safe)', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<KeyboardShortcutsModal isOpen={true} onClose={vi.fn()} />);
    });
    const dialog = tree!.root.findByProps({ role: 'dialog' });
    expect(dialog.props['aria-modal']).toBe('true');
    expect(dialog.props['aria-labelledby']).toBe('shortcuts-title');
    // Content unchanged: all three groups still listed.
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('Surfaces Navigation');
    expect(text).toContain('Deep Links (Surface Tabs)');
    expect(text).toContain('Workspace & Windows');
  });

  it('closes via the Got it button', async () => {
    const onClose = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<KeyboardShortcutsModal isOpen={true} onClose={onClose} />);
    });
    const gotIt = tree!.root
      .findAllByType('button')
      .find((b) => String(b.props.children).includes('Got it'));
    expect(gotIt).toBeDefined();
    await act(async () => {
      gotIt!.props.onClick();
    });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('closes on Escape (existing behavior preserved)', () => {
    const target = stubWindow();
    const onClose = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<KeyboardShortcutsModal isOpen={true} onClose={onClose} />);
    });
    act(() => {
      target.dispatchEvent(keydown('Escape'));
    });
    expect(onClose).toHaveBeenCalledTimes(1);
    // Unmount while window is still stubbed so the Escape-listener
    // cleanup does not run against an unstubbed global.
    act(() => {
      tree.unmount();
    });
  });
});
