import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { CommandPalette } from './CommandPalette';

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

const baseProps = {
  onClose: vi.fn(),
  onSelectSurface: vi.fn(),
  onNewCampaign: vi.fn(),
  onEmergencyAbort: vi.fn(),
  onOpenAbout: vi.fn(),
};

describe('CommandPalette focus trap (Wave-2)', () => {
  it('renders a modal dialog with the search input without a real DOM (SSR-safe)', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<CommandPalette isOpen={true} {...baseProps} />);
    });
    const dialog = tree!.root.findByProps({ role: 'dialog' });
    expect(dialog.props['aria-modal']).toBe('true');
    const input = tree!.root.findByType('input');
    expect(input.props['aria-label']).toBe('Search surfaces and actions');
  });

  it('keeps existing behavior: Escape closes, typing filters', async () => {
    const target = stubWindow();
    const onClose = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <CommandPalette isOpen={true} {...baseProps} onClose={onClose} />
      );
    });
    const input = tree!.root.findByType('input');
    await act(async () => {
      input.props.onChange({ target: { value: 'tribunal' } });
    });
    expect(JSON.stringify(tree!.toJSON())).toContain('Epistemic Tribunal Room');
    act(() => {
      target.dispatchEvent(keydown('Escape'));
    });
    expect(onClose).toHaveBeenCalled();
    // Unmount while window is still stubbed so the key-listener
    // cleanup does not run against an unstubbed global.
    act(() => {
      tree.unmount();
    });
  });
});
