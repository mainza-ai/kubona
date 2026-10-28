import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { AuthPanel } from './AuthPanel';
import * as api from '@/lib/api';

vi.mock('@/lib/api', () => ({
  authWhoami: vi.fn(),
  authSessions: vi.fn(),
  authChallenge: vi.fn(),
  authLogin: vi.fn(),
  authLogout: vi.fn(),
  authRevoke: vi.fn(),
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

describe('AuthPanel (A4 session auth UI)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.stubGlobal('confirm', vi.fn(() => true));
  });

  it('shows unauthenticated state and completes challenge-response login', async () => {
    vi.mocked(api.authWhoami)
      .mockResolvedValueOnce({ authenticated: false })
      .mockResolvedValueOnce({
        authenticated: true,
        tenant_id: 'lab-alpha',
        pubkey_fp: '0123456789abcdef',
      });
    vi.mocked(api.authChallenge).mockResolvedValue({
      nonce: 'deadbeef0123456789abcdef01234567',
      expires_at: '2030-01-01T00:01:00Z',
      message: 'kubona-auth/v1 login deadbeef0123456789abcdef01234567',
    });
    vi.mocked(api.authLogin).mockResolvedValue({
      ok: true,
      tenant_id: 'lab-alpha',
      pubkey_fp: '0123456789abcdef',
    });
    vi.mocked(api.authSessions).mockResolvedValue([
      {
        id: 'sess-token-01',
        pubkey_fp: '0123456789abcdef',
        created_at: '2030-01-01T00:00:00Z',
        expires_at: '2030-01-01T12:00:00Z',
      },
    ]);

    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<AuthPanel labs={LABS} />);
    });

    expect(textOf(root!.root)).toContain('anonymous (bearer only)');

    const signInBtn = root!.root
      .findAllByType('button')
      .find((b) => textOf(b).includes('Log in with browser key'));
    expect(signInBtn).toBeDefined();

    await act(async () => {
      await signInBtn!.props.onClick();
    });
    // login() is void-returning; wait for the custody sign + login promises.
    await vi.waitFor(() => expect(api.authLogin).toHaveBeenCalled());

    expect(api.authChallenge).toHaveBeenCalledTimes(1);
    expect(api.authLogin).toHaveBeenCalledTimes(1);
    const [tId, pubHex, nonce, sigHex] = vi.mocked(api.authLogin).mock.calls[0];
    expect(tId).toBe('lab-alpha');
    expect(nonce).toBe('deadbeef0123456789abcdef01234567');
    expect(pubHex).toMatch(/^[0-9a-f]{64}$/);
    expect(sigHex).toMatch(/^[0-9a-f]{128}$/);

    const afterText = textOf(root!.root);
    expect(afterText).toContain('lab-alpha');
    expect(afterText).toContain('0123456789ab');
    expect(afterText).toContain('sess-tok');
  });

  it('surfaces daemon login refusal verbatim without optimistic state', async () => {
    vi.mocked(api.authWhoami).mockResolvedValue({ authenticated: false });
    vi.mocked(api.authChallenge).mockResolvedValue({
      nonce: 'deadbeef0123456789abcdef01234567',
      expires_at: '2030-01-01T00:01:00Z',
      message: 'kubona-auth/v1 login deadbeef0123456789abcdef01234567',
    });
    vi.mocked(api.authLogin).mockResolvedValue({
      error: "unauthorized: key is not this lab's admin",
    });

    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<AuthPanel labs={LABS} />);
    });

    const signInBtn = root!.root
      .findAllByType('button')
      .find((b) => textOf(b).includes('Log in with browser key'));
    await act(async () => {
      await signInBtn!.props.onClick();
    });
    await vi.waitFor(() => expect(api.authLogin).toHaveBeenCalled());

    const text = textOf(root!.root);
    expect(text).toContain("login refused: unauthorized: key is not this lab's admin");
    expect(text).toContain('anonymous (bearer only)');
  });

  it('revokes a live session and signs out cleanly', async () => {
    vi.mocked(api.authWhoami)
      .mockResolvedValueOnce({
        authenticated: true,
        tenant_id: 'lab-alpha',
        pubkey_fp: '0123456789abcdef',
      })
      .mockResolvedValueOnce({
        authenticated: true,
        tenant_id: 'lab-alpha',
        pubkey_fp: '0123456789abcdef',
      })
      .mockResolvedValueOnce({ authenticated: false });
    vi.mocked(api.authSessions)
      .mockResolvedValueOnce([
        {
          id: 'sess-token-01',
          pubkey_fp: '0123456789abcdef',
          created_at: '2030-01-01T00:00:00Z',
          expires_at: '2030-01-01T12:00:00Z',
        },
      ])
      .mockResolvedValueOnce([]);
    vi.mocked(api.authRevoke).mockResolvedValue({ ok: true });
    vi.mocked(api.authLogout).mockResolvedValue(true);

    let root: TestRenderer.ReactTestRenderer | undefined;
    await act(async () => {
      root = TestRenderer.create(<AuthPanel labs={LABS} />);
    });

    const revokeBtn = root!.root
      .findAllByType('button')
      .find((b) => textOf(b).includes('Revoke'));
    expect(revokeBtn).toBeDefined();
    await act(async () => {
      await revokeBtn!.props.onClick();
    });
    expect(api.authRevoke).toHaveBeenCalledWith('sess-token-01');

    const signOutBtn = root!.root
      .findAllByType('button')
      .find((b) => textOf(b).includes('Log out'));
    expect(signOutBtn).toBeDefined();
    await act(async () => {
      await signOutBtn!.props.onClick();
    });
    expect(api.authLogout).toHaveBeenCalledTimes(1);
    expect(textOf(root!.root)).toContain('anonymous (bearer only)');
  });
});
