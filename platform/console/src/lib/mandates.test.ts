import { describe, it, expect } from 'vitest';
import {
  computeCanonicalPayload,
  canonicalJobBinding,
  createSignedSpendMandate,
  verifySpendMandateLocally,
  signLoginChallenge,
  type SpendMandate,
} from './mandates';
import * as ed from '@noble/ed25519';

// Cross-language fixture: produced once with @noble/ed25519 over the exact
// canonical string below. The mirror test in Go (mandates package,
// TestCrossLanguageMandateVector) verifies the same public key and signature
// bytes with crypto/ed25519 — together they prove the browser signer and the
// Go verifier agree byte-for-byte. Ed25519 is deterministic, so this vector
// is stable.
const VECTOR = {
  tenantId: 'tenant-vector',
  jobHash: 'jobhash-vector',
  maxBudgetUsd: 25.5,
  expiryIso: '2030-01-01T00:00:00Z',
  publicKeyHex: 'c99aff667c7578056bd7d4590df92bf2ae108a6a7f33dd52384a83bf325f0269',
  signatureHex:
    'ea6a9a67b22c82be13f918e22824d1fd7d8f4f6e70a51f14f19112782dbc9e6e2075ba41c3c141955f24278e1cb6b8e0f643fdd0ddc0bc818412e7ed5d97570e',
};

describe('mandate canonical payload (must match Go %.4f + RFC3339)', () => {
  it('formats the exact canonical string Go signs', () => {
    const bytes = computeCanonicalPayload(
      VECTOR.tenantId,
      VECTOR.jobHash,
      VECTOR.maxBudgetUsd,
      VECTOR.expiryIso
    );
    expect(new TextDecoder().decode(bytes)).toBe(
      'tenant-vector:jobhash-vector:25.5000:2030-01-01T00:00:00Z'
    );
  });

  it('verifies the fixed cross-language fixture signature', () => {
    const mandate: SpendMandate = {
      tenant_id: VECTOR.tenantId,
      job_hash: VECTOR.jobHash,
      max_budget_usd: VECTOR.maxBudgetUsd,
      expiry_timestamp: VECTOR.expiryIso,
      public_key_hex: VECTOR.publicKeyHex,
      signature_hex: VECTOR.signatureHex,
    };
    expect(verifySpendMandateLocally(mandate)).toBe(true);
  });

  it('rejects a tampered budget without re-signing', () => {
    const mandate: SpendMandate = {
      tenant_id: VECTOR.tenantId,
      job_hash: VECTOR.jobHash,
      max_budget_usd: 999.0,
      expiry_timestamp: VECTOR.expiryIso,
      public_key_hex: VECTOR.publicKeyHex,
      signature_hex: VECTOR.signatureHex,
    };
    expect(verifySpendMandateLocally(mandate)).toBe(false);
  });

  it('round-trips a freshly signed mandate', () => {
    const mandate = createSignedSpendMandate({ tenantId: 't', jobHash: 'j', maxBudgetUsd: 10 });
    expect(verifySpendMandateLocally(mandate)).toBe(true);
    expect(mandate.expiry_timestamp).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$/);
  });
});

describe('canonical job binding v1 (must match Go CanonicalJobBinding)', () => {
  const QASM = 'OPENQASM 2.0;\nqreg q[1];\n';
  it('computes the fixed cross-language vector', () => {
    expect(canonicalJobBinding('sess-vec', QASM)).toBe(
      '67cc6d01173411aecf553a038cc9873a15779416dafd58a30e3c15a98c63bbfb'
    );
  });
  it('rebinds on session, program, or type change', () => {
    const base = canonicalJobBinding('sess-vec', QASM);
    expect(canonicalJobBinding('sess-other', QASM)).not.toBe(base);
    expect(canonicalJobBinding('sess-vec', QASM + 'h q[0];\n')).not.toBe(base);
    expect(canonicalJobBinding('sess-vec', QASM, 'mitigate')).not.toBe(base);
  });
});

describe('signLoginChallenge (A4 Ed25519 login challenge)', () => {
  it('signs kubona-auth/v1 login <nonce> deterministically with Ed25519', () => {
    const secretKey = ed.etc.hexToBytes('01'.repeat(32));
    const publicKey = ed.getPublicKey(secretKey);
    const nonce = 'deadbeef0123456789abcdef01234567';
    const { pubkey_hex, signature_hex } = signLoginChallenge(nonce, { secretKey, publicKey });
    expect(pubkey_hex).toBe(ed.etc.bytesToHex(publicKey));
    const msg = new TextEncoder().encode(`kubona-auth/v1 login ${nonce}`);
    expect(
      ed.verify(ed.etc.hexToBytes(signature_hex), msg, publicKey)
    ).toBe(true);
    // Domain separation: does not verify under a different nonce or prefix
    const wrongMsg = new TextEncoder().encode(`kubona-tenant-admin/v1 ${nonce}`);
    expect(
      ed.verify(ed.etc.hexToBytes(signature_hex), wrongMsg, publicKey)
    ).toBe(false);
  });
});
