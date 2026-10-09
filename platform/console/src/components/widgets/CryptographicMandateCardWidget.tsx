'use client';

import React, { useState } from 'react';
import { KeyRound,  CheckCircle2, XCircle, ArrowDownCircle } from 'lucide-react';
import { createSignedSpendMandate } from '@/lib/mandates';

interface CryptographicMandateCardWidgetProps {
  actionDescription?: string;
  estimatedCostUsd?: number;
  targetBackend?: string;
  onAuthorize?: (signature: string) => void;
  onDowngrade?: () => void;
  onAbort?: () => void;
}

export function CryptographicMandateCardWidget({
  actionDescription = 'Dispatch FeMoco Quantum Krylov to QuEra Neutral Atom (48 Logical Qubits)',
  estimatedCostUsd = 48.50,
  targetBackend = 'QuEra Reconfigurable Neutral Atom (Harvard 2024)',
  onAuthorize,
  onDowngrade,
  onAbort,
}: CryptographicMandateCardWidgetProps) {
  const [isSigning, setIsSigning] = useState(false);
  const [signedToken, setSignedToken] = useState<string | null>(null);

  const handleSign = async () => {
    setIsSigning(true);
    try {
      // Deterministically hash the action description for the job hash
      const encoder = new TextEncoder();
      const digestBuffer = await window.crypto.subtle.digest(
        'SHA-256',
        encoder.encode(`${actionDescription}-${estimatedCostUsd}`)
      );
      const jobHashHex = Array.from(new Uint8Array(digestBuffer))
        .map((b) => b.toString(16).padStart(2, '0'))
        .join('');

      // Generate authentic AP2 Ed25519 SpendMandate
      const mandate = createSignedSpendMandate({
        tenantId: 'tenant-default',
        jobHash: jobHashHex,
        maxBudgetUsd: estimatedCostUsd,
        ttlSeconds: 3600,
      });

      setSignedToken(mandate.signature_hex);
      onAuthorize?.(mandate.signature_hex);
    } catch (err) {
      console.error('Failed to cryptographically sign mandate:', err);
    } finally {
      setIsSigning(false);
    }
  };

  return (
    <div className="rounded-2xl border border-rose-500/20 bg-rose-950/20 p-5 backdrop-blur-xl text-white space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-rose-500/20 pb-3">
        <div className="flex items-center space-x-3">
          <div className="p-2 rounded-xl bg-rose-500/20 border border-rose-500/30 text-rose-400">
            <KeyRound className="w-5 h-5" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-rose-200">AP2 High-Friction Spend Mandate Card (A2UI)</h3>
            <p className="text-xs text-rose-300/60">Autonomous Action Requiring Cryptographic Human Authorization</p>
          </div>
        </div>
        <span className="px-2.5 py-1 rounded-full text-xs font-mono bg-rose-500/10 border border-rose-500/30 text-rose-300">
          Autonomy Level 2 Gate
        </span>
      </div>

      {/* Target & Cost Details */}
      <div className="p-4 rounded-xl bg-black/60 border border-rose-500/10 space-y-3 text-xs">
        <div>
          <span className="text-white/40 block text-[10px]">Action Request</span>
          <p className="font-semibold text-white/90 text-sm mt-0.5">{actionDescription}</p>
        </div>
        <div className="grid grid-cols-2 gap-3 pt-2 border-t border-white/[0.04]">
          <div>
            <span className="text-white/40 block text-[10px]">Target QPU Backend</span>
            <span className="font-mono text-cyan-300">{targetBackend}</span>
          </div>
          <div>
            <span className="text-white/40 block text-[10px]">Estimated QPU Spend</span>
            <span className="font-mono text-amber-300 text-sm font-bold">${estimatedCostUsd.toFixed(2)} USD</span>
          </div>
        </div>
      </div>

      {/* Signature Status or Actions */}
      {signedToken ? (
        <div className="p-3 rounded-xl bg-emerald-500/10 border border-emerald-500/30 flex items-center space-x-2 text-xs text-emerald-300">
          <CheckCircle2 className="w-4 h-4 text-emerald-400 flex-shrink-0" />
          <div className="truncate">
            <span className="font-bold">Authorized & Signed: </span>
            <span className="font-mono text-[11px] text-emerald-200">{signedToken}</span>
          </div>
        </div>
      ) : (
        <div className="flex flex-wrap items-center gap-2 pt-1">
          <button
            onClick={handleSign}
            disabled={isSigning}
            className="flex-1 py-2 px-4 rounded-xl bg-gradient-to-r from-rose-600 to-amber-600 hover:from-rose-500 hover:to-amber-500 text-white font-medium text-xs shadow-lg transition flex items-center justify-center space-x-2 disabled:opacity-50"
          >
            <KeyRound className="w-3.5 h-3.5" />
            <span>{isSigning ? 'Signing WebCrypto...' : 'Authorize via Ed25519 Card'}</span>
          </button>
          <button
            onClick={onDowngrade}
            className="py-2 px-3 rounded-xl bg-white/5 hover:bg-white/10 border border-white/10 text-white/80 text-xs transition flex items-center space-x-1.5"
          >
            <ArrowDownCircle className="w-3.5 h-3.5 text-blue-400" />
            <span>Downgrade to Classical DMRG</span>
          </button>
          <button
            onClick={onAbort}
            className="py-2 px-3 rounded-xl bg-white/5 hover:bg-rose-500/20 border border-white/10 text-rose-300 text-xs transition flex items-center space-x-1.5"
          >
            <XCircle className="w-3.5 h-3.5 text-rose-400" />
            <span>Abort</span>
          </button>
        </div>
      )}
    </div>
  );
}
