/**
 * C11 — the exact bytes the operator signs for a run allowance. Must match
 * Go's `allowanceScope` field order, integer micro-dollars, and RFC3339
 * expiry without fractional seconds; the server rebuilds this canonical
 * object and verifies the Ed25519 signature over it.
 */

export interface RunAllowanceScopeInput {
  tenantId: string;
  sessionId: string;
  campaignId: string;
  backend: string;
  jobTypes: string[];
  maxPerJobUsd: number;
  maxTotalUsd: number;
  maxJobs: number;
  shotsCap: number;
  expiresAt: string;
  nonce: string;
}

export interface RunAllowanceDefaults {
  maxPerJobUsd: number;
  maxTotalUsd: number;
  maxJobs: number;
  shotsCap: number;
  ttlHours: number;
}

export function usdToMicro(usd: number): number {
  return Math.round(usd * 1_000_000);
}

/** RFC3339 UTC without milliseconds, matching time.RFC3339. */
export function isoSeconds(ms: number): string {
  return new Date(ms).toISOString().replace(/\.\d{3}Z$/, 'Z');
}

export function canonicalAllowanceScope(input: RunAllowanceScopeInput): string {
  return JSON.stringify({
    tenant_id: input.tenantId,
    session_id: input.sessionId,
    campaign_id: input.campaignId,
    backend: input.backend,
    job_types: [...input.jobTypes],
    max_per_job_micro_usd: usdToMicro(input.maxPerJobUsd),
    max_total_micro_usd: usdToMicro(input.maxTotalUsd),
    max_jobs: input.maxJobs,
    shots_cap: input.shotsCap,
    expires_at: input.expiresAt,
    nonce: input.nonce,
  });
}

/**
 * Last-used values, clamped to sane bounds; the server still enforces the
 * tenant ceiling and the 1 min–24 h TTL. A per-job estimate seeds the fields.
 */
export function defaultAllowanceDraft(perJobEstimateUsd: number, lastUsed?: Partial<RunAllowanceDefaults> | null): RunAllowanceDefaults {
  const perJob = Number.isFinite(perJobEstimateUsd) && perJobEstimateUsd > 0 ? perJobEstimateUsd : 1;
  const maxPerJobUsd = lastUsed?.maxPerJobUsd ?? Number(perJob.toFixed(2));
  return {
    maxPerJobUsd,
    maxTotalUsd: lastUsed?.maxTotalUsd ?? Number((maxPerJobUsd * 3).toFixed(2)),
    maxJobs: Math.max(1, Math.min(100, lastUsed?.maxJobs ?? 3)),
    shotsCap: Math.max(1, lastUsed?.shotsCap ?? 1024),
    ttlHours: Math.max(0.02, Math.min(24, lastUsed?.ttlHours ?? 8)),
  };
}
