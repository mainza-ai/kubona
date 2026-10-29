import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { AuthPanel, formatExpiry } from './AuthPanel';
import * as api from '@/lib/api';

vi.mock('@/lib/api', () => ({
  authWhoami: vi.fn(),
  authSessions: vi.fn(),
  authChallenge: vi.fn(),
  authLogin: vi.fn(),
  authLogout: vi.fn(),
  authRevoke: vi.fn(),
  wasRecentAuthFailure: vi.fn(() => false),
  UNAUTH_EVENT: 'kubona:unauthorized',
}));

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map((c) => textOf(c)).join(' ');
}

const LABS = [
  {
    id: 'lab-alpha',
    display_name: 'Alpha Lab',
    autonomy_level: 'L1' as const,
    spend_ceiling_usd: 150,
    spent_usd: 20,
    status: 'active' as const,
  },
];

const NOW = new Date('2030-01-01T12:00:00Z').getTime();
const iso = (ms: number) => new Date(ms).toISOString();

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

describe('AuthPanel expiry UX', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.wasRecentAuthFailure).mockReturnValue(false);
    vi.stubGlobal('confirm', vi.fn(() => true));
  });

  it('shows relative countdowns instead of raw ISO in the session list', async () => {
    vi.mocked(api.authWhoami).mockResolvedValue({
      authenticated: true,
      tenant_id: 'lab-alpha',
      pubkey_fp: '0123456789abcdef',
    });
    vi.mocked(api.authSessions).mockResolvedValue([
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
    ]);

    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<AuthPanel labs={LABS} />);
    });

    const text = textOf(root!.root);
    expect(text).toContain('expires in 42m');
    expect(text).toContain('expired 5m ago');
    expect(text).not.toContain('until 20');
  });

  it('expired sessions keep the full timestamp discoverable via title', async () => {
    const expiresAt = iso(Date.now() + 42 * 60_000);
    vi.mocked(api.authWhoami).mockResolvedValue({
      authenticated: true,
      tenant_id: 'lab-alpha',
      pubkey_fp: '0123456789abcdef',
    });
    vi.mocked(api.authSessions).mockResolvedValue([
      {
        id: 'sess-live-01',
        pubkey_fp: '0123456789abcdef',
        created_at: iso(NOW - 60_000),
        expires_at: expiresAt,
      },
    ]);

    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<AuthPanel labs={LABS} />);
    });

    const row = root!.root
      .findAllByType('span')
      .find((s) => textOf(s).includes('expires in'));
    expect(row).toBeDefined();
    expect(row!.props.title).toContain(expiresAt);
  });

  it('recent 401 turns the login button into the expired CTA', async () => {
    vi.mocked(api.authWhoami).mockResolvedValue({ authenticated: false });
    vi.mocked(api.wasRecentAuthFailure).mockReturnValue(true);

    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<AuthPanel labs={LABS} />);
    });

    expect(textOf(root!.root)).toContain('Session expired — log in again');
    expect(textOf(root!.root)).not.toContain('Log in with browser key');
  });

  it('first-run anonymous keeps the plain login label', async () => {
    vi.mocked(api.authWhoami).mockResolvedValue({ authenticated: false });
    vi.mocked(api.wasRecentAuthFailure).mockReturnValue(false);

    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<AuthPanel labs={LABS} />);
    });

    expect(textOf(root!.root)).toContain('Log in with browser key');
    expect(textOf(root!.root)).not.toContain('Session expired');
  });

  it('Switch lab logs out, clears stickiness, and reveals the lab selector', async () => {
    vi.mocked(api.authWhoami)
      .mockResolvedValueOnce({
        authenticated: true,
        tenant_id: 'lab-alpha',
        pubkey_fp: '0123456789abcdef',
      })
      .mockResolvedValue({ authenticated: false });
    vi.mocked(api.authSessions).mockResolvedValue([]);
    vi.mocked(api.authLogout).mockResolvedValue(true);

    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<AuthPanel labs={LABS} />);
    });

    const switchBtn = root!.root
      .findAllByType('button')
      .find((b) => textOf(b).includes('Switch lab'));
    expect(switchBtn).toBeDefined();

    await act(async () => {
      await switchBtn!.props.onClick();
    });
    await act(async () => {
      await Promise.resolve();
    });

    expect(api.authLogout).toHaveBeenCalledTimes(1);
    // Logged-out branch renders the discoverable lab selector again.
    const select = root!.root.findByProps({ 'aria-label': 'Login lab' });
    expect(select).toBeDefined();
    expect(textOf(root!.root)).toContain('anonymous (bearer only)');
  });
});
