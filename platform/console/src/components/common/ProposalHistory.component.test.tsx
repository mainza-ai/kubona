import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-react';
import React from 'react';
import { ProposalHistory } from './ProposalHistory';
import type { ProposalRecord } from '@/lib/api';

const rows: ProposalRecord[] = [
  {
    id: 'pr_1234567890abcdef',
    session_id: 's1',
    digest: 'sha256:abc',
    body: {
      target: 'H2 / STO-3G',
      recommended_tier: 'Local Simulator',
      classical_anchor: '-1.1372 Ha',
      spend_estimate: '$0.00',
    },
    status: 'offered',
    created_at: '2026-11-15T10:00:00Z',
  },
  {
    id: 'pr_abcdef1234567890',
    session_id: 's1',
    digest: 'sha256:def',
    body: {
      target: 'LiH / STO-3G',
      recommended_tier: 'IBM QPU',
      classical_anchor: '-7.8 Ha',
      spend_estimate: '$3.20',
    },
    status: 'launched',
    launched_job_id: 'job-12345678',
    created_at: '2026-11-15T11:00:00Z',
  },
];

describe('ProposalHistory in a real browser', () => {
  it('loads the session proposal history only when opened', async () => {
    const screen = await render(<ProposalHistory sessionId="s1" load={async () => rows} />);
    await screen.getByText('Proposal history').click();
    await expect.element(screen.getByText('H2 / STO-3G')).toBeVisible();
    await expect.element(screen.getByText('LiH / STO-3G')).toBeVisible();
    await expect.element(screen.getByText('launched')).toBeVisible();
    await expect.element(screen.getByText(/job-1234/)).toBeVisible();
  });
});
