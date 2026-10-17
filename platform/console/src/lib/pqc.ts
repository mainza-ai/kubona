import { ml_kem768 } from '@noble/post-quantum/ml-kem.js';
import { sha256 } from '@noble/hashes/sha2.js';
import { bytesToHex, hexToBytes } from '@noble/hashes/utils.js';

export interface HybridSpendMandatePayload {
  recipient_fingerprint: string;
  encapsulated_key_hex: string;
  nonce_hex: string;
  encrypted_payload_hex: string;
}

/**
 * Hybrid PQC — STAGED, NOT WIRED (2026-10-17).
 * No surface calls this module: AboutLabModal no longer claims ML-KEM+X25519
 * as shipped posture, and no mandate flow encapsulates. Kept for the
 * custody/transport design review, guarded fail-closed. Wiring requires a
 * recipient-key registry + server decapsulator that do not exist yet.
 */
export async function encapsulateSpendMandate(
  recipientEncapsulationKeyHex: string,
  mandatePayloadJson: string
): Promise<HybridSpendMandatePayload> {
  if (typeof window === 'undefined' || !window.crypto?.subtle || !window.crypto?.getRandomValues) {
    throw new Error('refused: WebCrypto unavailable — PQC encapsulation requires a secure browser context');
  }
  if (!recipientEncapsulationKeyHex || !/^[0-9a-fA-F]+$/.test(recipientEncapsulationKeyHex)) {
    throw new Error('refused: recipient encapsulation key must be non-empty hex');
  }
  if (!mandatePayloadJson) throw new Error('refused: mandate payload is required');
  let ekBytes: Uint8Array;
  try {
    ekBytes = hexToBytes(recipientEncapsulationKeyHex);
  } catch {
    throw new Error('refused: recipient encapsulation key is not valid hex');
  }
  if (ekBytes.length === 0) throw new Error('refused: recipient encapsulation key decodes to empty');
  const fingerprint = bytesToHex(sha256(ekBytes));
  let enc: { sharedSecret: Uint8Array; cipherText: Uint8Array };
  try {
    enc = ml_kem768.encapsulate(ekBytes);
  } catch (e) {
    throw new Error(`refused: ML-KEM encapsulation failed (${e instanceof Error ? e.message : String(e)})`);
  }
  const { sharedSecret, cipherText } = enc;
  let cryptoKey: CryptoKey;
  try {
    cryptoKey = await window.crypto.subtle.importKey(
      'raw',
      sharedSecret as unknown as ArrayBuffer,
      { name: 'AES-GCM' },
      false,
      ['encrypt']
    );
  } catch (e) {
    throw new Error(`refused: AES-GCM key import failed (${e instanceof Error ? e.message : String(e)})`);
  }
  const nonce = window.crypto.getRandomValues(new Uint8Array(12));
  const encodedPayload = new TextEncoder().encode(mandatePayloadJson);
  let encryptedBuffer: ArrayBuffer;
  try {
    encryptedBuffer = await window.crypto.subtle.encrypt(
      { name: 'AES-GCM', iv: nonce as unknown as ArrayBuffer },
      cryptoKey,
      encodedPayload as unknown as ArrayBuffer
    );
  } catch (e) {
    throw new Error(`refused: AES-GCM encrypt failed (${e instanceof Error ? e.message : String(e)})`);
  }
  return {
    recipient_fingerprint: fingerprint,
    encapsulated_key_hex: bytesToHex(cipherText),
    nonce_hex: bytesToHex(nonce),
    encrypted_payload_hex: bytesToHex(new Uint8Array(encryptedBuffer)),
  };
}

/**
 * Decapsulates a hybrid mandate using recipient's ML-KEM-768 decapsulation key.
 * Staged, not wired — same fail-closed guards as encapsulate.
 */
export async function decapsulateSpendMandate(
  recipientDecapsulationKeyHex: string,
  hybrid: HybridSpendMandatePayload
): Promise<string> {
  if (typeof window === 'undefined' || !window.crypto?.subtle) {
    throw new Error('refused: WebCrypto unavailable — PQC decapsulation requires a secure browser context');
  }
  if (!recipientDecapsulationKeyHex || !/^[0-9a-fA-F]+$/.test(recipientDecapsulationKeyHex)) {
    throw new Error('refused: recipient decapsulation key must be non-empty hex');
  }
  if (!hybrid?.encapsulated_key_hex || !hybrid?.nonce_hex || !hybrid?.encrypted_payload_hex) {
    throw new Error('refused: hybrid payload is missing encapsulated fields');
  }
  let dkBytes: Uint8Array;
  let cipherText: Uint8Array;
  let nonce: Uint8Array;
  let encryptedBytes: Uint8Array;
  try {
    dkBytes = hexToBytes(recipientDecapsulationKeyHex);
    cipherText = hexToBytes(hybrid.encapsulated_key_hex);
    nonce = hexToBytes(hybrid.nonce_hex);
    encryptedBytes = hexToBytes(hybrid.encrypted_payload_hex);
  } catch {
    throw new Error('refused: hybrid payload fields are not valid hex');
  }
  if (dkBytes.length === 0) throw new Error('refused: recipient decapsulation key decodes to empty');
  let sharedSecret: Uint8Array;
  try {
    sharedSecret = ml_kem768.decapsulate(cipherText, dkBytes);
  } catch (e) {
    throw new Error(`refused: ML-KEM decapsulation failed (${e instanceof Error ? e.message : String(e)})`);
  }
  let cryptoKey: CryptoKey;
  try {
    cryptoKey = await window.crypto.subtle.importKey(
      'raw',
      sharedSecret as unknown as ArrayBuffer,
      { name: 'AES-GCM' },
      false,
      ['decrypt']
    );
  } catch (e) {
    throw new Error(`refused: AES-GCM key import failed (${e instanceof Error ? e.message : String(e)})`);
  }
  let decryptedBuffer: ArrayBuffer;
  try {
    decryptedBuffer = await window.crypto.subtle.decrypt(
      { name: 'AES-GCM', iv: nonce as unknown as ArrayBuffer },
      cryptoKey,
      encryptedBytes as unknown as ArrayBuffer
    );
  } catch (e) {
    throw new Error(`refused: AES-GCM decrypt failed (${e instanceof Error ? e.message : String(e)})`);
  }
  return new TextDecoder().decode(decryptedBuffer);
}
