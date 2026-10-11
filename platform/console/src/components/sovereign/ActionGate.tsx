'use client';

import React, { useState } from 'react';
import { Fingerprint, ChevronDown, ChevronUp } from 'lucide-react';
import { CampaignState, Session } from '@/types';
import { createSignedSpendMandate } from '@/lib/mandates';
import { verifySpendMandate } from '@/lib/api';
import { ApprovalPacketCard } from '@/components/common/ApprovalPacketCard';
import { buildApprovalPacketFromCampaign } from '@/lib/approvalPacket';

interface ActionGateProps {
  campaign: CampaignState | null;
  session: Session | null;
  onAuthorized: () => void;
  onDenied: (reason: string) => void;
}

/**
 * Calibrated Action Gate: the single high-stakes friction point. Rendered
 * only when a dispatch awaits human authorization (parent gates visibility).
 * Approval signs a real AP2 Ed25519 SpendMandate and verifies it against the
 * Go backend — denial halts fail-closed with a required reason.
 */
export function ActionGate({ campaign, session, onAuthorized, onDenied }: ActionGateProps) {
  const estimatedCost = campaign?.quantum_factors?.estimated_spend_usd ?? 0.12;
  const recommendedBackend = campaign?.quantum_factors?.recommended_backend || 'Local simulator';
  const qubits = campaign?.quantum_factors?.qubit_count ?? 0;
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [showDetail, setShowDetail] = useState(false);
  const [showDeny, setShowDeny] = useState(false);
  const [denyReason, setDenyReason] = useState('');
  const [message, setMessage] = useState<string | null>(null);
  const [rejected, setRejected] = useState(false);

  const packet = buildApprovalPacketFromCampaign(
    campaign ?? null,
    session?.id || '',
    estimatedCost,
    session?.spend_limit_usd ?? 25.0,
    session?.current_spend_usd ?? 0
  );

  const handleAuthorize = async () => {
    setIsSubmitting(true);
    setMessage(null);
    try {
      const mandate = createSignedSpendMandate({
        tenantId: session?.id || 'tenant-default',
        jobHash: 'vqe-' + (campaign?.session_id || Date.now().toString(16)),
        maxBudgetUsd: estimatedCost,
        ttlSeconds: 3600,
      });
      const verifyRes = await verifySpendMandate(mandate);
      if (!verifyRes.valid) {
        setRejected(true);
        setMessage(`Mandate REJECTED (${verifyRes.error || 'invalid'}). Halted fail-closed — nothing dispatched, nothing spent.`);
        return;
      }
      onAuthorized();
    } catch (err) {
      console.error('Failed to authorize pulse:', err);
      setRejected(true);
      setMessage('Failed to sign mandate envelope. Halted fail-closed.');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div
      data-testid="action-gate"
      className="absolute bottom-6 left-6 z-20 w-80 bg-[#0E1117]/90 backdrop-blur-2xl border border-white/10 rounded-2xl p-4 shadow-2xl"
    >
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <span className="w-2 h-2 rounded-full bg-amber-400 animate-ping" />
          <span className="text-[11px] font-bold text-amber-400 font-code uppercase tracking-wider">
            Action Gate
          </span>
        </div>
        <span className="text-[10px] font-code text-slate-400">Pending Authorization</span>
      </div>

      <div className="mt-2.5">
        <div className="text-xs text-white font-medium">Deploy Physical Quantum Pulse</div>
        <div className="text-[11px] text-slate-400 mt-0.5">
          {recommendedBackend} · {qubits} Qubits · ${estimatedCost.toFixed(2)}
        </div>
      </div>

      {message && (
        <div className={`mt-2.5 p-2.5 rounded-xl text-[11px] ${rejected ? 'bg-rose-950/60 border border-rose-500/40 text-rose-200' : 'bg-cyan-950/60 border border-cyan-500/40 text-cyan-200'}`}>
          {message}
        </div>
      )}

      {!showDeny ? (
        <div className="mt-3 space-y-2">
          <button
            data-testid="action-gate-authorize"
            onClick={handleAuthorize}
            disabled={isSubmitting}
            className="w-full py-2 px-3 rounded-xl bg-gradient-to-r from-amber-500 to-amber-600 hover:from-amber-400 hover:to-amber-500 text-black font-semibold text-xs flex items-center justify-center gap-2 transition-all shadow-[0_0_20px_rgba(255,179,0,0.3)] active:scale-[0.98] disabled:opacity-50 focus-visible:ring-2 focus-visible:ring-amber-300 focus-visible:outline-none"
          >
            <Fingerprint className="w-4 h-4" />
            <span>{isSubmitting ? 'Signing mandate…' : 'Authorize Pulse (Ed25519)'}</span>
          </button>
          <div className="flex items-center justify-between">
            <button
              onClick={() => setShowDetail((v) => !v)}
              className="text-[10px] font-code text-slate-400 hover:text-white flex items-center gap-1 transition-colors"
            >
              {showDetail ? <ChevronUp className="w-3 h-3" /> : <ChevronDown className="w-3 h-3" />}
              <span>burden packet</span>
            </button>
            <button
              onClick={() => setShowDeny(true)}
              disabled={isSubmitting}
              className="text-[10px] font-code text-rose-300/80 hover:text-rose-200 transition-colors disabled:opacity-50"
            >
              Deny
            </button>
          </div>
        </div>
      ) : (
        <div className="mt-3 space-y-2">
          <input
            value={denyReason}
            onChange={(e) => setDenyReason(e.target.value)}
            placeholder="Denial reason (required)…"
            maxLength={280}
            className="w-full px-3 py-2 rounded-xl bg-black/60 border border-rose-500/30 text-xs text-white placeholder:text-slate-500 focus:outline-none focus:border-rose-400"
          />
          <div className="flex items-center gap-2">
            <button
              onClick={() => denyReason.trim() && onDenied(denyReason.trim())}
              disabled={isSubmitting || !denyReason.trim()}
              className="flex-1 py-2 px-4 rounded-xl bg-rose-600 hover:bg-rose-500 text-white text-xs font-semibold transition disabled:opacity-50"
            >
              Confirm denial
            </button>
            <button
              onClick={() => { setShowDeny(false); setDenyReason(''); }}
              className="py-2 px-4 rounded-xl bg-white/5 hover:bg-white/10 text-slate-300 text-xs transition"
            >
              Back
            </button>
          </div>
        </div>
      )}

      {showDetail && (
        <div className="mt-3 max-h-72 overflow-y-auto">
          <ApprovalPacketCard
            packet={packet}
            isSubmitting={isSubmitting}
            onApproveOnce={handleAuthorize}
            onApproveUpToMax={handleAuthorize}
            onDeny={(reason) => onDenied(reason)}
          />
        </div>
      )}
    </div>
  );
}
