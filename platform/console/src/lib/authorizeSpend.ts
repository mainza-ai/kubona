import type { CampaignState, Session } from '@/types';
import { canonicalJobBinding, createSignedSpendMandate, type SpendMandate } from './mandates';
import { verifySpendMandate } from './api';

export interface AuthorizeSpendInput {
  session?: Session | null;
  campaign?: CampaignState | null;
  // Direct overrides for surfaces that hold the exact program/cost/session
  // (e.g. QuantumDispatch) instead of a campaign.
  sessionId?: string;
  tenantId?: string;
  estimatedUsd?: number;
  programQasm?: string;
  ttlSeconds?: number;
}

export type AuthorizeSpendResult =
  | { ok: true; mandate: SpendMandate; tenantId: string; jobHash: string }
  | { ok: false; reason: string };

export interface AuthorizeSpendDeps {
  create?: typeof createSignedSpendMandate;
  verify?: typeof verifySpendMandate;
}

/**
 * Session-bound, priced-packet-only spend authorization (create + verify).
 * Fail-closed gates (same as ActionGate):
 *  - no session id -> refuse (unbound)
 *  - no priced estimate (finite number) -> refuse (unpriced)
 *  - no program (empty QASM) -> refuse (unbound program)
 * Tenant resolves from session.tenant_id (never session.id); the
 * 'tenant-default' fallback keeps lab-less dev sessions working without
 * inventing attribution — mandates.ts still throws on a truly empty tenant.
 * The job hash is the canonical v1 binding over (session + exact QASM) so a
 * mandate for one program cannot replay against another.
 */
export async function authorizeSpend(
  input: AuthorizeSpendInput,
  deps: AuthorizeSpendDeps = {}
): Promise<AuthorizeSpendResult> {
  const create = deps.create ?? createSignedSpendMandate;
  const verify = deps.verify ?? verifySpendMandate;
  const session = input.session;
  const campaign = input.campaign;

  const sessionId = (input.sessionId ?? session?.id ?? '').trim();
  if (!sessionId) {
    return { ok: false, reason: 'No session bound — authorization disabled.' };
  }
  const estimated = input.estimatedUsd ?? campaign?.quantum_factors?.estimated_spend_usd;
  if (typeof estimated !== 'number' || !Number.isFinite(estimated)) {
    return { ok: false, reason: 'No cost estimate on this campaign — authorization disabled until priced.' };
  }
  const programQasm = input.programQasm ?? campaign?.quantum_factors?.qasm ?? '';
  if (!programQasm) {
    return { ok: false, reason: 'No program in this campaign — authorization disabled until a circuit is proposed.' };
  }
  const tenantId = (input.tenantId ?? session?.tenant_id ?? '').trim() || 'tenant-default';
  let jobHash: string;
  try {
    jobHash = canonicalJobBinding(sessionId, programQasm);
  } catch (e) {
    return { ok: false, reason: `Job binding failed — authorization disabled (${e instanceof Error ? e.message : String(e)}).` };
  }
  let mandate: SpendMandate;
  try {
    mandate = create({
      tenantId,
      jobHash,
      maxBudgetUsd: estimated,
      ttlSeconds: input.ttlSeconds ?? 3600,
    });
  } catch (e) {
    return { ok: false, reason: `Mandate mint refused — authorization disabled (${e instanceof Error ? e.message : String(e)}).` };
  }
  try {
    const res = await verify(mandate);
    if (!res.valid) {
      return { ok: false, reason: `Mandate REJECTED by verifier (${res.error || 'invalid'}). Halted fail-closed — nothing dispatched, nothing spent.` };
    }
  } catch (e) {
    return { ok: false, reason: `Mandate verification failed — halted fail-closed (${e instanceof Error ? e.message : String(e)}).` };
  }
  return { ok: true, mandate, tenantId, jobHash };
}
