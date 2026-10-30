import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { LedgerVerifyPanel } from './LedgerVerifyPanel';
import * as api from '@/lib/api';

vi.mock('@/lib/api', () => ({ verifyLedger: vi.fn() }));

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map((c) => textOf(c)).join(' ');
}

describe('LedgerVerifyPanel', () => {
  beforeEach(() => vi.clearAllMocks());

  it('runs a scoped verification and renders the measured fault count', async () => {
    vi.mocked(api.verifyLedger).mockResolvedValue({ ok: false, fault_count: 2, faults: [] });
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<LedgerVerifyPanel sessionId="s1" />); });
    const btn = tree!.root.findAllByType('button').find((b) => textOf(b).includes('Verify ledger'));
    await act(async () => { await btn!.props.onClick(); });
    await vi.waitFor(() => expect(api.verifyLedger).toHaveBeenCalledWith('s1'));
    expect(textOf(tree!.root)).toContain('2 fault');
  });

  it('renders a refusal verbatim', async () => {
    vi.mocked(api.verifyLedger).mockResolvedValue({ ok: false, error: 'bearer required' });
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<LedgerVerifyPanel sessionId={null} />); });
    await act(async () => {
      await tree!.root.findAllByType('button').find((b) => textOf(b).includes('Verify ledger'))!.props.onClick();
    });
    await vi.waitFor(() => expect(textOf(tree!.root)).toContain('bearer required'));
  });
});
