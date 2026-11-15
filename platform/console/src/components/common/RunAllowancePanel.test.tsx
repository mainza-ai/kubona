import { describe, expect, it, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import * as api from '@/lib/api';
import * as custody from '@/lib/custody';
import { RunAllowancePanel } from './RunAllowancePanel';

vi.mock('@/lib/api', () => ({
  fetchRunAllowances: vi.fn(),
  mintRunAllowance: vi.fn(),
  revokeRunAllowance: vi.fn(),
}));
vi.mock('@/lib/custody', () => ({
  signWithCustody: vi.fn(),
}));

const allowance = {
  id: 'ra-12345678',
  tenant_id: 'lab-a',
  session_id: 'sess-1',
  backend: 'ibm_fez',
  job_types: ['circuit_run'],
  max_per_job_usd: 1,
  max_total_usd: 3,
  remaining_usd: 3,
  max_jobs: 3,
  jobs_used: 0,
  shots_cap: 1024,
  status: 'active' as const,
  issued_at: '2026-11-14T20:00:00Z',
  expires_at: '2026-11-15T04:00:00Z',
  signer_pubkey_hex: 'ab',
  scope_digest: 'sha256:x',
};

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(textOf).join(' ');
  if (node && typeof node === 'object' && 'children' in node) {
    const children = (node as { children?: unknown }).children;
    if (Array.isArray(children)) return children.map(textOf).join(' ');
  }
  return '';
}

beforeEach(() => {
  vi.mocked(api.fetchRunAllowances).mockResolvedValue({ ok: true, allowances: [] });
  vi.mocked(api.mintRunAllowance).mockResolvedValue({ ok: true, allowance });
  vi.mocked(api.revokeRunAllowance).mockResolvedValue({ ok: true });
  vi.mocked(custody.signWithCustody).mockResolvedValue({
    publicKeyHex: 'ab'.repeat(32),
    signature: new Uint8Array(64).fill(1),
  });
});

describe('RunAllowancePanel (C11)', () => {
  it('signs the canonical scope and mints a bounded allowance', async () => {
    const onSelect = vi.fn();
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <RunAllowancePanel sessionId="sess-1" tenantId="lab-a" backend="ibm_fez" perJobEstimateUsd={1} onSelect={onSelect} />,
      );
    });
    await act(async () => { await Promise.resolve(); });
    expect(api.fetchRunAllowances).toHaveBeenCalledWith('sess-1');

    await act(async () => {
      tree.root.findByProps({ 'data-testid': 'run-allowance-mint' }).props.onClick();
    });
    await act(async () => { await Promise.resolve(); });

    expect(custody.signWithCustody).toHaveBeenCalledTimes(1);
    const signed = vi.mocked(custody.signWithCustody).mock.calls[0][0] as Uint8Array;
    const canonical = new TextDecoder().decode(signed);
    expect(canonical).toContain('"max_per_job_micro_usd":1000000');
    expect(canonical).toContain('"max_total_micro_usd":3000000');
    expect(canonical).toContain('"backend":"ibm_fez"');
    expect(canonical).toContain('"job_types":["circuit_run"]');
    expect(canonical).not.toMatch(/\d{3}Z"/); // expiry has no milliseconds
    const minted = vi.mocked(api.mintRunAllowance).mock.calls[0][0];
    expect(minted.signer_pubkey_hex).toHaveLength(64);
    expect(minted.signature_hex).toHaveLength(128);
    expect(textOf(tree.root.findByProps({ 'data-testid': 'run-allowance-receipt' }))).toContain('Allowance active');
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: allowance.id }));
  });

  it('lists active allowances for selection and revocation', async () => {
    vi.mocked(api.fetchRunAllowances).mockResolvedValue({ ok: true, allowances: [allowance] });
    const onSelect = vi.fn();
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <RunAllowancePanel sessionId="sess-1" tenantId="lab-a" backend="ibm_fez" perJobEstimateUsd={1} onSelect={onSelect} />,
      );
    });
    await act(async () => { await Promise.resolve(); });
    const radio = tree.root.findByProps({ 'aria-label': 'Use allowance ra-12345' });
    await act(async () => { radio.props.onChange(); });
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ id: allowance.id }));
    const revoke = tree.root.findByProps({ 'aria-label': 'Revoke allowance ra-12345' });
    await act(async () => { revoke.props.onClick(); });
    await act(async () => { await Promise.resolve(); });
    expect(api.revokeRunAllowance).toHaveBeenCalledWith(allowance.id, 'sess-1', expect.any(String));
  });

  it('refuses to offer an allowance for a simulator backend', async () => {
    let tree!: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(
        <RunAllowancePanel sessionId="sess-1" tenantId="lab-a" backend="local_simulator" perJobEstimateUsd={0} onSelect={() => {}} />,
      );
    });
    expect(tree.root.findAllByProps({ 'data-testid': 'run-allowance' })).toHaveLength(0);
    expect(tree.root.findByProps({ 'data-testid': 'run-allowance-ineligible' })).toBeTruthy();
  });
});
