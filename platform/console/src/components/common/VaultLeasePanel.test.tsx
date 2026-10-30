import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { VaultLeasePanel } from './VaultLeasePanel';
import * as api from '@/lib/api';

vi.mock('@/lib/api', () => ({ issueVaultLease: vi.fn(), revokeVaultLease: vi.fn() }));

const LEASE = {
  lease_id: 'lease-123', scope: 'provider:session_token', ttl_seconds: 60,
  lease_signature: 'abcdef0123456789', issued_at: '2030-01-01T00:00:00Z', expires_at: '2030-01-01T00:01:00Z',
};

describe('VaultLeasePanel', () => {
  beforeEach(() => vi.clearAllMocks());

  it('issues and revokes an opaque lease id', async () => {
    vi.mocked(api.issueVaultLease).mockResolvedValue({ ok: true, lease: LEASE });
    vi.mocked(api.revokeVaultLease).mockResolvedValue({ ok: true });
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<VaultLeasePanel />); });
    await act(async () => {
      await tree!.root.findByProps({ 'aria-label': 'Lease TTL seconds' }).props.onChange({ target: { value: '60' } });
    });
    await act(async () => {
      await tree!.root.findAllByType('button').find((b) => JSON.stringify(b.props.children).includes('Issue lease'))!.props.onClick();
    });
    await vi.waitFor(() => expect(api.issueVaultLease).toHaveBeenCalled());
    expect(JSON.stringify(tree!.toJSON())).toContain('lease-123');
    await act(async () => {
      await tree!.root.findAllByType('button').find((b) => JSON.stringify(b.props.children).includes('Revoke lease'))!.props.onClick();
    });
    await vi.waitFor(() => expect(api.revokeVaultLease).toHaveBeenCalledWith('lease-123'));
    expect(JSON.stringify(tree!.toJSON())).not.toContain('lease-123');
  });
});
