import { describe, expect, it } from 'vitest';
import { canonicalAllowanceScope, defaultAllowanceDraft, isoSeconds, usdToMicro } from './runAllowance';

describe('run allowance canonical scope (C11)', () => {
  it('matches Go field order and integer micro-dollars', () => {
    const canonical = canonicalAllowanceScope({
      tenantId: 'lab-a',
      sessionId: 'sess-1',
      campaignId: 'sess-1',
      backend: 'ibm_fez',
      jobTypes: ['circuit_run'],
      maxPerJobUsd: 1.25,
      maxTotalUsd: 3.75,
      maxJobs: 3,
      shotsCap: 1024,
      expiresAt: '2026-11-14T23:00:00Z',
      nonce: 'nonce-123',
    });
    expect(canonical).toBe(
      '{"tenant_id":"lab-a","session_id":"sess-1","campaign_id":"sess-1","backend":"ibm_fez",' +
        '"job_types":["circuit_run"],"max_per_job_micro_usd":1250000,' +
        '"max_total_micro_usd":3750000,"max_jobs":3,"shots_cap":1024,' +
        '"expires_at":"2026-11-14T23:00:00Z","nonce":"nonce-123"}',
    );
  });

  it('formats expiry without fractional seconds and converts USD exactly', () => {
    expect(isoSeconds(Date.parse('2026-11-14T23:00:00.000Z'))).toBe('2026-11-14T23:00:00Z');
    expect(usdToMicro(0.01)).toBe(10_000);
    expect(usdToMicro(1.005)).toBe(1_005_000);
  });

  it('prefills last-used values but clamps unsafe ones', () => {
    expect(defaultAllowanceDraft(2.5)).toMatchObject({
      maxPerJobUsd: 2.5,
      maxTotalUsd: 7.5,
      maxJobs: 3,
      shotsCap: 1024,
      ttlHours: 8,
    });
    expect(defaultAllowanceDraft(2.5, { maxJobs: 500, ttlHours: 99, maxTotalUsd: 10 })).toMatchObject({
      maxJobs: 100,
      ttlHours: 24,
      maxTotalUsd: 10,
    });
  });
});
