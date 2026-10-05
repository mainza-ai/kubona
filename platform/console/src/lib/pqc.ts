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
 * Encapsulates a signed AP2 SpendMandate using recipient's ML-KEM-768 encapsulation key.
 * Implements NIST FIPS 203 hybrid key encapsulation with AES-256-GCM authenticated payload encryption.
 */
export async function encapsulateSpendMandate(
  recipientEncapsulationKeyHex: string,
  mandatePayloadJson: string
): Promise<HybridSpendMandatePayload> {
  const ekBytes = hexToBytes(recipientEncapsulationKeyHex);
  
  // 1. Calculate recipient key fingerprint
  const fingerprint = bytesToHex(sha256(ekBytes));

  // 2. Encapsulate shared secret via ML-KEM-768
  const { sharedSecret, cipherText } = ml_kem768.encapsulate(ekBytes);

  // 3. Encrypt mandate payload with AES-256-GCM using WebCrypto
  const cryptoKey = await window.crypto.subtle.importKey(
    'raw',
    sharedSecret as Uint8Array<ArrayBuffer>,
    { name: 'AES-GCM' },
    false,
    ['encrypt']
  );

  const nonce = window.crypto.getRandomValues(new Uint8Array(12));
  const encodedPayload = new TextEncoder().encode(mandatePayloadJson);

  const encryptedBuffer = await window.crypto.subtle.encrypt(
    { name: 'AES-GCM', iv: nonce },
    cryptoKey,
    encodedPayload as Uint8Array<ArrayBuffer>
  );

  return {
    recipient_fingerprint: fingerprint,
    encapsulated_key_hex: bytesToHex(cipherText),
    nonce_hex: bytesToHex(nonce),
    encrypted_payload_hex: bytesToHex(new Uint8Array(encryptedBuffer)),
  };
}

/**
 * Decapsulates a hybrid mandate using recipient's ML-KEM-768 decapsulation key.
 */
export async function decapsulateSpendMandate(
  recipientDecapsulationKeyHex: string,
  hybrid: HybridSpendMandatePayload
): Promise<string> {
  const dkBytes = hexToBytes(recipientDecapsulationKeyHex);
  const cipherText = hexToBytes(hybrid.encapsulated_key_hex);

  // 1. Decapsulate shared secret
  const sharedSecret = ml_kem768.decapsulate(cipherText, dkBytes);

  // 2. Decrypt AES-GCM payload
  const cryptoKey = await window.crypto.subtle.importKey(
    'raw',
    sharedSecret as Uint8Array<ArrayBuffer>,
    { name: 'AES-GCM' },
    false,
    ['decrypt']
  );

  const nonce = hexToBytes(hybrid.nonce_hex);
  const encryptedBytes = hexToBytes(hybrid.encrypted_payload_hex);

  const decryptedBuffer = await window.crypto.subtle.decrypt(
    { name: 'AES-GCM', iv: nonce },
    cryptoKey,
    encryptedBytes as Uint8Array<ArrayBuffer>
  );

  return new TextDecoder().decode(decryptedBuffer);
}
