import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { AuthPanel } from './AuthPanel';
import type { Lab } from '@/types';

const NONCE = 'deadbeef0123456789abcdef01234567';
const SESSION = {
  id: 'sess-token-01',
  pubkey_fp: '0123456789abcdef',
  created_at: '2030-01-01T00:00:00Z',
  expires_at: '2030-01-01T12:00:00Z',
};

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

type AuthStub = {
  whoami: { authenticated: boolean; tenant_id?: string; pubkey_fp?: string };
  sessions?: unknown[];
  /** Start the page already holding a live session. */
  startSignedIn?: boolean;
  /** Set to make POST /api/auth/login refuse with this backend reason. */
  loginRefusal?: string;
  /** Set by the logout route so the follow-up whoami reads anonymous. */
  loggedOut?: boolean;
  /** Set by the revoke route so the refreshed session list is empty. */
  revoked?: boolean;
};

function json(body: unknown) {
  return { ok: true, status: 200, json: async () => body };
}

function refusal(status: number, message: string) {
  const body = JSON.stringify({ error: message });
  return { ok: false, status, text: async () => body, json: async () => ({ error: message }) };
}

// The panel talks to the real transport wrappers (authChallenge/authLogin/
// authWhoami/authSessions/authRevoke/authLogout), so the stub answers those
// routes by method + path instead of mocking '@/lib/api'. Anything the panel
// should not call fails loudly.
function installFetch(stub: AuthStub) {
  const calls: Array<{ method: string; path: string; body?: unknown }> = [];
  let signedIn = stub.startSignedIn === true;
  vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
    const path = new URL(String(url)).pathname;
    const method = (init?.method ?? 'GET').toUpperCase();
    calls.push({ method, path, body: init?.body });
    switch (path) {
      case '/api/auth/whoami':
        return json(signedIn && !stub.loggedOut ? stub.whoami : { authenticated: false });
      case '/api/auth/challenge':
        return json({
          nonce: NONCE,
          expires_at: '2030-01-01T00:01:00Z',
          message: `kubona-auth/v1 login ${NONCE}`,
        });
      case '/api/auth/login':
        if (stub.loginRefusal) return refusal(403, stub.loginRefusal);
        signedIn = true;
        return json({
          ok: true,
          tenant_id: stub.whoami.tenant_id ?? 'lab-alpha',
          pubkey_fp: stub.whoami.pubkey_fp ?? '0123456789abcdef',
        });
      case '/api/auth/sessions':
        return json({ sessions: stub.revoked ? [] : stub.sessions ?? [] });
      case '/api/auth/revoke':
        stub.revoked = true;
        return json({ ok: true });
      case '/api/auth/logout':
        stub.loggedOut = true;
        return json({ ok: true });
      default:
        throw new Error(`unexpected fetch ${method} ${path}`);
    }
  });
  return calls;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('AuthPanel session auth in a real browser', () => {
  it('shows unauthenticated state and completes challenge-response login', async () => {
    const calls = installFetch({
      whoami: { authenticated: true, tenant_id: 'lab-alpha', pubkey_fp: '0123456789abcdef' },
      sessions: [SESSION],
    });

    const screen = await render(<AuthPanel labs={LABS} />);
    await expect.poll(() => screen.container.textContent).toContain('not signed in');

    const signIn = screen.getByRole('button', { name: 'Sign in with browser key', exact: true });
    await expect.element(signIn).toBeVisible();
    await signIn.click();

    // The daemon verifies the signed nonce, then the panel re-reads whoami and
    // lists the live sessions the login just created.
    await expect.poll(() => screen.container.textContent).toContain('sess-tok');

    const challengeCalls = calls.filter((c) => c.path === '/api/auth/challenge');
    expect(challengeCalls).toHaveLength(1);
    expect(challengeCalls[0].method).toBe('POST');
    const loginCalls = calls.filter((c) => c.path === '/api/auth/login');
    expect(loginCalls).toHaveLength(1);
    expect(loginCalls[0].method).toBe('POST');
    const sent = JSON.parse(String(loginCalls[0].body)) as {
      tenant_id: string;
      pubkey_hex: string;
      nonce: string;
      signature_hex: string;
    };
    expect(sent.tenant_id).toBe('lab-alpha');
    expect(sent.nonce).toBe(NONCE);
    expect(sent.pubkey_hex).toMatch(/^[0-9a-f]{64}$/);
    expect(sent.signature_hex).toMatch(/^[0-9a-f]{128}$/);

    const text = screen.container.textContent ?? '';
    expect(text).toContain('lab-alpha');
    expect(text).toContain('0123456789ab');
    expect(text).toContain('sess-tok');
  });

  it('surfaces daemon login refusal verbatim without optimistic state', async () => {
    installFetch({
      whoami: { authenticated: false },
      loginRefusal: "unauthorized: key is not this lab's admin",
    });

    const screen = await render(<AuthPanel labs={LABS} />);
    await screen.getByRole('button', { name: 'Sign in with browser key', exact: true }).click();

    await expect.poll(() => screen.container.textContent).toContain('login refused:');
    const alert = screen.getByRole('alert');
    await expect.element(alert).toBeVisible();
    expect(alert.element().textContent).toBe(
      "login refused: unauthorized: key is not this lab's admin",
    );

    // Refused is not a partial success: the panel stays anonymous and never
    // renders the signed-in session list.
    const text = screen.container.textContent ?? '';
    expect(text).toContain('not signed in');
    expect(text).not.toContain('Live sessions in this lab');
  });

  it('allows typing a lab id when anonymous multi-lab listing is refused', async () => {
    const calls = installFetch({
      whoami: { authenticated: true, tenant_id: 'lab-secret', pubkey_fp: 'abcdef0123456789' },
      sessions: [],
    });

    const screen = await render(<AuthPanel labs={[]} />);
    const labInput = screen.getByLabelText('Lab ID');
    await expect.element(labInput).toBeVisible();
    await labInput.fill('lab-secret');

    await screen.getByRole('button', { name: 'Sign in with browser key', exact: true }).click();

    await expect.poll(() => screen.container.textContent).toContain('lab-secret');
    const loginCall = calls.find((c) => c.path === '/api/auth/login');
    expect(loginCall).toBeDefined();
    expect((JSON.parse(String(loginCall!.body)) as { tenant_id: string }).tenant_id).toBe(
      'lab-secret',
    );
    expect(calls.filter((c) => c.path === '/api/auth/sessions')).toHaveLength(1);
  });

  it('revokes a live session and signs out cleanly', async () => {
    const calls = installFetch({
      whoami: { authenticated: true, tenant_id: 'lab-alpha', pubkey_fp: '0123456789abcdef' },
      sessions: [SESSION],
      startSignedIn: true,
    });

    const screen = await render(<AuthPanel labs={LABS} />);
    await expect.poll(() => screen.container.textContent).toContain('sess-tok');

    await screen.getByRole('button', { name: /^revoke session sess-token-01$/i }).click();

    // Wave-3: revoke stages a typed ConfirmDialog (session short-id), so the
    // confirm control stays inert until the exact text is typed.
    const dialog = screen.getByRole('alertdialog', { name: /revoke session sess-tok/i });
    await expect.element(dialog).toBeVisible();
    // The typed gate names the exact short-id the operator must type.
    expect(dialog.element().textContent).toContain('Type sess-tok to confirm');
    const confirm = screen.getByRole('button', { name: /^revoke session$/i });
    await expect.element(confirm).toBeDisabled();
    await screen.getByRole('textbox').fill('sess-tok');
    // The dialog re-arms for its settle window on open, so wait it out.
    await expect
      .poll(() => (confirm.element() as HTMLButtonElement).disabled, { timeout: 5000 })
      .toBe(false);
    await confirm.click();

    await expect
      .poll(() => calls.filter((c) => c.path === '/api/auth/revoke').length)
      .toBe(1);
    const revokeCall = calls.find((c) => c.path === '/api/auth/revoke')!;
    expect(revokeCall.method).toBe('POST');
    expect((JSON.parse(String(revokeCall.body)) as { id: string }).id).toBe('sess-token-01');
    // The refreshed list no longer carries the revoked session.
    await expect.poll(() => screen.container.textContent).not.toContain('sess-tok');

    await screen.getByRole('button', { name: /^log out \(revokes this session\)$/i }).click();
    await expect.poll(() => screen.container.textContent).toContain('not signed in');
    expect(calls.filter((c) => c.path === '/api/auth/logout')).toHaveLength(1);
  });
});
