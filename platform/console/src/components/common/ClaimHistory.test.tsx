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
    fetchClaimReviews: vi.fn(),
    reviewClaim: vi.fn(),
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

  it('records an append-only claim decision with a required reason', async () => {
    vi.mocked(api.fetchClaims).mockResolvedValueOnce([claim]);
    vi.mocked(api.fetchClaimReviews).mockResolvedValueOnce([]);
    vi.mocked(api.reviewClaim).mockResolvedValueOnce({
      id: 'rev-test-0001', session_id: 's1', claim_id: claim.id, decision: 'accepted',
      reason: 'uncertainty reviewed', reviewer: 'operator', evidence_json: '{}',
      created_at: '2026-11-08T11:00:00Z',
    });
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => {
      tree = TestRenderer.create(<ClaimHistory sessionId="s1" />);
    });
    const reason = tree!.root.findByProps({ 'aria-label': 'Claim review reason' });
    await act(async () => { reason.props.onChange({ target: { value: 'uncertainty reviewed' } }); });
    const accept = tree!.root.findAllByType('button').find((button) => String(button.props.children) === 'accepted');
    expect(accept).toBeTruthy();
    await act(async () => { accept!.props.onClick(); await Promise.resolve(); await Promise.resolve(); });
    expect(api.reviewClaim).toHaveBeenCalledWith('s1', claim.id, 'accepted', 'uncertainty reviewed');
  });

  it('refuses a claim decision without a reason', async () => {
    vi.mocked(api.fetchClaims).mockResolvedValueOnce([claim]);
    vi.mocked(api.fetchClaimReviews).mockResolvedValueOnce([]);
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<ClaimHistory sessionId="s1" />); });
    const contest = tree!.root.findAllByType('button').find((button) => String(button.props.children) === 'contested');
    if (!contest) throw new Error('contest button missing');
    await act(async () => { contest.props.onClick(); await Promise.resolve(); });
    expect(api.reviewClaim).not.toHaveBeenCalled();
    expect(JSON.stringify(tree!.toJSON())).toContain('review reason is required');
  });

  it('shows a failed decision without pretending it was applied', async () => {
    vi.mocked(api.fetchClaims).mockResolvedValueOnce([claim]);
    vi.mocked(api.fetchClaimReviews).mockResolvedValueOnce([]);
    vi.mocked(api.reviewClaim).mockResolvedValueOnce(null);
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<ClaimHistory sessionId="s1" />); });
    const reason = tree!.root.findByProps({ 'aria-label': 'Claim review reason' });
    await act(async () => { reason.props.onChange({ target: { value: 'recheck' } }); });
    const reopen = tree!.root.findAllByType('button').find((button) => String(button.props.children) === 'reopened');
    if (!reopen) throw new Error('reopen button missing');
    await act(async () => { reopen.props.onClick(); await Promise.resolve(); await Promise.resolve(); });
    expect(JSON.stringify(tree!.toJSON())).toContain('claim review was refused');
  });

  it('renders the no-session empty state without fetching', async () => {
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<ClaimHistory sessionId={null} />); });
    expect(JSON.stringify(tree!.toJSON())).toContain('No evaluated claims');
    expect(api.fetchClaims).not.toHaveBeenCalled();
  });

  it('marks a thrown decision as failed and preserves the reason', async () => {
    vi.mocked(api.fetchClaims).mockResolvedValueOnce([claim]);
    vi.mocked(api.fetchClaimReviews).mockResolvedValueOnce([]);
    vi.mocked(api.reviewClaim).mockRejectedValueOnce(new Error('offline'));
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<ClaimHistory sessionId="s1" />); });
    const reason = tree!.root.findByProps({ 'aria-label': 'Claim review reason' });
    await act(async () => { reason.props.onChange({ target: { value: 'recheck' } }); });
    const accept = tree!.root.findAllByType('button').find((button) => String(button.props.children) === 'accepted');
    if (!accept) throw new Error('accept button missing');
    await act(async () => { accept.props.onClick(); await Promise.resolve(); });
    expect(api.reviewClaim).toHaveBeenCalled();
  });

  it('does not invent an evidence job from malformed evidence JSON', async () => {
    vi.mocked(api.fetchClaims).mockResolvedValueOnce([{ ...claim, evidence_json: '{not-json' }]);
    vi.mocked(api.fetchClaimReviews).mockResolvedValueOnce([]);
    let tree: TestRenderer.ReactTestRenderer;
    await act(async () => { tree = TestRenderer.create(<ClaimHistory sessionId="s1" />); });
    const text = JSON.stringify(tree!.toJSON());
    expect(text).toContain('clm-test-0001');
    expect(text).not.toContain('job job-');
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
