import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { API_PROBLEM_EVENT, UNAUTH_EVENT } from '@/lib/api';
import { ApiProblemBanner, SessionExpiredBanner, describeCreateFailure } from './page';

afterEach(() => {
  vi.unstubAllGlobals();
});

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(textOf).join(' ');
  if (node !== null && typeof node === 'object' && 'children' in node) {
    const children = (node as { children?: unknown }).children;
    if (Array.isArray(children)) return children.map(textOf).join(' ');
  }
  return '';
}

function stubWindow(): EventTarget {
  const target = new EventTarget();
  vi.stubGlobal('window', target);
  return target;
}

describe('SessionExpiredBanner', () => {
  it('is hidden until UNAUTH_EVENT fires', () => {
    stubWindow();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<SessionExpiredBanner onOpenLabs={vi.fn()} />);
    });
    expect(tree!.toJSON()).toBeNull();
  });

  it('appears on UNAUTH_EVENT with the labs affordance, and dismisses', () => {
    const target = stubWindow();
    const onOpenLabs = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<SessionExpiredBanner onOpenLabs={onOpenLabs} />);
    });
    act(() => {
      target.dispatchEvent(new CustomEvent(UNAUTH_EVENT));
    });
    const banner = tree!.root.findByProps({ 'data-testid': 'auth-expired-banner' });
    expect(textOf(banner)).toContain('Session expired');
    const login = tree!.root.findByProps({ 'data-testid': 'auth-banner-login' });
    act(() => {
      login.props.onClick();
    });
    expect(onOpenLabs).toHaveBeenCalledTimes(1);
    const dismiss = tree!.root.findByProps({ 'data-testid': 'auth-banner-dismiss' });
    act(() => {
      dismiss.props.onClick();
    });
    expect(tree!.toJSON()).toBeNull();
  });

  it('unsubscribes on unmount', () => {
    const target = stubWindow();
    const removeSpy = vi.spyOn(target, 'removeEventListener');
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<SessionExpiredBanner onOpenLabs={vi.fn()} />);
    });
    act(() => {
      tree!.unmount();
    });
    expect(removeSpy).toHaveBeenCalledWith(UNAUTH_EVENT, expect.any(Function));
  });
});

describe('ApiProblemBanner', () => {
  it('surfaces a refusal that a legacy wrapper may have collapsed to empty', () => {
    const target = stubWindow();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<ApiProblemBanner />);
    });
    expect(tree!.toJSON()).toBeNull();
    act(() => {
      target.dispatchEvent(new CustomEvent(API_PROBLEM_EVENT, {
        detail: { path: '/api/loops', status: 500, message: 'Internal Server Error', at: Date.now() },
      }));
    });
    const banner = tree!.root.findByProps({ 'data-testid': 'api-problem-banner' });
    const text = textOf(banner);
    expect(text).toContain('/api/loops');
    expect(text).toContain('500');
    expect(text).toContain('empty data');
    act(() => {
      tree!.root.findByProps({ 'aria-label': 'Dismiss API refusal' }).props.onClick();
    });
    expect(tree!.toJSON()).toBeNull();
  });
});

describe('describeCreateFailure', () => {
  it('returns null on success', () => {
    expect(describeCreateFailure({ id: 's1' } as never, null)).toBeNull();
  });

  it('renders refusal verbatim when createSession returns null', () => {
    const msg = describeCreateFailure(null, null);
    expect(msg).toContain('createSession returned null');
    expect(msg).toContain('nothing was created');
  });

  it('renders thrown errors verbatim', () => {
    expect(describeCreateFailure(null, new Error('boom-123'))).toContain('boom-123');
    expect(describeCreateFailure(null, 'plain-failure-456')).toContain('plain-failure-456');
  });
});
