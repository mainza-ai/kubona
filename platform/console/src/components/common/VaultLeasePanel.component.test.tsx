import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { VaultLeasePanel } from './VaultLeasePanel';

const lease = {
  lease_id: 'lease-123', scope: 'provider:session_token', ttl_seconds: 60,
  lease_signature: 'abcdef0123456789', issued_at: '2030-01-01T00:00:00Z', expires_at: '2030-01-01T00:01:00Z',
};

afterEach(() => vi.unstubAllGlobals());

describe('VaultLeasePanel in a real browser', () => {
  it('issues and revokes an opaque lease id', async () => {
    vi.stubGlobal('fetch', async (url: string) => {
      if (String(url).includes('revoke')) {
        return { ok: true, status: 200, json: async () => ({ revoked: true }) };
      }
      return { ok: true, status: 200, json: async () => lease };
    });
    const screen = await render(<VaultLeasePanel />);
    await screen.getByRole('button', { name: 'Issue lease' }).click();
    await expect.poll(() => document.body.textContent).toContain('lease-123');
    await screen.getByRole('button', { name: 'Revoke lease' }).click();
    await expect.poll(() => document.body.textContent).toContain('Lease revoked');
    expect(document.body.textContent).not.toContain('lease-123');
  });
});
