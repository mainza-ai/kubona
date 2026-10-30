import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { ObligationsPanel } from './ObligationsPanel';
import * as api from '@/lib/api';

vi.mock('@/lib/api', () => ({
  fetchOpenObligations: vi.fn(),
  closeObligation: vi.fn(),
}));

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node);
  const kids = (node as { children?: unknown[] }).children ?? [];
  return kids.map((c) => textOf(c)).join(' ');
}

describe('ObligationsPanel', () => {
  beforeEach(() => vi.clearAllMocks());

  it('lists open obligations and triggers server closure evaluation', async () => {
    vi.mocked(api.fetchOpenObligations)
      .mockResolvedValueOnce({
        ok: true,
        obligations: [{
          id: 'ob-1234567890', subject_kind: 'mandate', subject_id: 'm-1',
          owner: 'session:s1', disposition: 'open', required_cases: [], required_authorities: [],
          close_ready: true, close_reasons: [],
        }],
      })
      .mockResolvedValueOnce({ ok: true, obligations: [] });
    vi.mocked(api.closeObligation).mockResolvedValue({ ok: true, closed: true });

    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<ObligationsPanel />); });
    await vi.waitFor(() => expect(textOf(tree!.root)).toContain('ob-123456789'));
    const btn = tree!.root.findAllByType('button').find((b) => textOf(b).includes('Evaluate close'));
    expect(btn).toBeDefined();
    await act(async () => { await btn!.props.onClick(); });
    await vi.waitFor(() => expect(api.closeObligation).toHaveBeenCalledWith('ob-1234567890'));
    expect(textOf(tree!.root)).toContain('closed');
  });

  it('renders a refusal instead of an empty queue', async () => {
    vi.mocked(api.fetchOpenObligations).mockResolvedValue({ ok: false, error: 'login required' });
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<ObligationsPanel />); });
    await vi.waitFor(() => expect(textOf(tree!.root)).toContain('login required'));
    expect(textOf(tree!.root)).toContain('Queue unavailable');
  });
});
