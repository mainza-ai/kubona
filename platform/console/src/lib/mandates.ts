import * as ed from '@noble/ed25519';
import { sha512, sha256 } from '@noble/hashes/sha2.js';
import { bytesToHex } from '@noble/hashes/utils.js';

// Configure synchronous sha512 for noble-ed25519 v3
ed.hashes.sha512 = (...m: Uint8Array[]) => sha512(ed.etc.concatBytes(...m));

export interface SpendMandate {
  tenant_id: string;
  job_hash: string;
  max_budget_usd: number;
  expiry_timestamp: string; // ISO 8601 UTC string (RFC 3339)
  public_key_hex: string;
  signature_hex: string;
}

export interface SpendMandateParams {
  tenantId?: string;
  jobHash?: string;
  maxBudgetUsd: number;
  ttlSeconds?: number;
}

let sessionKeyPair: { secretKey: Uint8Array; publicKey: Uint8Array } | null = null;

export function getOrCreateSessionKeyPair(): { secretKey: Uint8Array; publicKey: Uint8Array } {
  if (!sessionKeyPair) {
    sessionKeyPair = ed.keygen();
  }
  return sessionKeyPair;
}

/**
 * Computes the deterministic canonical payload string for a spend mandate matching Go's SpendMandate.CanonicalPayload():
 * fmt.Sprintf("%s:%s:%.4f:%s", m.TenantID, m.JobHash, m.MaxBudgetUSD, m.ExpiryTimestamp.UTC().Format(time.RFC3339))
 */
export function computeCanonicalPayload(
  tenantId: string,
  jobHash: string,
  maxBudgetUsd: number,
  expiryTimestampIso: string
): Uint8Array {
  // Format budget to 4 decimal places matching Go's %.4f
  const budgetStr = maxBudgetUsd.toFixed(4);
  const raw = `${tenantId}:${jobHash}:${budgetStr}:${expiryTimestampIso}`;
  return new TextEncoder().encode(raw);
}

/**
 * Canonical v1 job binding (W0.4): hex(sha256("kubona-job-v1\n" + jobType +
 * "\n" + sessionId + "\n" + qasm)). Must match Go's
 * mandates.CanonicalJobBinding byte-for-byte — pinned by the fixed vector
 * test on both sides. The daemon recomputes this from the submitted job at
 * admission and refuses a receipt bound to any other program.
 */
export function canonicalJobBinding(sessionId: string, qasm: string, jobType = 'circuit_run'): string {
  return bytesToHex(sha256(new TextEncoder().encode(`kubona-job-v1\n${jobType}\n${sessionId}\n${qasm}`)));
}

/**
 * Creates and signs a real Ed25519 SpendMandate envelope.
 */
export function createSignedSpendMandate(params: SpendMandateParams): SpendMandate {
  const tenantId = params.tenantId || 'tenant-default';
  const jobHash = params.jobHash || 'job-auth-' + Date.now().toString(16);
  const maxBudgetUsd = params.maxBudgetUsd;
  const ttlSeconds = params.ttlSeconds || 3600;

  const expirySeconds = Math.floor(Date.now() / 1000) + ttlSeconds;
  const expiry = new Date(expirySeconds * 1000);
  const expiryIso = expiry.toISOString().replace(/\.\d{3}Z$/, 'Z');

  const keyPair = getOrCreateSessionKeyPair();
  const payloadBytes = computeCanonicalPayload(tenantId, jobHash, maxBudgetUsd, expiryIso);

  const sigBytes = ed.sign(payloadBytes, keyPair.secretKey);

  return {
    tenant_id: tenantId,
    job_hash: jobHash,
    max_budget_usd: maxBudgetUsd,
    expiry_timestamp: expiryIso,
    public_key_hex: bytesToHex(keyPair.publicKey),
    signature_hex: bytesToHex(sigBytes),
  };
}

/**
 * Verifies that a signed spend mandate is authentic against its public key.
 */
export function verifySpendMandateLocally(mandate: SpendMandate): boolean {
  try {
    const payloadBytes = computeCanonicalPayload(
      mandate.tenant_id,
      mandate.job_hash,
      mandate.max_budget_usd,
      mandate.expiry_timestamp
    );
    const pubKeyBytes = ed.etc.hexToBytes(mandate.public_key_hex);
    const sigBytes = ed.etc.hexToBytes(mandate.signature_hex);
    return ed.verify(sigBytes, payloadBytes, pubKeyBytes);
  } catch {
    return false;
  }
}
