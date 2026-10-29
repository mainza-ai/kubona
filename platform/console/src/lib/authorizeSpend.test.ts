import { describe, it, expect, vi, afterEach } from 'vitest';
import { authorizeSpend } from './authorizeSpend';
import type { CampaignState, Session } from '@/types';

const SESSION = {
  id: 'sess-1',
  title: 't',
  tenant_id: 'lab-alpha',
  created_at: '',
  updated_at: '',
  status: 'active',
  spend_limit_usd: 150,
  current_spend_usd: 0,
} as Session;

function campaign(over?: Partial<CampaignState['quantum_factors']>): CampaignState {
  return {
    session_id: 'sess-1',
    target_system: 'H2',
    quantum_factors: {
      estimated_spend_usd: 12.5,
      qasm: 'OPENQASM 2.0;\nqreg q[1];\n',
      recommended_backend: 'ibm_brisbane',
      ...(over as Record<string, unknown>),
    },
  } as unknown as CampaignState;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('authorizeSpend (session-bound, priced-packet-only)', () => {
  it('refuses without a bound session (unbound)', async () => {
    const res = await authorizeSpend({ session: null, campaign: campaign() });
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.reason).toMatch(/No session bound/);
  });

  it('refuses unpriced campaigns (unpriced)', async () => {
    const c = campaign({ estimated_spend_usd: undefined } as unknown as Record<string, never>);
    const res = await authorizeSpend({ session: SESSION, campaign: c });
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.reason).toMatch(/No cost estimate/);
  });

  it('refuses program-less campaigns (unbound program)', async () => {
    const c = campaign({ qasm: '' });
    const res = await authorizeSpend({ session: SESSION, campaign: c });
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.reason).toMatch(/No program/);
  });

  it('refuses invalid mandates (verify fail-closed)', async () => {
    const verify = vi.fn().mockResolvedValue({ valid: false, error: 'bad sig' });
    const res = await authorizeSpend({ session: SESSION, campaign: campaign() }, { verify: verify as never });
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.reason).toMatch(/REJECTED/);
    expect(verify).toHaveBeenCalledOnce();
  });

  it('refuses tenant-less sessions (no tenant-default fallback)', async () => {
    const verify = vi.fn().mockResolvedValue({ valid: true });
    const tenantless = { ...SESSION, tenant_id: undefined };
    const res = await authorizeSpend({ session: tenantless, campaign: campaign() }, { verify: verify as never });
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.reason).toMatch(/No tenant bound/);
    // Refused before minting: the verifier is never consulted.
    expect(verify).not.toHaveBeenCalled();
  });

  it('refuses blank tenant overrides (whitespace is not a tenant)', async () => {
    const verify = vi.fn().mockResolvedValue({ valid: true });
    const res = await authorizeSpend({
      sessionId: 'sess-1', tenantId: '   ', estimatedUsd: 1.5,
      programQasm: 'OPENQASM 2.0; qreg q[1];',
    }, { verify: verify as never });
    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.reason).toMatch(/No tenant bound/);
    expect(verify).not.toHaveBeenCalled();
  });

  it('authorizes priced + bound + program with tenant + canonical binding', async () => {
    let seen: { tenant_id: string; job_hash: string } | null = null;
    const verify = vi.fn().mockImplementation(async (m: { tenant_id: string; job_hash: string }) => {
      seen = { tenant_id: m.tenant_id, job_hash: m.job_hash };
      return { valid: true };
    });
    const res = await authorizeSpend({ session: SESSION, campaign: campaign() }, { verify: verify as never });
    expect(res.ok).toBe(true);
    if (res.ok) {
      expect(res.tenantId).toBe('lab-alpha');
      expect(res.jobHash).toBe(seen!.job_hash);
      expect(seen!.tenant_id).toBe('lab-alpha');
      // Canonical binding over session + exact program
      const { canonicalJobBinding } = await import('./mandates');
      expect(seen!.job_hash).toBe(canonicalJobBinding('sess-1', 'OPENQASM 2.0;\nqreg q[1];\n'));
    }
  });

  it('authorizes direct session/program/estimate overrides', async () => {
    let seen: { job_hash: string } | null = null;
    const verify = vi.fn().mockImplementation(async (m: { job_hash: string }) => {
      seen = { job_hash: m.job_hash };
      return { valid: true };
    });
    const res = await authorizeSpend({
      sessionId: 'sess-1', tenantId: 'lab-alpha', estimatedUsd: 1.5,
      programQasm: 'OPENQASM 2.0; qreg q[1];',
    }, { verify: verify as never });
    expect(res.ok).toBe(true);
    const { canonicalJobBinding } = await import('./mandates');
    expect(seen!.job_hash).toBe(canonicalJobBinding('sess-1', 'OPENQASM 2.0; qreg q[1];'));
  });

  it('refuses when the mandate mint throws (fail-closed)', async () => {
    const c = campaign({ estimated_spend_usd: -5 });
    const res = await authorizeSpend({ session: SESSION, campaign: c });
    expect(res.ok).toBe(false);
  });
});
