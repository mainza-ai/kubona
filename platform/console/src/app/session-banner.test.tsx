import { describe, it, expect, vi, afterEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { API_PROBLEM_EVENT, UNAUTH_EVENT } from '@/lib/api';
import { ApiProblemChip, BannerStack, SessionExpiredBanner, describeCreateFailure } from '@/components/shell/Banners';

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

describe('ApiProblemChip', () => {
  it('renders a non-blocking API refusal chip that dismisses, never role=alert', () => {
    const onDismiss = vi.fn();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <ApiProblemChip
          problem={{ path: '/api/loops', status: 500, message: 'Internal Server Error', at: Date.now() }}
          onDismiss={onDismiss}
        />,
      );
    });
    const chip = tree!.root.findByProps({ 'data-testid': 'api-problem-chip' });
    expect(textOf(chip)).toContain('/api/loops');
    expect(textOf(chip)).toContain('500');
    // Status is polite; the only role="alert" in the shell is blocking auth.
    expect(chip.props.role).toBe('status');
    act(() => {
      tree!.root.findByProps({ 'aria-label': 'Dismiss API refusal' }).props.onClick();
    });
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it('renders a typed need sentence and remedy in the refusal chip', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <ApiProblemChip
          problem={{
            path: '/api/chat/launch', status: 403, message: 'ceiling exceeded', at: Date.now(),
            need: { kind: 'budget_exceeded', reason: 'tenant spend ceiling refused this work' },
          }}
          onDismiss={() => {}}
        />,
      );
    });
    const chip = tree!.root.findByProps({ 'data-testid': 'api-problem-chip' });
    expect(textOf(chip)).toContain('budget_exceeded');
    expect(chip.props.title).toContain('tenant spend ceiling refused this work');
    expect(chip.props.title).toContain('Review spend & mandates');
  });

  it('renders contract drift as its own chip with the schema name', () => {
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <ApiProblemChip
          problem={{ path: 'contract:loops.list', status: null, message: 'response contract drifted: loops: expected array', at: Date.now() }}
          onDismiss={() => {}}
        />,
      );
    });
    const chip = tree!.root.findByProps({ 'data-testid': 'contract-drift-chip' });
    expect(textOf(chip)).toContain('contract drift');
    expect(textOf(chip)).toContain('loops.list');
    expect(chip.props.title).toContain('expected array');
    expect(tree!.root.findByProps({ 'aria-label': 'Dismiss contract drift' })).toBeTruthy();
  });
});

describe('BannerStack', () => {
  it('shows one banner and lets auth expiry outrank an API refusal', () => {
    const target = stubWindow();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<BannerStack onOpenLabs={vi.fn()} />);
    });
    act(() => {
      target.dispatchEvent(new CustomEvent(API_PROBLEM_EVENT, {
        detail: { path: '/api/jobs', status: 500, message: 'failed', at: Date.now() },
      }));
      target.dispatchEvent(new CustomEvent(UNAUTH_EVENT));
    });
    expect(tree!.root.findAllByProps({ 'data-testid': 'auth-expired-banner' })).toHaveLength(1);
    expect(tree!.root.findAllByProps({ 'data-testid': 'api-problem-banner' })).toHaveLength(0);
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
describe('BannerStack', () => {
  it('shows one banner and lets auth expiry outrank an API refusal', () => {
    const target = stubWindow();
    let tree: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(<BannerStack onOpenLabs={vi.fn()} />);
    });
    act(() => {
      target.dispatchEvent(new CustomEvent(API_PROBLEM_EVENT, {
        detail: { path: '/api/jobs', status: 500, message: 'failed', at: Date.now() },
      }));
      target.dispatchEvent(new CustomEvent(UNAUTH_EVENT));
    });
    expect(tree!.root.findAllByProps({ 'data-testid': 'auth-expired-banner' })).toHaveLength(1);
    expect(tree!.root.findAllByProps({ 'data-testid': 'api-problem-banner' })).toHaveLength(0);
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
