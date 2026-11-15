import { describe, expect, it, vi } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { NeedsYouTray } from './NeedsYouTray';
import type { NeedItem } from '@/lib/needs-you';

function item(overrides: Partial<NeedItem> = {}): NeedItem {
  return {
    id: 'ob_1',
    subjectKind: 'job',
    subjectId: 'job-abcdef1234567890',
    subjectClass: 'job',
    owner: 'session:s1',
    state: 'decidable',
    reason: 'budget approval pending',
    reasons: ['budget approval pending'],
    waitingSince: '2026-11-14T11:55:00Z',
    signature: 'ob_1@2026-11-14T11:55:00Z',
    ...overrides,
  };
}

function rows(tree: TestRenderer.ReactTestRenderer) {
  return tree.root.findAll((node) => node.props['data-testid'] === 'needs-you-row');
}

/** Flatten rendered text so assertions see what a reader sees, not node JSON. */
function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map((child) => textOf(child)).join('');
  if (node && typeof node === 'object' && 'children' in node) {
    return textOf((node as { children?: unknown }).children);
  }
  return '';
}

describe('NeedsYouTray', () => {
  it('renders nothing while closed', () => {
    let tree!: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <NeedsYouTray items={[item()]} open={false} onClose={() => {}} onFocus={() => {}} onOpenQueue={() => {}} />,
      );
    });
    expect(tree.toJSON()).toBeNull();
  });

  it('renders one row per waiting obligation with the server reason verbatim', () => {
    let tree!: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <NeedsYouTray
          items={[
            item(),
            item({
              id: 'ob_2',
              subjectKind: 'claim',
              subjectClass: 'claim',
              subjectId: 'tribunal-run-1',
              state: 'blocked',
              reason: 'missing reviewer authority',
              reasons: ['missing reviewer authority', 'anchor missing'],
              signature: 'ob_2@2026-11-14T11:00:00Z',
            }),
          ]}
          open
          nowMs={Date.parse('2026-11-14T12:00:00Z')}
          onClose={() => {}}
          onFocus={() => {}}
          onOpenQueue={() => {}}
        />,
      );
    });
    expect(rows(tree)).toHaveLength(2);
    const text = textOf(tree.toJSON());
    expect(text).toContain('budget approval pending');
    expect(text).toContain('missing reviewer authority');
    expect(text).toContain('+1 more');
    expect(text).toContain('decidable now');
    expect(text).toContain('blocked');
    expect(text).toContain('waiting 5m');
  });

  it('never dresses a missing reason as a fact', () => {
    let tree!: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <NeedsYouTray
          items={[item({ reason: null, reasons: [], waitingSince: null })]}
          open
          onClose={() => {}}
          onFocus={() => {}}
          onOpenQueue={() => {}}
        />,
      );
    });
    const text = textOf(tree.toJSON());
    expect(text).toContain('No blocking reason recorded by the server.');
    expect(text).toContain('waiting unknown');
  });

  it('routes Review to the caller and Close to onClose', () => {
    const onFocus = vi.fn();
    const onClose = vi.fn();
    const target = item();
    let tree!: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <NeedsYouTray items={[target]} open onClose={onClose} onFocus={onFocus} onOpenQueue={() => {}} />,
      );
    });
    act(() => {
      tree.root.findByProps({ 'aria-label': 'Review job job-abcdef12' }).props.onClick();
    });
    expect(onFocus).toHaveBeenCalledWith(target);
    act(() => {
      tree.root.findByProps({ 'aria-label': 'Close needs-you tray' }).props.onClick();
    });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('never renders an unreadable queue as "nothing waiting"', () => {
    let tree!: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <NeedsYouTray items={[]} open queueStatus="unavailable" onClose={() => {}} onFocus={() => {}} onOpenQueue={() => {}} />,
      );
    });
    const text = textOf(tree.toJSON());
    expect(text).toContain('Queue unavailable — sign in or set the operator token.');
    expect(text).not.toContain('Nothing is waiting on a decision.');

    act(() => {
      tree.update(
        <NeedsYouTray items={[]} open queueStatus="loading" onClose={() => {}} onFocus={() => {}} onOpenQueue={() => {}} />,
      );
    });
    expect(textOf(tree.toJSON())).toContain('Reading the decision queue…');
  });

  it('says plainly when nothing is waiting and still offers the queue', () => {
    const onOpenQueue = vi.fn();
    let tree!: TestRenderer.ReactTestRenderer;
    act(() => {
      tree = TestRenderer.create(
        <NeedsYouTray items={[]} open onClose={() => {}} onFocus={() => {}} onOpenQueue={onOpenQueue} />,
      );
    });
    expect(rows(tree)).toHaveLength(0);
    expect(textOf(tree.toJSON())).toContain('Nothing is waiting on a decision.');
    act(() => {
      tree.root.findByProps({ 'aria-label': 'Open Guardrails queue' }).props.onClick();
    });
    expect(onOpenQueue).toHaveBeenCalledTimes(1);
  });
});
