import { describe, it, expect, vi, beforeEach } from 'vitest';
import React from 'react';
import TestRenderer, { act } from 'react-test-renderer';
import { ClaimHistory } from './ClaimHistory';
import * as api from '@/lib/api';

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>();
  return {
    ...actual,
    fetchClaims: vi.fn(),
    sendChatMessage: vi.fn(),
    appendLedgerEvent: vi.fn(),
  };
});

const claim: api.ClaimRecord = {
  id: 'clm-test-0001',
  session_id: 's1',
  turn_kind: 'vqe_run',
  statement: 'VQE measured -1.1378 Ha vs FCI -1.1372 Ha.',
  alpha: -1,
  beta: -1,
  gamma: -1,
  verdict: 'verified',
  evidence_json: JSON.stringify({ job_id: 'job-12345678' }),
  created_at: '2026-11-08T10:00:00Z',
};

describe('ClaimHistory', () => {
  beforeEach(() => vi.clearAllMocks());

  it('renders an honest empty state', async () => {
    vi.mocked(api.fetchClaims).mockResolvedValueOnce([]);
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<ClaimHistory sessionId="s1" />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('No evaluated claims');
    expect(text).not.toContain('verified');
  });

  it('renders claim evidence and requests a review without mutating the record', async () => {
    vi.mocked(api.fetchClaims).mockResolvedValueOnce([claim]);
    vi.mocked(api.sendChatMessage).mockResolvedValueOnce({ user: {}, assistant: {} } as never);
    vi.mocked(api.appendLedgerEvent).mockResolvedValueOnce({ ok: true, record_hash: 'rec-review' } as never);
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<ClaimHistory sessionId="s1" />);
    });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('clm-test-0001');
    expect(text).toContain('job-1234');
    expect(text).toContain('not evaluated');
    const review = tree!.root.findAllByType('button').find((button) => String(button.props.children).includes('Request review'));
    expect(review).toBeTruthy();
    await act(async () => { review!.props.onClick(); await Promise.resolve(); });
    expect(api.appendLedgerEvent).toHaveBeenCalledWith('s1', 'peer_review', expect.objectContaining({ claim_id: 'clm-test-0001' }));
    expect(api.sendChatMessage).toHaveBeenCalledWith('s1', expect.stringContaining('clm-test-0001'));
    expect(JSON.stringify(tree!.toJSON())).toContain('Review requested');
  });

  it('surfaces an unavailable history instead of pretending it is empty', async () => {
    vi.mocked(api.fetchClaims).mockResolvedValueOnce(null);
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<ClaimHistory sessionId="s1" />);
    });
    expect(JSON.stringify(tree!.toJSON())).toContain('claim history unavailable');
  });
});
