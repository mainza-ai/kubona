import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { LabsView } from './LabsView';
import { resetDedupForTests } from '@/lib/api';

// Migrated from react-test-renderer (LabsView.test.tsx). The old suite mocked
// '@/lib/api' wholesale and walked the test tree; this one answers the real
// transport wrappers at the fetch boundary and reads rendered text/ARIA in a
// real Chromium page, so the refused-vs-empty and progress-bar contracts are
// observed the way an operator sees them.
type Call = { method: string; path: string };

type LabRow = {
  id: string;
  display_name: string;
  autonomy_level: string;
  spend_ceiling_usd: number;
  spent_usd: number;
  status: string;
};

type SessionRow = {
  id: string;
  title: string;
  tenant_id: string;
  created_at: string;
  updated_at: string;
  status: string;
  spend_limit_usd: number;
  current_spend_usd: number;
};

const LAB_A: LabRow = {
  id: 'lab-a', display_name: 'Lab A', autonomy_level: 'L1',
  spend_ceiling_usd: 150, spent_usd: 100, status: 'active',
};
const LAB_B: LabRow = {
  id: 'lab-b', display_name: 'Lab B', autonomy_level: 'L2',
  spend_ceiling_usd: 150, spent_usd: 140, status: 'suspended',
};
const DRILL: SessionRow = {
  id: 's1', title: 'drill', tenant_id: 'lab-a', created_at: '', updated_at: '',
  status: 'active', spend_limit_usd: 120, current_spend_usd: 70,
};

function ok(payload: unknown) {
  return { ok: true, status: 200, json: async () => payload };
}

function installFetch(routes: {
  labs: unknown[] | 'pending' | 'unauthorized';
  sessions?: unknown[] | 'pending';
  authenticated?: boolean | 'pending';
}) {
  const calls: Call[] = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const path = new URL(String(url)).pathname;
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push({ method, path });
    if (method === 'GET' && path === '/api/tenants') {
      if (routes.labs === 'pending') return new Promise(() => undefined);
      if (routes.labs === 'unauthorized') {
        return { ok: false, status: 401, text: async () => '{"error":"unauthorized"}', json: async () => ({ error: 'unauthorized' }) };
      }
      return ok({ tenants: routes.labs });
    }
    if (method === 'GET' && path === '/api/sessions') {
      if (routes.sessions === 'pending') return new Promise(() => undefined);
      return ok({ sessions: routes.sessions ?? [] });
    }
    if (method === 'GET' && path === '/api/auth/whoami') {
      if (routes.authenticated === 'pending') return new Promise(() => undefined);
      return ok({ authenticated: routes.authenticated === true });
    }
    if (method === 'GET' && path === '/api/auth/sessions') {
      return ok({ sessions: [] });
    }
    return {
      ok: false,
      status: 404,
      json: async () => ({ error: `unexpected ${method} ${path}` }),
      text: async () => `unexpected ${method} ${path}`,
    };
  });
  return calls;
}

function labToggles(screen: { container: HTMLElement }) {
  return [...screen.container.querySelectorAll('button[aria-expanded]')] as HTMLButtonElement[];
}

afterEach(() => {
  // A never-settling GET (the loading-skeleton case) would otherwise stay in
  // the dedupe inflight map and starve every later /api/tenants read.
  resetDedupForTests();
  vi.unstubAllGlobals();
});

describe('LabsView in a real browser', () => {
  it('renders labs with spend figures and autonomy badges for a signed-in admin', async () => {
    installFetch({ labs: [LAB_A, LAB_B], sessions: [DRILL], authenticated: true });
    const screen = await render(<LabsView />);
    await expect.poll(() => screen.container.textContent ?? '').toContain('$100.00 / $150.00');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('Lab A');
    expect(text).toContain('Lab B');
    expect(text).toContain('L1');
    expect(text).toContain('L2');
    expect(text).toContain('suspended');
    expect(text).toContain('Lab session');
  });

  it('shows only the sign-in view to an anonymous researcher', async () => {
    installFetch({ labs: [LAB_A], sessions: [], authenticated: false });
    const screen = await render(<LabsView />);
    await expect.poll(() => screen.container.textContent ?? '').toContain('Sign in with browser key');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('Lab session');
    // The admin-only panels never mount for an anonymous researcher.
    expect(text).not.toContain('Cross-lab grants');
    expect(text).not.toContain('Rotate + download kit');
    // Nor does the spend table leak lab balances.
    expect(text).not.toContain('$100.00 / $150.00');
  });

  it('expands a lab to show its sessions with limits', async () => {
    installFetch({ labs: [LAB_A, LAB_B], sessions: [DRILL], authenticated: true });
    const screen = await render(<LabsView />);
    await expect.poll(() => screen.container.textContent ?? '').toContain('$100.00 / $150.00');
    const toggles = labToggles(screen);
    expect(toggles.length).toBeGreaterThanOrEqual(2);
    expect(toggles[0].getAttribute('aria-expanded')).toBe('false');
    await toggles[0].click();
    await expect.poll(() => screen.container.textContent ?? '').toContain('$70.00 / $120.00');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('drill');
    expect(toggles[0].getAttribute('aria-expanded')).toBe('true');
  });

  it('renders empty state when no labs exist', async () => {
    installFetch({ labs: [], sessions: [], authenticated: true });
    const screen = await render(<LabsView />);
    await expect.poll(() => screen.container.textContent ?? '').toContain('No laboratories registered.');
    expect(screen.container.textContent ?? '').not.toContain('Sign in to manage laboratories.');
  });

  it('renders a login prompt (not a fake empty) when labs are refused while logged out', async () => {
    installFetch({ labs: [], sessions: [], authenticated: false });
    const screen = await render(<LabsView />);
    await expect.poll(() => screen.container.textContent ?? '').toContain('Sign in to manage laboratories.');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('Lab session');
    expect(text).not.toContain('No laboratories registered.');
  });
});

describe('LabsView money precision + spend bar (Wave-2) in a real browser', () => {
  const preciseLab: LabRow = {
    id: 'lab-a', display_name: 'Lab A', autonomy_level: 'L1',
    spend_ceiling_usd: 99.9, spent_usd: 10.5, status: 'active',
  };
  const preciseSession: SessionRow = {
    id: 's1', title: 'drill', tenant_id: 'lab-a', created_at: '', updated_at: '',
    status: 'active', spend_limit_usd: 120.126, current_spend_usd: 3.14159,
  };

  it('renders all money at 2dp', async () => {
    installFetch({ labs: [preciseLab], sessions: [preciseSession], authenticated: true });
    const screen = await render(<LabsView />);
    await expect.poll(() => screen.container.textContent ?? '').toContain('$10.50 / $99.90');
    await labToggles(screen)[0].click();
    await expect.poll(() => screen.container.textContent ?? '').toContain('$3.14 / $120.13');
  });

  it('exposes the spend bar as a progressbar with value, range, and name', async () => {
    installFetch({ labs: [preciseLab], sessions: [], authenticated: true });
    const screen = await render(<LabsView />);
    await expect.poll(() => screen.container.textContent ?? '').toContain('$10.50 / $99.90');
    const bar = screen.getByRole('progressbar').element();
    expect(bar.getAttribute('aria-valuemin')).toBe('0');
    expect(bar.getAttribute('aria-valuemax')).toBe('100');
    // 10.5 / 99.9 -> 11%.
    expect(bar.getAttribute('aria-valuenow')).toBe('11');
    expect(String(bar.getAttribute('aria-label'))).toMatch(/Lab A/);
  });

  it('names an unbounded bar without inventing a ceiling', async () => {
    installFetch({
      labs: [{ id: 'lab-x', display_name: 'Lab X', autonomy_level: 'L1', spend_ceiling_usd: 0, spent_usd: 5, status: 'active' }],
      sessions: [],
      authenticated: true,
    });
    const screen = await render(<LabsView />);
    await expect.poll(() => screen.container.textContent ?? '').toContain('UNBOUNDED');
    const bar = screen.getByRole('progressbar').element();
    expect(String(bar.getAttribute('aria-label'))).toMatch(/Lab X/);
    expect(String(bar.getAttribute('aria-valuetext'))).toMatch(/unbounded/);
  });
});

describe('LabsView first-load skeleton (Wave-3) in a real browser', () => {
  it('renders an honest loading skeleton while labs load', async () => {
    installFetch({ labs: 'pending', sessions: 'pending' });
    const screen = await render(<LabsView />);
    await expect.element(screen.getByTestId('labs-skeleton')).toBeInTheDocument();
    expect(screen.container.textContent ?? '').toContain('Loading laboratories');
  });

  it('hides the skeleton once labs settle (refused-vs-empty logic untouched)', async () => {
    installFetch({ labs: [LAB_A], sessions: [], authenticated: true });
    const screen = await render(<LabsView />);
    await expect.poll(() => screen.container.textContent ?? '').toContain('Lab A');
    expect(screen.container.querySelector('[data-testid="labs-skeleton"]')).toBeNull();
    expect(screen.container.textContent ?? '').not.toContain('Loading laboratories');
  });
});

// Last on purpose: a real 401 sets the module-level lastAuthFailureAt for 30s
// (lib/api wasRecentAuthFailure), which is exactly the state under test here.
describe('LabsView recent-401 refusal in a real browser', () => {
  it('treats a recent 401 as refused even before whoami resolves', async () => {
    installFetch({ labs: 'unauthorized', sessions: [], authenticated: 'pending' });
    const screen = await render(<LabsView />);
    // whoami never answers, so only the observed 401 can label the empty list
    // as a refusal instead of a genuine "no laboratories".
    await expect.poll(() => screen.container.textContent ?? '').toContain('Sign in to manage laboratories.');
    expect(screen.container.textContent ?? '').not.toContain('No laboratories registered.');
  });
});
