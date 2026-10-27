'use client';

import React, { useState } from 'react';
import { KeyRound,  CheckCircle2, XCircle, ArrowDownCircle } from 'lucide-react';
import { authorizeSpend } from '@/lib/authorizeSpend';

interface CryptographicMandateCardWidgetProps {
  actionDescription?: string;
  estimatedCostUsd?: number;
  targetBackend?: string;
  // J4: lab tenant the mandate authorizes against. Defaults to
  // tenant-default for static mounts; live callers pass the session lab.
  tenantId?: string;
  // The exact session + program the mandate authorizes. Without both,
  // authorizeSpend refuses (no unbound signatures).
  sessionId?: string;
  programQasm?: string;
  onAuthorize?: (mandateJson: string) => void;
  onDowngrade?: () => void;
  onAbort?: () => void;
}

export function CryptographicMandateCardWidget({
  actionDescription,
  estimatedCostUsd,
  targetBackend,
  tenantId,
  sessionId,
  programQasm,
  onAuthorize,
  onDowngrade,
  onAbort,
}: CryptographicMandateCardWidgetProps) {
  const [isSigning, setIsSigning] = useState(false);
  const [signedToken, setSignedToken] = useState<string | null>(null);
  const [verifiedLocal, setVerifiedLocal] = useState<boolean | null>(null);
  const [signError, setSignError] = useState<string | null>(null);
  // No demo defaults: authorization chrome with a fictional dispatch would
  // sign spend for work that was never proposed. Without props the card
  // states the gap and signing stays disabled.
  const hasProposal = actionDescription != null && estimatedCostUsd != null && targetBackend != null;
  const costFinite = typeof estimatedCostUsd === 'number' && Number.isFinite(estimatedCostUsd);

  const handleSign = async () => {
    if (!hasProposal) return;
    // Guarded above: all three are defined here. Locals keep TS honest
    // without reintroducing demo fallbacks.
    const cost = estimatedCostUsd as number;
    setIsSigning(true);
    setSignError(null);
    setVerifiedLocal(null);
    try {
      // Canonical job binding + server verification: the mandate covers
      // (tenant, session, exact program, budget) and /api/mandates/verify
      // must accept it before the card reports authorization.
      const res = await authorizeSpend({
        sessionId,
        tenantId,
        estimatedUsd: cost,
        programQasm,
      });
      if (!res.ok) {
        throw new Error(res.reason);
      }
      const mandateJson = JSON.stringify(res.mandate);
      setVerifiedLocal(true);
      setSignedToken(mandateJson);
      onAuthorize?.(mandateJson);
    } catch (err) {
      setSignError(err instanceof Error ? err.message : String(err));
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
            <h3 className="text-sm font-semibold text-rose-200">AP2 High-Friction Spend Mandate Card (Preview)</h3>
            <p className="text-xs text-rose-300/60">Autonomous Action Requiring Cryptographic Human Authorization</p>
          </div>
        </div>
        <span className="px-2.5 py-1 rounded-full text-xs font-mono bg-rose-500/10 border border-rose-500/30 text-rose-300">
          Autonomy Level 2 Gate
        </span>
      </div>

      {/* Target & Cost Details */}
      {!hasProposal ? (
        <div className="p-4 rounded-xl bg-black/60 border border-white/10 text-xs text-slate-400">
          No action proposed — nothing to authorize. This card signs only explicit proposals.
        </div>
      ) : (
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
            {costFinite ? (
              <span className="font-mono text-amber-300 text-sm font-bold">${(estimatedCostUsd as number).toFixed(2)} USD</span>
            ) : (
              <span className="font-mono text-amber-300 text-sm font-bold">UNPRICED — authorization disabled</span>
            )}
          </div>
        </div>
      </div>
      )}

      {/* Signature Status or Actions */}
      {signedToken ? (
        <div className="p-3 rounded-xl bg-emerald-500/10 border border-emerald-500/30 flex items-center space-x-2 text-xs text-emerald-300">
          <CheckCircle2 className="w-4 h-4 text-emerald-400 flex-shrink-0" />
          <div className="truncate">
            <span className="font-bold">Authorized & Server-Verified{verifiedLocal === false ? ' (UNVERIFIED)' : ''}: </span>
            <span className="font-mono text-[11px] text-emerald-200">{signedToken}</span>
            <div className="text-[10px] font-code text-slate-400">canonical session+program mandate — server-verified before dispatch</div>
          </div>
        </div>
      ) : !hasProposal ? (
        <p className="text-[11px] font-code text-slate-500">Authorize disabled — no proposal bound.</p>
      ) : (
        <div className="flex flex-wrap items-center gap-2 pt-1">
          <button
            onClick={handleSign}
            disabled={isSigning || !costFinite}
            title={!costFinite ? 'Disabled: unpriced — authorization disabled' : undefined}
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
            <span>Downgrade to simulator</span>
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
      {signError && (
        <div className="p-2.5 rounded-xl bg-rose-950/60 border border-rose-500/40 text-rose-200 text-[11px]">
          Sign failed — nothing authorized, nothing dispatched ({signError.slice(0, 200)}).
        </div>
      )}
    </div>
  );
}
