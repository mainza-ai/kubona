import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { TenantGrantPanel } from './TenantGrantPanel';

const LABS = [{ id: 'lab-a', display_name: 'Lab A', autonomy_level: 'L1', status: 'active' } as never];

afterEach(() => vi.unstubAllGlobals());

describe('TenantGrantPanel in a real browser', () => {
  it('issues a signed grant and revokes by returned id', async () => {
    const calls: Array<{ path: string; body: Record<string, unknown> }> = [];
    vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
      const path = new URL(String(url)).pathname;
      if (init?.method !== 'POST') throw new Error(`unexpected fetch ${init?.method ?? 'GET'} ${url}`);
      calls.push({ path, body: JSON.parse(String(init.body)) as Record<string, unknown> });
      if (path === '/api/tenant-grants/issue') {
        return { ok: true, status: 200, json: async () => ({ grant_id: 'grant-1' }) };
      }
      if (path === '/api/tenant-grants/revoke') {
        return { ok: true, status: 200, json: async () => ({ ok: true, grant_id: 'grant-1' }) };
      }
      throw new Error(`unexpected fetch POST ${url}`);
    });

    const screen = await render(<TenantGrantPanel labs={LABS} />);
    await screen.getByLabelText('Grant target lab').fill('lab-b');
    await screen.getByRole('button', { name: 'Issue grant' }).click();

    await expect.poll(() => document.body.textContent).toContain('Grant issued: grant-1');
    expect(calls).toHaveLength(1);
    expect(calls[0].path).toBe('/api/tenant-grants/issue');
    const issued = calls[0].body;
    expect(issued.source_tenant).toBe('lab-a');
    expect(issued.target_tenant).toBe('lab-b');
    expect(issued.scopes).toEqual(['read']);
    expect(typeof issued.expiry).toBe('string');
    // The grant is signed in-browser (custody), not stubbed here.
    expect(String(issued.grantor_pubkey_hex)).toMatch(/^[0-9a-f]{64}$/);
    expect(String(issued.signature_hex)).toMatch(/^[0-9a-f]{128}$/);
    expect((screen.getByLabelText('Grant ID').element() as HTMLInputElement).value).toBe('grant-1');

    await screen.getByRole('button', { name: 'Revoke grant' }).click();

    await expect.poll(() => document.body.textContent).toContain('Grant grant-1 revoked.');
    expect(calls).toHaveLength(2);
    expect(calls[1].path).toBe('/api/tenant-grants/revoke');
    const revoked = calls[1].body;
    expect(revoked.grant_id).toBe('grant-1');
    expect(revoked.reason).toBe('operator revoke');
    expect(revoked.source_tenant).toBe('lab-a');
    expect(String(revoked.grantor_pubkey_hex)).toMatch(/^[0-9a-f]{64}$/);
    expect(String(revoked.signature_hex)).toMatch(/^[0-9a-f]{128}$/);
  });

  it('refuses an issue with no target lab and posts nothing', async () => {
    const fetchStub = vi.fn(async () => ({ ok: true, status: 200, json: async () => ({ grant_id: 'grant-1' }) }));
    vi.stubGlobal('fetch', fetchStub);

    const screen = await render(<TenantGrantPanel labs={LABS} />);
    await screen.getByRole('button', { name: 'Issue grant' }).click();

    await expect.poll(() => document.body.textContent).toContain('source, target, scope, and a positive expiry are required.');
    expect(fetchStub).not.toHaveBeenCalled();
  });
});
