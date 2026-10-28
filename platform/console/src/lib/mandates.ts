import * as ed from '@noble/ed25519';
import { sha512, sha256 } from '@noble/hashes/sha2.js';
import { bytesToHex } from '@noble/hashes/utils.js';
import { signWithCustody } from './custody';

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

// Key custody (Q-56 C5, 2026-10-28): production paths use lib/custody.ts —
// a non-extractable WebCrypto Ed25519 key persisted in IndexedDB. The
// synchronous keypair helpers below are the explicit-keypair/legacy path
// (tests and deliberate injection); production surfaces must call the
// *WithCustody variants instead.

// Signs an auth challenge for /api/auth/login. The exact bytes the server
// verifies: "kubona-auth/v1 login " + nonce (must match the daemon's
// loginChallengeMessage — never add fields here without updating both).
/** Production path: non-extractable WebCrypto custody (Q-56 C5). */
export async function signLoginChallengeWithCustody(
  nonce: string
): Promise<{ pubkey_hex: string; signature_hex: string }> {
  const msg = new TextEncoder().encode(`kubona-auth/v1 login ${nonce}`);
  const { publicKeyHex, signature } = await signWithCustody(msg);
  return { pubkey_hex: publicKeyHex, signature_hex: bytesToHex(signature) };
}

export function signLoginChallenge(
  nonce: string,
  keyPair: { secretKey: Uint8Array; publicKey: Uint8Array }
): { pubkey_hex: string; signature_hex: string } {
  const encoder = new TextEncoder();
  const sig = ed.sign(encoder.encode(`kubona-auth/v1 login ${nonce}`), keyPair.secretKey);
  return { pubkey_hex: bytesToHex(keyPair.publicKey), signature_hex: bytesToHex(sig) };
}

export function getOrCreateSessionKeyPair(): { secretKey: Uint8Array; publicKey: Uint8Array } {
  if (!sessionKeyPair) {
    sessionKeyPair = ed.keygen();
  }
  return sessionKeyPair;
}

// Peeks the browser keypair's public key WITHOUT generating one (unlike
// getOrCreateSessionKeyPair). Tenant-write surfaces use this to decide
// whether the operator holds a key at all — key creation stays an explicit
// keyring action, never a side effect of rendering a lab row.
export function peekSessionPubkeyHex(): string | null {
  if (!sessionKeyPair) return null;
  return bytesToHex(sessionKeyPair.publicKey);
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
 * Fail-closed: missing tenant/job throws (no fail-open defaults), and a
 * non-finite or non-positive budget throws — callers must refuse, never mint.
 */
export async function createSignedSpendMandateWithCustody(params: SpendMandateParams): Promise<SpendMandate> {
  const tenantId = (params.tenantId ?? '').trim();
  const jobHash = (params.jobHash ?? '').trim();
  const maxBudgetUsd = params.maxBudgetUsd;
  if (!tenantId) throw new Error('refused: tenant_id is required (no fail-open default)');
  if (!jobHash) throw new Error('refused: job_hash is required (no fail-open default)');
  if (typeof maxBudgetUsd !== 'number' || !Number.isFinite(maxBudgetUsd) || maxBudgetUsd <= 0) {
    throw new Error('refused: maxBudgetUsd must be a finite number > 0');
  }
  const ttlSeconds = params.ttlSeconds || 3600;
  const expirySeconds = Math.floor(Date.now() / 1000) + ttlSeconds;
  const expiry = new Date(expirySeconds * 1000);
  const expiryIso = expiry.toISOString().replace(/\.\d{3}Z$/, 'Z');
  const payloadBytes = computeCanonicalPayload(tenantId, jobHash, maxBudgetUsd, expiryIso);
  const { publicKeyHex, signature } = await signWithCustody(payloadBytes);
  return {
    tenant_id: tenantId,
    job_hash: jobHash,
    max_budget_usd: maxBudgetUsd,
    expiry_timestamp: expiryIso,
    public_key_hex: publicKeyHex,
    signature_hex: bytesToHex(signature),
  };
}

export function createSignedSpendMandate(params: SpendMandateParams): SpendMandate {
  const tenantId = (params.tenantId ?? '').trim();
  const jobHash = (params.jobHash ?? '').trim();
  const maxBudgetUsd = params.maxBudgetUsd;
  if (!tenantId) throw new Error('refused: tenant_id is required (no fail-open default)');
  if (!jobHash) throw new Error('refused: job_hash is required (no fail-open default)');
  if (typeof maxBudgetUsd !== 'number' || !Number.isFinite(maxBudgetUsd) || maxBudgetUsd <= 0) {
    throw new Error('refused: maxBudgetUsd must be a finite number > 0');
  }
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
