import { describe, it, expect } from 'vitest';
import {
  APPROVAL_TTL_MS,
  buildApprovalPacketFromCampaign,
  canConfirmDenial,
} from './approvalPacket';

const CAMPAIGN = {
  session_id: 'sess-abcdef123456',
  target_system: 'H2',
  quantum_factors: {
    recommended_backend: 'ibm_brisbane',
    qubit_count: 20,
  },
  classical_anchor: {
    method: 'PySCF STO-3G FCI',
    discrepancy_mha: 0.47,
  },
  notebook: {
    provenance_hash: 'sha256:7f83b1657ff1fc53b92dc18148a1d65dfc2d4b1fa3d677284addd200126d9069',
  },
} as const;

describe('approval packet builder (Q-11 burden-scaled packet)', () => {
  it('assembles every competent-approval field from campaign telemetry', () => {
    const p = buildApprovalPacketFromCampaign(
      CAMPAIGN as never,
      'sess-abcdef123456',
      48,
      212,
      12,
      Date.parse('2026-10-09T14:45:00Z')
    );
    expect(p.backend).toBe('ibm_brisbane');
    expect(p.programHash).toBe('sha256:7f83b1657ff1fc53b');
    expect(p.qubits).toBe(20);
    expect(p.shots).toBe(1024);
    expect(p.costMaxUsd).toBe(48);
    expect(p.budgetRemainingUsd).toBe(200);
    expect(p.costBasis).toContain('2026-10-01');
    expect(p.blastRadius).toContain('no data egress');
    expect(p.reversibility).toContain('irreversible');
    expect(p.evidence.classicalAnchor).toContain('0.47 mHa');
    expect(p.alternatives.simFirstSavings).toContain('$0.00');
    expect(p.expiresAtIso).toBe('2026-10-09T15:00:00.000Z');
    expect(p.noncePreview).toBe('sess-abc');
    expect(p.packetId).toBe('appr-sess-abc');
  });

  it('clamps remaining budget at zero and never invents numbers', () => {
    const p = buildApprovalPacketFromCampaign(null, '', 48, 25, 100);
    expect(p.budgetRemainingUsd).toBe(0);
    expect(p.backend).toBe('local-simulator');
    expect(p.programHash).toBe('uncomputed');
    expect(p.evidence.classicalAnchor).toBeUndefined();
    expect(p.packetId).toBe('appr-local');
  });

  it('expiry is exactly TTL after now', () => {
    const now = Date.parse('2026-10-09T00:00:00Z');
    const p = buildApprovalPacketFromCampaign(CAMPAIGN as never, 's', 1, 10, 0, now);
    expect(Date.parse(p.expiresAtIso) - now).toBe(APPROVAL_TTL_MS);
  });

  it('denial requires a non-blank reason', () => {
    expect(canConfirmDenial('')).toBe(false);
    expect(canConfirmDenial('   ')).toBe(false);
    expect(canConfirmDenial('too pricey')).toBe(true);
  });
});

describe('isMeteredBackend', () => {
  it('treats simulator-class backends as auto-allow (never gated)', async () => {
    const { isMeteredBackend } = await import('./approvalPacket');
    for (const b of ['local_simulator', 'Local Simulator (qiskit-aer-simulator)', 'statevector', 'starter / simulator-first']) {
      expect(isMeteredBackend(b)).toBe(false);
    }
  });

  it('treats hardware and unknown tiers as metered (fail-closed visibility)', async () => {
    const { isMeteredBackend } = await import('./approvalPacket');
    for (const b of ['ibm_brisbane', 'QuEra Aquila', 'mystery-backend-9', '', null, undefined]) {
      expect(isMeteredBackend(b)).toBe(true);
    }
  });
});
