import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';

vi.mock('next/image', () => ({
  default: (props: Record<string, unknown>) => <img alt="" {...(props as object)} />,
}));

import { AboutLabModal } from './AboutLabModal';

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

describe('AboutLabModal focus trap (Wave-2)', () => {
  it('renders nothing when closed', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<AboutLabModal isOpen={false} onClose={vi.fn()} />);
    });
    expect(tree!.toJSON()).toBeNull();
  });

  it('renders a labelled modal dialog without a real DOM (SSR-safe)', () => {
    // No window stub: the Escape listener must guard and not throw.
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<AboutLabModal isOpen={true} onClose={vi.fn()} />);
    });
    const dialog = tree!.root.findByProps({ role: 'dialog' });
    expect(dialog.props['aria-modal']).toBe('true');
    expect(dialog.props['aria-labelledby']).toBe('about-lab-title');
  });

  it('closes via the Done button', async () => {
    const onClose = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<AboutLabModal isOpen={true} onClose={onClose} />);
    });
    const done = tree!.root
      .findAllByType('button')
      .find((b) => String(b.props.children).includes('Done'));
    expect(done).toBeDefined();
    await act(async () => {
      done!.props.onClick();
    });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('closes on Escape (existing behavior preserved)', () => {
    const target = stubWindow();
    const onClose = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<AboutLabModal isOpen={true} onClose={onClose} />);
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
