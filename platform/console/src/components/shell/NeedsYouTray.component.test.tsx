import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
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

function rows(screen: Awaited<ReturnType<typeof render>>) {
  return screen.container.querySelectorAll('[data-testid="needs-you-row"]');
}

function textOf(screen: Awaited<ReturnType<typeof render>>) {
  return screen.container.textContent ?? '';
}

describe('NeedsYouTray in a real browser', () => {
  it('renders nothing while closed', async () => {
    const screen = await render(
      <NeedsYouTray items={[item()]} open={false} onClose={() => {}} onFocus={() => {}} onOpenQueue={() => {}} />,
    );

    expect(screen.container.childElementCount).toBe(0);
    expect(screen.container.querySelector('[data-testid="needs-you-tray"]')).toBeNull();
  });

  it('renders one row per waiting obligation with the server reason verbatim', async () => {
    const screen = await render(
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

    await expect.element(screen.getByRole('dialog', { name: 'Needs you' })).toBeVisible();
    expect(rows(screen)).toHaveLength(2);
    const text = textOf(screen);
    expect(text).toContain('budget approval pending');
    expect(text).toContain('missing reviewer authority');
    expect(text).toContain('+1 more');
    expect(text).toContain('decidable now');
    expect(text).toContain('blocked');
    expect(text).toContain('waiting 5m');
  });

  it('never dresses a missing reason as a fact', async () => {
    const screen = await render(
      <NeedsYouTray
        items={[item({ reason: null, reasons: [], waitingSince: null })]}
        open
        onClose={() => {}}
        onFocus={() => {}}
        onOpenQueue={() => {}}
      />,
    );

    const text = textOf(screen);
    expect(text).toContain('No blocking reason recorded by the server.');
    expect(text).toContain('waiting unknown');
  });

  it('routes Review to the caller and Close to onClose', async () => {
    const onFocus = vi.fn();
    const onClose = vi.fn();
    const target = item();
    const screen = await render(
      <NeedsYouTray items={[target]} open onClose={onClose} onFocus={onFocus} onOpenQueue={() => {}} />,
    );

    await screen.getByRole('button', { name: 'Review job job-abcdef12' }).click();
    expect(onFocus).toHaveBeenCalledWith(target);

    await screen.getByRole('button', { name: 'Close needs-you tray' }).click();
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('never renders an unreadable queue as "nothing waiting"', async () => {
    const screen = await render(
      <NeedsYouTray items={[]} open queueStatus="unavailable" onClose={() => {}} onFocus={() => {}} onOpenQueue={() => {}} />,
    );

    const text = textOf(screen);
    expect(text).toContain('Queue unavailable — sign in or set the operator token.');
    expect(text).not.toContain('Nothing is waiting on a decision.');

    await screen.rerender(
      <NeedsYouTray items={[]} open queueStatus="loading" onClose={() => {}} onFocus={() => {}} onOpenQueue={() => {}} />,
    );
    expect(textOf(screen)).toContain('Reading the decision queue…');
  });

  it('says plainly when nothing is waiting and still offers the queue', async () => {
    const onOpenQueue = vi.fn();
    const screen = await render(
      <NeedsYouTray items={[]} open onClose={() => {}} onFocus={() => {}} onOpenQueue={onOpenQueue} />,
    );

    expect(rows(screen)).toHaveLength(0);
    expect(textOf(screen)).toContain('Nothing is waiting on a decision.');

    await screen.getByRole('button', { name: 'Open Guardrails queue' }).click();
    expect(onOpenQueue).toHaveBeenCalledTimes(1);
  });
});
