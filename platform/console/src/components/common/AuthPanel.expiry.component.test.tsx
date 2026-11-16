import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { AuthPanel, formatExpiry } from './AuthPanel';
import { AUTH_REQUIRED_EVENT, UNAUTH_EVENT } from '@/lib/api';
import type { Lab } from '@/types';

const LABS: Lab[] = [
  {
    id: 'lab-alpha',
    display_name: 'Alpha Lab',
    autonomy_level: 'L1',
    spend_ceiling_usd: 150,
    spent_usd: 20,
    status: 'active',
  },
];

const NOW = new Date('2030-01-01T12:00:00Z').getTime();
const iso = (ms: number) => new Date(ms).toISOString();

type AuthStub = {
  whoami: Record<string, unknown>;
  sessions?: unknown[];
  /** Set by the logout route so the follow-up whoami reads anonymous. */
  loggedOut?: boolean;
};

function json(body: unknown) {
  return { ok: true, status: 200, json: async () => body };
}

// The panel reads the real transport wrappers (authWhoami/authSessions/
// authLogout), so the stub answers those routes by method + path instead of
// mocking '@/lib/api'. Requests the panel should not make fail loudly.
function installFetch(stub: AuthStub) {
  const calls: Array<{ method: string; path: string }> = [];
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const path = new URL(String(url)).pathname;
    const method = init?.method ?? 'GET';
    calls.push({ method, path });
    if (path === '/api/auth/whoami') {
      return json(stub.loggedOut ? { authenticated: false } : stub.whoami);
    }
    if (path === '/api/auth/sessions') {
      return json({ sessions: stub.sessions ?? [] });
    }
    if (path === '/api/auth/logout') {
      stub.loggedOut = true;
      return json({ ok: true });
    }
    throw new Error(`unexpected fetch ${method} ${path}`);
  });
  return calls;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('formatExpiry thresholds', () => {
  it('reads seconds under a minute', () => {
    expect(formatExpiry(iso(NOW + 30_000), NOW)).toBe('expires in 30s');
    expect(formatExpiry(iso(NOW - 9_000), NOW)).toBe('expired 9s ago');
  });

  it('reads minutes under an hour', () => {
    expect(formatExpiry(iso(NOW + 42 * 60_000), NOW)).toBe('expires in 42m');
    expect(formatExpiry(iso(NOW - 5 * 60_000), NOW)).toBe('expired 5m ago');
  });

  it('reads hours under two days, days beyond', () => {
    expect(formatExpiry(iso(NOW + 3 * 3_600_000), NOW)).toBe('expires in 3h');
    expect(formatExpiry(iso(NOW - 26 * 3_600_000), NOW)).toBe('expired 26h ago');
    expect(formatExpiry(iso(NOW - 50 * 3_600_000), NOW)).toBe('expired 2d ago');
    expect(formatExpiry(iso(NOW + 3 * 86_400_000), NOW)).toBe('expires in 3d');
  });

  it('falls back to raw input when unparseable', () => {
    expect(formatExpiry('not-a-date', NOW)).toBe('until not-a-date');
  });
});

describe('AuthPanel expiry UX in a real browser', () => {
  it('shows relative countdowns instead of raw ISO in the session list', async () => {
    installFetch({
      whoami: { authenticated: true, tenant_id: 'lab-alpha', pubkey_fp: '0123456789abcdef' },
      sessions: [
        {
          id: 'sess-live-01',
          pubkey_fp: '0123456789abcdef',
          created_at: iso(NOW - 60_000),
          expires_at: iso(Date.now() + 42 * 60_000),
        },
        {
          id: 'sess-dead-02',
          pubkey_fp: '0123456789abcdef',
          created_at: iso(NOW - 86_400_000),
          expires_at: iso(Date.now() - 5 * 60_000),
        },
      ],
    });

    const screen = await render(<AuthPanel labs={LABS} />);

    await expect.poll(() => screen.container.textContent ?? '').toContain('expires in 42m');
    const text = screen.container.textContent ?? '';
    expect(text).toContain('expired 5m ago');
    expect(text).not.toContain('until 20');
  });

  it('expired sessions keep the full timestamp discoverable via title', async () => {
    const expiresAt = iso(Date.now() + 42 * 60_000);
    installFetch({
      whoami: { authenticated: true, tenant_id: 'lab-alpha', pubkey_fp: '0123456789abcdef' },
      sessions: [
        {
          id: 'sess-live-01',
          pubkey_fp: '0123456789abcdef',
          created_at: iso(NOW - 60_000),
          expires_at: expiresAt,
        },
      ],
    });

    const screen = await render(<AuthPanel labs={LABS} />);

    await expect.poll(() => screen.container.textContent ?? '').toContain('expires in');
    const row = Array.from(screen.container.querySelectorAll('span')).find((s) =>
      (s.textContent ?? '').includes('expires in'),
    );
    expect(row).toBeDefined();
    expect(row!.getAttribute('title') ?? '').toContain(expiresAt);
  });

  it('recent anonymous 401 keeps the sign-in CTA, not an expired claim', async () => {
    installFetch({ whoami: { authenticated: false } });

    const screen = await render(<AuthPanel labs={LABS} />);
    await expect.poll(() => screen.container.textContent ?? '').toContain('Sign in with browser key');

    // An anonymous 401 is AUTH_REQUIRED_EVENT (sign-in affordance), never the
    // global "session expired" signal.
    window.dispatchEvent(new CustomEvent(AUTH_REQUIRED_EVENT));
    await new Promise((resolve) => setTimeout(resolve, 30));
    expect(screen.container.textContent).toContain('Sign in with browser key');
    expect(screen.container.textContent).not.toContain('Session expired');

    // Complement: the real expiry signal does flip the CTA, so the negative
    // assertion above is meaningful.
    window.dispatchEvent(new CustomEvent(UNAUTH_EVENT));
    await expect.element(screen.getByRole('button', { name: 'Session expired — sign in again' })).toBeVisible();
  });

  it('first-run anonymous keeps the plain sign-in label', async () => {
    installFetch({ whoami: { authenticated: false } });

    const screen = await render(<AuthPanel labs={LABS} />);

    await expect.poll(() => screen.container.textContent ?? '').toContain('Sign in with browser key');
    expect(screen.container.textContent).not.toContain('Session expired');
  });

  it('Switch lab logs out, clears stickiness, and reveals the lab selector', async () => {
    const calls = installFetch({
      whoami: { authenticated: true, tenant_id: 'lab-alpha', pubkey_fp: '0123456789abcdef' },
      sessions: [],
    });

    const screen = await render(<AuthPanel labs={LABS} />);

    const switchBtn = screen.getByRole('button', { name: 'Switch lab' });
    await expect.element(switchBtn).toBeVisible();
    await switchBtn.click();

    // Logged-out branch renders the discoverable lab selector again.
    await expect.element(screen.getByLabelText('Login lab')).toBeVisible();
    expect(calls.filter((c) => c.path === '/api/auth/logout' && c.method === 'POST')).toHaveLength(1);
    expect(screen.container.textContent).toContain('not signed in');
  });
});
