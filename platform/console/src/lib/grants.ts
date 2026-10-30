import { bytesToHex } from '@noble/hashes/utils.js';
import { signWithCustody } from './custody';

// Canonical bytes mirror platform/core/pkg/tenants/grants.go exactly.
// Scopes are sorted; expiry is UTC seconds precision.
export function grantCanonical(sourceTenant: string, targetTenant: string, scopes: string[], expiryIso: string): Uint8Array {
  const sorted = [...scopes].sort();
  const expiry = new Date(expiryIso).toISOString().replace(/\.\d{3}Z$/, 'Z');
  return new TextEncoder().encode(`grant|${sourceTenant}|${targetTenant}|${sorted.join(',')}|${expiry}`);
}

export function revokeCanonical(grantId: string, reason: string): Uint8Array {
  return new TextEncoder().encode(`grant-revoke|${grantId}|${reason}`);
}

export async function signGrant(sourceTenant: string, targetTenant: string, scopes: string[], expiryIso: string) {
  const { publicKeyHex, signature } = await signWithCustody(grantCanonical(sourceTenant, targetTenant, scopes, expiryIso));
  return { grantor_pubkey_hex: publicKeyHex, signature_hex: bytesToHex(signature) };
}

export async function signGrantRevoke(grantId: string, reason: string) {
  const { publicKeyHex, signature } = await signWithCustody(revokeCanonical(grantId, reason));
  return { grantor_pubkey_hex: publicKeyHex, signature_hex: bytesToHex(signature) };
}
