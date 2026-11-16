import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { API_PROBLEM_EVENT, UNAUTH_EVENT } from '@/lib/api';
import { ApiProblemChip, BannerStack, SessionExpiredBanner, describeCreateFailure } from '@/components/shell/Banners';

afterEach(() => {
  vi.restoreAllMocks();
});

describe('SessionExpiredBanner in a real browser', () => {
  it('is hidden until UNAUTH_EVENT fires', async () => {
    const screen = await render(<SessionExpiredBanner onOpenLabs={vi.fn()} />);
    expect(screen.container.innerHTML).toBe('');
  });

  it('appears on UNAUTH_EVENT with the labs affordance, and dismisses', async () => {
    const onOpenLabs = vi.fn();
    const screen = await render(<SessionExpiredBanner onOpenLabs={onOpenLabs} />);

    window.dispatchEvent(new CustomEvent(UNAUTH_EVENT));

    await expect.element(screen.getByTestId('auth-expired-banner')).toBeVisible();
    expect(screen.getByTestId('auth-expired-banner').element().textContent).toContain('Session expired');

    await screen.getByTestId('auth-banner-login').click();
    expect(onOpenLabs).toHaveBeenCalledTimes(1);

    await screen.getByTestId('auth-banner-dismiss').click();
    await expect.element(screen.getByTestId('auth-expired-banner')).not.toBeInTheDocument();
  });

  it('unsubscribes on unmount', async () => {
    const removeSpy = vi.spyOn(window, 'removeEventListener');
    const screen = await render(<SessionExpiredBanner onOpenLabs={vi.fn()} />);

    await screen.unmount();

    expect(removeSpy).toHaveBeenCalledWith(UNAUTH_EVENT, expect.any(Function));
  });
});

describe('ApiProblemChip in a real browser', () => {
  it('renders a non-blocking API refusal chip that dismisses, never role=alert', async () => {
    const onDismiss = vi.fn();
    const screen = await render(
      <ApiProblemChip
        problem={{ path: '/api/loops', status: 500, message: 'Internal Server Error', at: Date.now() }}
        onDismiss={onDismiss}
      />,
    );

    const chip = screen.getByTestId('api-problem-chip');
    await expect.element(chip).toBeVisible();
    expect(chip.element().textContent).toContain('/api/loops');
    expect(chip.element().textContent).toContain('500');
    // Status is polite; the only role="alert" in the shell is blocking auth.
    expect(chip.element().getAttribute('role')).toBe('status');

    await screen.getByRole('button', { name: 'Dismiss API refusal' }).click();
    expect(onDismiss).toHaveBeenCalledTimes(1);
  });

  it('renders a typed need sentence and remedy in the refusal chip', async () => {
    const screen = await render(
      <ApiProblemChip
        problem={{
          path: '/api/chat/launch',
          status: 403,
          message: 'ceiling exceeded',
          at: Date.now(),
          need: { kind: 'budget_exceeded', reason: 'tenant spend ceiling refused this work' },
        }}
        onDismiss={() => {}}
      />,
    );

    const chip = screen.getByTestId('api-problem-chip');
    await expect.element(chip).toBeVisible();
    expect(chip.element().textContent).toContain('budget_exceeded');
    const title = chip.element().getAttribute('title') ?? '';
    expect(title).toContain('tenant spend ceiling refused this work');
    expect(title).toContain('Review spend & mandates');
  });

  it('renders contract drift as its own chip with the schema name', async () => {
    const screen = await render(
      <ApiProblemChip
        problem={{
          path: 'contract:loops.list',
          status: null,
          message: 'response contract drifted: loops: expected array',
          at: Date.now(),
        }}
        onDismiss={() => {}}
      />,
    );

    const chip = screen.getByTestId('contract-drift-chip');
    await expect.element(chip).toBeVisible();
    expect(chip.element().textContent).toContain('contract drift');
    expect(chip.element().textContent).toContain('loops.list');
    expect(chip.element().getAttribute('title') ?? '').toContain('expected array');
    await expect.element(screen.getByRole('button', { name: 'Dismiss contract drift' })).toBeVisible();
  });
});

describe('BannerStack in a real browser', () => {
  it('shows one banner and lets auth expiry outrank an API refusal', async () => {
    const screen = await render(<BannerStack onOpenLabs={vi.fn()} />);

    window.dispatchEvent(
      new CustomEvent(API_PROBLEM_EVENT, {
        detail: { path: '/api/jobs', status: 500, message: 'failed', at: Date.now() },
      }),
    );
    window.dispatchEvent(new CustomEvent(UNAUTH_EVENT));

    await expect.element(screen.getByTestId('auth-expired-banner')).toBeVisible();
    expect(screen.container.querySelectorAll('[data-testid="auth-expired-banner"]')).toHaveLength(1);
    expect(screen.container.querySelectorAll('[data-testid="api-problem-banner"]')).toHaveLength(0);
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
