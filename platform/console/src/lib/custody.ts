import * as ed from '@noble/ed25519';
import { sha512 } from '@noble/hashes/sha2.js';
import { bytesToHex } from '@noble/hashes/utils.js';
import { openRecovery, sealRecovery, type RecoveryEnvelope } from './recovery';

ed.hashes.sha512 = (...m: Uint8Array[]) => sha512(ed.etc.concatBytes(...m));

/**
 * Browser key custody (Q-56 C5).
 *
 * Production keeps the Ed25519 signing key as a non-extractable WebCrypto
 * CryptoKey in IndexedDB. Scripts cannot read the private bytes; signing is
 * an explicit `crypto.subtle.sign` call. Tests and browsers without
 * WebCrypto Ed25519 fall back to an in-memory noble key ONLY outside
 * production; a production build refuses to mint an extractable key.
 *
 * WebAuthn/passkey attestation is NOT implemented here: this is
 * non-extractable-key custody, the minimum that removes the "key bytes in a
 * JS variable" failure. A passkey ceremony is a separate UX/credential layer.
 */

const DB_NAME = 'kubona-custody-v1';
const STORE = 'keys';
const KEY_ID = 'lab-admin-ed25519';
const BACKUP_PREFIX = 'lab-admin-ed25519-backup-';

export type CustodyBackend = 'webcrypto' | 'noble-memory';

export interface CustodySigner {
  publicKeyHex: string;
  backend: CustodyBackend;
  durable: boolean;
  sign(bytes: Uint8Array): Promise<Uint8Array>;
}

interface StoredKey {
  id: string;
  publicKeyHex: string;
  privateKey: CryptoKey;
}

let cached: CustodySigner | null = null;

function isProduction(): boolean {
  return typeof process !== 'undefined' && process.env?.NODE_ENV === 'production';
}

function cryptoAvailable(): boolean {
  return typeof globalThis.crypto !== 'undefined' && !!globalThis.crypto.subtle;
}

function openDb(): Promise<IDBDatabase | null> {
  if (typeof indexedDB === 'undefined') return Promise.resolve(null);
  return new Promise((resolve) => {
    let req: IDBOpenDBRequest;
    try {
      req = indexedDB.open(DB_NAME, 1);
    } catch {
      resolve(null);
      return;
    }
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(STORE)) db.createObjectStore(STORE, { keyPath: 'id' });
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => resolve(null);
  });
}

async function loadStored(): Promise<StoredKey | null> {
  const db = await openDb();
  if (!db) return null;
  try {
    return await new Promise<StoredKey | null>((resolve) => {
      const tx = db.transaction(STORE, 'readonly');
      const req = tx.objectStore(STORE).get(KEY_ID);
      req.onsuccess = () => {
        const row = req.result as StoredKey | undefined;
        resolve(row && row.privateKey && row.publicKeyHex ? row : null);
      };
      req.onerror = () => resolve(null);
    });
  } finally {
    db.close();
  }
}

async function storeKey(row: StoredKey): Promise<boolean> {
  const db = await openDb();
  if (!db) return false;
  try {
    return await new Promise<boolean>((resolve) => {
      const tx = db.transaction(STORE, 'readwrite');
      tx.objectStore(STORE).put(row);
      tx.oncomplete = () => resolve(true);
      tx.onerror = () => resolve(false);
      tx.onabort = () => resolve(false);
    });
  } finally {
    db.close();
  }
}

async function generateWebCrypto(): Promise<StoredKey> {
  // Generate extractable only long enough to export the public key, then
  // re-import the private key with extractable=false and drop the original.
  const pair = (await globalThis.crypto.subtle.generateKey(
    { name: 'Ed25519' }, true, ['sign', 'verify']
  )) as CryptoKeyPair;
  const rawPub = new Uint8Array(await globalThis.crypto.subtle.exportKey('raw', pair.publicKey));
  const pkcs8 = await globalThis.crypto.subtle.exportKey('pkcs8', pair.privateKey);
  const privateKey = await globalThis.crypto.subtle.importKey(
    'pkcs8', pkcs8, { name: 'Ed25519' }, false, ['sign']
  );
  return { id: KEY_ID, publicKeyHex: bytesToHex(rawPub), privateKey };
}

function webCryptoSkinner(row: StoredKey, durable = true): CustodySigner {
  return {
    publicKeyHex: row.publicKeyHex,
    backend: 'webcrypto',
    durable,
    async sign(bytes: Uint8Array): Promise<Uint8Array> {
      return new Uint8Array(await globalThis.crypto.subtle.sign(
        { name: 'Ed25519' }, row.privateKey, bytes as unknown as BufferSource
      ));
    },
  };
}

function memorySkinner(): CustodySigner {
  if (isProduction()) {
    throw new Error('refused: non-extractable WebCrypto Ed25519 custody is unavailable in production');
  }
  const kp = ed.keygen();
  return {
    publicKeyHex: bytesToHex(kp.publicKey),
    backend: 'noble-memory',
    durable: false,
    async sign(bytes: Uint8Array): Promise<Uint8Array> {
      return ed.sign(bytes, kp.secretKey);
    },
  };
}


export interface RecoverableCustody {
  signer: CustodySigner;
  recovery: RecoveryEnvelope;
  prepare(): Promise<void>;
  activate(): void;
  rollback(): Promise<void>;
}

// Generate a NEW admin key with an offline recovery envelope. The old key is
// copied to a backup slot before the new key is installed, so a failed or
// ambiguous rotation remains recoverable. prepare() must run before the server
// call; activate() after success; rollback() restores the old active key.
export async function createRecoverableSigner(passphrase: string, tenantId: string): Promise<RecoverableCustody> {
  if (!cryptoAvailable()) throw new Error('WebCrypto is required to create a recoverable admin key');
  const old = await loadStored();
  if (!old) throw new Error('no durable admin key to rotate; mint one first');
  const pair = (await globalThis.crypto.subtle.generateKey({ name: 'Ed25519' }, true, ['sign', 'verify'])) as CryptoKeyPair;
  const rawPub = new Uint8Array(await globalThis.crypto.subtle.exportKey('raw', pair.publicKey));
  const publicKeyHex = bytesToHex(rawPub);
  const pkcs8 = await globalThis.crypto.subtle.exportKey('pkcs8', pair.privateKey);
  const recovery = await sealRecovery(pkcs8, passphrase, tenantId, publicKeyHex);
  const privateKey = await globalThis.crypto.subtle.importKey('pkcs8', pkcs8, { name: 'Ed25519' }, false, ['sign']);
  const row: StoredKey = { id: KEY_ID, publicKeyHex, privateKey };
  const oldSigner = webCryptoSkinner(old, true);
  let prepared = false;
  return {
    signer: webCryptoSkinner(row, false),
    recovery,
    async prepare() {
      const backup = { ...old, id: `${BACKUP_PREFIX}${Date.now()}` };
      if (!(await storeKey(backup))) throw new Error('could not back up the old admin key in this browser');
      if (!(await storeKey(row))) throw new Error('could not persist the new admin key in this browser');
      prepared = true;
    },
    activate() {
      if (!prepared) throw new Error('recovery key was not prepared');
      cached = webCryptoSkinner(row, true);
    },
    async rollback() {
      if (!prepared) return;
      if (!(await storeKey(old))) throw new Error('could not restore the old admin key; use the downloaded recovery kit');
      cached = oldSigner;
      prepared = false;
    },
  };
}

// Restore a recovery envelope into this browser. The imported private key is
// non-extractable; the envelope's public key is used as the identity and must
// match the server's registered admin key at login.
export async function installRecoveryEnvelope(envelope: RecoveryEnvelope, passphrase: string): Promise<CustodySigner> {
  if (!cryptoAvailable()) throw new Error('WebCrypto is required to restore an admin key');
  const old = await loadStored();
  if (old) {
    const backup = { ...old, id: `${BACKUP_PREFIX}${Date.now()}` };
    if (!(await storeKey(backup))) throw new Error('could not back up the current admin key before restore');
  }
  const pkcs8 = await openRecovery(envelope, passphrase);
  const privateKey = await globalThis.crypto.subtle.importKey('pkcs8', pkcs8, { name: 'Ed25519' }, false, ['sign']);
  const row: StoredKey = { id: KEY_ID, publicKeyHex: envelope.public_key_hex, privateKey };
  if (!(await storeKey(row))) throw new Error('could not persist the restored admin key in this browser');
  cached = webCryptoSkinner(row, true);
  return cached;
}

/** Load or mint the durable signer. Never returns a production memory key. */
export async function getOrCreateSigner(): Promise<CustodySigner> {
  if (cached) return cached;
  const stored = await loadStored();
  if (stored) {
    cached = webCryptoSkinner(stored);
    return cached;
  }
  if (cryptoAvailable()) {
    try {
      const row = await generateWebCrypto();
      const persisted = await storeKey(row);
      if (!persisted && isProduction()) {
        throw new Error('refused: durable IndexedDB custody is unavailable in production');
      }
      cached = webCryptoSkinner(row, persisted);
      return cached;
    } catch (e) {
      // Fail closed in production; memory only in dev/test.
      if (isProduction()) throw e;
    }
  }
  cached = memorySkinner();
  return cached;
}

/** Public key if a key already exists; never mints one. */
export async function peekSignerPubkeyHex(): Promise<string | null> {
  if (cached) return cached.publicKeyHex;
  const stored = await loadStored();
  if (stored) {
    cached = webCryptoSkinner(stored);
    return cached.publicKeyHex;
  }
  return null;
}

export async function signWithCustody(bytes: Uint8Array): Promise<{ publicKeyHex: string; signature: Uint8Array }> {
  const signer = await getOrCreateSigner();
  return { publicKeyHex: signer.publicKeyHex, signature: await signer.sign(bytes) };
}
