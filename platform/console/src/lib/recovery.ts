// Offline recovery kit for the lab-admin Ed25519 key. The browser's live
// key stays non-extractable; a new key is generated extractable only long
// enough to encrypt PKCS#8 with a passphrase, then re-imported non-extractable.
// PBKDF2-SHA256 + AES-256-GCM; AAD binds tenant + public key.
const VERSION = 1 as const;
const ITERATIONS = 600_000;
const SALT_BYTES = 16;
const IV_BYTES = 12;

export interface RecoveryEnvelope {
  version: typeof VERSION;
  tenant_id: string;
  public_key_hex: string;
  kdf: 'PBKDF2-SHA256';
  iterations: number;
  salt_b64: string;
  iv_b64: string;
  ciphertext_b64: string;
  created_at: string;
}

function b64(bytes: Uint8Array): string {
  let s = '';
  for (const b of bytes) s += String.fromCharCode(b);
  return btoa(s);
}
function unb64(value: string): Uint8Array {
  const s = atob(value);
  const out = new Uint8Array(s.length);
  for (let i = 0; i < s.length; i++) out[i] = s.charCodeAt(i);
  return out;
}
function aad(tenantId: string, publicKeyHex: string): Uint8Array {
  return new TextEncoder().encode(`kubona-recovery-v1|${tenantId}|${publicKeyHex}`);
}
async function derive(passphrase: string, salt: Uint8Array, iterations: number): Promise<CryptoKey> {
  const material = await crypto.subtle.importKey('raw', new TextEncoder().encode(passphrase), 'PBKDF2', false, ['deriveKey']);
  return crypto.subtle.deriveKey(
    { name: 'PBKDF2', salt: salt as unknown as BufferSource, iterations, hash: 'SHA-256' },
    material,
    { name: 'AES-GCM', length: 256 },
    false,
    ['encrypt', 'decrypt'],
  );
}

export function validateRecoveryEnvelope(value: unknown): RecoveryEnvelope {
  const v = value as Partial<RecoveryEnvelope> | null;
  if (!v || v.version !== VERSION || typeof v.tenant_id !== 'string' || typeof v.public_key_hex !== 'string'
      || v.kdf !== 'PBKDF2-SHA256' || typeof v.iterations !== 'number' || v.iterations < 100_000
      || typeof v.salt_b64 !== 'string' || typeof v.iv_b64 !== 'string' || typeof v.ciphertext_b64 !== 'string'
      || typeof v.created_at !== 'string') {
    throw new Error('invalid recovery envelope: refusing');
  }
  return v as RecoveryEnvelope;
}

export async function sealRecovery(pkcs8: ArrayBuffer, passphrase: string, tenantId: string, publicKeyHex: string): Promise<RecoveryEnvelope> {
  if (passphrase.length < 12) throw new Error('recovery passphrase must be at least 12 characters');
  const salt = crypto.getRandomValues(new Uint8Array(SALT_BYTES));
  const iv = crypto.getRandomValues(new Uint8Array(IV_BYTES));
  const key = await derive(passphrase, salt, ITERATIONS);
  const ciphertext = new Uint8Array(await crypto.subtle.encrypt(
    { name: 'AES-GCM', iv: iv as unknown as BufferSource, additionalData: aad(tenantId, publicKeyHex) as unknown as BufferSource },
    key,
    pkcs8,
  ));
  return {
    version: VERSION, tenant_id: tenantId, public_key_hex: publicKeyHex,
    kdf: 'PBKDF2-SHA256', iterations: ITERATIONS,
    salt_b64: b64(salt), iv_b64: b64(iv), ciphertext_b64: b64(ciphertext),
    created_at: new Date().toISOString(),
  };
}

export async function openRecovery(envelope: RecoveryEnvelope, passphrase: string): Promise<ArrayBuffer> {
  const env = validateRecoveryEnvelope(envelope);
  const salt = unb64(env.salt_b64);
  const iv = unb64(env.iv_b64);
  const key = await derive(passphrase, salt, env.iterations);
  try {
    return await crypto.subtle.decrypt(
      { name: 'AES-GCM', iv: iv as unknown as BufferSource, additionalData: aad(env.tenant_id, env.public_key_hex) as unknown as BufferSource },
      key,
      unb64(env.ciphertext_b64) as unknown as BufferSource,
    );
  } catch {
    throw new Error('recovery passphrase or file is invalid: refusing');
  }
}

export function recoveryFilename(tenantId: string, publicKeyHex: string): string {
  return `kubona-recovery-${tenantId}-${publicKeyHex.slice(0, 12)}.json`;
}
