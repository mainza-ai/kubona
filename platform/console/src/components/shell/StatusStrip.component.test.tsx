import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { API_PROBLEM_EVENT } from '@/lib/api';
import { StatusStrip } from './StatusStrip';

// The strip composes the real backend-readiness bar and check-in strip; their
// two reads are stubbed at the transport so the chip behaviour is exercised
// against the DOM the operator actually sees.
function installFetch() {
  vi.stubGlobal('fetch', async (url: string) => {
    const path = new URL(String(url)).pathname;
    if (path === '/api/health') {
      return { ok: true, status: 200, json: async () => ({ service: 'kubonad', version: 'test' }) };
    }
    if (path === '/api/ready') {
      return { ok: true, status: 200, json: async () => ({ ok: true }) };
    }
    if (path === '/api/cost/summary') {
      return {
        ok: true,
        status: 200,
        json: async () => ({
          cap_usd: 0,
          spent_usd: 0,
          remaining_usd: 0,
          unmetered_count: 0,
          by_provider: {},
          active_provider: 'none',
          local_bypass: false,
          heuristic: true,
          currency: 'USD',
          track_only: true,
        }),
      };
    }
    return { ok: true, status: 200, json: async () => ({}) };
  });
}

function problemChip(screen: Awaited<ReturnType<typeof render>>) {
  return screen.container.querySelector('[data-testid="api-problem-chip"]');
}

function emitProblem(detail: Record<string, unknown>) {
  window.dispatchEvent(new CustomEvent(API_PROBLEM_EVENT, { detail: { at: Date.now(), ...detail } }));
}

beforeEach(() => {
  // Dismissals live in sessionStorage keyed by problem signature; a clean
  // session per test keeps one case from suppressing the next.
  window.sessionStorage.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('StatusStrip problem chips in a real browser (3.9)', () => {
  it('shows a refusal as a chip, dismisses it by signature, and re-arms on a different problem', async () => {
    installFetch();
    const screen = await render(<StatusStrip loops={[]} />);
    // The real readiness bar reads the stubbed transport and reports ready.
    await expect.element(screen.getByRole('status', { name: 'Backend status: kubonad test · ready' })).toBeVisible();
    expect(problemChip(screen)).toBeNull();

    emitProblem({ path: '/api/loops', status: 500, message: 'failed' });
    await expect.element(screen.getByTestId('api-problem-chip')).toBeVisible();
    expect(screen.container.querySelectorAll('[data-testid="api-problem-chip"]')).toHaveLength(1);

    await screen.getByRole('button', { name: 'Dismiss API refusal' }).click();
    await expect.element(screen.getByTestId('api-problem-chip')).not.toBeInTheDocument();

    // The identical problem stays dismissed this session: give React a tick to
    // flush any state change, then prove the chip did not come back.
    emitProblem({ path: '/api/loops', status: 500, message: 'failed' });
    await new Promise((resolve) => setTimeout(resolve, 30));
    expect(problemChip(screen)).toBeNull();

    // A different signature is new information and surfaces.
    emitProblem({ path: '/api/jobs', status: 503, message: 'bridge down' });
    await expect.element(screen.getByTestId('api-problem-chip')).toBeVisible();
    expect(screen.getByTestId('api-problem-chip').element().getAttribute('title')).toContain('bridge down');
  });

  it('renders contract drift as a warning chip, not a blocking alert', async () => {
    installFetch();
    const screen = await render(<StatusStrip loops={[]} />);

    emitProblem({
      path: 'contract:jobs.list',
      status: null,
      message: 'response contract drifted: jobs: expected array',
    });

    await expect.element(screen.getByTestId('contract-drift-chip')).toBeVisible();
    expect(screen.getByTestId('contract-drift-chip').element().getAttribute('role')).toBe('status');
    expect(problemChip(screen)).toBeNull();
  });
});
