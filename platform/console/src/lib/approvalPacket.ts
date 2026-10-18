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
    anchor_computed?: boolean;
  };
  notebook?: {
    provenance_hash?: string;
  };
}

export const APPROVAL_TTL_MS = 15 * 60 * 1000;

// Single price-sheet const: every packet cites the same sheet. Bump the
// date here, not in call sites, so renderers cannot drift apart.
export const PRICE_SHEET = 'price sheet v2026-10-01 + queue fee';

/**
 * Explicit backend tier enum. Unknown maps to metered-only handling: an
 * unrecognized backend never auto-allows — isMeteredBackend(true) forces
 * the Action Gate. Keep this mapping in one place.
 */
export type BackendTier = 'simulator' | 'managed-simulator' | 'hardware-qpu' | 'unknown';

export function classifyBackendTier(backend?: string | null): BackendTier {
  if (!backend || !backend.trim()) return 'unknown';
  const lower = backend.toLowerCase();
  if (/(local|simulator|statevector|aer|sim-first|sim first|starter|shot-based|numpy)/.test(lower)) {
    return 'simulator';
  }
  if (/(sv1|tn1|braket|cuda-q|managed|cloud-sim)/.test(lower)) return 'managed-simulator';
  if (/(ibm|hera|brisbane|quera|aquila|quantinuum|rigetti|qpu|hardware)/.test(lower)) return 'hardware-qpu';
  return 'unknown';
}

/**
 * Metered-backend test (calibrated friction): simulator-class backends run
 * under L1 auto-allow and must NEVER raise the Action Gate; anything else
 * (hardware names, unknown tiers) needs human eyes. Unknown defaults to
 * metered (fail-closed visibility). Delegates to classifyBackendTier so
 * tier classification lives in one place.
 */
export function isMeteredBackend(backend?: string | null): boolean {
  return classifyBackendTier(backend) !== 'simulator';
}

/** Denial requires a non-blank reason (returned to the agent). Pure. */
export function canConfirmDenial(reason: string): boolean {
  return reason.trim().length > 0;
}

/**
 * Assemble a burden-scaled approval packet from campaign telemetry.
 * No verified data -> explicit UNVERIFIED markers, never invented numbers.
 * Shots is an explicit param (no hardcoded dispatch size in callers);
 * budgetBreached flags ceiling-spent < 0 so renderers alarm instead of
 * clamping silently (budgetRemainingUsd stays clamped for display).
 */
export function buildApprovalPacketFromCampaign(
  campaign: CampaignLike | null | undefined,
  sessionId: string,
  estimatedCostUsd: number,
  spendCeilingUsd: number,
  spentUsd: number,
  nowMs: number = Date.now(),
  shots = 1024
): ApprovalPacketView {
  const qf = campaign?.quantum_factors;
  const anchor = campaign?.classical_anchor;
  const provenanceHash = campaign?.notebook?.provenance_hash;
  const ceiling = Number.isFinite(spendCeilingUsd) ? spendCeilingUsd : 0;
  const spent = Number.isFinite(spentUsd) ? spentUsd : 0;
  const breached = ceiling - spent < 0;
  const anchorComputed =
    anchor?.anchor_computed !== false &&
    Boolean(anchor?.method) &&
    anchor?.method !== 'uncomputed' &&
    typeof anchor?.discrepancy_mha === 'number' &&
    Number.isFinite(anchor.discrepancy_mha);
  return {
    packetId: `appr-${(sessionId || 'local').slice(0, 8)}`,
    agentId: 'ansatz-scout',
    backend: qf?.recommended_backend || 'local-simulator',
    programHash: provenanceHash ? provenanceHash.slice(0, 24) : 'uncomputed',
    qubits: qf?.qubit_count ?? 2,
    shots: Number.isFinite(shots) && shots > 0 ? Math.floor(shots) : 1024,
    costMaxUsd: estimatedCostUsd,
    budgetRemainingUsd: Math.max(0, ceiling - spent),
    budgetBreached: breached,
    costBasis: PRICE_SHEET,
    blastRadius: 'spend only; no data egress',
    reversibility: 'irreversible spend once dequeued; cancel possible before dispatch',
    evidence: {
      dryRun: 'simulator pre-flight required before paid queue (cost-governance rail)',
      classicalAnchor: anchorComputed
        ? `${anchor!.method}: Δ ${anchor!.discrepancy_mha!.toFixed(2)} mHa`
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
