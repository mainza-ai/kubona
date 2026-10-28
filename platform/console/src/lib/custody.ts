import * as ed from '@noble/ed25519';
import { sha512 } from '@noble/hashes/sha2.js';
import { bytesToHex } from '@noble/hashes/utils.js';

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
