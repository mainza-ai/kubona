import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { SessionRefundPanel } from './SessionRefundPanel';
import * as api from '@/lib/api';
import * as admin from '@/lib/adminAction';

vi.mock('@/lib/api', () => ({ refundSessionSpend: vi.fn() }));
vi.mock('@/lib/adminAction', () => ({
  signAdminActionWithCustody: vi.fn().mockResolvedValue({
    tenant_id: 'lab-a', action: 'session_refund_spend', params_hash: 'h', expiry: '2030-01-01T00:00:00Z', pubkey_hex: 'pk', signature_hex: 'sig',
  }),
}));

const LABS = [{ id: 'lab-a', display_name: 'Lab A', autonomy_level: 'L1', status: 'active' } as never];
const SESSIONS = [{ id: 'sess-1', title: 'Campaign', tenant_id: 'lab-a', created_at: '', updated_at: '', status: 'active' } as never];

describe('SessionRefundPanel', () => {
  beforeEach(() => vi.clearAllMocks());

  it('signs the tenant-bound refund and reports success', async () => {
    vi.mocked(api.refundSessionSpend).mockResolvedValue({ ok: true });
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<SessionRefundPanel labs={LABS} sessions={SESSIONS} />); });
    await act(async () => {
      tree!.root.findByProps({ 'aria-label': 'Session refund amount' }).props.onChange({ target: { value: '2.50' } });
    });
    await act(async () => {
      tree!.root.findByProps({ 'aria-label': 'Session refund reason' }).props.onChange({ target: { value: 'overcharge' } });
    });
    await act(async () => {
      await tree!.root.findAllByType('button').find((b) => JSON.stringify(b.props.children).includes('Refund session'))!.props.onClick();
    });
    await vi.waitFor(() => expect(api.refundSessionSpend).toHaveBeenCalled());
    expect(admin.signAdminActionWithCustody).toHaveBeenCalled();
    expect(JSON.stringify(tree!.toJSON())).toContain('Refunded');
  });
});
