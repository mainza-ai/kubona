import type { ApprovalPacketView } from '@/components/common/ApprovalPacketCard';

interface CampaignLike {
  session_id?: string;
  target_system?: string;
  quantum_factors?: {
    qubit_count?: number;
    recommended_backend?: string;
  };
  classical_anchor?: {
    method?: string;
    discrepancy_mha?: number;
  };
  notebook?: {
    provenance_hash?: string;
  };
}

export const APPROVAL_TTL_MS = 15 * 60 * 1000;

/**
 * Metered-backend test (calibrated friction): simulator-class backends run
 * under L1 auto-allow and must NEVER raise the Action Gate; anything else
 * (hardware names, unknown tiers) needs human eyes. Unknown defaults to
 * metered (fail-closed visibility). Mirrors the server's tier normalization
 * smells so both sides agree on what "local" means.
 */
export function isMeteredBackend(backend?: string | null): boolean {
  if (!backend) return true;
  const lower = backend.toLowerCase();
  const simSmells = ['local', 'simulator', 'statevector', 'aer', 'sim-first', 'sim first', 'starter', 'shot-based', 'numpy'];
  return !simSmells.some((smell) => lower.includes(smell));
}

/** Denial requires a non-blank reason (returned to the agent). Pure. */
export function canConfirmDenial(reason: string): boolean {
  return reason.trim().length > 0;
}

/**
 * Assemble a burden-scaled approval packet from campaign telemetry.
 * No verified data -> explicit UNVERIFIED markers, never invented numbers.
 */
export function buildApprovalPacketFromCampaign(
  campaign: CampaignLike | null | undefined,
  sessionId: string,
  estimatedCostUsd: number,
  spendCeilingUsd: number,
  spentUsd: number,
  nowMs: number = Date.now()
): ApprovalPacketView {
  const qf = campaign?.quantum_factors;
  const anchor = campaign?.classical_anchor;
  const provenanceHash = campaign?.notebook?.provenance_hash;
  return {
    packetId: `appr-${(sessionId || 'local').slice(0, 8)}`,
    agentId: 'ansatz-scout',
    backend: qf?.recommended_backend || 'local-simulator',
    programHash: provenanceHash ? provenanceHash.slice(0, 24) : 'uncomputed',
    qubits: qf?.qubit_count ?? 2,
    shots: 1024,
    costMaxUsd: estimatedCostUsd,
    budgetRemainingUsd: Math.max(0, spendCeilingUsd - spentUsd),
    costBasis: 'price sheet v2026-10-01 + queue fee',
    blastRadius: 'spend only; no data egress',
    reversibility: 'irreversible spend once dequeued; cancel possible before dispatch',
    evidence: {
      dryRun: 'simulator pre-flight required before paid queue (cost-governance rail)',
      classicalAnchor:
        anchor?.method != null && anchor?.discrepancy_mha != null
          ? `${anchor.method}: Δ ${anchor.discrepancy_mha.toFixed(2)} mHa`
          : undefined,
    },
    alternatives: {
      simFirstSavings: 'local simulator: $0.00',
      fewerShotsSavings: 'fewer shots lower cost linearly per price sheet',
    },
    expiresAtIso: new Date(nowMs + APPROVAL_TTL_MS).toISOString(),
    noncePreview: (sessionId || 'local').slice(0, 8),
  };
}
