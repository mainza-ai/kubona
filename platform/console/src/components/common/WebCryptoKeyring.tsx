'use client';

import React, { useState, useEffect } from 'react';
import { KeyRound,  Download,   AlertCircle, Copy, Check } from 'lucide-react';
import { ed25519 } from '@noble/curves/ed25519.js';

interface KeyringState {
  hasKey: boolean;
  publicKeyHex: string;
  keyFingerprint: string;
  createdAt: string;
}

// Convert Uint8Array to Hex string
export const toHex = (buf: Uint8Array): string => {
  return Array.from(buf)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('');
};

// Convert Hex string to Uint8Array
export const fromHex = (hex: string): Uint8Array => {
  const bytes = new Uint8Array(hex.length / 2);
  for (let i = 0; i < hex.length; i += 2) {
    bytes[i / 2] = parseInt(hex.substring(i, i + 2), 16);
  }
  return bytes;
};

export function getStoredKeypair(): { pubHex: string; privHex: string } | null {
  if (typeof window === 'undefined') return null;
  const pub = localStorage.getItem('kubona_ed25519_pub');
  const priv = localStorage.getItem('kubona_ed25519_priv');
  if (pub && priv && pub.length === 64 && priv.length === 64) {
    return { pubHex: pub, privHex: priv };
  }
  return null;
}

export function generateAndStoreKeypair(): { pubHex: string; privHex: string; createdAt: string } {
  const priv = ed25519.utils.randomSecretKey();
  const pub = ed25519.getPublicKey(priv);
  const pubHex = toHex(pub);
  const privHex = toHex(priv);
  const createdAt = new Date().toISOString();
  if (typeof window !== 'undefined') {
    localStorage.setItem('kubona_ed25519_pub', pubHex);
    localStorage.setItem('kubona_ed25519_priv', privHex);
    localStorage.setItem('kubona_ed25519_date', createdAt);
  }
  return { pubHex, privHex, createdAt };
}

export function signSpendMandate(
  tenantId: string,
  jobHash: string,
  maxBudgetUSD: number,
  expiryIso: string
): {
  tenant_id: string;
  job_hash: string;
  max_budget_usd: number;
  expiry_timestamp: string;
  public_key_hex: string;
  signature_hex: string;
} {
  let kp = getStoredKeypair();
  if (!kp) {
    kp = generateAndStoreKeypair();
  }
  const cleanExpiry = expiryIso.replace(/\.\d{3}Z$/, 'Z');
  const canonical = `${tenantId}:${jobHash}:${maxBudgetUSD.toFixed(4)}:${cleanExpiry}`;
  const privBytes = fromHex(kp.privHex);
  const msgBytes = new TextEncoder().encode(canonical);
  const sigBytes = ed25519.sign(msgBytes, privBytes);
  const sigHex = toHex(sigBytes);

  return {
    tenant_id: tenantId,
    job_hash: jobHash,
    max_budget_usd: maxBudgetUSD,
    expiry_timestamp: cleanExpiry,
    public_key_hex: kp.pubHex,
    signature_hex: sigHex,
  };
}

export function WebCryptoKeyring() {
  const [keyring, setKeyring] = useState<KeyringState>({
    hasKey: false,
    publicKeyHex: '',
    keyFingerprint: '',
    createdAt: '',
  });
  const [copied, setCopied] = useState(false);
  const [statusMsg, setStatusMsg] = useState('');

  // Load existing key from localStorage
  useEffect(() => {
    const kp = getStoredKeypair();
    const storedDate = localStorage.getItem('kubona_ed25519_date');
    if (kp) {
      const fp = kp.pubHex.slice(0, 8) + '...' + kp.pubHex.slice(-8);
      const timer = setTimeout(() => {
        setKeyring({
          hasKey: true,
          publicKeyHex: kp.pubHex,
          keyFingerprint: fp,
          createdAt: storedDate || 'Active Session',
        });
      }, 0);
      return () => clearTimeout(timer);
    }
  }, []);

  const handleGenerate = () => {
    try {
      setStatusMsg('Deriving RFC 8032 Ed25519 keypair in browser memory...');
      const { pubHex, createdAt } = generateAndStoreKeypair();
      const fp = pubHex.slice(0, 8) + '...' + pubHex.slice(-8);
      setKeyring({
        hasKey: true,
        publicKeyHex: pubHex,
        keyFingerprint: fp,
        createdAt,
      });
      setStatusMsg('✅ Genuine RFC 8032 Ed25519 Keypair generated & stored in browser memory.');
    } catch (err: unknown) {
      setStatusMsg(`❌ Key generation error: ${err instanceof Error ? err.message : String(err)}`);
    }
  };

  const copyPublicKey = () => {
    navigator.clipboard.writeText(keyring.publicKeyHex);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const exportKeyBackup = () => {
    const data = JSON.stringify({
      public_key_hex: keyring.publicKeyHex,
      created_at: keyring.createdAt,
      client: 'Kubona WebCrypto AP2 Keyring v1.0 (RFC 8032 Ed25519)',
    }, null, 2);
    const blob = new Blob([data], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `kubona-ap2-key-${keyring.keyFingerprint.replace('...', '_')}.json`;
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <div className="bg-[#0A0D14]/90 border border-slate-800 rounded-xl p-5 space-y-4 shadow-xl">
      <div className="flex items-center justify-between border-b border-slate-800 pb-3">
        <div className="flex items-center space-x-2">
          <KeyRound className="w-5 h-5 text-amber-400" />
          <h3 className="text-sm font-bold text-white tracking-wide">
            Client-Side Browser WebCrypto AP2 Keyring (RFC 8032 Ed25519)
          </h3>
        </div>
        <span className="text-[10px] font-mono px-2 py-0.5 rounded bg-emerald-950 text-emerald-300 border border-emerald-800">
          ZERO SERVER EXPOSURE
        </span>
      </div>

      <p className="text-xs text-slate-300 leading-relaxed">
        AP2 spend mandates are signed <strong>directly inside this browser window</strong> using RFC 8032 Ed25519.
        Your private key never leaves local storage or travels across the network to the server.
      </p>

      {keyring.hasKey ? (
        <div className="space-y-3 bg-[#05070B] p-4 rounded-lg border border-slate-800">
          <div className="flex items-center justify-between">
            <span className="text-xs text-slate-200 font-medium">Public Key Fingerprint:</span>
            <div className="flex items-center space-x-2">
              <span className="text-xs font-mono font-bold text-cyan-300 bg-cyan-950/40 px-2 py-0.5 rounded border border-cyan-800/40">
                {keyring.keyFingerprint}
              </span>
              <button
                onClick={copyPublicKey}
                className="p-1 text-slate-300 hover:text-white transition-colors"
                title="Copy Full Public Key"
              >
                {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
              </button>
            </div>
          </div>

          <div className="text-xs font-mono text-slate-300 truncate">
            Full Hex: {keyring.publicKeyHex}
          </div>

          <div className="flex items-center justify-between pt-2 border-t border-slate-800/80">
            <span className="text-[11px] text-slate-300 font-mono font-medium">
              Created: {new Date(keyring.createdAt).toLocaleDateString()}
            </span>
            <div className="flex space-x-2">
              <button
                onClick={exportKeyBackup}
                className="px-2.5 py-1 text-[11px] font-medium bg-slate-800 hover:bg-slate-700 text-slate-200 rounded flex items-center space-x-1 transition-colors"
              >
                <Download className="w-3 h-3" />
                <span>Backup JSON</span>
              </button>
              <button
                onClick={handleGenerate}
                className="px-2.5 py-1 text-[11px] font-medium bg-indigo-600 hover:bg-indigo-700 text-white rounded transition-colors"
              >
                Rotate Key
              </button>
            </div>
          </div>
        </div>
      ) : (
        <div className="bg-[#05070B] p-4 rounded-lg border border-slate-800 text-center space-y-3">
          <AlertCircle className="w-6 h-6 text-amber-400 mx-auto" />
          <div className="text-xs text-slate-300">
            No active Ed25519 keypair found in this browser.
          </div>
          <button
            onClick={handleGenerate}
            className="px-4 py-2 bg-indigo-600 hover:bg-indigo-700 text-white text-xs font-bold rounded-lg shadow transition-colors"
          >
            Generate Ed25519 WebCrypto Keypair
          </button>
        </div>
      )}

      {statusMsg && (
        <div className="text-[11px] font-mono text-slate-300 p-2 bg-slate-900 rounded border border-slate-800">
          {statusMsg}
        </div>
      )}
    </div>
  );
}
