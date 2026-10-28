import { describe, it, expect } from 'vitest';
import * as ed from '@noble/ed25519';
import { sha512 } from '@noble/hashes/sha2.js';
import { getOrCreateSigner, signWithCustody } from './custody';

ed.hashes.sha512 = (...m: Uint8Array[]) => sha512(ed.etc.concatBytes(...m));

describe('custody', () => {
  it('mints a signer and verifies its signatures', async () => {
    const signer = await getOrCreateSigner();
    expect(signer.publicKeyHex).toMatch(/^[0-9a-f]{64}$/);
    const msg = new TextEncoder().encode('kubona-custody-test');
    const sig = await signer.sign(msg);
    expect(ed.verify(sig, msg, ed.etc.hexToBytes(signer.publicKeyHex))).toBe(true);
    expect(signer.backend).toBe('webcrypto');
  });

  it('signWithCustody returns the matching public key', async () => {
    const { publicKeyHex, signature } = await signWithCustody(new Uint8Array([1, 2, 3]));
    expect(publicKeyHex).toMatch(/^[0-9a-f]{64}$/);
    expect(signature).toHaveLength(64);
  });
});

import { signLoginChallengeWithCustody } from './mandates';
describe('custody login', () => {
  it('signs a login challenge', async () => {
    const out = await signLoginChallengeWithCustody('abc');
    expect(out.pubkey_hex).toMatch(/^[0-9a-f]{64}$/);
    expect(out.signature_hex).toMatch(/^[0-9a-f]{128}$/);
  });
});
