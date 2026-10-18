'use client';

import React, { useState } from 'react';
import { ShieldCheck, Check, X, AlertTriangle, ExternalLink, Clock } from 'lucide-react';

export interface ApprovalPacketView {
  packetId: string;
  agentId: string;
  backend: string;
  programHash: string;
  qubits: number;
  shots: number;
  costMaxUsd: number;
  budgetRemainingUsd: number;
  costBasis: string;
  blastRadius: string;
  reversibility: string;
  evidence: {
    dryRun?: string;
    classicalAnchor?: string;
    quoteUrl?: string;
    notebookUrl?: string;
  };
  alternatives: {
    simFirstSavings?: string;
    fewerShotsSavings?: string;
  };
  expiresAtIso: string;
  noncePreview: string;
  // Fail-closed overspend flag: true when ceiling - spent < 0 at build time.
  // Renderers must surface this as a breach, never clamp silently.
  budgetBreached?: boolean;
}

interface ApprovalPacketCardProps {
  packet: ApprovalPacketView;
  isSubmitting?: boolean;
  decisionError?: string | null;
  onApproveOnce: () => void;
  onApproveUpToMax: () => void;
  onDeny: (reason: string) => void;
}

/**
 * Burden-scaled approval card (Q-11): everything a competent approver needs
 * to reconstruct the decision later — exact action, cost bound + budget left,
 * blast radius, reversibility, evidence links, alternatives, expiry — with
 * single-use decision semantics (approve-once / up-to-max / deny+reason).
 * Denial requires a reason, which is returned to the agent.
 */
export function ApprovalPacketCard({
  packet,
  isSubmitting = false,
  decisionError = null,
  onApproveOnce,
  onApproveUpToMax,
  onDeny,
}: ApprovalPacketCardProps) {
  const [denyReason, setDenyReason] = useState('');
  const [showDeny, setShowDeny] = useState(false);

  return (
    <div className="w-full bg-black/60 border border-amber-500/30 rounded-3xl p-6 backdrop-blur-2xl shadow-xl space-y-4">
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold text-white flex items-center space-x-2">
          <ShieldCheck className="w-4 h-4 text-amber-400" />
          <span>Approval Packet</span>
        </span>
        <span className="text-[11px] font-mono text-amber-300 bg-amber-950/80 px-2.5 py-0.5 rounded-full border border-amber-800/40">
          ≤ ${typeof packet.costMaxUsd === 'number' && Number.isFinite(packet.costMaxUsd) ? packet.costMaxUsd.toFixed(2) : '—'} · ${typeof packet.budgetRemainingUsd === 'number' && Number.isFinite(packet.budgetRemainingUsd) ? packet.budgetRemainingUsd.toFixed(2) : '—'} left{packet.budgetBreached ? ' · OVERSPENT' : ''}
        </span>
      </div>
      {packet.budgetBreached && (
        <div className="p-2.5 rounded-xl bg-rose-950/60 border border-rose-500/40 text-rose-200 text-[11px] font-code text-center">
          Budget breached — ceiling exceeded. Authorization must refuse until funded.
        </div>
      )}

      <div className="grid grid-cols-2 gap-3 text-xs">
        <div>
          <span className="text-white/40 block text-[10px]">Backend</span>
          <span className="font-mono text-cyan-300">{packet.backend}</span>
        </div>
        <div>
          <span className="text-white/40 block text-[10px]">Program</span>
          <span className="font-mono text-slate-200">{packet.programHash}</span>
        </div>
        <div>
          <span className="text-white/40 block text-[10px]">Qubits × Shots</span>
          <span className="font-mono text-slate-200">
            {packet.qubits} × {packet.shots.toLocaleString()}
          </span>
        </div>
        <div>
          <span className="text-white/40 block text-[10px]">Cost basis</span>
          <span className="text-slate-300">{packet.costBasis}</span>
        </div>
        <div>
          <span className="text-white/40 block text-[10px]">Blast radius</span>
          <span className="text-slate-300">{packet.blastRadius}</span>
        </div>
        <div>
          <span className="text-white/40 block text-[10px]">Reversibility</span>
          <span className="text-slate-300">{packet.reversibility}</span>
        </div>
      </div>

      {(packet.evidence.dryRun || packet.evidence.classicalAnchor) && (
        <div className="p-3 rounded-xl bg-cyan-950/40 border border-cyan-800/40 text-[11px] text-cyan-200 space-y-1">
          {packet.evidence.dryRun && <div>Dry run: {packet.evidence.dryRun}</div>}
          {packet.evidence.classicalAnchor && <div>Anchor: {packet.evidence.classicalAnchor}</div>}
        </div>
      )}

      {(packet.alternatives.simFirstSavings || packet.alternatives.fewerShotsSavings) && (
        <div className="p-3 rounded-xl bg-white/[0.03] border border-white/10 text-[11px] text-slate-300 space-y-1">
          {packet.alternatives.simFirstSavings && (
            <div>Sim first: {packet.alternatives.simFirstSavings}</div>
          )}
          {packet.alternatives.fewerShotsSavings && (
            <div>Fewer shots: {packet.alternatives.fewerShotsSavings}</div>
          )}
        </div>
      )}

      <div className="flex items-center justify-between text-[11px] font-mono text-slate-400">
        <span className="flex items-center space-x-1">
          <Clock className="w-3 h-3" />
          <span>Expires {packet.expiresAtIso}</span>
        </span>
        <span>
          {packet.packetId} · nonce {packet.noncePreview} · {packet.agentId}
        </span>
      </div>

      {decisionError && (
        <div className="p-3 rounded-xl bg-rose-950/60 border border-rose-500/40 text-rose-200 text-xs flex items-center space-x-2">
          <AlertTriangle className="w-4 h-4 shrink-0" />
          <span>{decisionError}</span>
        </div>
      )}

      {!showDeny ? (
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={onApproveOnce}
            disabled={isSubmitting || Boolean(packet.budgetBreached)}
            className="flex-1 py-2.5 px-4 text-xs font-semibold rounded-xl flex items-center justify-center space-x-2 transition-all bg-gradient-to-r from-emerald-600 to-teal-600 hover:from-emerald-500 hover:to-teal-500 text-white shadow-lg active:scale-[0.98] disabled:opacity-50"
          >
            <Check className="w-4 h-4" />
            <span>{isSubmitting ? 'Dispatching…' : 'Approve once'}</span>
          </button>
          <button
            onClick={onApproveUpToMax}
            disabled={isSubmitting || Boolean(packet.budgetBreached)}
            className="py-2.5 px-4 bg-white/5 hover:bg-white/10 border border-white/10 text-slate-200 text-xs font-medium rounded-xl transition-all disabled:opacity-50"
          >
            Approve up to max
          </button>
          <button
            onClick={() => setShowDeny(true)}
            disabled={isSubmitting}
            className="py-2.5 px-4 bg-white/5 hover:bg-rose-500/20 border border-white/10 text-rose-300 text-xs font-medium rounded-xl transition-all flex items-center space-x-1.5 disabled:opacity-50"
          >
            <X className="w-3.5 h-3.5" />
            <span>Deny</span>
          </button>
        </div>
      ) : (
        <div className="space-y-2">
          <input
            value={denyReason}
            onChange={(e) => setDenyReason(e.target.value)}
            placeholder="Denial reason (required — returned to the agent)…"
            maxLength={280}
            className="w-full px-3 py-2 rounded-xl bg-black/60 border border-rose-500/30 text-xs text-white placeholder:text-slate-500 focus:outline-none focus:border-rose-400"
          />
          <div className="flex items-center gap-2">
            <button
              onClick={() => denyReason.trim() && onDeny(denyReason.trim())}
              disabled={isSubmitting || !denyReason.trim()}
              className="flex-1 py-2 px-4 rounded-xl bg-rose-600 hover:bg-rose-500 text-white text-xs font-semibold transition disabled:opacity-50"
            >
              Confirm denial
            </button>
            <button
              onClick={() => {
                setShowDeny(false);
                setDenyReason('');
              }}
              className="py-2 px-4 rounded-xl bg-white/5 hover:bg-white/10 text-slate-300 text-xs transition"
            >
              Back
            </button>
          </div>
        </div>
      )}

      {packet.evidence.notebookUrl || packet.evidence.quoteUrl ? (
        <div className="flex items-center gap-3 text-[11px]">
          {packet.evidence.notebookUrl && (
            <a
              href={packet.evidence.notebookUrl}
              target="_blank"
              rel="noreferrer"
              className="flex items-center space-x-1 text-cyan-300 hover:text-cyan-200"
            >
              <ExternalLink className="w-3 h-3" />
              <span>Notebook</span>
            </a>
          )}
          {packet.evidence.quoteUrl && (
            <a
              href={packet.evidence.quoteUrl}
              target="_blank"
              rel="noreferrer"
              className="flex items-center space-x-1 text-cyan-300 hover:text-cyan-200"
            >
              <ExternalLink className="w-3 h-3" />
              <span>Quote</span>
            </a>
          )}
        </div>
      ) : null}
    </div>
  );
}
