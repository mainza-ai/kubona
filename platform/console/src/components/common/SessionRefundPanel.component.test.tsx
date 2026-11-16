import { afterEach, describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { SessionRefundPanel } from './SessionRefundPanel';

const LABS = [{ id: 'lab-a', display_name: 'Lab A', autonomy_level: 'L1', status: 'active' } as never];
const SESSIONS = [{ id: 'sess-1', title: 'Campaign', tenant_id: 'lab-a', created_at: '', updated_at: '', status: 'active' } as never];

afterEach(() => vi.unstubAllGlobals());

describe('SessionRefundPanel in a real browser', () => {
  it('signs the tenant-bound refund and reports success', async () => {
    const bodies: Array<Record<string, unknown>> = [];
    vi.stubGlobal('fetch', async (url: string, init?: RequestInit) => {
      if (init?.method !== 'POST' || !String(url).includes('/api/sessions/refund')) {
        throw new Error(`unexpected fetch ${init?.method ?? 'GET'} ${url}`);
      }
      bodies.push(JSON.parse(String(init.body)) as Record<string, unknown>);
      return { ok: true, status: 200, json: async () => ({ id: 'sess-1', current_spend_usd: 2.5 }) };
    });

    const screen = await render(<SessionRefundPanel labs={LABS} sessions={SESSIONS} />);
    await screen.getByLabelText('Session refund amount').fill('2.50');
    await screen.getByLabelText('Session refund reason').fill('overcharge');
    await screen.getByRole('button', { name: 'Refund session' }).click();

    await expect.poll(() => document.body.textContent).toContain('Refunded $2.50 to sess-1.');
    expect(bodies).toHaveLength(1);
    const body = bodies[0];
    expect(body.session_id).toBe('sess-1');
    expect(body.amount_usd).toBe(2.5);
    expect(body.reason).toBe('overcharge');
    // The admin envelope is signed in-browser (custody), not stubbed here.
    const action = body.action as Record<string, unknown>;
    expect(action.tenant_id).toBe('lab-a');
    expect(action.action).toBe('session_refund_spend');
    expect(String(action.params_hash)).toMatch(/^[0-9a-f]{64}$/);
    expect(String(action.pubkey_hex)).toMatch(/^[0-9a-f]{64}$/);
    expect(String(action.signature_hex)).toMatch(/^[0-9a-f]{128}$/);
    await expect.poll(() => document.body.textContent).toContain('applied · $2.50 refunded to sess-1');
  });

  it('refuses a non-positive amount without calling the backend', async () => {
    const fetchStub = vi.fn(async () => ({ ok: true, status: 200, json: async () => ({ id: 'sess-1', current_spend_usd: 0 }) }));
    vi.stubGlobal('fetch', fetchStub);

    const screen = await render(<SessionRefundPanel labs={LABS} sessions={SESSIONS} />);
    await screen.getByLabelText('Session refund amount').fill('0');
    await screen.getByLabelText('Session refund reason').fill('overcharge');
    await screen.getByRole('button', { name: 'Refund session' }).click();

    await expect.poll(() => document.body.textContent).toContain('amount must be positive.');
    expect(fetchStub).not.toHaveBeenCalled();
  });
});
