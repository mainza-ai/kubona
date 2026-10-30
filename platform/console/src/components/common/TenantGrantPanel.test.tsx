import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { TenantGrantPanel } from './TenantGrantPanel';
import * as api from '@/lib/api';
import * as grants from '@/lib/grants';

vi.mock('@/lib/api', () => ({ issueTenantGrant: vi.fn(), revokeTenantGrant: vi.fn() }));
vi.mock('@/lib/grants', () => ({
  signGrant: vi.fn().mockResolvedValue({ grantor_pubkey_hex: 'pk', signature_hex: 'sig' }),
  signGrantRevoke: vi.fn().mockResolvedValue({ grantor_pubkey_hex: 'pk', signature_hex: 'sig' }),
}));

const LABS = [{ id: 'lab-a', display_name: 'Lab A', autonomy_level: 'L1', status: 'active' } as never];

describe('TenantGrantPanel', () => {
  beforeEach(() => vi.clearAllMocks());

  it('issues a signed grant and revokes by returned id', async () => {
    vi.mocked(api.issueTenantGrant).mockResolvedValue({ ok: true, grant: { grant_id: 'grant-1' } });
    vi.mocked(api.revokeTenantGrant).mockResolvedValue({ ok: true });
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<TenantGrantPanel labs={LABS} />); });
    await act(async () => {
      tree!.root.findByProps({ 'aria-label': 'Grant target lab' }).props.onChange({ target: { value: 'lab-b' } });
    });
    await act(async () => {
      await tree!.root.findAllByType('button').find((b) => JSON.stringify(b.props.children).includes('Issue grant'))!.props.onClick();
    });
    await vi.waitFor(() => expect(api.issueTenantGrant).toHaveBeenCalled());
    expect(grants.signGrant).toHaveBeenCalled();
    expect(JSON.stringify(tree!.toJSON())).toContain('grant-1');

    await act(async () => {
      await tree!.root.findAllByType('button').find((b) => JSON.stringify(b.props.children).includes('Revoke grant'))!.props.onClick();
    });
    await vi.waitFor(() => expect(api.revokeTenantGrant).toHaveBeenCalled());
    expect(grants.signGrantRevoke).toHaveBeenCalledWith('grant-1', 'operator revoke');
  });
});
