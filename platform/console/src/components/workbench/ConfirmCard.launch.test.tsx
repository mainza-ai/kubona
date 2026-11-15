import { describe, expect, it, vi } from 'vitest';
import { executeMeteredAuthorization } from './ConfirmCard';
import { launchProposal, mintProposal } from '@/lib/api';
import { authorizeSpend } from '@/lib/authorizeSpend';
import { reconcileOperationFull, UNCONFIRMED_LAUNCH } from '@/domain/write-settlement';
import type { CampaignState, Session } from '@/types';

vi.mock('@/lib/authorizeSpend', () => ({
  authorizeSpend: vi.fn(async () => ({
    ok: true,
    mandate: { signature_hex: 'ab' },
    tenantId: 'lab',
    jobHash: 'hash',
  })),
}));

vi.mock('@/lib/api', () => ({
  launchProposal: vi.fn(),
  mintProposal: vi.fn(async () => ({
    ok: true,
    proposal: { id: 'pr_1', digest: 'sha256:abc', body: { backend: 'ibm_test' } },
  })),
}));

vi.mock('@/domain/write-settlement', () => ({
  UNCONFIRMED_LAUNCH: 'The launch reply was lost. Whether a job was created is unknown. Retry stays disabled until this request is reconciled.',
  launchJobId: (receipt: unknown) => {
    const job = (receipt as { job?: { id?: unknown } } | null)?.job;
    return job?.id == null ? null : String(job.id);
  },
  reconcileOperationFull: vi.fn(),
}));

const session = { id: 'sess-1', tenant_id: 'lab' } as Session;
const campaign = {
  quantum_factors: {
    estimated_spend_usd: 1.5,
    qasm: 'OPENQASM 2.0;',
    recommended_backend: 'ibm_test',
  },
} as CampaignState;

describe('lost launch reply', () => {
  it('never renders that no job was created', async () => {
    vi.mocked(launchProposal).mockResolvedValueOnce({ outcome: 'unknown', requestId: 'req-lost' });
    vi.mocked(reconcileOperationFull).mockResolvedValueOnce({
      phase: 'unknown',
      requestId: 'req-lost',
      detail: 'not confirmed',
    });
    const phases: string[] = [];
    const outcome = await executeMeteredAuthorization(campaign, session, () => {
      phases.push('working');
    });
    expect(authorizeSpend).toHaveBeenCalled();
    // B1: launch names the minted proposal, never a client-built plan.
    expect(mintProposal).toHaveBeenCalledWith('sess-1', 'campaign');
    expect(launchProposal).toHaveBeenCalledWith(
      'sess-1',
      { proposalId: 'pr_1', digest: 'sha256:abc' },
      expect.objectContaining({ requestId: expect.any(String) }),
    );
    expect(reconcileOperationFull).toHaveBeenCalledWith('req-lost', 'sess-1');
    expect(phases).toEqual(['working']);
    expect(outcome).toEqual({ ok: false, unknown: true, message: UNCONFIRMED_LAUNCH });
    expect(outcome.ok ? '' : outcome.message).not.toMatch(/no job was created/i);
  });

  it('launches under a signed allowance without a per-job mandate (C11)', async () => {
    vi.mocked(authorizeSpend).mockClear();
    vi.mocked(launchProposal).mockClear();
    vi.mocked(mintProposal).mockClear();
    vi.mocked(mintProposal).mockResolvedValueOnce({
      ok: true,
      proposal: { id: 'pr_allow', digest: 'sha256:allow', body: { backend: 'ibm_test' } },
    } as never);
    vi.mocked(launchProposal).mockResolvedValueOnce({
      job: { id: 'job-allow' },
      loop: { id: 'loop-allow' },
    } as never);
    const outcome = await executeMeteredAuthorization(campaign, session, undefined, 'ra-1');
    expect(authorizeSpend).not.toHaveBeenCalled();
    expect(launchProposal).toHaveBeenCalledWith(
      'sess-1',
      { proposalId: 'pr_allow', digest: 'sha256:allow' },
      expect.objectContaining({ runAllowanceId: 'ra-1' }),
    );
    expect(outcome).toMatchObject({ ok: true, jobId: 'job-allow' });
  });

  it('refuses to dispatch when the server cannot mint an honest plan', async () => {
    vi.mocked(mintProposal).mockResolvedValueOnce({
      ok: false,
      error: 'metered backend "ibm_test" has no numeric max_cost_usd',
    });
    vi.mocked(launchProposal).mockClear();
    const outcome = await executeMeteredAuthorization(campaign, session);
    expect(launchProposal).not.toHaveBeenCalled();
    expect(outcome.ok).toBe(false);
    expect(outcome.ok ? '' : outcome.message).toContain('No launchable plan');
    expect(outcome.ok ? '' : outcome.message).toContain('max_cost_usd');
  });

  it('keeps the job when reconciliation finds the receipt', async () => {
    vi.mocked(launchProposal).mockResolvedValueOnce({ outcome: 'unknown', requestId: 'req-kept' });
    vi.mocked(reconcileOperationFull).mockResolvedValueOnce({
      phase: 'applied',
      requestId: 'req-kept',
      detail: 'HTTP 200',
      receipt: { job: { id: 'job-42' } },
    });
    const outcome = await executeMeteredAuthorization(campaign, session);
    expect(outcome).toEqual({ ok: true, jobId: 'job-42', backend: 'ibm_test' });
  });
});
