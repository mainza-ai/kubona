import { describe, it, expect } from 'vitest';
import { openRecovery, sealRecovery, validateRecoveryEnvelope } from './recovery';

describe('recovery envelope', () => {
  it('round-trips PKCS#8 and refuses a wrong passphrase / malformed envelope', async () => {
    const pair = (await crypto.subtle.generateKey({ name: 'Ed25519' }, true, ['sign', 'verify'])) as CryptoKeyPair;
    const pkcs8 = await crypto.subtle.exportKey('pkcs8', pair.privateKey);
    const env = await sealRecovery(pkcs8, 'correct horse battery staple', 'lab-a', 'ab'.repeat(32));
    const opened = await openRecovery(env, 'correct horse battery staple');
    expect(new Uint8Array(opened)).toEqual(new Uint8Array(pkcs8));
    await expect(openRecovery(env, 'wrong passphrase')).rejects.toThrow(/invalid/);
    expect(() => validateRecoveryEnvelope({ ...env, version: 2 })).toThrow();
  });
});
